package dragonwilds_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/safwyls/dwcon/internal/game"
	"github.com/safwyls/dwcon/internal/games/dragonwilds"
)

// fakeAgent is a palagent just complete enough for the derived client:
// /v1/health with a supervised-game status, /v1/power/logs with a ring.
type fakeAgent struct {
	mu        sync.Mutex
	state     string
	startedAt time.Time
	lines     []string
}

func newFakeAgent(t *testing.T) (*fakeAgent, string) {
	t.Helper()
	f := &fakeAgent{state: "running", startedAt: time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/health":
			json.NewEncoder(w).Encode(map[string]any{
				"agent": "palagent", "apiVersion": 1, "mode": "supervisor",
				"game": map[string]any{"state": f.state, "startedAt": f.startedAt},
				"job":  nil,
			})
		case "/v1/power/logs":
			json.NewEncoder(w).Encode(map[string]any{"lines": f.lines})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func (f *fakeAgent) set(state string, lines ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = state
	f.lines = lines
}

func newClient(t *testing.T, url string) game.Client {
	t.Helper()
	return dragonwilds.New(game.Conn{AgentURL: url, AgentToken: "tok"})
}

func TestInfoAndPlayersDeriveFromAgent(t *testing.T) {
	agent, url := newFakeAgent(t)
	agent.set("running",
		"[x][1]LogDomMatcherSession: Player ADDED to session [aaaa000000000000000000000000aaaa]-[Vexmarrow]",
		"[x][2]LogDomMatcherSession: Player ADDED to session [bbbb000000000000000000000000bbbb]-[Kaelith]",
		"[x][3]LogDomMatcherSession: Player Removed from session [bbbb000000000000000000000000bbbb]-[Kaelith]",
	)
	c := newClient(t, url)
	info, err := c.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.PlayerCount != 1 || info.Transport != "agent" {
		t.Fatalf("info = %+v", info)
	}
	players, err := c.Players(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].Name != "Vexmarrow" || players[0].UserID != "aaaa000000000000000000000000aaaa" {
		t.Fatalf("players = %+v", players)
	}
}

func TestStoppedProcessReadsAsUnreachableNotUnsupported(t *testing.T) {
	agent, url := newFakeAgent(t)
	agent.set("stopped")
	c := newClient(t, url)
	_, err := c.Info(context.Background())
	if err == nil {
		t.Fatal("expected error for stopped process")
	}
	var unsupported *game.UnsupportedError
	if errors.As(err, &unsupported) {
		t.Fatal("a stopped process is a 502-shaped condition, not a capability gap")
	}
}

func TestTrackerSurvivesClientRebuilds(t *testing.T) {
	agent, url := newFakeAgent(t)
	agent.set("running", "[x][1]LogDomMatcherSession: Player ADDED to session [aaaa000000000000000000000000aaaa]-[Vexmarrow]")
	if _, err := newClient(t, url).Players(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A fresh client (as every API call builds) must still know Vexmarrow
	// even if his join line has scrolled out of the ring.
	agent.set("running", "[x][9]LogWorld: noise")
	players, err := newClient(t, url).Players(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].Name != "Vexmarrow" {
		t.Fatalf("players = %+v", players)
	}
}

func TestRestartResetsSessions(t *testing.T) {
	agent, url := newFakeAgent(t)
	agent.set("running", "[x][1]LogDomMatcherSession: Player ADDED to session [aaaa000000000000000000000000aaaa]-[Vexmarrow]")
	c := newClient(t, url)
	if _, err := c.Players(context.Background()); err != nil {
		t.Fatal(err)
	}
	agent.mu.Lock()
	agent.startedAt = agent.startedAt.Add(time.Hour)
	agent.lines = nil
	agent.mu.Unlock()
	players, err := c.Players(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 0 {
		t.Fatalf("players after restart = %+v", players)
	}
}

func TestCommandsReturnTypedUnsupported(t *testing.T) {
	_, url := newFakeAgent(t)
	c := newClient(t, url)
	ctx := context.Background()
	calls := map[string]func() error{
		"broadcast": func() error { return c.Broadcast(ctx, "hi") },
		"kick":      func() error { return c.Kick(ctx, "uid", "msg") },
		"ban":       func() error { return c.Ban(ctx, "uid", "msg") },
		"unban":     func() error { return c.Unban(ctx, "uid") },
		"save":      func() error { return c.Save(ctx) },
		"shutdown":  func() error { return c.Shutdown(ctx, 30, "msg") },
	}
	for op, call := range calls {
		var unsupported *game.UnsupportedError
		if err := call(); !errors.As(err, &unsupported) {
			t.Errorf("%s: err = %v, want UnsupportedError", op, err)
		} else if unsupported.Op != op {
			t.Errorf("%s: Op = %q", op, unsupported.Op)
		}
	}
}

func TestMetricsReportHonestSubset(t *testing.T) {
	agent, url := newFakeAgent(t)
	agent.set("running", "[x][1]LogDomMatcherSession: Player ADDED to session [aaaa000000000000000000000000aaaa]-[Vexmarrow]")
	ext, ok := newClient(t, url).(game.ExtendedClient)
	if !ok {
		t.Fatal("client should implement ExtendedClient for metrics")
	}
	m, err := ext.Metrics(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.CurrentPlayerNum != 1 || m.MaxPlayerNum != dragonwilds.MaxPlayers {
		t.Fatalf("metrics = %+v", m)
	}
	if m.UptimeSeconds <= 0 {
		t.Fatalf("uptime = %d, want > 0", m.UptimeSeconds)
	}
	if m.ServerFPS != 0 || m.ServerFrameTime != 0 {
		t.Fatalf("fps/frametime should stay zero (not reported): %+v", m)
	}
}

func TestNoAgentConfiguredIsDeferredError(t *testing.T) {
	c := dragonwilds.New(game.Conn{})
	if _, err := c.Info(context.Background()); err == nil {
		t.Fatal("expected configuration error")
	}
}

// The ids below are synthetic but real-shaped (32 hex, EOS ProductUserId
// form). A live account's id was used to establish the shape; committing
// someone's actual account identifier into a repo is a different thing
// entirely, so the fixtures are made up.
func TestCanonicalUID(t *testing.T) {
	const lower = "0a1b2c3d4e5f60718293a4b5c6d7e8f9"
	upper := strings.ToUpper(lower)

	cases := []struct{ name, in, want string }{
		// The case-folding case is the whole point: the Settings screen
		// shows a Player ID lowercase, while the values the server writes
		// itself (ServerGuid, WorldSaveGuid) are uppercase. Both spellings
		// have to land on the same key or a match silently never happens.
		{"lowercase id is left alone", lower, lower},
		{"uppercase id folds to lowercase", upper, lower},
		{"mixed case folds too", "0A1b2C3d4E5f60718293a4b5c6d7e8f9", lower},
		{"surrounding space is trimmed", "  " + upper + "  ", lower},
		// Anything not 32-hex is trimmed and otherwise untouched: folding
		// an unknown format could collide two distinct ids.
		{"non-hex is preserved", "Player-Name_42", "Player-Name_42"},
		{"wrong length is preserved", "0A1B2C3D", "0A1B2C3D"},
		{"empty stays empty", "   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dragonwilds.CanonicalUID(tc.in); got != tc.want {
				t.Errorf("CanonicalUID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Canonicalizing twice must not change the answer — the collector applies
// it on every poll, so a non-idempotent transform would drift.
func TestCanonicalUIDIsIdempotent(t *testing.T) {
	for _, in := range []string{"0A1B2C3D4E5F60718293A4B5C6D7E8F9", "  Name  ", ""} {
		once := dragonwilds.CanonicalUID(in)
		if twice := dragonwilds.CanonicalUID(once); twice != once {
			t.Errorf("CanonicalUID(%q): %q then %q", in, once, twice)
		}
	}
}
