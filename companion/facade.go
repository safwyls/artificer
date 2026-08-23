package companion

// The engine's in-process face, for a UI that lives in this process
// rather than behind the local HTTP server.
//
// The browser page polls GET /api/state every few seconds and renders
// whatever JSON comes back. A desktop shell in the same process should
// not pay for that: it can hold the same values as Go types, be told
// when they change, and call the verbs directly. So this file adds three
// things and nothing else:
//
//   - Snapshot: the state the page renders, as one exported struct. The
//     /api/state handler renders this same value, so there is one
//     assembly and the two shells cannot drift.
//   - Subscribe: a coalescing nudge fired whenever a mutation lands. No
//     payload rides the channel — the subscriber calls Snapshot, which
//     keeps ordering trivial and makes a missed nudge harmless.
//   - Typed actions: the verbs the HTTP handlers wrap, exported with Go
//     errors. Where a handler held real logic it now calls the method,
//     never the other way round.
//
// Everything here is a view onto state that already existed. No custody
// logic lives in this file, and none may: a shell that needs behaviour
// the engine does not have gets an addition to the engine.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// The engine's own types, exported under names a shell can name. They
// are aliases rather than new structs on purpose: one definition, so a
// field added to the scan or the custody DTO reaches both shells with
// nothing to keep in step. JSON tags are untouched, so the frozen
// /api/state shape is exactly what it was.
type (
	// Game is one installed game the scan found (discover.go).
	Game = discoveredGame
	// SaveCandidate is one possible save folder and why it was offered.
	SaveCandidate = saveCandidate
	// Probe is one place the scan looked and what it found there.
	Probe = probe
	// Discovery is one scan's result: the games and the trail.
	Discovery = discovery
	// World is the service's custody status for one world.
	World = syncWorldDTO
	// Holder is whoever holds a world, as the service reports it.
	Holder = syncHolder
	// SyncState is what the UI shows about custody.
	SyncState = syncState
	// UpdateState is what GitHub last said about this build.
	UpdateState = updateState
	// Art is one game's cover, as the service resolved it.
	Art = gameArt
	// SavePathSplit is a save folder in its two halves.
	SavePathSplit = savePathSplit
)

// ConfigView is the settings half of a snapshot. It deliberately does
// not carry the token: a UI needs to know whether one is saved, never
// what it is. Same fields the page's `config` object has always had.
type ConfigView struct {
	ServerURL string   `json:"serverUrl"`
	TokenSet  bool     `json:"tokenSet"`
	SteamDirs []string `json:"steamDirs"`
	// LaunchOnCheckout is the stored setting with its default applied.
	LaunchOnCheckout bool `json:"launchOnCheckout"`
}

// State is everything a companion UI renders, taken in one consistent
// read under the lock. This is the value GET /api/state marshals.
type State struct {
	Config     ConfigView  `json:"config"`
	Links      []WorldLink `json:"links"`
	Discovered Discovery   `json:"discovered"`
	Sync       SyncState   `json:"sync"`
	Version    string      `json:"version"`
	Update     UpdateState `json:"update"`
}

// Snapshot is the state the UI renders, assembled once under the lock.
//
// The empty-not-absent rule below is load-bearing and was learned the
// hard way: a nil slice marshals to JSON null, the page reads these as
// arrays, and `links.length` on null threw before anything else
// rendered — one bug that looked like three.
func (a *App) Snapshot() State {
	a.mu.Lock()
	defer a.mu.Unlock()

	st := a.worldSync
	st.Configured = a.cfg.configured()

	links := append([]WorldLink{}, a.cfg.Links...)

	discovered := a.discovered
	// Hidden is resolved here rather than in the scan: the UI needs the
	// whole library to offer "show hidden", and unhiding must not cost a
	// filesystem walk.
	games := make([]Game, 0, len(discovered.Games))
	for _, g := range discovered.Games {
		g.Hidden = a.cfg.isHidden(g)
		g.Key = gameKey(g)
		games = append(games, g)
	}
	discovered.Games = games
	if discovered.Probes == nil {
		discovered.Probes = []Probe{}
	}

	return State{
		Config: ConfigView{
			ServerURL:        a.cfg.ServerURL,
			TokenSet:         a.cfg.Token != "",
			SteamDirs:        append([]string{}, a.cfg.SteamDirs...),
			LaunchOnCheckout: a.cfg.launchOnCheckout(),
		},
		Links:      links,
		Discovered: discovered,
		Sync:       st,
		Version:    Version,
		Update:     a.update,
	}
}

// --- change notification ---

// Subscribe returns a channel nudged whenever engine state changes, and
// the function that stops it. The channel is buffered to one and sends
// are dropped when it is full: a subscriber that has not caught up
// already has a nudge waiting, and "something changed" does not
// accumulate. Read it, call Snapshot, render.
//
// The cancel function is idempotent, and closes the channel so a
// subscriber ranging over it ends.
func (a *App) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	a.mu.Lock()
	if a.subs == nil {
		a.subs = map[chan struct{}]bool{}
	}
	a.subs[ch] = true
	a.mu.Unlock()
	var once bool
	return ch, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if once || !a.subs[ch] {
			return
		}
		once = true
		delete(a.subs, ch)
		close(ch)
	}
}

// changed nudges every subscriber. Never call it holding a.mu.
func (a *App) changed() {
	a.mu.Lock()
	subs := make([]chan struct{}, 0, len(a.subs))
	for ch := range a.subs {
		subs = append(subs, ch)
	}
	a.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// changedLocked nudges from inside the lock, without ever sending while
// holding it — the sends are non-blocking, so this is safe, but it is
// kept separate so the ordinary path stays obviously lock-free.
func (a *App) changedLocked() {
	for ch := range a.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// MarkSeen records that someone is looking at the companion's state, and
// polls the service if the view has gone stale.
//
// This is what the page's GET has always done, exported so a window can
// do it too. It is not decoration: the custody poll runs at a few
// seconds while someone is watching and a minute otherwise
// (pollIntervalLocked), so a UI that never calls this shows custody up
// to a minute old — somebody else checks a world in and the person
// staring at the screen sees nothing happen. A desktop shell calls it on
// a ticker while its window is visible and stops when it hides.
func (a *App) MarkSeen() {
	a.mu.Lock()
	a.pageSeen = time.Now()
	a.mu.Unlock()
	go a.refreshIfStale()
}

// --- typed actions ---

// ConfigUpdate is a partial settings write: absent fields keep their
// stored values, which is what lets the connection panel and the
// discovery panel save independently. An empty Token keeps the saved
// one — a blank box must never erase a credential.
type ConfigUpdate struct {
	ServerURL        *string
	Token            string
	SteamDirs        *[]string
	LaunchOnCheckout *bool
}

// SetConfig saves whichever settings the update carries. A completed
// connection is proven with a status poll rather than assumed: a typo'd
// token should fail here, not silently every minute forever.
func (a *App) SetConfig(in ConfigUpdate) error {
	a.mu.Lock()
	if in.ServerURL != nil {
		a.cfg.ServerURL = normalizeServerURL(*in.ServerURL)
	}
	if strings.TrimSpace(in.Token) != "" {
		a.cfg.Token = strings.TrimSpace(in.Token)
	}
	if in.SteamDirs != nil {
		dirs := make([]string, 0, len(*in.SteamDirs))
		for _, d := range *in.SteamDirs {
			if d = strings.TrimSpace(d); d != "" {
				dirs = append(dirs, d)
			}
		}
		a.cfg.SteamDirs = dirs
	}
	if in.LaunchOnCheckout != nil {
		v := *in.LaunchOnCheckout
		a.cfg.LaunchOnCheckout = &v
	}
	a.mu.Unlock()
	if err := a.saveCfg(); err != nil {
		return errors.New("saving config: " + err.Error())
	}
	if in.SteamDirs != nil {
		a.Rescan()
	}
	if in.ServerURL != nil && a.SyncConfigured() {
		return a.SyncRefresh()
	}
	return nil
}

// SyncNow polls the service immediately and answers how many worlds it
// has. For the moment someone wants to be certain rather than patient —
// and for saying plainly when the service cannot be reached, which a
// silent background poll never does.
func (a *App) SyncNow() (int, error) {
	if !a.SyncConfigured() {
		return 0, errors.New("not connected — set the service URL and your token in Settings")
	}
	if err := a.SyncRefresh(); err != nil {
		return 0, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.worldSync.Worlds), nil
}

// Artwork resolves covers for the discovered games, asking the service
// only for what is not cached here.
func (a *App) Artwork() map[string]Art { return a.artwork() }

// ArtStatus is why the last cover lookup failed and how many games were
// actually asked about, so "nothing was ever asked" stays
// distinguishable from "asked, got nothing".
func (a *App) ArtStatus() (asked int, failure string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.artAsked, a.artError
}

// SaveHints folds the service's save-location catalogue into the
// discovered games' candidates, and reports what it managed.
func (a *App) SaveHints() (available bool, known int, failure string) {
	a.saveHints()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, locs := range a.hints {
		if len(locs) > 0 {
			known++
		}
	}
	return a.hintsAvailable, known, a.hintsError
}

// Hide puts a shelf entry away, or brings it back. Local and reversible:
// it never touches a link or the service.
func (a *App) Hide(key string, hidden bool) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("no entry given")
	}
	a.mu.Lock()
	a.cfg.setHidden(key, hidden)
	a.mu.Unlock()
	return a.saveCfg()
}

// Link records that a world on the service lives in a folder here.
func (a *App) Link(worldID int64, gameTitle, dir, meta, appID string) error {
	if worldID == 0 {
		return errors.New("no world given")
	}
	return a.linkWorld(worldID, gameTitle, strings.TrimSpace(dir), meta, appID)
}

// CreateWorld makes a new world on the service and links it here,
// optionally seeding it with what is in the folder now.
func (a *App) CreateWorld(name, gameTitle, dir, meta, appID, savePath string, seed bool) error {
	return a.createWorld(strings.TrimSpace(name), gameTitle, strings.TrimSpace(dir), meta, appID, savePath, seed)
}

// Unlink forgets a world's folder here. Nothing is deleted.
func (a *App) Unlink(worldID int64) error { return a.unlink(worldID) }

// LinkEdit is the parts of a link the player owns. Each is optional, and
// each saves independently — the folder lives here, the name lives on
// the service.
type LinkEdit struct {
	LaunchTarget *string
	Dir          *string
	WorldName    *string
}

// EditLink applies an edit to a link. The folder cannot move while this
// machine holds the world: a checked-out save is in the old folder, and
// repointing under it would strand it.
func (a *App) EditLink(worldID int64, in LinkEdit) error {
	a.mu.Lock()
	l := a.cfg.link(worldID)
	if l == nil {
		a.mu.Unlock()
		return errors.New("no such link")
	}
	if in.LaunchTarget != nil {
		l.LaunchTarget = strings.TrimSpace(*in.LaunchTarget)
	}
	held := l.SessionID != 0
	a.mu.Unlock()

	if in.Dir != nil {
		dir := strings.TrimSpace(*in.Dir)
		if held {
			return errors.New("check the world in before changing its folder")
		}
		if err := checkSaveDir(dir); err != nil {
			return err
		}
		a.mu.Lock()
		if l := a.cfg.link(worldID); l != nil {
			l.Dir = dir
		}
		a.mu.Unlock()
	}
	if err := a.saveCfg(); err != nil {
		return errors.New("saving config: " + err.Error())
	}
	if in.WorldName != nil {
		if !a.SyncConfigured() {
			return errors.New("set the server URL and token first")
		}
		return a.renameWorld(worldID, strings.TrimSpace(*in.WorldName))
	}
	return nil
}

// CheckoutResult is the two halves of one intention. A save on disk with
// a game that would not start is a real outcome: the custody half
// succeeded, and the player has to be told the other half did not
// without being told the whole thing failed.
type CheckoutResult struct {
	Launched    bool
	LaunchError error
}

// Checkout takes the world and, when asked and able, starts the game.
// play=false is the plain checkout: the save lands here and nothing is
// launched, whatever the launch-on-checkout setting says.
func (a *App) Checkout(worldID int64, takeover, play bool) (CheckoutResult, error) {
	if !a.SyncConfigured() {
		return CheckoutResult{}, errors.New("set the server URL and token first")
	}
	if !play {
		return CheckoutResult{}, a.syncCheckout(worldID, takeover)
	}
	launched, launchErr, err := a.checkoutAndPlay(worldID, takeover)
	if err != nil {
		return CheckoutResult{}, err
	}
	return CheckoutResult{Launched: launched, LaunchError: launchErr}, nil
}

// Checkin ends this machine's hold, uploading what is in the folder.
func (a *App) Checkin(worldID int64) error { return a.needSync(a.syncCheckin, worldID) }

// Checkpoint pushes a mid-session version without moving the head.
func (a *App) Checkpoint(worldID int64) error { return a.needSync(a.syncCheckpointNow, worldID) }

// Renew extends this machine's hold.
func (a *App) Renew(worldID int64) error { return a.needSync(a.syncRenew, worldID) }

// Claim asks to be next: the world downloads here automatically when it
// frees up.
func (a *App) Claim(worldID int64) error { return a.needSync(a.syncClaim, worldID) }

// Launch starts a linked world's game without touching custody — for
// coming back to a world already held.
func (a *App) Launch(worldID int64) error { return a.launch(worldID) }

// needSync refuses the custody verbs before there is a service to talk
// to, so the failure names the setting rather than the network.
func (a *App) needSync(fn func(int64) error, worldID int64) error {
	if !a.SyncConfigured() {
		return errors.New("set the server URL and token first")
	}
	return fn(worldID)
}

// SplitSavePath reports where a chosen folder divides into the part a
// joining player supplies and the part the world carries with it. Shown
// before anything is recorded, because a guess nobody can see is a guess
// nobody can correct.
func (a *App) SplitSavePath(dir, appID, name string) (SavePathSplit, error) {
	dir = cleanPastedPath(dir)
	if dir == "" {
		return SavePathSplit{}, errors.New("no folder given")
	}
	a.mu.Lock()
	libs := append([]string(nil), a.discovered.Libraries...)
	var roots []string
	for _, g := range a.discovered.Games {
		if g.AppID == appID || strings.EqualFold(g.Name, name) {
			for _, c := range g.SaveDirs {
				roots = append(roots, c.Path)
			}
			for _, loc := range a.hints[gameKey(g)] {
				if !loc.appliesHere() {
					continue
				}
				roots = append(roots, expandTemplate(loc.Template, g.InstallDir, libs)...)
			}
		}
	}
	a.mu.Unlock()
	return splitSaveDir(dir, roots), nil
}

// ResolveSavePath joins a world's own folder under a root the player
// chose, creating it when asked. This is the join flow: the second
// player to take a world cannot type an opaque id they have never seen,
// so they supply the half they know and the companion makes the rest.
func (a *App) ResolveSavePath(root, leaf string, create bool) (dir string, exists bool, err error) {
	if create {
		dir, err = prepareWorldDir(root, leaf)
	} else {
		dir, err = joinSavePath(root, leaf)
	}
	if err != nil {
		return "", false, err
	}
	if info, serr := os.Stat(dir); serr == nil && info.IsDir() {
		exists = true
	}
	return dir, exists, nil
}

// CheckUpdate asks GitHub now rather than waiting for the timer, and
// answers with what it learned.
func (a *App) CheckUpdate(ctx context.Context) UpdateState {
	a.checkUpdate(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.update
}

// UpdateStatus is what the last check said, without asking again.
func (a *App) UpdateStatus() UpdateState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.update
}

// ApplyUpdate replaces this binary in place. It does not restart —
// RestartAfterUpdate does, and the caller decides when, because a shell
// with a window and a tray has things to tear down first.
func (a *App) ApplyUpdate(ctx context.Context) error { return a.applyUpdate(ctx) }

// RestartAfterUpdate launches the replacement and ends this process
// through ExitForRestart, which an entrypoint with a tray replaces so no
// ghost icon is left behind.
func (a *App) RestartAfterUpdate() error {
	if err := restartSelf(); err != nil {
		return err
	}
	ExitForRestart()
	return nil
}

// --- cover bytes ---

// maxCoverBytes bounds one cover download. Covers are small JPEGs; this
// is a sanity limit, not a real one.
const maxCoverBytes = 8 << 20

// Cover fetches a game's cover art as bytes, cached forever per key —
// including the misses, so a URL that 404s is asked about once.
//
// The browser build hands the page a URL and lets it fetch. An
// in-process UI has no such luxury: a widget that re-fetches its image
// whenever state changes makes a shelf of covers flicker on every poll,
// which is exactly the bug the web shelf's memoization exists to
// prevent. So the rule the web UI learned — covers must not remount on
// poll — becomes, here: fetch each cover once and hold the bytes.
//
// The empty slice with a nil error means "there is no cover for this
// game", which is an answer, not a failure: a shelf without covers is
// still a shelf.
func (a *App) Cover(key string) ([]byte, error) {
	a.mu.Lock()
	if a.covers == nil {
		a.covers = map[string][]byte{}
	}
	if data, ok := a.covers[key]; ok {
		a.mu.Unlock()
		return data, nil
	}
	url := a.art[key].Cover
	a.mu.Unlock()

	if strings.TrimSpace(url) == "" {
		a.rememberCover(key, nil)
		return nil, nil
	}
	data, err := a.fetchCover(url)
	if err != nil {
		// Remembered as a miss too: a cover that cannot be fetched must
		// not be retried on every render.
		a.rememberCover(key, nil)
		return nil, err
	}
	a.rememberCover(key, data)
	return data, nil
}

func (a *App) rememberCover(key string, data []byte) {
	a.mu.Lock()
	if a.covers == nil {
		a.covers = map[string][]byte{}
	}
	a.covers[key] = data
	a.mu.Unlock()
}

func (a *App) fetchCover(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("cover art: " + resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxCoverBytes))
}
