package enshrouded_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/safwyls/artificer/core/api/apitest"
	"net/http"
	"testing"
	"time"

	"github.com/safwyls/artificer/core/store"
)

// The Steam query as the console consumes it. Two sources describe a live
// Enshrouded server and they are deliberately not interchangeable: the
// game's own query answers what is true right now, and the log tail
// answers who those people are and whether the server ever finished
// coming up. These tests pin which one wins where.

type infoDTO struct {
	ServerName  string `json:"servername"`
	Version     string `json:"version"`
	PlayerCount int    `json:"playerCount"`
	Transport   string `json:"transport"`
	Readiness   string `json:"readiness"`
}

// a2sReply is a query answer for a 4-slot server with 3 people on it —
// deliberately not the 16-slot hard cap, since telling those apart is the
// point.
func a2sReply(players, maxPlayers int) map[string]any {
	return map[string]any{
		"info": map[string]any{
			"name":       "Grimwood Bastion",
			"map":        "L_World",
			"players":    players,
			"maxPlayers": maxPlayers,
			"version":    "1024233",
			"appId":      2278520,
		},
		"players": []any{},
	}
}

func esServerWithAgent(t *testing.T, app *apitest.App, agentURL string) int64 {
	t.Helper()
	id, err := app.Store.CreateServer(context.Background(), &store.Server{
		Name: "Grimwood", Game: "enshrouded", Host: "127.0.0.1", Enabled: true,
		AgentURL: agentURL, AgentToken: agentToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func serverInfo(t *testing.T, app *apitest.App, id int64, admin []*http.Cookie) infoDTO {
	t.Helper()
	rec := app.Do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/info", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("info: %d %s", rec.Code, rec.Body)
	}
	var got infoDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode %q: %v", rec.Body, err)
	}
	return got
}

// The count the game reports beats the count inferred from the log, and
// it is the case where they disagree that proves which one is being used:
// a player whose join line has scrolled out of the agent's ring is
// invisible to the log and still present to the game.
func TestQueryOwnsThePresentAndTheLogOwnsTheRoster(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	agentURL := fakeEnshroudedAgentWith(t, []string{
		"[Session] 'HostOnline' (up)!",
		"[online] Added peer 0(1) (steamid:76561190000000001)",
		"[server] Player 'Ember' logged in with Permissions:",
	}, time.Now().UTC().Add(-time.Minute), a2sReply(3, 4))
	id := esServerWithAgent(t, app, agentURL)

	got := serverInfo(t, app, id, admin)
	if got.PlayerCount != 3 {
		t.Errorf("playerCount = %d, want the game's own 3 rather than the log's 1", got.PlayerCount)
	}
	if got.ServerName != "Grimwood Bastion" || got.Version != "1024233" {
		t.Errorf("the game's name and build didn't come through: %+v", got)
	}
	// The transport says which sources answered, so "why does this number
	// look different" has an answer on the page.
	if got.Transport != "agent+a2s" {
		t.Errorf("transport = %q, want agent+a2s", got.Transport)
	}

	// The roster stays log-derived: A2S carries names but no account ids,
	// so it cannot produce a row anyone could moderate.
	rec := app.Do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/players", id), nil, admin)
	var players []struct {
		Name   string `json:"name"`
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &players); err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].UserID != "76561190000000001" {
		t.Fatalf("players = %+v, want the one identifiable session", players)
	}
}

// A server that doesn't answer its query is the ordinary state of one
// still booting. The dashboard must not go blank for it.
func TestInfoFallsBackToTheLogWhenTheQueryIsSilent(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	agentURL := fakeEnshroudedAgentWith(t, []string{
		"[Session] 'HostOnline' (up)!",
		"[online] Added peer 0(1) (steamid:76561190000000001)",
		"[server] Player 'Ember' logged in with Permissions:",
	}, time.Now().UTC().Add(-time.Minute), nil)
	id := esServerWithAgent(t, app, agentURL)

	got := serverInfo(t, app, id, admin)
	if got.PlayerCount != 1 {
		t.Errorf("playerCount = %d, want the log-derived 1", got.PlayerCount)
	}
	if got.Transport != "agent" {
		t.Errorf("transport = %q; a silent query should not claim a2s answered", got.Transport)
	}
	// And the log still knows the thing the query never could.
	if got.Readiness != "ready" {
		t.Errorf("readiness = %q, want ready", got.Readiness)
	}
}

// The configured slot count exists nowhere but the query: the log never
// mentions it, so charts have been drawing against the game's 16-slot
// hard cap.
func TestMetricsTakeTheRealSlotCountFromTheQuery(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	agentURL := fakeEnshroudedAgentWith(t, nil, time.Now().UTC().Add(-time.Minute), a2sReply(3, 4))
	id := esServerWithAgent(t, app, agentURL)

	rec := app.Do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/metrics", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics: %d %s", rec.Code, rec.Body)
	}
	var m struct {
		Current int `json:"currentplayernum"`
		Max     int `json:"maxplayernum"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Max != 4 {
		t.Errorf("maxplayernum = %d, want the configured 4 rather than the 16-slot cap", m.Max)
	}
	if m.Current != 3 {
		t.Errorf("currentplayernum = %d, want 3", m.Current)
	}
}

// Readiness is the difference between a friend joining and a friend
// getting an error: the port is bound and the process is "running" for
// some time before anyone can get in.
func TestReadiness(t *testing.T) {
	cases := []struct {
		name    string
		lines   []string
		started time.Time
		want    string
	}{
		{
			"the host-online line is the proof",
			[]string{"[Session] 'HostOnline' (up)!"},
			time.Now().UTC().Add(-time.Minute),
			"ready",
		},
		{
			"a freshly started process without it is still coming up",
			[]string{"[server] loading world"},
			time.Now().UTC().Add(-time.Minute),
			"starting",
		},
		{
			// The marker is logged once, at boot, and the agent's ring holds
			// about 80 minutes. A console that started watching a long-running
			// server never sees it — and must not therefore call that server
			// "starting" forever.
			"a long-running server without it is unknown, not starting",
			[]string{"[server] idle"},
			time.Now().UTC().Add(-4 * time.Hour),
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, admin := newTestAppWithAdmin(t)
			id := esServerWithAgent(t, app, fakeEnshroudedAgentWith(t, tc.lines, tc.started, nil))
			if got := serverInfo(t, app, id, admin).Readiness; got != tc.want {
				t.Errorf("readiness = %q, want %q", got, tc.want)
			}
		})
	}
}

// The query answering is deliberately not proof of readiness: the game
// and the query share one UDP port, so a reply means the socket is up —
// which is exactly what happens too early to promise anyone a join.
func TestAnAnsweringQueryIsNotTreatedAsReady(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	agentURL := fakeEnshroudedAgentWith(t, []string{"[server] loading world"},
		time.Now().UTC().Add(-time.Minute), a2sReply(0, 4))
	id := esServerWithAgent(t, app, agentURL)

	if got := serverInfo(t, app, id, admin).Readiness; got != "starting" {
		t.Errorf("readiness = %q, want starting", got)
	}
}
