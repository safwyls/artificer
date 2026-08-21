package main

// Save-sync custody, from the player's side of the wire
// (docs/save-sync-architecture.md): check a shared world out of the
// console into a local directory, push mid-session checkpoints while the
// game runs, and check it back in when the hosting stretch ends. The
// personal sync token from the console's Worlds page is the whole
// credential, and every answer is sniffed the way the character relay's
// are — a 200 that isn't the console's own JSON ack is an interceptor,
// not a success.
//
// Torn-save guards, both learned elsewhere in this repo the hard way:
// a full check-in refuses while the game process runs, and any packaging
// waits out a settle window on the directory's mtimes. Checkpoints
// deliberately skip the process check — a mid-session push while the
// host plays is their entire point — and lean on the settle window plus
// the console's own verification.

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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// syncPollEvery paces the status poll against the console; the watch
	// loop ticks more often, this keeps custody polling polite.
	syncPollEvery = time.Minute
	// checkinSettle / checkpointSettle: how long the world directory must
	// be quiet before packaging. Check-in is short (the game is closed);
	// checkpoints wait longer so a push lands between autosaves, not
	// during one.
	checkinSettle    = 5 * time.Second
	checkpointSettle = 60 * time.Second
	// dragonwildsProcess is the game client's image name for the
	// running-game guard. Recon-pending like the save location — the
	// settle window is the load-bearing guard; this one is belt.
	dragonwildsProcess = "RSDragonwilds"
)

// syncWorldDTO is the console's world status, the subset this side reads.
type syncWorldDTO struct {
	World struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
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
	WorldID    int64          `json:"worldId,omitempty"`
	WorldDir   string         `json:"worldDir,omitempty"`
	SessionID  int64          `json:"sessionId,omitempty"`
	Worlds     []syncWorldDTO `json:"worlds,omitempty"`
	Busy       bool           `json:"busy"`
	LastError  string         `json:"lastError,omitempty"`
	LastAction string         `json:"lastAction,omitempty"`
	PolledAt   *time.Time     `json:"polledAt,omitempty"`
}

func (a *app) syncConfigured() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.ConsoleURL != "" && a.cfg.Sync.configured()
}

func (a *app) syncBase() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return normalizeConsoleURL(a.cfg.ConsoleURL) + "/api/public/sync/" + a.cfg.Sync.Token
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

// syncDo performs one custody call and decodes the console's ack. Any
// 200 whose body is not the console's own JSON (an Access login page,
// a tunnel interstitial) is a failure with the interceptor named.
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
	if resp.StatusCode != http.StatusOK {
		var parsed struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &parsed) == nil && parsed.Error != "" {
			return fmt.Errorf("console answered %d: %s", resp.StatusCode, parsed.Error)
		}
		return fmt.Errorf("console answered %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
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

// syncRefresh polls the console's custody status.
func (a *app) syncRefresh() error {
	var out struct {
		Username string         `json:"username"`
		Worlds   []syncWorldDTO `json:"worlds"`
	}
	if err := a.syncDo(http.MethodGet, "", nil, &out); err != nil {
		a.setSyncErr(err)
		return err
	}
	now := time.Now()
	a.mu.Lock()
	a.worldSync.Username = out.Username
	a.worldSync.Worlds = out.Worlds
	a.worldSync.PolledAt = &now
	a.worldSync.LastError = ""
	a.mu.Unlock()
	return nil
}

// syncTick rides the watch loop: poll, adopt a handoff, push checkpoints.
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
	a.adoptHandoff()
	a.autoCheckpoint()
}

// adoptHandoff notices that the console handed this player the world
// while nobody was looking — a queued claim consumed by someone else's
// check-in — and fetches it, so "your companion app will fetch it" is a
// promise this code keeps.
func (a *app) adoptHandoff() {
	a.mu.Lock()
	worldID, sessionID, me := a.cfg.Sync.WorldID, a.cfg.Sync.SessionID, a.worldSync.Username
	var world *syncWorldDTO
	for i := range a.worldSync.Worlds {
		if a.worldSync.Worlds[i].World.ID == worldID {
			world = &a.worldSync.Worlds[i]
		}
	}
	a.mu.Unlock()
	if world == nil || world.Holder == nil || world.Holder.Username != me || world.Holder.SessionID == sessionID {
		return
	}
	// The hold is ours but the session is new. Its base is the current
	// head: nothing can move the head under an active hold.
	base := int64(0)
	if world.World.HeadVersion != nil {
		base = *world.World.HeadVersion
	}
	if err := a.installHold(world.World.ID, world.Holder.SessionID, base); err != nil {
		a.setSyncErr(fmt.Errorf("fetching the handed-off world: %w", err))
		return
	}
	a.noteSync(fmt.Sprintf("adopted the handoff of %q — the world is on this machine", world.World.Name))
}

// autoCheckpoint pushes a checkpoint when the hosted world has changed
// and settled. Failures are recorded, not fatal — the next tick retries.
func (a *app) autoCheckpoint() {
	a.mu.Lock()
	sessionID := a.cfg.Sync.SessionID
	dir := a.cfg.Sync.WorldDir
	lastPush := a.lastCheckpoint
	var world *syncWorldDTO
	for i := range a.worldSync.Worlds {
		if a.worldSync.Worlds[i].World.ID == a.cfg.Sync.WorldID {
			world = &a.worldSync.Worlds[i]
		}
	}
	a.mu.Unlock()
	if sessionID == 0 || world == nil || !world.World.Checkpoints {
		return
	}
	newest, err := newestModTime(dir)
	if err != nil || !newest.After(lastPush) || time.Since(newest) < checkpointSettle {
		return
	}
	if err := a.pushBundle(sessionID, "checkpoint"); err != nil {
		a.setSyncErr(fmt.Errorf("checkpoint push: %w", err))
		return
	}
	a.mu.Lock()
	a.lastCheckpoint = time.Now()
	a.mu.Unlock()
	a.noteSync("checkpoint pushed")
}

// syncCheckout acquires the world and installs its head locally.
func (a *app) syncCheckout(worldID int64, takeover bool) error {
	if !a.setBusy(true) {
		return errors.New("a transfer is already running")
	}
	defer a.setBusy(false)
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

// installHold records the session and places its base version into the
// world directory. base 0 means a world with no versions yet: nothing to
// download, the directory as it stands is the starting point.
func (a *app) installHold(worldID, sessionID, base int64) error {
	if base != 0 {
		if err := a.installVersion(worldID, base); err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.cfg.Sync.WorldID = worldID
	a.cfg.Sync.SessionID = sessionID
	a.cfg.Sync.BaseVersion = base
	cfg, path := a.cfg, a.cfgPath
	a.mu.Unlock()
	return saveConfig(path, cfg)
}

// installVersion downloads a version bundle and swaps it into the world
// directory: extract beside it, keep one .pre-checkout copy of what was
// there, rename into place — a torn download never leaves the directory
// half-new, and the previous local state survives one level of regret.
func (a *app) installVersion(worldID, versionID int64) error {
	a.mu.Lock()
	dir := a.cfg.Sync.WorldDir
	a.mu.Unlock()
	if dir == "" {
		return errors.New("no world folder configured")
	}
	if running, name := gameRunning(); running {
		return fmt.Errorf("%s is running — close the game before replacing its save", name)
	}
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

// syncCheckin packages the world directory and returns the hold. The
// game must be closed and the directory settled — committing a torn save
// as the canonical version is the one unforgivable failure here.
func (a *app) syncCheckin() error {
	if !a.setBusy(true) {
		return errors.New("a transfer is already running")
	}
	defer a.setBusy(false)
	a.mu.Lock()
	sessionID := a.cfg.Sync.SessionID
	dir := a.cfg.Sync.WorldDir
	a.mu.Unlock()
	if sessionID == 0 {
		return errors.New("no hold to check in")
	}
	if running, name := gameRunning(); running {
		err := fmt.Errorf("%s is running — close the game, let it finish saving, then check in", name)
		a.setSyncErr(err)
		return err
	}
	if newest, err := newestModTime(dir); err != nil {
		a.setSyncErr(err)
		return err
	} else if since := time.Since(newest); since < checkinSettle {
		time.Sleep(checkinSettle - since) // written moments ago: wait out the settle window instead of failing
	}
	if err := a.pushBundle(sessionID, "checkin"); err != nil {
		a.setSyncErr(err)
		return err
	}
	a.mu.Lock()
	a.cfg.Sync.SessionID = 0
	a.cfg.Sync.BaseVersion = 0
	cfg, path := a.cfg, a.cfgPath
	a.mu.Unlock()
	if err := saveConfig(path, cfg); err != nil {
		return err
	}
	a.noteSync("checked in — the world is free")
	a.syncRefresh()
	return nil
}

// syncCheckpointNow is the page's manual checkpoint button.
func (a *app) syncCheckpointNow() error {
	a.mu.Lock()
	sessionID := a.cfg.Sync.SessionID
	a.mu.Unlock()
	if sessionID == 0 {
		return errors.New("no hold to checkpoint")
	}
	if err := a.pushBundle(sessionID, "checkpoint"); err != nil {
		a.setSyncErr(err)
		return err
	}
	a.mu.Lock()
	a.lastCheckpoint = time.Now()
	a.mu.Unlock()
	a.noteSync("checkpoint pushed")
	return nil
}

// pushBundle streams the packaged world directory to the console.
func (a *app) pushBundle(sessionID int64, verb string) error {
	a.mu.Lock()
	dir := a.cfg.Sync.WorldDir
	a.mu.Unlock()
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
			return fmt.Errorf("console answered %d: %s", resp.StatusCode, parsed.Error)
		}
		return fmt.Errorf("console answered %d", resp.StatusCode)
	}
	var ack struct {
		Accepted bool `json:"accepted"`
	}
	if err := json.Unmarshal(raw, &ack); err != nil || !ack.Accepted {
		return errors.New(interceptedHint(raw))
	}
	return nil
}

func (a *app) syncRenew() error {
	a.mu.Lock()
	sessionID := a.cfg.Sync.SessionID
	a.mu.Unlock()
	if sessionID == 0 {
		return errors.New("no hold to renew")
	}
	if err := a.syncDo(http.MethodPost, fmt.Sprintf("/sessions/%d/renew", sessionID), nil, nil); err != nil {
		a.setSyncErr(err)
		return err
	}
	a.noteSync("hold renewed")
	a.syncRefresh()
	return nil
}

func (a *app) syncClaim(worldID int64) error {
	if err := a.syncDo(http.MethodPost, fmt.Sprintf("/worlds/%d/claim", worldID), nil, nil); err != nil {
		a.setSyncErr(err)
		return err
	}
	a.mu.Lock()
	a.cfg.Sync.WorldID = worldID
	cfg, path := a.cfg, a.cfgPath
	a.mu.Unlock()
	saveConfig(path, cfg) // the claim's handoff needs to know which world to adopt
	a.noteSync("claimed the next hold")
	a.syncRefresh()
	return nil
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

// packageWorldDir writes the world directory as a save bundle — the same
// entry rules as the agent's (regular files, relative paths, PAX mtimes,
// rolling backup folders skipped; core/agent/files.go listSaveFiles).
func packageWorldDir(dir string, w io.Writer) error {
	if dir == "" {
		return errors.New("no world folder configured")
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

// newestModTime finds the most recent write anywhere under the world
// directory — the settle-window input.
func newestModTime(dir string) (time.Time, error) {
	if dir == "" {
		return time.Time{}, errors.New("no world folder configured")
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
		return time.Time{}, fmt.Errorf("reading the world folder: %w", err)
	}
	if newest.IsZero() {
		return time.Time{}, errors.New("the world folder is empty")
	}
	return newest, nil
}

// gameRunning reports whether the game client is up, best-effort. Only
// Windows has a game client; elsewhere this is always false.
func gameRunning() (bool, string) {
	if runtime.GOOS != "windows" {
		return false, ""
	}
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq "+dragonwildsProcess+"*", "/NH").Output()
	if err != nil {
		return false, "" // tasklist unavailable: fall back to the settle window alone
	}
	if strings.Contains(strings.ToLower(string(out)), strings.ToLower(dragonwildsProcess)) {
		return true, dragonwildsProcess
	}
	return false, ""
}
