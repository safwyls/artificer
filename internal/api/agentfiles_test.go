package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/safwyls/palcon/internal/store"
)

// seedAgentWorld gives the agent's install dir a real-looking world (a
// Level.sav with valid container magic, so the backup archiver accepts
// it) and a PalWorldSettings.ini.
func seedAgentWorld(t *testing.T, install string) string {
	t.Helper()
	world := filepath.Join(install, "Pal", "Saved", "SaveGames", "0", "TESTWORLD")
	cfgDir := filepath.Join(install, "Pal", "Saved", "Config", "LinuxServer")
	for _, d := range []string{filepath.Join(world, "Players"), cfgDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	level := append(make([]byte, 8), []byte("PlZ")...)
	level = append(level, make([]byte, 32)...)
	if err := os.WriteFile(filepath.Join(world, "Level.sav"), level, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(world, "Players", "1111.sav"), []byte("p1"), 0o644); err != nil {
		t.Fatal(err)
	}
	ini := "[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(ExpRate=1.000000,PalCaptureRate=1.000000)\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "PalWorldSettings.ini"), []byte(ini), 0o644); err != nil {
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
	rec = app.do(t, "PUT", base, map[string]any{"changes": map[string]string{"ExpRate": "2.5"}}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("put config: %d (body %s)", rec.Code, rec.Body)
	}
	onAgent, err := os.ReadFile(filepath.Join(install, "Pal", "Saved", "Config", "LinuxServer", "PalWorldSettings.ini"))
	if err != nil || !strings.Contains(string(onAgent), "ExpRate=2.5") {
		t.Errorf("agent-side ini = %q, %v — edit did not round-trip", onAgent, err)
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

func TestSaveEndpointsUnconfigured(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	// Neither savePath nor agent: pals report the setup-guidance 400.
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "bare", Host: "10.0.0.5", RCONPort: 25575, RESTPort: 8212, UseREST: true, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := app.do(t, "GET", "/api/servers/"+itoa(id)+"/pals", nil, admin)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "no save path configured") {
		t.Errorf("bare pals: %d %s, want the setup-guidance 400", rec.Code, rec.Body)
	}
}
