package api_test

// The restore button against a real agent rather than a stand-in: the
// console mirrors the agent's save, snapshots the mirror, and pushes a
// snapshot back through the same agent. The save layout is Dragonwilds'
// (a discovered SaveGames dir), which is where the console-side flow is
// actually used.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/safwyls/artificer/core/agent"
	"github.com/safwyls/artificer/core/store"
)

// dwSaveDir mirrors games/dragonwilds/dwagent.findSaveDir: two spellings
// exist in the wild and the dir counts only when it holds a .sav.
func dwSaveDir(installDir string) (string, error) {
	for _, dir := range []string{"SaveGames", "Savegames"} {
		full := filepath.Join(installDir, "RSDragonwilds", "Saved", dir)
		matches, err := filepath.Glob(filepath.Join(full, "*.sav"))
		if err == nil && len(matches) > 0 {
			return full, nil
		}
	}
	return "", errors.New("no world save found under the install dir (has the server run yet?)")
}

func TestRestoreBackupThroughRealAgent(t *testing.T) {
	install := t.TempDir()
	saveDir := filepath.Join(install, "RSDragonwilds", "Saved", "SaveGames")
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	world := filepath.Join(saveDir, "Ashenfall.sav")
	if err := os.WriteFile(world, []byte("GVAS-original-world-payload-0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}

	ag, err := agent.New(agent.Config{
		Token:       agentToken,
		InstallDir:  install,
		Mode:        "supervisor",
		GameCommand: "does-not-exist",
		Game: agent.Game{
			AgentName:     "wkagent",
			AppID:         1234,
			ConfigRelPath: filepath.Join("RSDragonwilds", "Saved", "Config", "DedicatedServer.ini"),
			SaveDirName:   filepath.Join("RSDragonwilds", "Saved", "SaveGames"),
			FindSaveDir:   dwSaveDir,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	agentSrv := httptest.NewServer(ag.Handler())
	defer agentSrv.Close()

	app, admin := newTestAppWithAdmin(t)
	// No SavePath: an agent-backed server, the normal wildskeeper shape.
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "main", Host: "10.0.0.5", RCONPort: 25575, RESTPort: 8212, UseREST: true, Enabled: true,
		AgentURL: agentSrv.URL, AgentToken: agentToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/servers/" + itoa(id)

	if rec := app.do(t, "POST", base+"/backups/run", nil, admin); rec.Code != http.StatusAccepted {
		t.Fatalf("run: got %d (body %s)", rec.Code, rec.Body)
	}
	var name string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := app.do(t, "GET", base+"/backups", nil, admin)
		var res struct {
			Snapshots []struct {
				Name  string `json:"name"`
				Bytes int64  `json:"bytes"`
			} `json:"snapshots"`
		}
		json.Unmarshal(rec.Body.Bytes(), &res)
		if len(res.Snapshots) == 1 && res.Snapshots[0].Bytes > 0 {
			name = res.Snapshots[0].Name
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if name == "" {
		t.Fatal("snapshot never appeared")
	}

	// The world moves on after the snapshot — the reason to restore.
	if err := os.WriteFile(world, []byte("GVAS-a-later-and-unwanted-world-9876543210"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := app.do(t, "POST", base+"/backups/"+name+"/restore", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore: got %d (body %s)", rec.Code, rec.Body)
	}
	back, err := os.ReadFile(world)
	if err != nil {
		t.Fatal(err)
	}
	if string(back) != "GVAS-original-world-payload-0123456789" {
		t.Errorf("world after restore = %q, want the snapshot's contents", back)
	}
}
