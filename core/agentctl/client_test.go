package agentctl_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/safwyls/artificer/core/agent"
	"github.com/safwyls/artificer/core/agentctl"
)

func TestBaseURLIsTheConfiguredEndpoint(t *testing.T) {
	client, err := agentctl.New("http://flameagent-main:8811/", token)
	if err != nil {
		t.Fatal(err)
	}
	// The trailing slash is trimmed, so path joining can't double it.
	if client.BaseURL() != "http://flameagent-main:8811" {
		t.Errorf("BaseURL = %q", client.BaseURL())
	}
}

// Power and the file verbs are supervisor-mode features. A companion agent
// answers 400, which the client surfaces as a rejection so flametender can fall
// back to the docker proxy instead of reporting an outage.
func TestSupervisorOnlyVerbsAgainstACompanion(t *testing.T) {
	srv := newAgent(t, "exit 0")
	client, err := agentctl.New(srv.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := client.Power(ctx, "start", 0); !errors.Is(err, agentctl.ErrRejected) {
		t.Errorf("Power on a companion: %v, want ErrRejected", err)
	}
	if _, err := client.Power(ctx, "stop", 5*time.Second); !errors.Is(err, agentctl.ErrRejected) {
		t.Errorf("graceful Power on a companion: %v, want ErrRejected", err)
	}
	if _, err := client.GameLogs(ctx, 50); !errors.Is(err, agentctl.ErrRejected) {
		t.Errorf("GameLogs on a companion: %v, want ErrRejected", err)
	}
}

// newAgentWithConfig seeds the settings file the supervisor (or the game)
// writes before first boot; the agent edits it in place rather than
// creating one, so an install that has never run has nothing to serve.
func newAgentWithConfig(t *testing.T, cfg string) *agentctl.Client {
	t.Helper()
	install := t.TempDir()
	cfgPath := filepath.Join(install, "gametest.json")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	agent, err := agent.New(agent.Config{
		Token: token, InstallDir: install, Version: "test", Game: ctlTestGame(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	client, err := agentctl.New(srv.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestConfigRoundTrip(t *testing.T) {
	// Valid JSON both ways: the agent validates uploads now, so the round
	// trip has to speak the file's real shape.
	const seed = `{"name":"old"}`
	client := newAgentWithConfig(t, seed)
	ctx := context.Background()

	got, err := client.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if string(got) != seed {
		t.Errorf("GetConfig returned %q, want the seeded file", got)
	}

	const updated = `{"name":"new"}`
	if err := client.PutConfig(ctx, []byte(updated)); err != nil {
		t.Fatalf("PutConfig: %v", err)
	}
	got, err = client.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig after a write: %v", err)
	}
	if string(got) != updated {
		t.Errorf("config round-trip mismatch:\n got %q\nwant %q", got, updated)
	}
}

// An install that has never booted has no settings file, which is a
// rejection the UI can explain rather than a transport failure.
func TestGetConfigOnAnUnbootedInstall(t *testing.T) {
	srv := newAgent(t, "exit 0")
	client, err := agentctl.New(srv.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetConfig(context.Background()); !errors.Is(err, agentctl.ErrRejected) {
		t.Errorf("GetConfig with no config file: %v, want ErrRejected", err)
	}
}

// An agent that isn't there at all is a transport failure, not a rejection —
// the distinction is what lets flametender say "unreachable" rather than
// "misconfigured".
func TestUnreachableAgentIsNotARejection(t *testing.T) {
	client, err := agentctl.New("http://127.0.0.1:1", token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Health(context.Background())
	if err == nil {
		t.Fatal("a dead endpoint reported success")
	}
	if errors.Is(err, agentctl.ErrRejected) {
		t.Errorf("an unreachable agent was reported as a rejection: %v", err)
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("error should say unreachable: %v", err)
	}
}
