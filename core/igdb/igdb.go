// Package igdb resolves game artwork and titles from IGDB, so a list of
// installed folder names can be shown as a shelf of covers.
//
// It lives server-side on purpose. The credentials are a Twitch client
// id and secret, and putting them in the companion would mean shipping
// them to every player's machine; here one deployment holds them, every
// companion asks through its own sync token, and the answers are cached
// once for everyone.
//
// Nothing here is load-bearing: artwork is decoration over metadata the
// companion already reports. Every failure — no credentials, a rate
// limit, IGDB down — degrades to "no cover" rather than an error the
// player has to care about. Degrading quietly is not the same as
// failing invisibly, though: the first cut swallowed every error, so a
// wrong credential and a game IGDB has never heard of looked identical
// from the outside. Status() is what the admin page reads to tell those
// apart.
package igdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	tokenURL = "https://id.twitch.tv/oauth2/token"
	apiURL   = "https://api.igdb.com/v4"
	// coverSize is IGDB's 264x374 cover; big enough for a grid tile at
	// 2x without pulling full-size art.
	coverSize = "t_cover_big"

	hitTTL  = 7 * 24 * time.Hour
	missTTL = 6 * time.Hour
	// maxNameLookups bounds a single batch's search queries — IGDB's
	// rate limit is four requests a second, and a big Steam library
	// would otherwise fire one per unmatched game.
	maxNameLookups = 12
)

// steamFilters are the ways IGDB has named "this external id is a Steam
// app id" on the external_games endpoint. `category` is the older field
// and `external_game_source` the newer one; which of them a given
// deployment's API accepts is not something this repo can settle from
// the outside, so the client tries them in order and remembers the one
// that answered. A shape that fails with a 400 is not a missing game —
// it is the wrong dialect, and the other one gets a turn.
var steamFilters = []string{"external_game_source = 1", "category = 1"}

// Game is what a lookup yields: IGDB's title and a cover image URL.
// Both may be empty, which means "IGDB knows nothing about this" and is
// a perfectly ordinary answer.
type Game struct {
	Name     string `json:"name,omitempty"`
	Cover    string `json:"cover,omitempty"`
	Summary  string `json:"summary,omitempty"`
	IGDBSlug string `json:"slug,omitempty"`
}

type entry struct {
	game Game
	at   time.Time
	miss bool
}

// Status is the diagnostic view: enough for an admin to tell "no
// credentials" from "bad credentials" from "IGDB simply doesn't have
// this game" without reading the service log.
type Status struct {
	Configured bool   `json:"configured"`
	Source     string `json:"source,omitempty"` // "settings" or "env"
	ClientID   string `json:"clientId,omitempty"`
	LastError  string `json:"lastError,omitempty"`
	// Timestamps as RFC3339 strings rather than time.Time: a zero
	// time.Time serializes as year 1 and omitempty does not drop it,
	// leaving the page to special-case a date that means "never".
	LastErrorAt string `json:"lastErrorAt,omitempty"`
	LastOKAt    string `json:"lastOkAt,omitempty"`
	Lookups     int    `json:"lookups"`
	Hits        int    `json:"hits"`
	Misses      int    `json:"misses"`
	Cached      int    `json:"cached"`
	Filter      string `json:"filter,omitempty"` // the app-id filter shape in use
}

// stamp renders a time for the status view, or "" for "never".
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

type Client struct {
	http *http.Client
	// Endpoint bases, overridable only so tests can stand in for Twitch
	// and IGDB; production leaves them at the constants above.
	tokenURL, apiURL string

	mu         sync.Mutex
	id, secret string
	source     string
	token      string
	expiry     time.Time
	cache      map[string]entry

	filter      string
	lastError   string
	lastErrorAt time.Time
	lastOKAt    time.Time
	lookups     int
	hits        int
	misses      int
}

// New returns a client. It is always non-nil and always usable: an
// unconfigured one answers nothing, which is exactly what a deployment
// without credentials wants, and credentials arriving later through the
// admin page turn it on without a restart. A nil *Client is tolerated
// too, for callers that never built one.
func New(clientID, clientSecret string) *Client {
	c := &Client{
		http:     &http.Client{Timeout: 15 * time.Second},
		cache:    map[string]entry{},
		tokenURL: tokenURL,
		apiURL:   apiURL,
	}
	c.SetCredentials(clientID, clientSecret, "env")
	return c
}

// UseEndpoints redirects the Twitch token URL and the IGDB API base.
// Tests are the only caller — a deployment always talks to the real
// services — but the seam is honest: without it nothing here can be
// exercised without the internet.
func (c *Client) UseEndpoints(token, api string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokenURL, c.apiURL = token, api
	c.token, c.expiry = "", time.Time{}
}

// SetCredentials replaces the pair. Changing it drops the cached token
// and every miss: a lookup that failed under the old credentials
// deserves another try under the new ones, and the whole point of
// pasting a key into the admin page is to see it work immediately.
func (c *Client) SetCredentials(clientID, clientSecret, source string) {
	if c == nil {
		return
	}
	id, secret := strings.TrimSpace(clientID), strings.TrimSpace(clientSecret)
	c.mu.Lock()
	defer c.mu.Unlock()
	if id == c.id && secret == c.secret {
		c.source = source
		return
	}
	c.id, c.secret, c.source = id, secret, source
	c.token, c.expiry = "", time.Time{}
	c.lastError, c.lastErrorAt = "", time.Time{}
	for k, e := range c.cache {
		if e.miss {
			delete(c.cache, k)
		}
	}
}

// Configured reports whether both halves of the credential are present.
func (c *Client) Configured() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.id != "" && c.secret != ""
}

// Status snapshots the diagnostics. The client id is not a secret (it
// travels in a header on every request); the secret never leaves here.
func (c *Client) Status() Status {
	if c == nil {
		return Status{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return Status{
		Configured:  c.id != "" && c.secret != "",
		Source:      c.source,
		ClientID:    c.id,
		LastError:   c.lastError,
		LastErrorAt: stamp(c.lastErrorAt),
		LastOKAt:    stamp(c.lastOKAt),
		Lookups:     c.lookups,
		Hits:        c.hits,
		Misses:      c.misses,
		Cached:      len(c.cache),
		Filter:      c.filter,
	}
}

// Test proves the credentials end to end: a token, then one real query.
// The admin page's "test" button calls this, because a saved credential
// that has never been exercised tells nobody anything.
func (c *Client) Test(ctx context.Context) error {
	if !c.Configured() {
		return fmt.Errorf("no IGDB credentials are configured")
	}
	if _, err := c.accessToken(ctx); err != nil {
		return err
	}
	var rows []struct {
		ID int64 `json:"id"`
	}
	if err := c.query(ctx, "games", `fields id; limit 1;`, &rows); err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("igdb answered an empty result for a trivial query")
	}
	return nil
}

// Query is one game to look up: the Steam app id when known (exact), and
// the name as a fallback.
type Query struct {
	AppID string `json:"appId,omitempty"`
	Name  string `json:"name,omitempty"`
}

// Key is how a query's answer is addressed in the result map — the app
// id when there is one, else the lowercased name.
func (q Query) Key() string {
	if q.AppID != "" {
		return "app:" + q.AppID
	}
	return "name:" + strings.ToLower(strings.TrimSpace(q.Name))
}

// Lookup resolves a batch, cache first. The map is keyed by Query.Key();
// games IGDB doesn't know are simply absent.
//
// An app id that IGDB's external-id table doesn't carry falls back to a
// name search under the same key, so a game whose Steam entry is missing
// from IGDB still gets its cover from its title.
func (c *Client) Lookup(ctx context.Context, queries []Query) map[string]Game {
	out := map[string]Game{}
	if !c.Configured() {
		return out
	}
	var needAppIDs []Query
	var needNames []Query

	c.mu.Lock()
	c.lookups += len(queries)
	seen := map[string]bool{}
	for _, q := range queries {
		key := q.Key()
		if seen[key] {
			continue
		}
		seen[key] = true
		if e, ok := c.cache[key]; ok && time.Since(e.at) < ttlFor(e) {
			if !e.miss {
				out[key] = e.game
			}
			continue
		}
		switch {
		case q.AppID != "":
			needAppIDs = append(needAppIDs, q)
		case q.Name != "":
			needNames = append(needNames, q)
		}
	}
	c.mu.Unlock()

	if len(needAppIDs) > 0 {
		ids := make([]string, 0, len(needAppIDs))
		for _, q := range needAppIDs {
			ids = append(ids, q.AppID)
		}
		found := c.byAppIDs(ctx, ids)
		for _, q := range needAppIDs {
			if g, ok := found[q.AppID]; ok {
				c.remember(q.Key(), g, false)
				out[q.Key()] = g
				continue
			}
			if strings.TrimSpace(q.Name) != "" {
				needNames = append(needNames, q)
				continue
			}
			c.remember(q.Key(), Game{}, true)
		}
	}
	for i, q := range needNames {
		if i >= maxNameLookups {
			break
		}
		g, ok := c.byName(ctx, q.Name)
		c.remember(q.Key(), g, !ok)
		if ok {
			out[q.Key()] = g
		}
	}
	return out
}

func ttlFor(e entry) time.Duration {
	if e.miss {
		return missTTL
	}
	return hitTTL
}

func (c *Client) remember(key string, g Game, miss bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = entry{game: g, at: time.Now(), miss: miss}
	if miss {
		c.misses++
	} else {
		c.hits++
	}
}

// note records the outcome of a call to IGDB. Success clears the last
// error: a stale one on a working client reads as a live fault.
func (c *Client) note(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.lastError, c.lastErrorAt = err.Error(), time.Now()
		return
	}
	c.lastError, c.lastErrorAt = "", time.Time{}
	c.lastOKAt = time.Now()
}

// accessToken fetches or reuses the client-credentials token. Twitch
// tokens last ~60 days; the minute of slack keeps a request from racing
// an expiry.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.expiry.Add(-time.Minute)) {
		defer c.mu.Unlock()
		return c.token, nil
	}
	id, secret, base := c.id, c.secret, c.tokenURL
	c.mu.Unlock()
	if id == "" || secret == "" {
		return "", fmt.Errorf("igdb token: no credentials configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(
		"%s?client_id=%s&client_secret=%s&grant_type=client_credentials", base, id, secret), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("igdb token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("igdb token: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("igdb token: %w", err)
	}
	c.mu.Lock()
	c.token = out.AccessToken
	c.expiry = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	c.mu.Unlock()
	return out.AccessToken, nil
}

// query runs one APIcalypse request against an IGDB endpoint, recording
// what happened either way.
func (c *Client) query(ctx context.Context, endpoint, body string, out any) error {
	err := c.doQuery(ctx, endpoint, body, out)
	c.note(err)
	return err
}

func (c *Client) doQuery(ctx context.Context, endpoint, body string, out any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	id, base := c.id, c.apiURL
	c.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/"+endpoint, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Client-ID", id)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("igdb %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("igdb %s: %s: %s", endpoint, resp.Status, strings.TrimSpace(string(raw)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func coverURL(imageID string) string {
	if imageID == "" {
		return ""
	}
	return "https://images.igdb.com/igdb/upload/" + coverSize + "/" + imageID + ".jpg"
}

type externalRow struct {
	UID  string `json:"uid"`
	Game struct {
		Name    string `json:"name"`
		Slug    string `json:"slug"`
		Summary string `json:"summary"`
		Cover   struct {
			ImageID string `json:"image_id"`
		} `json:"cover"`
	} `json:"game"`
}

// byAppIDs resolves Steam app ids in one request — the exact path, since
// the id came from the game's own Steam manifest. It tries each known
// spelling of the Steam filter until one is accepted, then sticks with
// it.
func (c *Client) byAppIDs(ctx context.Context, ids []string) map[string]Game {
	quoted := make([]string, 0, len(ids))
	for _, id := range ids {
		quoted = append(quoted, `"`+id+`"`)
	}
	uids := strings.Join(quoted, ",")

	for _, filter := range c.filterOrder() {
		body := fmt.Sprintf(
			`fields uid,game.name,game.slug,game.summary,game.cover.image_id; where %s & uid = (%s); limit %d;`,
			filter, uids, len(ids))
		var rows []externalRow
		if err := c.query(ctx, "external_games", body, &rows); err != nil {
			continue
		}
		c.rememberFilter(filter)
		out := map[string]Game{}
		for _, r := range rows {
			if r.Game.Name == "" {
				continue
			}
			out[r.UID] = Game{
				Name:     r.Game.Name,
				Cover:    coverURL(r.Game.Cover.ImageID),
				Summary:  r.Game.Summary,
				IGDBSlug: r.Game.Slug,
			}
		}
		return out
	}
	return nil
}

// filterOrder puts the filter shape that last worked first, so the
// steady state is one request per batch and only a dialect change costs
// a retry.
func (c *Client) filterOrder() []string {
	c.mu.Lock()
	known := c.filter
	c.mu.Unlock()
	if known == "" {
		return steamFilters
	}
	order := []string{known}
	for _, f := range steamFilters {
		if f != known {
			order = append(order, f)
		}
	}
	return order
}

func (c *Client) rememberFilter(filter string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.filter = filter
}

// byName is the fallback for a game with no Steam app id — a folder
// name from common/, say — and for one whose app id IGDB has no external
// record of.
func (c *Client) byName(ctx context.Context, name string) (Game, bool) {
	safe := strings.ReplaceAll(name, `"`, "")
	body := fmt.Sprintf(`search "%s"; fields name,slug,summary,cover.image_id; limit 1;`, safe)
	var rows []struct {
		Name    string `json:"name"`
		Slug    string `json:"slug"`
		Summary string `json:"summary"`
		Cover   struct {
			ImageID string `json:"image_id"`
		} `json:"cover"`
	}
	if err := c.query(ctx, "games", body, &rows); err != nil || len(rows) == 0 {
		return Game{}, false
	}
	r := rows[0]
	return Game{Name: r.Name, Cover: coverURL(r.Cover.ImageID), Summary: r.Summary, IGDBSlug: r.Slug}, true
}
