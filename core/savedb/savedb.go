// Package savedb answers "where does this game keep its saves" from the
// Ludusavi manifest — a community-curated catalogue of save locations
// for tens of thousands of games, largely derived from PCGamingWiki.
//
// It lives server-side for the same reasons the IGDB client does: one
// deployment fetches a 17MB catalogue on a schedule and every companion
// asks it, rather than every player's machine pulling and re-pulling the
// whole thing. The companion stays game-blind — it receives path
// *templates* with placeholders it knows how to expand locally, tests
// which of them exist on that machine, and still shows the player what
// it found before anything syncs.
//
// Nothing here is load-bearing. A deployment that cannot reach the
// manifest falls back to exactly what the companion did before: Steam
// Cloud paths, a small built-in catalogue, and a name search.
//
// The manifest is MIT-licensed; see docs/companion.md for attribution.
package savedb

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultURL is the manifest's canonical home.
const DefaultURL = "https://raw.githubusercontent.com/mtkennerly/ludusavi-manifest/master/data/manifest.yaml"

const (
	// refreshEvery paces the background refresh. The manifest changes
	// daily but a save location almost never moves for a game already in
	// it, so weekly is generous and costs one 17MB fetch.
	refreshEvery = 7 * 24 * time.Hour
	// maxManifest bounds what we will read, so a redirect to something
	// unexpected cannot exhaust the container's memory.
	maxManifest = 64 << 20
	// maxChunk bounds one game's YAML. The largest real entries are a
	// few kilobytes; anything past this is malformed.
	maxChunk = 1 << 20
)

// Location is one place a game's saves may live, as the manifest states
// it: a path template in Ludusavi's placeholder vocabulary, plus the
// conditions it applies under. Expansion happens on the player's
// machine, which is the only place that knows what the placeholders mean.
type Location struct {
	Template string `json:"template"`
	// OS and Store narrow when the entry applies ("windows", "steam").
	// Empty means "any" — the manifest omits the constraint.
	OS    string `json:"os,omitempty"`
	Store string `json:"store,omitempty"`
}

// Query is one game to look up. Any of the three may be empty; the more
// that are set, the better the match.
type Query struct {
	AppID      string `json:"appId,omitempty"`
	Name       string `json:"name,omitempty"`
	InstallDir string `json:"installDir,omitempty"`
}

// Key addresses a query's answer in the result map — the same key the
// artwork lookup uses, so a page holds one identity per game.
func (q Query) Key() string {
	if q.AppID != "" {
		return "app:" + q.AppID
	}
	return "name:" + strings.ToLower(strings.TrimSpace(q.Name))
}

// Status is the admin view: whether the catalogue loaded, how big it is,
// and what went wrong if it didn't.
type Status struct {
	Loaded      bool   `json:"loaded"`
	Games       int    `json:"games"`
	SteamIDs    int    `json:"steamIds"`
	FetchedAt   string `json:"fetchedAt,omitempty"`
	LastError   string `json:"lastError,omitempty"`
	LastErrorAt string `json:"lastErrorAt,omitempty"`
	Refreshing  bool   `json:"refreshing"`
	URL         string `json:"url,omitempty"`
}

type Client struct {
	url  string
	http *http.Client
	log  *slog.Logger

	mu           sync.RWMutex
	byAppID      map[string][]Location
	byName       map[string][]Location
	byInstallDir map[string][]Location
	games        int
	fetchedAt    time.Time
	lastError    string
	lastErrorAt  time.Time
	refreshing   bool
}

// New returns a client. A nil *Client is usable and answers nothing, so
// callers never branch on configuration.
func New(url string, logger *slog.Logger) *Client {
	if strings.TrimSpace(url) == "" {
		url = DefaultURL
	}
	return &Client{
		url: strings.TrimSpace(url),
		// Generous: this is a 17MB fetch over whatever link the host has.
		http: &http.Client{Timeout: 3 * time.Minute},
		log:  logger,
	}
}

func (c *Client) Loaded() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.games > 0
}

func (c *Client) Status() Status {
	if c == nil {
		return Status{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	st := Status{
		Loaded:     c.games > 0,
		Games:      c.games,
		SteamIDs:   len(c.byAppID),
		LastError:  c.lastError,
		Refreshing: c.refreshing,
		URL:        c.url,
	}
	if !c.fetchedAt.IsZero() {
		st.FetchedAt = c.fetchedAt.UTC().Format(time.RFC3339)
	}
	if !c.lastErrorAt.IsZero() {
		st.LastErrorAt = c.lastErrorAt.UTC().Format(time.RFC3339)
	}
	return st
}

// Run refreshes at startup and then weekly, until the context ends. A
// failed refresh keeps whatever was loaded before — a stale catalogue
// beats no catalogue.
func (c *Client) Run(ctx context.Context) {
	if c == nil {
		return
	}
	if err := c.Refresh(ctx); err != nil && c.log != nil {
		c.log.Error("save-location manifest", "error", err)
	}
	t := time.NewTicker(refreshEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := c.Refresh(ctx); err != nil && c.log != nil {
				c.log.Error("save-location manifest", "error", err)
			}
		}
	}
}

// Refresh fetches and re-indexes the manifest. Concurrent calls collapse:
// the second one returns immediately rather than pulling 17MB twice.
func (c *Client) Refresh(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.refreshing {
		c.mu.Unlock()
		return nil
	}
	c.refreshing = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.refreshing = false
		c.mu.Unlock()
	}()

	err := c.refresh(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.lastError, c.lastErrorAt = err.Error(), time.Now()
		return err
	}
	c.lastError, c.lastErrorAt = "", time.Time{}
	c.fetchedAt = time.Now()
	return nil
}

func (c *Client) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetching the save-location manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("save-location manifest: %s", resp.Status)
	}
	idx, err := parse(io.LimitReader(resp.Body, maxManifest))
	if err != nil {
		return err
	}
	if idx.games == 0 {
		return fmt.Errorf("save-location manifest parsed to nothing — did the format change?")
	}
	c.mu.Lock()
	c.byAppID, c.byName, c.byInstallDir, c.games = idx.byAppID, idx.byName, idx.byInstallDir, idx.games
	c.mu.Unlock()
	if c.log != nil {
		c.log.Info("save-location manifest loaded", "games", idx.games, "steamIds", len(idx.byAppID))
	}
	return nil
}

// Lookup answers a batch. Games the manifest doesn't carry, or carries
// with no save locations, are simply absent.
func (c *Client) Lookup(queries []Query) map[string][]Location {
	out := map[string][]Location{}
	if c == nil {
		return out
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, q := range queries {
		// Strongest first: the Steam app id is exact, the install folder
		// name is the manifest's own key for "what Steam calls this", and
		// the title is the last resort.
		var locs []Location
		if q.AppID != "" {
			locs = c.byAppID[q.AppID]
		}
		if len(locs) == 0 && q.InstallDir != "" {
			locs = c.byInstallDir[normalize(q.InstallDir)]
		}
		if len(locs) == 0 && q.Name != "" {
			locs = c.byName[normalize(q.Name)]
		}
		if len(locs) > 0 {
			out[q.Key()] = locs
		}
	}
	return out
}

// normalize folds a title or folder name to letters and digits, so
// "Baldur's Gate 3", "Baldurs Gate 3" and "BaldursGate3" all meet.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type index struct {
	byAppID      map[string][]Location
	byName       map[string][]Location
	byInstallDir map[string][]Location
	games        int
}

// manifestGame is the slice of a manifest entry this package uses. YAML
// decoding ignores the rest (launch commands, registry keys, other
// stores), which is most of the file.
type manifestGame struct {
	Files map[string]struct {
		Tags []string `yaml:"tags"`
		When []struct {
			OS    string `yaml:"os"`
			Store string `yaml:"store"`
		} `yaml:"when"`
	} `yaml:"files"`
	InstallDir map[string]yaml.Node `yaml:"installDir"`
	Steam      struct {
		ID int64 `yaml:"id"`
	} `yaml:"steam"`
	ID struct {
		SteamExtra []int64 `yaml:"steamExtra"`
	} `yaml:"id"`
}

// parse reads the manifest one game at a time.
//
// The whole file is a single YAML document holding tens of thousands of
// top-level keys; decoding it in one pass would hold the entire tree in
// memory at once, which is a lot to ask of the small host a reliquary
// usually runs on. Top-level keys are the only lines that start in
// column zero, so the stream splits cleanly into per-game chunks that
// each go through the real YAML decoder — bounded memory without
// hand-rolling YAML semantics for quoted keys and nested maps.
func parse(r io.Reader) (index, error) {
	idx := index{
		byAppID:      map[string][]Location{},
		byName:       map[string][]Location{},
		byInstallDir: map[string][]Location{},
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxChunk)

	var chunk strings.Builder
	flush := func() error {
		if chunk.Len() == 0 {
			return nil
		}
		text := chunk.String()
		chunk.Reset()
		var entry map[string]manifestGame
		if err := yaml.Unmarshal([]byte(text), &entry); err != nil {
			// One malformed game is not worth failing the catalogue over.
			return nil
		}
		for name, g := range entry {
			idx.add(name, g)
		}
		return nil
	}

	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "---") || strings.TrimSpace(line) == "" {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' && line[0] != '#' && line[0] != '-' {
			if err := flush(); err != nil {
				return idx, err
			}
		}
		chunk.WriteString(line)
		chunk.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return idx, fmt.Errorf("reading the save-location manifest: %w", err)
	}
	if err := flush(); err != nil {
		return idx, err
	}
	return idx, nil
}

func (idx *index) add(name string, g manifestGame) {
	locs := saveLocations(g)
	if len(locs) == 0 {
		return
	}
	idx.games++
	if g.Steam.ID > 0 {
		idx.byAppID[strconv.FormatInt(g.Steam.ID, 10)] = locs
	}
	for _, extra := range g.ID.SteamExtra {
		if extra > 0 {
			idx.byAppID[strconv.FormatInt(extra, 10)] = locs
		}
	}
	if key := normalize(name); key != "" {
		idx.byName[key] = locs
	}
	for dir := range g.InstallDir {
		if key := normalize(dir); key != "" {
			idx.byInstallDir[key] = locs
		}
	}
}

// saveLocations keeps the file entries tagged as saves. Config and
// screenshot entries are deliberately dropped: this syncs worlds, and a
// settings folder is not one.
func saveLocations(g manifestGame) []Location {
	var out []Location
	for template, entry := range g.Files {
		if !hasTag(entry.Tags, "save") {
			continue
		}
		if len(entry.When) == 0 {
			out = append(out, Location{Template: template})
			continue
		}
		for _, w := range entry.When {
			out = append(out, Location{Template: template, OS: w.OS, Store: w.Store})
		}
	}
	return out
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}
