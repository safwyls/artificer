package companion

// The in-process facade the desktop shell renders from (facade.go). The
// guards here are the three things a shell silently depends on: the
// snapshot carries everything the page's JSON carried, a nudge always
// arrives after a change and never piles up, and MarkSeen puts the
// custody poll into its fast mode the way the page's GET always has.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The snapshot and the frozen /api/state body are the same value. If a
// field is ever added to one and not the other, the two shells have
// drifted — which is the whole thing this facade exists to prevent.
func TestSnapshotIsWhatTheStateRouteServes(t *testing.T) {
	a := NewApp(Config{
		ServerURL: "https://vault.example.com",
		Token:     "tok",
		SteamDirs: []string{"D:\\SteamLibrary"},
		Links:     []WorldLink{{WorldID: 7, GameTitle: "A Game", Dir: "C:\\saves"}},
	}, filepath.Join(t.TempDir(), "config.json"))

	rec := httptest.NewRecorder()
	a.handleState(rec, httptest.NewRequest("GET", "/api/state", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("state: %d", rec.Code)
	}
	var fromRoute, fromSnapshot map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &fromRoute); err != nil {
		t.Fatalf("decode route body: %v", err)
	}
	snap, err := json.Marshal(a.Snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := json.Unmarshal(snap, &fromSnapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if !reflect.DeepEqual(fromRoute, fromSnapshot) {
		t.Errorf("the page's state and Snapshot() disagree:\n route: %v\n snap:  %v", fromRoute, fromSnapshot)
	}
}

// Everything a shell renders has to be reachable from the snapshot
// alone. A window that has to reach past this into the engine is a
// window that will grow its own copy of the state.
func TestSnapshotCarriesWhatAShellRenders(t *testing.T) {
	a := NewApp(Config{
		ServerURL: "https://vault.example.com",
		Token:     "tok",
		SteamDirs: []string{"D:\\SteamLibrary"},
		Links:     []WorldLink{{WorldID: 7, GameTitle: "A Game", Dir: "C:\\saves"}},
	}, filepath.Join(t.TempDir(), "config.json"))
	a.mu.Lock()
	a.worldSync.Username = "hilda"
	a.worldSync.Worlds = []syncWorldDTO{makeDTO(7, "Shared World")}
	a.mu.Unlock()

	st := a.Snapshot()
	if st.Config.ServerURL != "https://vault.example.com" {
		t.Errorf("server url = %q", st.Config.ServerURL)
	}
	if !st.Config.TokenSet {
		t.Error("a saved token must show as set")
	}
	if !st.Config.LaunchOnCheckout {
		t.Error("launch-on-checkout defaults to on; an absent setting must come through as true")
	}
	if !st.Sync.Configured || st.Sync.Username != "hilda" || len(st.Sync.Worlds) != 1 {
		t.Errorf("custody state did not survive the snapshot: %+v", st.Sync)
	}
	if len(st.Links) != 1 || st.Links[0].WorldID != 7 {
		t.Errorf("links = %+v", st.Links)
	}
	if st.Version == "" {
		t.Error("the snapshot carries no version; both shells show it for bug reports")
	}
	// Empty, not nil: the same rule the JSON has, because a shell that
	// ranges over these must not have to nil-check each one.
	if st.Discovered.Probes == nil || st.Config.SteamDirs == nil || st.Links == nil {
		t.Errorf("a nil slice reached a shell: %+v", st)
	}
}

// The window names the machine it is running on rather than saying "this
// machine", which says nothing you cannot already see. It only stops
// being a truism once the same account syncs from two PCs — which is the
// case custody exists to disambiguate — so the name has to come from the
// machine and cannot be inferred by the UI.
func TestSnapshotNamesThisMachine(t *testing.T) {
	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))

	want, err := os.Hostname()
	if err != nil {
		t.Skip("this host will not say what it is called")
	}
	if got := a.Snapshot().Hostname; got != want {
		t.Errorf("Hostname = %q, want %q", got, want)
	}

	// Asked once and held: Snapshot runs on every poll and every change
	// nudge, and a syscall per render is a syscall per render.
	if a.Snapshot().Hostname != a.Snapshot().Hostname {
		t.Error("the hostname changed between two snapshots")
	}
}

// The token itself never reaches a UI. The page has only ever been told
// whether one is saved, and the window must not learn more.
func TestSnapshotWithholdsTheToken(t *testing.T) {
	a := NewApp(Config{ServerURL: "https://vault.example.com", Token: "supersecrettoken"},
		filepath.Join(t.TempDir(), "config.json"))
	body, err := json.Marshal(a.Snapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(body) == "" || strings.Contains(string(body), "supersecrettoken") {
		t.Errorf("the snapshot carries the sync token: %s", body)
	}
}

// A mutation nudges every subscriber, and a subscriber that has not
// caught up gets one nudge rather than a queue: "something changed" does
// not accumulate, because the answer is always the current snapshot.
func TestSubscribeCoalesces(t *testing.T) {
	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
	ch, stop := a.Subscribe()
	defer stop()

	if err := a.Hide("app:1", true); err != nil {
		t.Fatalf("hide: %v", err)
	}
	if err := a.Hide("app:2", true); err != nil {
		t.Fatalf("hide: %v", err)
	}
	if err := a.Hide("app:3", true); err != nil {
		t.Fatalf("hide: %v", err)
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("a config change did not nudge the subscriber")
	}
	// Three changes, one pending nudge: the buffer is one deep and full
	// sends are dropped.
	select {
	case <-ch:
		t.Error("nudges are stacking up; they must coalesce into one")
	default:
	}

	// And after draining, the next change nudges again — coalescing must
	// not mean going deaf.
	if err := a.Hide("app:4", true); err != nil {
		t.Fatalf("hide: %v", err)
	}
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no nudge after the channel was drained")
	}
}

// Two windows (or a window and a tray) both see every change.
func TestSubscribeReachesEverySubscriber(t *testing.T) {
	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
	one, stopOne := a.Subscribe()
	two, stopTwo := a.Subscribe()
	defer stopOne()
	defer stopTwo()

	a.noteSync("something happened")
	for i, ch := range []<-chan struct{}{one, two} {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Errorf("subscriber %d was not nudged", i)
		}
	}
}

// Unsubscribing stops the nudges and closes the channel, so a shell that
// ranges over it ends rather than blocking forever. Calling it twice is
// not a panic — a window closing and the app quitting both want to.
func TestUnsubscribeIsFinalAndIdempotent(t *testing.T) {
	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
	ch, stop := a.Subscribe()
	stop()
	stop()

	a.noteSync("after the unsubscribe")
	if _, open := <-ch; open {
		t.Error("an unsubscribed channel still received a nudge")
	}
	a.mu.Lock()
	left := len(a.subs)
	a.mu.Unlock()
	if left != 0 {
		t.Errorf("%d subscriber(s) left registered after unsubscribing", left)
	}
}

// MarkSeen is what puts the custody poll into its fast mode. Without it
// a window shows custody up to a minute old while someone watches it —
// the subtlest way this facade could be wrong.
func TestMarkSeenPacesTheCustodyPoll(t *testing.T) {
	a := NewApp(Config{ServerURL: "http://example.invalid", Token: "tok"},
		filepath.Join(t.TempDir(), "config.json"))

	a.mu.Lock()
	idle := a.pollIntervalLocked()
	a.mu.Unlock()
	if idle != syncPollEvery {
		t.Fatalf("interval with nobody looking = %s, want %s", idle, syncPollEvery)
	}

	a.MarkSeen()
	a.mu.Lock()
	watching := a.pollIntervalLocked()
	a.mu.Unlock()
	if watching != syncPollWatching {
		t.Errorf("interval after MarkSeen = %s, want the page's fast pace (%s)", watching, syncPollWatching)
	}

	// And it lapses when the window stops calling — a window hidden to
	// the tray must not keep the poll running hot forever.
	a.mu.Lock()
	a.pageSeen = time.Now().Add(-pageWatchWindow - time.Second)
	lapsed := a.pollIntervalLocked()
	a.mu.Unlock()
	if lapsed != syncPollEvery {
		t.Errorf("interval after the watch window = %s, want %s", lapsed, syncPollEvery)
	}
}

// Covers are fetched once and held as bytes. The web shelf learned this
// the hard way with <img> remounts; an in-process UI has the same bug
// available to it unless the engine answers from cache.
func TestCoverIsFetchedOnceAndCached(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte("\xff\xd8\xff-not-really-a-jpeg"))
	}))
	defer srv.Close()

	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
	a.mu.Lock()
	a.art = map[string]gameArt{"app:1": {Name: "A Game", Cover: srv.URL + "/cover.jpg"}}
	a.mu.Unlock()

	first, err := a.Cover("app:1")
	if err != nil {
		t.Fatalf("cover: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("no cover bytes came back")
	}
	for range 5 {
		again, err := a.Cover("app:1")
		if err != nil {
			t.Fatalf("cover again: %v", err)
		}
		if string(again) != string(first) {
			t.Fatal("the cached cover differs from the fetched one")
		}
	}
	if hits != 1 {
		t.Errorf("the cover was fetched %d times; a cached cover is what stops a shelf flickering", hits)
	}
}

// A game with no cover is an answer, not a failure — a shelf without
// covers is still a shelf — and it must not be re-asked either.
func TestCoverMissesAreCachedToo(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	defer srv.Close()

	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
	a.mu.Lock()
	a.art = map[string]gameArt{
		"app:1": {Name: "Coverless", Cover: ""},
		"app:2": {Name: "Gone", Cover: srv.URL + "/missing.jpg"},
	}
	a.mu.Unlock()

	data, err := a.Cover("app:1")
	if err != nil || data != nil {
		t.Errorf("a game with no cover = (%v, %v), want (nil, nil)", data, err)
	}
	if _, err := a.Cover("app:2"); err == nil {
		t.Error("a 404 cover should report why, once")
	}
	for range 3 {
		if _, err := a.Cover("app:2"); err != nil {
			t.Errorf("a cached miss must answer from cache, not re-ask: %v", err)
		}
	}
	if hits != 1 {
		t.Errorf("a missing cover was fetched %d times; misses must be cached", hits)
	}
}

// The typed verbs refuse before there is a service to talk to, and the
// refusal names the setting rather than the network — the same answer
// the routes have always given.
func TestCustodyVerbsRefuseWithoutAService(t *testing.T) {
	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
	verbs := map[string]func() error{
		"checkin":    func() error { return a.Checkin(1) },
		"checkpoint": func() error { return a.Checkpoint(1) },
		"renew":      func() error { return a.Renew(1) },
		"claim":      func() error { return a.Claim(1) },
		"checkout": func() error {
			_, err := a.Checkout(1, false, true)
			return err
		},
	}
	for name, fn := range verbs {
		if err := fn(); err == nil {
			t.Errorf("%s succeeded with nothing configured", name)
		}
	}
	if _, err := a.SyncNow(); err == nil {
		t.Error("sync now succeeded with nothing configured")
	}
}

// SetConfig is a partial write: a blank token box keeps the saved token,
// which is the whole reason the settings dialog can save a Steam folder
// without re-asking for a credential.
func TestSetConfigKeepsWhatItIsNotGiven(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	a := NewApp(Config{ServerURL: "https://vault.example.com", Token: "keepme"}, path)

	dirs := []string{"D:\\SteamLibrary"}
	if err := a.SetConfig(ConfigUpdate{SteamDirs: &dirs}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	st := a.Snapshot()
	if !st.Config.TokenSet {
		t.Error("saving a Steam folder erased the saved token")
	}
	if st.Config.ServerURL != "https://vault.example.com" {
		t.Errorf("server url = %q; an absent field must keep its value", st.Config.ServerURL)
	}
	if len(st.Config.SteamDirs) != 1 || st.Config.SteamDirs[0] != "D:\\SteamLibrary" {
		t.Errorf("steam dirs = %v", st.Config.SteamDirs)
	}

	off := false
	if err := a.SetConfig(ConfigUpdate{LaunchOnCheckout: &off}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	if a.Snapshot().Config.LaunchOnCheckout {
		t.Error("an explicit false for launch-on-checkout did not stick")
	}
	// And it survives a reload: this is a setting, not a session.
	saved := NewApp(mustLoad(t, path), path)
	if saved.Snapshot().Config.LaunchOnCheckout {
		t.Error("the setting did not reach the config file")
	}
}

func mustLoad(t *testing.T, path string) Config {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	cfg, err := parseConfig(data)
	if err != nil {
		t.Fatalf("parsing config: %v", err)
	}
	return cfg
}
