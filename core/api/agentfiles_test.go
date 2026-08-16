package api_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// seedAgentWorld gives the agent's install dir a real-looking Enshrouded
// world (extensionless hex-named blobs under savegame/) and an
// gametest.json at the install root. One file keeps a .sav name:
// the save *sync* is extension-agnostic, but the console-side backup
// archiver (internal/backup) still keys on .sav — the world file below is
// what proves an agent-synced backup actually archives something.
func seedAgentWorld(t *testing.T, install string) string {
	t.Helper()
	world := filepath.Join(install, "savegame")
	if err := os.MkdirAll(world, 0o755); err != nil {
		t.Fatal(err)
	}
	blob := make([]byte, 44)
	copy(blob, "enshrouded-world-bytes")
	for _, name := range []string{"3ad85aea", "3ad85aea-index", "world.sav"} {
		if err := os.WriteFile(filepath.Join(world, name), blob, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := `{"name":"Grimwood","queryPort":"15637","slotCount":"16"}`
	if err := os.WriteFile(filepath.Join(install, "gametest.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return world
}

func TestConfigViaAgent(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, install := agentServer(t, app)
	seedAgentWorld(t, install)
	base := "/api/servers/" + itoa(id) + "/config"

	rec := app.do(t, "GET", base, nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config: %d (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Settings []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"settings"`
		Writable bool `json:"writable"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Settings) == 0 || !res.Writable {
		t.Fatalf("config = %+v, want writable settings", res)
	}

	// Edit a value; the change must land in the agent's file.
	rec = app.do(t, "PUT", base, map[string]any{"changes": map[string]string{"name": "Renamed Keep"}}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("put config: %d (body %s)", rec.Code, rec.Body)
	}
	onAgent, err := os.ReadFile(filepath.Join(install, "gametest.json"))
	if err != nil || !strings.Contains(string(onAgent), `"name": "Renamed Keep"`) {
		t.Errorf("agent-side json = %q, %v — edit did not round-trip", onAgent, err)
	}
}

func TestBackupViaAgent(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, install := agentServer(t, app)
	seedAgentWorld(t, install)
	base := "/api/servers/" + itoa(id) + "/backups"

	// Available despite no savePath: the agent supplies the files.
	rec := app.do(t, "GET", base, nil, admin)
	if rec.Code != http.StatusOK || decodeMap(t, rec)["available"] != true {
		t.Fatalf("list: %d %s, want available", rec.Code, rec.Body)
	}

	if rec := app.do(t, "POST", base+"/run", nil, admin); rec.Code != http.StatusAccepted {
		t.Fatalf("run: %d (body %s)", rec.Code, rec.Body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec = app.do(t, "GET", base, nil, admin)
		var res struct {
			Running   bool `json:"running"`
			Snapshots []struct {
				Name  string `json:"name"`
				Bytes int64  `json:"bytes"`
			} `json:"snapshots"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		if len(res.Snapshots) == 1 && res.Snapshots[0].Bytes > 0 {
			return // snapshot of the agent-synced save landed
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no snapshot appeared from the agent-backed backup")
}
