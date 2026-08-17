package agentctl_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/safwyls/artificer/core/agent"
	"github.com/safwyls/artificer/core/agentctl"
)

const token = "client-test-token-0123456789"

func ctlTestGame() agent.Game {
	return agent.Game{
		AgentName: "gtagent", AppID: 4000001, DefaultGamePort: 25600,
		ConfigRelPath: "gametest.json", ConfigContentType: "application/json; charset=utf-8",
		SaveDirName: "savegame",
	}
}

// newAgent spins a real agent (the kit) over a temp install dir — the client is
// tested against the actual server implementation, not a stub, so the two
// can't drift.
func newAgent(t *testing.T, script string) *httptest.Server {
	t.Helper()
	install := t.TempDir()
	if err := os.MkdirAll(filepath.Join(install, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install, "steamapps", "appmanifest_2278520.acf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	steamcmd := filepath.Join(t.TempDir(), "steamcmd")
	if err := os.WriteFile(steamcmd, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	agent, err := agent.New(agent.Config{
		Token: token, InstallDir: install, SteamCmd: steamcmd, Version: "test", Game: ctlTestGame(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	return srv
}

func TestNewValidation(t *testing.T) {
	if _, err := agentctl.New("", token); !errors.Is(err, agentctl.ErrNotConfigured) {
		t.Errorf("empty url: got %v, want ErrNotConfigured", err)
	}
	if _, err := agentctl.New("flameagent:8811", token); err == nil {
		t.Error("schemeless url accepted")
	}
}

func TestClientRoundTrip(t *testing.T) {
	srv := newAgent(t, `echo "Success! App '2278520' fully installed."`)
	client, err := agentctl.New(srv.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	health, err := client.Health(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if health.APIVersion != agent.APIVersion || !health.InstallDirOk {
		t.Errorf("health = %+v", health)
	}

	removed, err := client.ClearSteamCache(ctx)
	if err != nil || removed != 1 {
		t.Errorf("clear = %d, %v; want 1, nil", removed, err)
	}

	job, err := client.StartUpdate(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for job.State == "running" && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		if job, err = client.Job(ctx, job.ID); err != nil {
			t.Fatal(err)
		}
	}
	if job.State != "done" {
		t.Fatalf("job = %+v, want done", job)
	}
}

func TestClientSyncSave(t *testing.T) {
	// newAgent's install dir isn't exposed; spin our own with a world —
	// Enshrouded's extensionless hex-named blob under savegame/.
	install := t.TempDir()
	world := filepath.Join(install, "savegame")
	if err := os.MkdirAll(world, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(world, "3ad85aea"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	agent, err := agent.New(agent.Config{
		Token: token, InstallDir: install, SteamCmd: "/bin/true", Version: "test", Game: ctlTestGame(),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	client, _ := agentctl.New(srv.URL, token)
	ctx := context.Background()

	dest := filepath.Join(t.TempDir(), "cache")
	etag, changed, err := client.SyncSave(ctx, dest, "")
	if err != nil || !changed || etag == "" {
		t.Fatalf("first sync: etag=%q changed=%v err=%v", etag, changed, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "3ad85aea")); string(got) != "v1" {
		t.Fatalf("synced save blob = %q", got)
	}

	// Unchanged: 304 path, no rewrite.
	if _, changed, err = client.SyncSave(ctx, dest, etag); err != nil || changed {
		t.Fatalf("unchanged sync: changed=%v err=%v", changed, err)
	}

	// Save rewritten → new etag, new content.
	if err := os.WriteFile(filepath.Join(world, "3ad85aea"), []byte("v2-longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, changed, err = client.SyncSave(ctx, dest, etag); err != nil || !changed {
		t.Fatalf("changed sync: changed=%v err=%v", changed, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "3ad85aea")); string(got) != "v2-longer" {
		t.Errorf("resynced save blob = %q", got)
	}
}

func TestClientErrorMapping(t *testing.T) {
	srv := newAgent(t, "sleep 2")

	// Wrong token → ErrRejected with a re-check hint.
	bad, _ := agentctl.New(srv.URL, "wrong-token-0123456789abcdef")
	if _, err := bad.Health(context.Background()); !errors.Is(err, agentctl.ErrRejected) {
		t.Errorf("wrong token: got %v, want ErrRejected", err)
	}

	// Second concurrent update → ErrBusy.
	client, _ := agentctl.New(srv.URL, token)
	if _, err := client.StartUpdate(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.StartUpdate(context.Background(), false); !errors.Is(err, agentctl.ErrBusy) {
		t.Errorf("busy: got %v, want ErrBusy", err)
	}

	// Unreachable agent reads as such.
	gone, _ := agentctl.New("http://127.0.0.1:1", token)
	if _, err := gone.Health(context.Background()); err == nil {
		t.Error("unreachable agent returned no error")
	}
}
