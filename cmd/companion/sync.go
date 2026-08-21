package main

// Save-sync custody, from the player's side of the wire
// (docs/save-sync-architecture.md): link installed games' save folders
// to worlds on the save-sync service, check a world out into its folder
// to host it, push mid-session checkpoints, and check it back in when
// the hosting stretch ends. The personal sync token is the whole
// credential, and every answer is sniffed — a 200 that isn't the
// service's own JSON ack is an interceptor, not a success.
//
// Torn-save guard, learned elsewhere in this repo the hard way: any
// packaging waits out a settle window on the folder's mtimes, longer
// for checkpoints so a push lands between autosaves, not during one.
// The companion is game-blind, so there is no process-name check — the
// settle window is the guard, and the service verifies every upload
// again before anything becomes canonical.

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// syncPollEvery paces the status poll against the service; the watch
	// loop ticks more often, this keeps polling polite.
	syncPollEvery = time.Minute
	// checkinSettle / checkpointSettle: how long a world folder must be
	// quiet before packaging. Check-in is short (the game is closed);
	// checkpoints wait longer so a push lands between autosaves.
	checkinSettle    = 10 * time.Second
	checkpointSettle = 60 * time.Second
)

// syncWorldDTO is the service's world status, the subset this side reads.
type syncWorldDTO struct {
	World struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		GameTitle   string `json:"gameTitle"`
		SaveHint    string `json:"saveHint"`
		Checkpoints bool   `json:"checkpoints"`
		HeadVersion *int64 `json:"headVersion"`
	} `json:"world"`
	Holder *struct {
		SessionID int64     `json:"sessionId"`
		Username  string    `json:"username"`
		ExpiresAt time.Time `json:"expiresAt"`
		Claimable bool      `json:"claimable"`
	} `json:"holder,omitempty"`
	ClaimedBy string `json:"claimedBy,omitempty"`
	Head      *struct {
		ID        int64     `json:"id"`
		Bytes     int64     `json:"bytes"`
		CreatedAt time.Time `json:"createdAt"`
	} `json:"head,omitempty"`
}

// syncState is what the page shows about custody.
type syncState struct {
	Configured bool           `json:"configured"`
	Username   string         `json:"username,omitempty"`
	Worlds     []syncWorldDTO `json:"worlds,omitempty"`
	Busy       bool           `json:"busy"`
	LastError  string         `json:"lastError,omitempty"`
	LastAction string         `json:"lastAction,omitempty"`
	PolledAt   *time.Time     `json:"polledAt,omitempty"`
	// ServerVersion is the service's own build, reported by its status
	// call. Shown beside this app's version so a bug report about a
	// transfer can name both halves rather than one.
	ServerVersion string `json:"serverVersion,omitempty"`
}

func (a *app) syncConfigured() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.configured()
}

func (a *app) syncBase() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return normalizeServerURL(a.cfg.ServerURL) + "/api/public/sync/" + a.cfg.Token
}

func (a *app) setSyncErr(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err == nil {
		a.worldSync.LastError = ""
		return
	}
	a.worldSync.LastError = err.Error()
	log.Printf("sync: %v", err)
}

func (a *app) noteSync(action string) {
	a.mu.Lock()
	a.worldSync.LastAction = action
	a.worldSync.LastError = ""
	a.mu.Unlock()
	log.Printf("sync: %s", action)
}

// syncDo performs one custody call and decodes the service's ack. Any
// 200 whose body is not the service's own JSON (an Access login page, a
// tunnel interstitial) is a failure with the interceptor named.
func (a *app) syncDo(method, path string, body any, out any) error {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = strings.NewReader(string(data))
	}
	req, err := http.NewRequest(method, a.syncBase()+path, payload)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var parsed struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &parsed) == nil && parsed.Error != "" {
			return fmt.Errorf("service answered %d: %s", resp.StatusCode, parsed.Error)
		}
		return fmt.Errorf("service answered %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var ack struct {
		Accepted bool `json:"accepted"`
	}
	if err := json.Unmarshal(raw, &ack); err != nil || !ack.Accepted {
		return errors.New(interceptedHint(raw))
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// syncRefresh polls the service's custody status.
func (a *app) syncRefresh() error {
	var out struct {
		Username      string         `json:"username"`
		Worlds        []syncWorldDTO `json:"worlds"`
		ServerVersion string         `json:"serverVersion"`
	}
	if err := a.syncDo(http.MethodGet, "", nil, &out); err != nil {
		a.setSyncErr(err)
		return err
	}
	now := time.Now()
	a.mu.Lock()
	a.worldSync.Username = out.Username
	a.worldSync.Worlds = out.Worlds
	a.worldSync.ServerVersion = out.ServerVersion
	a.worldSync.PolledAt = &now
	a.worldSync.LastError = ""
	a.mu.Unlock()
	return nil
}

// syncTick rides the watch loop: poll, adopt handoffs, push checkpoints.
func (a *app) syncTick() {
	if !a.syncConfigured() {
		return
	}
	a.mu.Lock()
	due := a.worldSync.PolledAt == nil || time.Since(*a.worldSync.PolledAt) >= syncPollEvery
	busy := a.worldSync.Busy
	a.mu.Unlock()
	if !due || busy {
		return
	}
	if err := a.syncRefresh(); err != nil {
		return
	}
	for _, worldID := range a.linkedWorldIDs() {
		a.adoptHandoff(worldID)
		a.autoCheckpoint(worldID)
	}
}

func (a *app) linkedWorldIDs() []int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	ids := make([]int64, 0, len(a.cfg.Links))
	for _, l := range a.cfg.Links {
		ids = append(ids, l.WorldID)
	}
	return ids
}

// world returns the polled status for one world, nil when the service
// doesn't list it.
func (a *app) world(worldID int64) *syncWorldDTO {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.worldSync.Worlds {
		if a.worldSync.Worlds[i].World.ID == worldID {
			w := a.worldSync.Worlds[i]
			return &w
		}
	}
	return nil
}

// adoptHandoff notices that the service handed this player a linked
// world while nobody was looking — a queued claim consumed by someone
// else's check-in — and fetches it, so "your companion will fetch it" is
// a promise this code keeps.
func (a *app) adoptHandoff(worldID int64) {
	world := a.world(worldID)
	a.mu.Lock()
	link := a.cfg.link(worldID)
	me := a.worldSync.Username
	var sessionID int64
	if link != nil {
		sessionID = link.SessionID
	}
	a.mu.Unlock()
	if link == nil || world == nil || world.Holder == nil || world.Holder.Username != me || world.Holder.SessionID == sessionID {
		return
	}
	// The hold is ours but the session is new. Its base is the current
	// head: nothing can move the head under an active hold.
	base := int64(0)
	if world.World.HeadVersion != nil {
		base = *world.World.HeadVersion
	}
	if err := a.installHold(worldID, world.Holder.SessionID, base); err != nil {
		a.setSyncErr(fmt.Errorf("fetching the handed-off world: %w", err))
		return
	}
	a.noteSync(fmt.Sprintf("adopted the handoff of %q — the world is on this machine", world.World.Name))
}

// autoCheckpoint pushes a checkpoint when a held world's folder has
// changed and settled. Failures are recorded, not fatal — the next tick
// retries.
func (a *app) autoCheckpoint(worldID int64) {
	world := a.world(worldID)
	a.mu.Lock()
	link := a.cfg.link(worldID)
	var sessionID int64
	var dir string
	if link != nil {
		sessionID, dir = link.SessionID, link.Dir
	}
	lastPush := a.lastCheckpoint[worldID]
	a.mu.Unlock()
	if link == nil || sessionID == 0 || world == nil || !world.World.Checkpoints {
		return
	}
	newest, err := newestModTime(dir)
	if err != nil || !newest.After(lastPush) || time.Since(newest) < checkpointSettle {
		return
	}
	if err := a.pushBundle(dir, sessionID, "checkpoint"); err != nil {
		a.setSyncErr(fmt.Errorf("checkpoint push (%s): %w", world.World.Name, err))
		return
	}
	a.mu.Lock()
	a.lastCheckpoint[worldID] = time.Now()
	a.mu.Unlock()
	a.noteSync(fmt.Sprintf("checkpoint pushed for %q", world.World.Name))
}

// syncCheckout acquires a linked world and installs its head locally.
func (a *app) syncCheckout(worldID int64, takeover bool) error {
	if !a.setBusy(true) {
		return errors.New("a transfer is already running")
	}
	defer a.setBusy(false)
	if a.link(worldID) == nil {
		return errors.New("link this world to a save folder first")
	}
	var out struct {
		Session struct {
			ID          int64  `json:"id"`
			BaseVersion *int64 `json:"baseVersion"`
		} `json:"session"`
		World string `json:"world"`
	}
	if err := a.syncDo(http.MethodPost, fmt.Sprintf("/worlds/%d/checkout", worldID), map[string]bool{"takeover": takeover}, &out); err != nil {
		a.setSyncErr(err)
		return err
	}
	base := int64(0)
	if out.Session.BaseVersion != nil {
		base = *out.Session.BaseVersion
	}
	if err := a.installHold(worldID, out.Session.ID, base); err != nil {
		a.setSyncErr(err)
		return err
	}
	a.noteSync(fmt.Sprintf("checked out %q", out.World))
	a.syncRefresh()
	return nil
}

func (a *app) link(worldID int64) *WorldLink {
	a.mu.Lock()
	defer a.mu.Unlock()
	l := a.cfg.link(worldID)
	if l == nil {
		return nil
	}
	cp := *l
	return &cp
}

// installHold records the session and places its base version into the
// linked folder. base 0 means a world with no versions yet: nothing to
// download, the folder as it stands is the starting point.
func (a *app) installHold(worldID, sessionID, base int64) error {
	if base != 0 {
		if err := a.installVersion(worldID, base); err != nil {
			return err
		}
	}
	a.mu.Lock()
	if l := a.cfg.link(worldID); l != nil {
		l.SessionID, l.BaseVersion = sessionID, base
	}
	a.mu.Unlock()
	return a.saveCfg()
}

// installVersion downloads a version bundle and swaps it into the linked
// folder: extract beside it, keep one .pre-checkout copy of what was
// there, rename into place — a torn download never leaves the folder
// half-new, and the previous local state survives one level of regret.
func (a *app) installVersion(worldID, versionID int64) error {
	link := a.link(worldID)
	if link == nil || link.Dir == "" {
		return errors.New("no save folder linked for this world")
	}
	dir := link.Dir
	resp, err := a.client.Get(a.syncBase() + fmt.Sprintf("/worlds/%d/versions/%d/download", worldID, versionID))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download answered %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	tmp := dir + ".sync-tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := extractBundleTo(resp.Body, tmp); err != nil {
		os.RemoveAll(tmp)
		return fmt.Errorf("extracting the world: %w", err)
	}
	backup := dir + ".pre-checkout"
	if _, err := os.Stat(dir); err == nil {
		if err := os.RemoveAll(backup); err != nil {
			os.RemoveAll(tmp)
			return err
		}
		if err := os.Rename(dir, backup); err != nil {
			os.RemoveAll(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	return nil
}

// syncCheckin packages a held world's folder and returns the hold. The
// folder must be settled — committing a torn save as the canonical
// version is the one unforgivable failure here, so close the game and
// let it finish saving first.
func (a *app) syncCheckin(worldID int64) error {
	if !a.setBusy(true) {
		return errors.New("a transfer is already running")
	}
	defer a.setBusy(false)
	link := a.link(worldID)
	if link == nil || link.SessionID == 0 {
		return errors.New("no hold to check in")
	}
	if newest, err := newestModTime(link.Dir); err != nil {
		a.setSyncErr(err)
		return err
	} else if since := time.Since(newest); since < checkinSettle {
		time.Sleep(checkinSettle - since) // written moments ago: wait out the settle window instead of failing
	}
	if err := a.pushBundle(link.Dir, link.SessionID, "checkin"); err != nil {
		a.setSyncErr(err)
		return err
	}
	a.mu.Lock()
	if l := a.cfg.link(worldID); l != nil {
		l.SessionID, l.BaseVersion = 0, 0
	}
	a.mu.Unlock()
	if err := a.saveCfg(); err != nil {
		return err
	}
	a.noteSync("checked in — the world is free")
	a.syncRefresh()
	return nil
}

// syncCheckpointNow is the page's manual checkpoint button.
func (a *app) syncCheckpointNow(worldID int64) error {
	link := a.link(worldID)
	if link == nil || link.SessionID == 0 {
		return errors.New("no hold to checkpoint")
	}
	if err := a.pushBundle(link.Dir, link.SessionID, "checkpoint"); err != nil {
		a.setSyncErr(err)
		return err
	}
	a.mu.Lock()
	a.lastCheckpoint[worldID] = time.Now()
	a.mu.Unlock()
	a.noteSync("checkpoint pushed")
	return nil
}

// pushBundle streams a packaged world folder to the service.
func (a *app) pushBundle(dir string, sessionID int64, verb string) error {
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(packageWorldDir(dir, pw)) }()
	req, err := http.NewRequest(http.MethodPost, a.syncBase()+fmt.Sprintf("/sessions/%d/%s", sessionID, verb), pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-tar")
	// The default client timeout is sized for JSON, not a world upload.
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var parsed struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &parsed) == nil && parsed.Error != "" {
			return fmt.Errorf("service answered %d: %s", resp.StatusCode, parsed.Error)
		}
		return fmt.Errorf("service answered %d", resp.StatusCode)
	}
	var ack struct {
		Accepted bool `json:"accepted"`
	}
	if err := json.Unmarshal(raw, &ack); err != nil || !ack.Accepted {
		return errors.New(interceptedHint(raw))
	}
	return nil
}

func (a *app) syncRenew(worldID int64) error {
	link := a.link(worldID)
	if link == nil || link.SessionID == 0 {
		return errors.New("no hold to renew")
	}
	if err := a.syncDo(http.MethodPost, fmt.Sprintf("/sessions/%d/renew", link.SessionID), nil, nil); err != nil {
		a.setSyncErr(err)
		return err
	}
	a.noteSync("hold renewed")
	a.syncRefresh()
	return nil
}

func (a *app) syncClaim(worldID int64) error {
	if a.link(worldID) == nil {
		return errors.New("link this world to a save folder first — the handoff needs somewhere to land")
	}
	if err := a.syncDo(http.MethodPost, fmt.Sprintf("/worlds/%d/claim", worldID), nil, nil); err != nil {
		a.setSyncErr(err)
		return err
	}
	a.noteSync("claimed the next hold")
	a.syncRefresh()
	return nil
}

// --- linking installed games to worlds ---

// linkWorld ties an existing world to a local save folder and reports
// the game details to the service (metadata only; the service stores
// what companions report, it never interprets it).
// checkSaveDir is the one gate a link cannot pass without. It is
// separate so createWorld can run it *before* creating anything on the
// service — see the note there.
func checkSaveDir(dir string) error {
	if dir == "" {
		return errors.New("a link needs a save folder")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("save folder: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("save folder: %s is a file, not a folder", dir)
	}
	return nil
}

func (a *app) linkWorld(worldID int64, gameTitle, dir, meta, appID string) error {
	if err := checkSaveDir(dir); err != nil {
		return err
	}
	if gameTitle != "" || meta != "" {
		if err := a.syncDo(http.MethodPut, fmt.Sprintf("/worlds/%d/meta", worldID), map[string]string{
			"gameTitle": gameTitle, "saveHint": dir, "gameMeta": meta,
		}, nil); err != nil {
			a.setSyncErr(err)
			return err
		}
	}
	a.mu.Lock()
	if l := a.cfg.link(worldID); l != nil {
		l.GameTitle, l.Dir = gameTitle, dir
		if appID != "" {
			l.AppID = appID
		}
	} else {
		a.cfg.Links = append(a.cfg.Links, WorldLink{WorldID: worldID, GameTitle: gameTitle, Dir: dir, AppID: appID})
	}
	a.mu.Unlock()
	if err := a.saveCfg(); err != nil {
		return err
	}
	a.noteSync(fmt.Sprintf("linked world %d to %s", worldID, dir))
	a.syncRefresh()
	return nil
}

// createWorld makes a world on the service from a discovered game, links
// it, and optionally seeds it with the folder's current save.
func (a *app) createWorld(name, gameTitle, dir, meta, appID string, seed bool) error {
	if name == "" {
		return errors.New("a world needs a name")
	}
	// Check the folder before creating anything on the service. The
	// order used to be the other way round, so a link refused for a
	// missing save folder — the one thing discovery genuinely cannot
	// guess — still left a world behind on the service, with nothing
	// linked to it here. An orphan world nobody asked for is worse than
	// a refusal.
	if err := checkSaveDir(dir); err != nil {
		return err
	}
	var out struct {
		Status struct {
			World struct {
				ID int64 `json:"id"`
			} `json:"world"`
		} `json:"status"`
	}
	if err := a.syncDo(http.MethodPost, "/worlds", map[string]string{
		"name": name, "gameTitle": gameTitle, "saveHint": dir, "gameMeta": meta,
	}, &out); err != nil {
		a.setSyncErr(err)
		return err
	}
	worldID := out.Status.World.ID
	if err := a.linkWorld(worldID, gameTitle, dir, meta, appID); err != nil {
		return err
	}
	if seed {
		if err := a.seedWorld(worldID, dir); err != nil {
			a.setSyncErr(fmt.Errorf("world created and linked, but seeding failed: %w", err))
			return err
		}
		a.noteSync(fmt.Sprintf("created %q and seeded it with the current save", name))
	} else {
		a.noteSync(fmt.Sprintf("created %q", name))
	}
	a.syncRefresh()
	return nil
}

// seedWorld imports the folder's current save as the world's first
// version.
func (a *app) seedWorld(worldID int64, dir string) error {
	if !a.setBusy(true) {
		return errors.New("a transfer is already running")
	}
	defer a.setBusy(false)
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(packageWorldDir(dir, pw)) }()
	req, err := http.NewRequest(http.MethodPost, a.syncBase()+fmt.Sprintf("/worlds/%d/import", worldID), pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-tar")
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var ack struct {
		Accepted bool   `json:"accepted"`
		Error    string `json:"error"`
	}
	json.Unmarshal(raw, &ack)
	if resp.StatusCode != http.StatusOK || !ack.Accepted {
		if ack.Error != "" {
			return errors.New(ack.Error)
		}
		return errors.New(interceptedHint(raw))
	}
	return nil
}

func (a *app) unlink(worldID int64) error {
	a.mu.Lock()
	links := a.cfg.Links[:0]
	for _, l := range a.cfg.Links {
		if l.WorldID != worldID {
			links = append(links, l)
		}
	}
	a.cfg.Links = links
	a.mu.Unlock()
	return a.saveCfg()
}

func (a *app) setBusy(busy bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if busy && a.worldSync.Busy {
		return false
	}
	a.worldSync.Busy = busy
	return true
}

// packageWorldDir writes a save folder as a save bundle — the same entry
// rules as the agent's (regular files, relative paths, PAX mtimes,
// rolling backup folders skipped; core/agent/files.go listSaveFiles).
func packageWorldDir(dir string, w io.Writer) error {
	if dir == "" {
		return errors.New("no save folder linked")
	}
	tw := tar.NewWriter(w)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.EqualFold(d.Name(), "backup") || strings.EqualFold(d.Name(), "backups") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr := &tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: info.Size(), ModTime: info.ModTime(), Format: tar.FormatPAX}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.CopyN(tw, f, info.Size())
		f.Close()
		return err
	})
	if err != nil {
		return err
	}
	return tw.Close()
}

// extractBundleTo unpacks a bundle, admitting only relative regular
// files that resolve inside dir — the same paranoia as agentctl's
// extractTar, sized for a world save.
func extractBundleTo(r io.Reader, dir string) error {
	const (
		maxFiles = 20_000
		maxBytes = 4 << 30
	)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(r)
	var total int64
	files := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if files++; files > maxFiles {
			return errors.New("bundle exceeds the file-count bound")
		}
		if total += hdr.Size; total > maxBytes {
			return errors.New("bundle exceeds the size bound")
		}
		name := filepath.FromSlash(hdr.Name)
		if filepath.IsAbs(name) || strings.Contains(hdr.Name, "..") {
			return fmt.Errorf("bundle entry %q escapes the destination", hdr.Name)
		}
		dest := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		if !hdr.ModTime.IsZero() {
			_ = os.Chtimes(dest, hdr.ModTime, hdr.ModTime)
		}
	}
}

// newestModTime finds the most recent write anywhere under the folder —
// the settle-window input.
func newestModTime(dir string) (time.Time, error) {
	if dir == "" {
		return time.Time{}, errors.New("no save folder linked")
	}
	var newest time.Time
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("reading the save folder: %w", err)
	}
	if newest.IsZero() {
		return time.Time{}, errors.New("the save folder is empty")
	}
	return newest, nil
}

// --- cover art ---

// gameArt mirrors the service's artwork answer. The companion holds no
// IGDB credentials of its own: the vault looks art up once for everyone
// and this side just renders what comes back, so a service without
// artwork configured simply yields names.
type gameArt struct {
	Name    string `json:"name,omitempty"`
	Cover   string `json:"cover,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type artQuery struct {
	AppID string `json:"appId,omitempty"`
	Name  string `json:"name,omitempty"`
}

func artKey(q artQuery) string {
	if q.AppID != "" {
		return "app:" + q.AppID
	}
	return "name:" + strings.ToLower(strings.TrimSpace(q.Name))
}

// artwork resolves covers for the discovered games, asking the service
// only for what isn't already cached here. Failures are silent by
// design — a shelf without covers is still a shelf.
func (a *app) artwork() map[string]gameArt {
	a.mu.Lock()
	games := append([]discoveredGame(nil), a.discovered.Games...)
	if a.art == nil {
		a.art = map[string]gameArt{}
	}
	out := map[string]gameArt{}
	var need []artQuery
	for _, g := range games {
		q := artQuery{AppID: g.AppID, Name: g.Name}
		key := artKey(q)
		if hit, ok := a.art[key]; ok {
			if hit.Name != "" || hit.Cover != "" {
				out[key] = hit
			}
			continue
		}
		need = append(need, q)
	}
	configured := a.cfg.configured()
	a.mu.Unlock()

	if len(need) == 0 || !configured {
		return out
	}
	a.mu.Lock()
	a.artAsked += len(need)
	a.mu.Unlock()
	var resp struct {
		Art map[string]gameArt `json:"art"`
	}
	if err := a.syncDo(http.MethodPost, "/artwork", map[string]any{"games": need}, &resp); err != nil {
		// Artwork never blocks custody, so this stays out of the sync
		// error line — but it is not silent either: the page shows it
		// under the shelf, where a missing cover is what prompts the
		// question.
		log.Printf("artwork lookup: %v", err)
		a.mu.Lock()
		a.artError = err.Error()
		a.mu.Unlock()
		return out
	}
	a.mu.Lock()
	a.artError = ""
	for _, q := range need {
		key := artKey(q)
		hit := resp.Art[key] // a miss caches as empty, so it isn't re-asked every rescan
		a.art[key] = hit
		if hit.Name != "" || hit.Cover != "" {
			out[key] = hit
		}
	}
	a.mu.Unlock()
	return out
}
