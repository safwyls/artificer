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
	"testing"
	"time"

	"github.com/safwyls/palcon/internal/palagent"
	"github.com/safwyls/palcon/internal/store"
)

const agentToken = "api-test-agent-token-0123456789"

// agentServer runs a real palagent over a fake install dir and registers a
// server row pointing at it, exercising the palcon→agent path end to end.
func agentServer(t *testing.T, app *testApp) (int64, string) {
	t.Helper()
	install := fakeInstallDir(t)
	steamcmd := filepath.Join(t.TempDir(), "steamcmd")
	script := "#!/bin/sh\necho \"Success! App '2394010' fully installed.\"\n"
	if err := os.WriteFile(steamcmd, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	agent, err := palagent.New(palagent.Config{
		Token: agentToken, InstallDir: install, SteamCmd: steamcmd, Version: "test",
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)

	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "agented", Host: "10.0.0.5", RCONPort: 25575, RESTPort: 8212, UseREST: true, Enabled: true,
		AgentURL: srv.URL, AgentToken: agentToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id, install
}

func TestAgentBackedClearSteamCache(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, install := agentServer(t, app)

	rec := app.do(t, "POST", "/api/servers/"+itoa(id)+"/steam-cache/clear", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear: got %d (body %s)", rec.Code, rec.Body)
	}
	if got := decodeMap(t, rec)["removed"]; got != float64(3) {
		t.Errorf("removed = %v, want 3", got)
	}
	if entries, err := os.ReadDir(filepath.Join(install, "steamapps")); err != nil || len(entries) != 0 {
		t.Errorf("agent did not empty steamapps: %v %d", err, len(entries))
	}
}

func TestSteamUpdateLifecycle(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, _ := agentServer(t, app)
	base := "/api/servers/" + itoa(id) + "/steam/update"

	rec := app.do(t, "POST", base, map[string]bool{"validate": true}, admin)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start: got %d (body %s)", rec.Code, rec.Body)
	}

	// Poll the status endpoint (backed by agent health) until it settles.
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec = app.do(t, "GET", base, nil, admin)
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d", rec.Code)
		}
		var res struct {
			Job *struct {
				State string `json:"state"`
			} `json:"job"`
			Agent struct {
				APIVersion int `json:"apiVersion"`
			} `json:"agent"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		if res.Agent.APIVersion != palagent.APIVersion {
			t.Fatalf("status did not report agent apiVersion: %s", rec.Body)
		}
		if res.Job != nil && res.Job.State != "running" {
			if res.Job.State != "done" {
				t.Fatalf("job state = %q, want done", res.Job.State)
			}
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatal("update never finished")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSteamUpdateRequiresAgent(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	// Install path only — cache clearing works locally, but updates need
	// an agent.
	id := installServer(t, app, t.TempDir())
	if rec := app.do(t, "POST", "/api/servers/"+itoa(id)+"/steam/update", nil, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("update without agent: got %d, want 400", rec.Code)
	}
}
