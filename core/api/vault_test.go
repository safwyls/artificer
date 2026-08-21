package api_test

// The vault surface (VaultRoutes): the standalone save-sync service's
// assembly. Auth, users and custody are there; the console furniture is
// not — a vault answering /api/servers would mean the assemblies
// re-merged by accident.

import (
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/safwyls/artificer/core/api"
	"github.com/safwyls/artificer/core/crypto"
	"github.com/safwyls/artificer/core/db"
	"github.com/safwyls/artificer/core/notify"
	"github.com/safwyls/artificer/core/savesync"
	"github.com/safwyls/artificer/core/store"
)

func newVaultApp(t *testing.T) (*testApp, []*http.Cookie) {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	box, err := crypto.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	st := store.New(sqlDB, box)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := api.New(st, []byte("test-jwt-secret-0123456789abcdef"), logger, nil, notify.New(st, logger), nil, nil, nil)
	srv.SaveSync = savesync.New(st, nil, logger, t.TempDir())
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>vault</html>")}}
	app := &testApp{handler: srv.VaultRoutes(staticFS), store: st, api: srv}
	if err := api.BootstrapAdmin(t.Context(), st, adminName, adminPass); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	return app, app.login(t, adminName, adminPass)
}

func TestVaultSurface(t *testing.T) {
	app, admin := newVaultApp(t)

	// The custody loop works end to end on the vault assembly, including
	// the token tier a companion drives — create with game metadata,
	// seed, check out, check in.
	app.createUser(t, admin, "alice", "alicepassword", "user", []string{store.PermSync})
	alice := app.login(t, "alice", "alicepassword")
	rec := app.do(t, "POST", "/api/me/sync-token", nil, alice)
	if rec.Code != http.StatusOK {
		t.Fatalf("mint token: %d (body %s)", rec.Code, rec.Body)
	}
	token := decodeMap(t, rec)["token"].(string)

	rec = app.do(t, "POST", "/api/public/sync/"+token+"/worlds", map[string]string{
		"name": "midgard", "gameTitle": "RuneScape: Dragonwilds",
		"saveHint": `C:\Users\alice\AppData\Local\RSDragonwilds\Saved\SaveGames`,
		"gameMeta": `{"appId":"1374490"}`,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("companion world create: %d (body %s)", rec.Code, rec.Body)
	}
	if rec := app.doTar(t, "/api/public/sync/"+token+"/worlds/1/import", nil); rec.Code != http.StatusOK {
		t.Fatalf("companion seed: %d (body %s)", rec.Code, rec.Body)
	}
	rec = app.do(t, "GET", "/api/sync/worlds/1", nil, admin)
	world := decodeMap(t, rec)["status"].(map[string]any)["world"].(map[string]any)
	if world["gameTitle"] != "RuneScape: Dragonwilds" || world["headVersion"] == nil {
		t.Errorf("companion-created world = %v, want game metadata and a seeded head", world)
	}
	rec = app.do(t, "POST", "/api/public/sync/"+token+"/worlds/1/checkout", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("token checkout: %d (body %s)", rec.Code, rec.Body)
	}
	sessionID := int64(decodeMap(t, rec)["session"].(map[string]any)["id"].(float64))
	if rec := app.doTar(t, "/api/public/sync/"+token+"/sessions/"+itoa(sessionID)+"/checkin", nil); rec.Code != http.StatusOK {
		t.Fatalf("token checkin: %d (body %s)", rec.Code, rec.Body)
	}

	// The console furniture is absent, not just empty.
	for _, path := range []string{"/api/servers", "/api/host", "/api/servers/1/backups"} {
		if rec := app.do(t, "GET", path, nil, admin); rec.Code != http.StatusNotFound {
			t.Errorf("vault answers %s with %d, want 404", path, rec.Code)
		}
	}

	// The SPA fallback serves the embedded page.
	rec = app.do(t, "GET", "/anything", nil, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "<html>vault</html>" {
		t.Errorf("spa fallback: %d %q", rec.Code, rec.Body.String())
	}
}
