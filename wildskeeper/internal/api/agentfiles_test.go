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

// seedAgentWorld gives the agent's install dir a real-looking Dragonwilds
// world (a .sav in SaveGames) and a DedicatedServer.ini.
func seedAgentWorld(t *testing.T, install string) string {
	t.Helper()
	world := filepath.Join(install, "RSDragonwilds", "Saved", "SaveGames")
	cfgDir := filepath.Join(install, "RSDragonwilds", "Saved", "Config", "LinuxServer")
	for _, d := range []string{world, cfgDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sav := append(make([]byte, 8), []byte("GVAS")...)
	sav = append(sav, make([]byte, 32)...)
	if err := os.WriteFile(filepath.Join(world, "Ashenfall.sav"), sav, 0o644); err != nil {
		t.Fatal(err)
	}
	ini := "[/Script/Dominion.DedicatedServerSettings]\nServerName=Grimwood\nOwnerId=owner-abc\nAdminPassword=old\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "DedicatedServer.ini"), []byte(ini), 0o644); err != nil {
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
	rec = app.do(t, "PUT", base, map[string]any{"changes": map[string]string{"ServerName": "Renamed Keep"}}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("put config: %d (body %s)", rec.Code, rec.Body)
	}
	onAgent, err := os.ReadFile(filepath.Join(install, "RSDragonwilds", "Saved", "Config", "LinuxServer", "DedicatedServer.ini"))
	if err != nil || !strings.Contains(string(onAgent), "ServerName=Renamed Keep") {
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
