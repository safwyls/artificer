package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/safwyls/flamekeeper/internal/store"
	"github.com/safwyls/flamekeeper/internal/flameagent"
)

// supervisorServer registers a server whose agent supervises a fake game.
// Host points at a closed loopback port so prepareForStop's RCON/REST
// courtesy calls fail fast instead of timing out against a black hole.
func supervisorServer(t *testing.T, app *testApp) int64 {
	t.Helper()
	install := t.TempDir()
	game := `#!/bin/sh
trap 'echo "caught TERM"; exit 0' TERM
echo "Palworld server booting"
while true; do sleep 0.05; done
`
	if err := os.WriteFile(filepath.Join(install, "RSDragonwildsServer.sh"), []byte(game), 0o755); err != nil {
		t.Fatal(err)
	}
	steamcmd := filepath.Join(t.TempDir(), "steamcmd")
	if err := os.WriteFile(steamcmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	agent, err := flameagent.New(flameagent.Config{
		Token: agentToken, InstallDir: install, SteamCmd: steamcmd, Version: "test",
		Mode: "supervisor", StopGrace: time.Second,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		req, _ := http.NewRequest("POST", srv.URL+"/v1/power/stop", nil)
		req.Header.Set("Authorization", "Bearer "+agentToken)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	})

	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "supervised", Host: "127.0.0.1", RCONPort: 1, RESTPort: 1, UseREST: true, Enabled: true,
		AgentURL: srv.URL, AgentToken: agentToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPowerViaSupervisorAgent(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := supervisorServer(t, app)
	base := "/api/servers/" + itoa(id) + "/container"

	// Status flows from the agent even though docker is nil and no
	// container name is set.
	rec := app.do(t, "GET", base, nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d (body %s)", rec.Code, rec.Body)
	}
	var state struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Running bool   `json:"running"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Running || !strings.Contains(state.Name, "flameagent") {
		t.Fatalf("initial state = %+v, want stopped flameagent-managed", state)
	}

	// Start → running.
	rec = app.do(t, "POST", base+"/start", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("start: %d (body %s)", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil || !state.Running {
		t.Fatalf("post-start state = %+v, %v", state, err)
	}

	// Game output through the container-logs endpoint.
	deadline := time.Now().Add(3 * time.Second)
	for {
		rec = app.do(t, "GET", base+"/logs?tail=50", nil, admin)
		if rec.Code != http.StatusOK {
			t.Fatalf("logs: %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "booting") {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatal("game output never appeared via /container/logs")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Stop (prepareForStop's RCON courtesy fails fast against the closed
	// port and must not block the stop) → stopped.
	rec = app.do(t, "POST", base+"/stop", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop: %d (body %s)", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil || state.Running {
		t.Fatalf("post-stop state = %+v, %v", state, err)
	}
}
