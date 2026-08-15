package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/safwyls/flametender/internal/store"
)

// An Enshrouded server end to end through the API: the config editor speaks
// esconfig, rotate-admin-password works, the player list is derived from
// the agent's log tail, and the command tier answers 501 with the
// capability truth rather than 502.

const esCfg = `{
    "name": "Grimwood Bastion",
    "queryPort": 15637,
    "slotCount": 8,
    "gameSettings": {
        "playerHealthFactor": 1.5,
        "enableDurability": true
    },
    "userGroups": [
        {"name": "Keepers", "password": "old-password", "canKickBan": true},
        {"name": "Friends", "password": "join-pw", "canKickBan": false}
    ]
}
`

func newEnshroudedServer(t *testing.T, app *testApp, configPath string) int64 {
	t.Helper()
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "Grimwood", Game: "enshrouded", Host: "127.0.0.1",
		Enabled: true, ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	return id
}

func TestEnshroudedConfigEditor(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "enshrouded_server.json")
	if err := os.WriteFile(file, []byte(esCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	id := newEnshroudedServer(t, app, file)

	rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/config", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config: %d %s", rec.Code, rec.Body)
	}
	var res struct {
		Settings []struct {
			Key, Value, Type, Section string
		} `json:"settings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]struct{ Value, Type, Section string }{}
	for _, s := range res.Settings {
		byKey[s.Key] = struct{ Value, Type, Section string }{s.Value, s.Type, s.Section}
	}
	if got := byKey["name"]; got.Value != "Grimwood Bastion" || got.Section != "server" {
		t.Fatalf("name not served: %+v", res.Settings)
	}
	// gameSettings scalars surface flattened and typed, so the editor can
	// render a numeric control for them.
	if got := byKey["gameSettings.playerHealthFactor"]; got.Value != "1.5" || got.Type != "float" || got.Section != "gameSettings" {
		t.Fatalf("gameSettings.playerHealthFactor not served: %+v", res.Settings)
	}

	rec = app.do(t, http.MethodPut, fmt.Sprintf("/api/servers/%d/config", id),
		map[string]any{"changes": map[string]string{
			"name":                            "Renamed Keep",
			"gameSettings.playerHealthFactor": "2.5",
		}}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("put config: %d %s", rec.Code, rec.Body)
	}
	data, _ := os.ReadFile(file)
	var doc struct {
		Name         string `json:"name"`
		GameSettings struct {
			PlayerHealthFactor float64 `json:"playerHealthFactor"`
		} `json:"gameSettings"`
		UserGroups []struct {
			Password string `json:"password"`
		} `json:"userGroups"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("edited config does not parse: %v\n%s", err, data)
	}
	if doc.Name != "Renamed Keep" || doc.GameSettings.PlayerHealthFactor != 2.5 {
		t.Fatalf("edit not written:\n%s", data)
	}
	// The role groups are not part of the flat view and must survive an
	// edit untouched (the never-remove policy the ini editors set).
	if len(doc.UserGroups) != 2 || doc.UserGroups[0].Password != "old-password" {
		t.Fatalf("userGroups disturbed by a settings edit:\n%s", data)
	}
}

func TestEnshroudedRotateAdminPassword(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "enshrouded_server.json")
	if err := os.WriteFile(file, []byte(esCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	id := newEnshroudedServer(t, app, file)

	rec := app.do(t, http.MethodPost, fmt.Sprintf("/api/servers/%d/config/rotate-admin-password", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", rec.Code, rec.Body)
	}
	var res struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Password) != 24 {
		t.Fatalf("password = %q, want 24 hex chars", res.Password)
	}
	data, _ := os.ReadFile(file)
	var doc struct {
		UserGroups []struct {
			Password   string `json:"password"`
			CanKickBan bool   `json:"canKickBan"`
		} `json:"userGroups"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	admins := ""
	for _, g := range doc.UserGroups {
		if g.CanKickBan {
			admins = g.Password
		}
	}
	if admins != res.Password {
		t.Fatal("new password not on the kick/ban-capable group")
	}
	if strings.Contains(string(data), "old-password") {
		t.Fatal("old password still on disk")
	}
	// The non-admin group's password is not the rotated credential and must
	// not have been touched.
	if !strings.Contains(string(data), "join-pw") {
		t.Fatal("rotating the admin password disturbed the join password")
	}
}

func TestRotateAdminPasswordNeedsAConfigPath(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := newEnshroudedServer(t, app, "")
	rec := app.do(t, http.MethodPost, fmt.Sprintf("/api/servers/%d/config/rotate-admin-password", id), nil, admin)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("rotate without a config path: %d, want the setup-guidance 400", rec.Code)
	}
}

// fakeEnshroudedAgent is a supervisor-shaped flameagent whose game is
// running and whose log ring holds the given lines — the whole transport
// the enshrouded client derives its state from.
func fakeEnshroudedAgent(t *testing.T, lines []string) string {
	t.Helper()
	started := time.Now().UTC().Add(-time.Minute)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/health":
			json.NewEncoder(w).Encode(map[string]any{
				"agent": "flameagent", "mode": "supervisor", "apiVersion": 3,
				"installDir": "/enshrouded", "installDirOk": true,
				"game": map[string]any{"state": "running", "startedAt": started},
			})
		case "/v1/power/logs":
			json.NewEncoder(w).Encode(map[string]any{"lines": lines})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// The player list is derived from the agent's log tail: an accepted-id
// line supplies the SteamID64, the Added Peer line opens the session, and
// Removed Peer closes it. Names never appear in Enshrouded's log, so the
// id doubles as the display name.
func TestEnshroudedPlayersDerivedFromAgentLogs(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	agentURL := fakeEnshroudedAgent(t, []string{
		"[online] Session accepted with peer ( id 76561198000000001 ).",
		"[online] Added Peer #0.",
		"[online] Session accepted with peer ( id 76561198000000002 ).",
		"[online] Added Peer #1.",
		"[online] Removed Peer #1.",
	})
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "Grimwood", Game: "enshrouded", Host: "127.0.0.1", Enabled: true,
		AgentURL: agentURL, AgentToken: agentToken,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/players", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("players: %d %s", rec.Code, rec.Body)
	}
	var players []struct {
		Name      string `json:"name"`
		PlayerUID string `json:"playerId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &players); err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 {
		t.Fatalf("players = %+v, want just the peer that never left", players)
	}
	if players[0].PlayerUID != "76561198000000001" || players[0].Name != "76561198000000001" {
		t.Errorf("player = %+v, want the SteamID64 as both id and name", players[0])
	}
}

func TestEnshroudedCommandsAnswer501(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := newEnshroudedServer(t, app, "")

	cases := []struct {
		path string
		body any
	}{
		{"broadcast", map[string]string{"message": "hi"}},
		{"kick", map[string]string{"playerUid": "u"}},
		{"ban", map[string]string{"playerUid": "u"}},
		{"unban", map[string]string{"playerUid": "u"}},
		{"save", nil},
		{"shutdown", map[string]any{"waitSeconds": 5, "message": "bye"}},
	}
	for _, tc := range cases {
		rec := app.do(t, http.MethodPost, fmt.Sprintf("/api/servers/%d/%s", id, tc.path), tc.body, admin)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s: %d %s, want 501", tc.path, rec.Code, rec.Body)
		}
		// The reason must say where the ability actually lives — kick and
		// ban exist in-game, and the 501 is where an operator learns that.
		if tc.path == "kick" && !strings.Contains(rec.Body.String(), "in-game player menu") {
			t.Errorf("kick refusal should point at the in-game player menu: %s", rec.Body)
		}
	}
}

func TestEnshroudedFeaturesOnServerPayload(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := newEnshroudedServer(t, app, "")
	rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("get server: %d", rec.Code)
	}
	var res struct {
		Game     string   `json:"game"`
		Features []string `json:"features"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Game != "enshrouded" {
		t.Fatalf("game = %q", res.Game)
	}
	want := []string{"pals", "saves", "logs"}
	if len(res.Features) != len(want) {
		t.Fatalf("features = %v, want %v", res.Features, want)
	}
	for i := range want {
		if res.Features[i] != want[i] {
			t.Fatalf("features = %v, want %v", res.Features, want)
		}
	}
}

// The capabilities endpoint is what lets the console stop guessing. For
// Enshrouded every command is a stable no — the game has no command
// channel at all — and the reasons must say where each ability actually
// lives, because that reason is the only thing telling an operator what
// their real options are.
func TestEnshroudedCapabilitiesReportWhereAbilitiesLive(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := newEnshroudedServer(t, app, "")

	rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/capabilities", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("capabilities: %d %s", rec.Code, rec.Body)
	}
	var got struct {
		Probed   bool `json:"probed"`
		Commands map[string]struct {
			Supported bool   `json:"supported"`
			Reason    string `json:"reason"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Probed {
		t.Error("enshrouded should be probeable; the UI falls back to optimism otherwise")
	}
	save, ok := got.Commands["save"]
	if !ok {
		t.Fatal("no answer for save")
	}
	if save.Supported {
		t.Error("save reported supported, but the game has no on-demand save trigger")
	}
	if !strings.Contains(save.Reason, "autosaves") {
		t.Errorf("reason should explain what covers saves instead, got %q", save.Reason)
	}
	kick, ok := got.Commands["kick"]
	if !ok {
		t.Fatal("no answer for kick")
	}
	if kick.Supported || !strings.Contains(kick.Reason, "in-game player menu") {
		t.Errorf("kick should point at the in-game player menu: %+v", kick)
	}
	// The endpoint answers for every command the console might offer, so a
	// caller never has to special-case a missing key.
	for _, op := range []string{"broadcast", "ban", "unban", "shutdown"} {
		if _, ok := got.Commands[op]; !ok {
			t.Errorf("no answer for %s", op)
		}
	}
}

// A game whose client can't be probed must not be reported as incapable —
// that would hide working controls. Absent knowledge, the answer is the
// optimism every caller had before probing existed.
func TestCapabilitiesDefaultToSupportedForAnUnprobeableGame(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	_, addr := newFakeGame(t)
	id := newServerPointingAt(t, app, addr, nil)

	rec := app.do(t, http.MethodGet, fmt.Sprintf("/api/servers/%d/capabilities", id), nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("capabilities: %d %s", rec.Code, rec.Body)
	}
	var got struct {
		Probed   bool                      `json:"probed"`
		Commands map[string]map[string]any `json:"commands"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Probed {
		t.Error("the test game has no prober, so probed should be false")
	}
	if supported, _ := got.Commands["save"]["supported"].(bool); !supported {
		t.Error("an unprobeable game must not have its commands hidden")
	}
}
