package api_test

// The vault surface (VaultRoutes): the standalone save-sync service's
// assembly. Auth, users and custody are there; the console furniture is
// not — a vault answering /api/servers would mean the assemblies
// re-merged by accident.

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/safwyls/artificer/core/api"
	"github.com/safwyls/artificer/core/crypto"
	"github.com/safwyls/artificer/core/db"
	"github.com/safwyls/artificer/core/igdb"
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

// Artwork and live events are additive: with no IGDB credentials the
// lookup answers "not available" rather than failing, and the event
// stream opens and reports itself ready.
func TestVaultArtworkAndEvents(t *testing.T) {
	app, admin := newVaultApp(t)

	rec := app.do(t, "POST", "/api/sync/artwork", map[string]any{
		"games": []map[string]string{{"appId": "1623730", "name": "Palworld"}},
	}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("artwork: got %d (body %s)", rec.Code, rec.Body)
	}
	out := decodeMap(t, rec)
	if out["available"] != false {
		t.Errorf("artwork available = %v with no credentials, want false", out["available"])
	}
	if _, ok := out["art"]; !ok {
		t.Error("artwork answer carries no art map")
	}

	// The stream opens, announces itself and stays open until the request
	// context ends — which is what the page's EventSource relies on.
	req := httptest.NewRequest("GET", "/api/sync/events", nil)
	for _, c := range admin {
		req.AddCookie(c)
	}
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	streamed := make(chan string, 1)
	go func() {
		w := httptest.NewRecorder()
		app.handler.ServeHTTP(w, req)
		streamed <- w.Body.String()
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	select {
	case body := <-streamed:
		if !strings.Contains(body, "event: ready") {
			t.Errorf("event stream opened with %q, want a ready frame", body)
		}
	case <-time.After(2 * time.Second):
		t.Error("the event stream did not return after its context ended")
	}
}

// The admin artwork surface: credentials in, diagnostics out. The point
// is that a deployment's owner can tell "no credentials" from "these
// credentials don't work" without reading the service log — the
// distinction the first cut swallowed.
func TestVaultArtworkSettings(t *testing.T) {
	app, admin := newVaultApp(t)

	// A stand-in for Twitch and IGDB, so saving a credential proves
	// itself without the internet.
	var reject bool
	igdbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reject {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"message":"invalid client secret"}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/token") {
			io.WriteString(w, `{"access_token":"tok","expires_in":3600}`)
			return
		}
		io.WriteString(w, `[{"id":1}]`)
	}))
	defer igdbSrv.Close()
	app.api.Artwork = igdb.New("", "")
	app.api.Artwork.UseEndpoints(igdbSrv.URL+"/token", igdbSrv.URL+"/v4")

	rec := app.do(t, "GET", "/api/sync/artwork/settings", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("artwork settings: %d (body %s)", rec.Code, rec.Body)
	}
	out := decodeMap(t, rec)
	if out["status"].(map[string]any)["configured"] != false || out["stored"] != false {
		t.Errorf("fresh vault reports %v, want unconfigured and nothing stored", out)
	}

	// Half a pair is refused: IGDB authenticates through Twitch, and one
	// half cannot.
	rec = app.do(t, "PUT", "/api/sync/artwork/settings", map[string]string{"clientId": "abc"}, admin)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("half a credential pair: %d, want 400", rec.Code)
	}

	rec = app.do(t, "PUT", "/api/sync/artwork/settings", map[string]string{
		"clientId": "abc", "clientSecret": "shh",
	}, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("saving credentials: %d (body %s)", rec.Code, rec.Body)
	}
	out = decodeMap(t, rec)
	if out["test"].(map[string]any)["ok"] != true {
		t.Errorf("save reported test %v, want a proven credential", out["test"])
	}
	if out["stored"] != true || out["status"].(map[string]any)["configured"] != true {
		t.Errorf("after saving: %v, want stored and configured", out)
	}
	if id := out["status"].(map[string]any)["clientId"]; id != "abc" {
		t.Errorf("status client id = %v, want the saved one", id)
	}
	// The secret never comes back out.
	if strings.Contains(rec.Body.String(), "shh") {
		t.Error("the artwork status echoed the client secret")
	}

	// A credential IGDB rejects is a 200 that says so, not an HTTP error:
	// the caller asked whether it works.
	reject = true
	rec = app.do(t, "POST", "/api/sync/artwork/test", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("artwork test: %d (body %s)", rec.Code, rec.Body)
	}
	test := decodeMap(t, rec)["test"].(map[string]any)
	if test["ok"] != false || !strings.Contains(test["error"].(string), "invalid client secret") {
		t.Errorf("test result = %v, want a named failure", test)
	}

	rec = app.do(t, "DELETE", "/api/sync/artwork/settings", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("removing credentials: %d (body %s)", rec.Code, rec.Body)
	}
	if decodeMap(t, rec)["stored"] != false {
		t.Error("credentials still stored after removal")
	}

	// A player with the custody grant is not an admin: shared credentials
	// are the admin's business.
	app.createUser(t, admin, "bob", "bobpassword12", "user", []string{store.PermSync})
	bob := app.login(t, "bob", "bobpassword12")
	if rec := app.do(t, "GET", "/api/sync/artwork/settings", nil, bob); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin reading artwork settings: %d, want 403", rec.Code)
	}
}

// The companion download is never cached by anything in the path, and
// says which build it is.
//
// The URL is stable for every player forever, so the bytes are the only
// thing that changes between builds — which makes an intermediary's
// cache a real hazard: .exe is in Cloudflare's default-cached extension
// list, and a browser re-serves a same-named download it already has.
// Either one hands out a companion this service stopped shipping.
func TestCompanionDownloadIsNotCacheable(t *testing.T) {
	app, admin := newVaultApp(t)
	app.api.Version = "main-abc123"
	exe := filepath.Join(t.TempDir(), "artificer-companion.exe")
	if err := os.WriteFile(exe, []byte("MZ fake companion"), 0o600); err != nil {
		t.Fatalf("write exe: %v", err)
	}
	app.api.CompanionExe = exe

	app.createUser(t, admin, "carol", "carolpassword", "user", []string{store.PermSync})
	carol := app.login(t, "carol", "carolpassword")
	rec := app.do(t, "POST", "/api/me/sync-token", nil, carol)
	token := decodeMap(t, rec)["token"].(string)

	rec = app.do(t, "GET", "/api/public/sync/"+token+"/companion/download", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("download: %d (body %s)", rec.Code, rec.Body)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store — a cached exe outlives the build it came from", cc)
	}
	if v := rec.Header().Get("X-Companion-Version"); v != "main-abc123" {
		t.Errorf("X-Companion-Version = %q, want the service's build", v)
	}
	if rec.Body.String() != "MZ fake companion" {
		t.Errorf("download body = %q, want the bundled exe", rec.Body.String())
	}
}

// The build is reported without a session: the login page shows it too,
// and a version is not a secret.
func TestVaultVersion(t *testing.T) {
	app, _ := newVaultApp(t)
	rec := app.do(t, "GET", "/api/version", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("version: %d (body %s)", rec.Code, rec.Body)
	}
	if v := decodeMap(t, rec)["version"]; v != "dev" {
		t.Errorf("version = %v on an unstamped build, want \"dev\"", v)
	}
	app.api.Version = "main-abc123"
	rec = app.do(t, "GET", "/api/version", nil, nil)
	if v := decodeMap(t, rec)["version"]; v != "main-abc123" {
		t.Errorf("version = %v, want the stamped build", v)
	}
}
