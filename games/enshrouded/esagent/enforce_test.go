// The enforcement suite, home from flameagent (drift ledger, agent-kit
// row): what PrepareRuntime concretely does for Enshrouded — seed a
// complete config before the first start so the server is never open and
// unnamed, enforce exactly the operator's identity settings into an
// existing file, and touch nothing else. Runs the real kit with the real
// esagent spec over a fake game process.
package esagent_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/safwyls/sampo/core/agent"
	"github.com/safwyls/sampo/games/enshrouded/esagent"
)

const testToken = "test-token-0123456789abcdef"

const steadyGame = `trap 'echo "caught INT, flushing world"; exit 0' INT TERM
echo "Enshrouded server booting"
while true; do sleep 0.05; done`

func do(t *testing.T, srv *httptest.Server, method, path, token string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, buf)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp, m
}

func writeGame(t *testing.T, install, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(install, "game.sh"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func gameState(t *testing.T, srv *httptest.Server) map[string]any {
	t.Helper()
	_, health := do(t, srv, "GET", "/v1/health", testToken, nil)
	game, _ := health["game"].(map[string]any)
	if game == nil {
		t.Fatalf("health has no game block: %v", health)
	}
	return game
}

func waitGameState(t *testing.T, srv *httptest.Server, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if game := gameState(t, srv); game["state"] == want {
			return game
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("game never reached state %q (now %v)", want, gameState(t, srv))
	return nil
}

// readServerJSON parses the enshrouded_server.json under install.
func readServerJSON(t *testing.T, install string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(install, "enshrouded_server.json"))
	if err != nil {
		t.Fatalf("reading enshrouded_server.json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("enshrouded_server.json does not parse: %v\n%s", err, data)
	}
	return doc
}

// groupPassword digs the password out of the first role group matching the
// wanted canKickBan capability — the same match Enforce uses, so a renamed
// group can't fool the assertion either.
func groupPassword(t *testing.T, doc map[string]any, admin bool) string {
	t.Helper()
	groups, _ := doc["userGroups"].([]any)
	for _, g := range groups {
		m, _ := g.(map[string]any)
		if m == nil {
			continue
		}
		if kickBan, _ := m["canKickBan"].(bool); kickBan == admin {
			pw, _ := m["password"].(string)
			return pw
		}
	}
	t.Fatalf("no role group with canKickBan=%v in %v", admin, doc["userGroups"])
	return ""
}

// A supervised server keeps the operator's identity settings enforced:
// every start rewrites the name and the role-group passwords in an
// existing enshrouded_server.json, whatever edits accumulated there.
func TestSupervisorEnforcesManagementConfig(t *testing.T) {
	install := t.TempDir()
	writeGame(t, install, steadyGame)
	seed := `{"name":"Grimwood","queryPort":15637,"userGroups":[` +
		`{"name":"Admins","password":"stale","canKickBan":true},` +
		`{"name":"Friends","password":"oldjoin","canKickBan":false}]}`
	if err := os.WriteFile(filepath.Join(install, "enshrouded_server.json"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	agent, err := agent.New(agent.Config{
		Token: testToken, InstallDir: install, SteamCmd: "/bin/true", Version: "test", Game: esagent.Game(esagent.WineConfig{}),
		Mode: "supervisor", GameCommand: "./game.sh", StopGrace: 500 * time.Millisecond,
		AdminPassword: "hunter2-but-longer",
		JoinPassword:  "friends-only",
		ServerName:    "Grimwood Bastion",
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		req, _ := http.NewRequest("POST", srv.URL+"/v1/power/stop", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	})

	do(t, srv, "POST", "/v1/power/start", testToken, nil)
	waitGameState(t, srv, "running")

	doc := readServerJSON(t, install)
	if doc["name"] != "Grimwood Bastion" {
		t.Errorf("name = %v, want the enforced server name", doc["name"])
	}
	if got := groupPassword(t, doc, true); got != "hunter2-but-longer" {
		t.Errorf("admin group password = %q, want the enforced one", got)
	}
	if got := groupPassword(t, doc, false); got != "friends-only" {
		t.Errorf("join group password = %q, want the enforced one", got)
	}
}

// The game would generate its own config on first boot — but that default
// is an *open* server named "Enshrouded Server". Seeding before the first
// start is what makes a provisioned server named and password-protected
// from its first second online.
func TestSupervisorSeedsConfigForAFreshInstall(t *testing.T) {
	install := t.TempDir()
	writeGame(t, install, steadyGame)
	agent, err := agent.New(agent.Config{
		Token: testToken, InstallDir: install, SteamCmd: "/bin/true", Version: "test", Game: esagent.Game(esagent.WineConfig{}),
		Mode: "supervisor", GameCommand: "./game.sh", StopGrace: 500 * time.Millisecond,
		AdminPassword: "hunter2-but-longer",
		JoinPassword:  "friends-only",
		ServerName:    "Grimwood Bastion",
		GamePort:      25637,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		req, _ := http.NewRequest("POST", srv.URL+"/v1/power/stop", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	})

	do(t, srv, "POST", "/v1/power/start", testToken, nil)
	waitGameState(t, srv, "running")

	doc := readServerJSON(t, install)
	if doc["name"] != "Grimwood Bastion" {
		t.Errorf("seeded name = %v, want the configured server name", doc["name"])
	}
	if port, _ := doc["queryPort"].(float64); port != 25637 {
		t.Errorf("seeded queryPort = %v, want the configured 25637", doc["queryPort"])
	}
	if got := groupPassword(t, doc, true); got != "hunter2-but-longer" {
		t.Errorf("seeded admin password = %q, want the configured one", got)
	}
	if got := groupPassword(t, doc, false); got != "friends-only" {
		t.Errorf("seeded join password = %q, want the configured one", got)
	}
}

// Enforcement rewrites the identity settings and nothing else: the rest of
// the file belongs to the operator (or the game), and a start must not
// bulldoze their edits. The .bak is the escape hatch when it ever does.
func TestSupervisorEnforcementLeavesUnrelatedKeysAlone(t *testing.T) {
	install := t.TempDir()
	writeGame(t, install, steadyGame)
	seed := `{"name":"Grimwood","slotCount":8,"operatorCustomKey":"keep-me","userGroups":[` +
		`{"name":"Admins","password":"stale","canKickBan":true}]}`
	cfgPath := filepath.Join(install, "enshrouded_server.json")
	if err := os.WriteFile(cfgPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	agent, err := agent.New(agent.Config{
		Token: testToken, InstallDir: install, SteamCmd: "/bin/true", Version: "test", Game: esagent.Game(esagent.WineConfig{}),
		Mode: "supervisor", GameCommand: "./game.sh", StopGrace: 500 * time.Millisecond,
		AdminPassword: "hunter2-but-longer",
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		req, _ := http.NewRequest("POST", srv.URL+"/v1/power/stop", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	})

	do(t, srv, "POST", "/v1/power/start", testToken, nil)
	waitGameState(t, srv, "running")

	doc := readServerJSON(t, install)
	if got := groupPassword(t, doc, true); got != "hunter2-but-longer" {
		t.Errorf("admin password = %q, want the enforced one", got)
	}
	if slots, _ := doc["slotCount"].(float64); slots != 8 {
		t.Errorf("slotCount = %v; enforcement touched a key it doesn't own", doc["slotCount"])
	}
	if doc["operatorCustomKey"] != "keep-me" {
		t.Errorf("operatorCustomKey = %v; enforcement dropped an unknown key", doc["operatorCustomKey"])
	}
	if _, err := os.Stat(cfgPath + ".bak"); err != nil {
		t.Errorf("no .bak kept of the pre-enforcement config: %v", err)
	}
}
