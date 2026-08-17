package esapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/safwyls/artificer/core/agent"
	"github.com/safwyls/artificer/games/enshrouded/esagent"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/artificer/core/agentfiles"
	"github.com/safwyls/artificer/core/api"
	"github.com/safwyls/artificer/core/api/apitest"
	"github.com/safwyls/artificer/core/game"
	"github.com/safwyls/artificer/core/store"
	"github.com/safwyls/artificer/games/enshrouded"
	"github.com/safwyls/artificer/games/enshrouded/banqueue"
	"github.com/safwyls/artificer/games/enshrouded/esapi"
)

func init() {
	// The tests run the console as a flametender main would wire it:
	// enshrouded registered (the import's init) and default.
	game.DefaultID = enshrouded.Definition.ID
}

// newTestAppWithAdmin builds the app with Enshrouded's contributions —
// ban queue, provisioning profile, contributed routes — mirroring
// cmd/flametender.
func newTestAppWithAdmin(t *testing.T) (*apitest.App, []*http.Cookie) {
	return apitest.NewWithAdmin(t, apitest.Options{
		Bans: func(st *store.Store, files *agentfiles.Syncer, logger *slog.Logger) api.OfflineConfigWork {
			return banqueue.New(st, files, logger)
		},
		Provision:  enshrouded.ProvisionProfile(),
		GameRoutes: func(s *api.Server) func(chi.Router) { return esapi.Mount(s) },
	})
}

const agentToken = "api-test-agent-token-0123456789"

// supervisorServerWithInstall runs the real agent kit with Enshrouded's
// own Game spec (esagent) over a fake game process, and registers a
// server row pointing at it — the console→agent→config path end to end,
// including the seed/enforce hook writing a real enshrouded_server.json.
func supervisorServerWithInstall(t *testing.T, app *apitest.App) (int64, string) {
	t.Helper()
	install := t.TempDir()
	gameScript := `#!/bin/sh
trap 'echo "caught INT"; exit 0' INT TERM
echo "Enshrouded server booting"
while true; do sleep 0.05; done
`
	if err := os.WriteFile(filepath.Join(install, "game.sh"), []byte(gameScript), 0o755); err != nil {
		t.Fatal(err)
	}
	steamcmd := filepath.Join(t.TempDir(), "steamcmd")
	if err := os.WriteFile(steamcmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := agent.New(agent.Config{
		Token: agentToken, InstallDir: install, SteamCmd: steamcmd, Version: "test",
		Game: esagent.Game(esagent.WineConfig{}),
		Mode: "supervisor", GameCommand: "./game.sh", StopGrace: time.Second,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		req, _ := http.NewRequest("POST", srv.URL+"/v1/power/stop", nil)
		req.Header.Set("Authorization", "Bearer "+agentToken)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	})

	id, err := app.Store.CreateServer(context.Background(), &store.Server{
		Name: "supervised", Host: "127.0.0.1", RCONPort: 1, RESTPort: 1, UseREST: true, Enabled: true,
		AgentURL: srv.URL, AgentToken: agentToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id, install
}

func newEnshroudedServer(t *testing.T, app *apitest.App, configPath string) int64 {
	t.Helper()
	id, err := app.Store.CreateServer(context.Background(), &store.Server{
		Name: "Grimwood", Game: "enshrouded", Host: "127.0.0.1",
		Enabled: true, ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	return id
}

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
