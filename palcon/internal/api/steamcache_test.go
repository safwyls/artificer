package api_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/safwyls/palcon/internal/store"
)

// fakeInstallDir builds a Palworld install root: cache directories with
// contents to wipe, plus a game file that must survive.
func fakeInstallDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, p := range []string{
		filepath.Join("steamapps", "downloading", "2394010"),
		filepath.Join("steam", "packages"),
	} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{
		filepath.Join("steamapps", "appmanifest_2394010.acf"),
		filepath.Join("steamapps", "downloading", "2394010", "chunk"),
		filepath.Join("steam", "packages", "12345.manifest"),
		"PalServer.sh",
	} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func installServer(t *testing.T, app *testApp, installPath string) int64 {
	t.Helper()
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "main", Host: "10.0.0.5", RCONPort: 25575, RESTPort: 8212, UseREST: true, Enabled: true,
		InstallPath: installPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestClearSteamCache(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	install := fakeInstallDir(t)
	id := installServer(t, app, install)

	rec := app.do(t, "POST", "/api/servers/"+itoa(id)+"/steam-cache/clear", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear: got %d (body %s)", rec.Code, rec.Body)
	}
	// Two steamapps entries (manifest + downloading dir) and one package.
	if got := decodeMap(t, rec)["removed"]; got != float64(3) {
		t.Errorf("removed = %v, want 3", got)
	}

	// The cache directories survive empty; everything inside is gone.
	for _, rel := range []string{"steamapps", filepath.Join("steam", "packages")} {
		entries, err := os.ReadDir(filepath.Join(install, rel))
		if err != nil {
			t.Fatalf("%s should still exist: %v", rel, err)
		}
		if len(entries) != 0 {
			t.Errorf("%s not emptied: %d entries left", rel, len(entries))
		}
	}
	// Game files outside the cache directories are untouched.
	if _, err := os.Stat(filepath.Join(install, "PalServer.sh")); err != nil {
		t.Errorf("PalServer.sh should survive: %v", err)
	}

	// Idempotent: clearing an already-empty cache succeeds with nothing to do.
	rec = app.do(t, "POST", "/api/servers/"+itoa(id)+"/steam-cache/clear", nil, admin)
	if rec.Code != http.StatusOK || decodeMap(t, rec)["removed"] != float64(0) {
		t.Errorf("second clear: got %d %s, want 200 removed=0", rec.Code, rec.Body)
	}
}

func TestClearSteamCacheValidation(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	// No install path configured.
	id := installServer(t, app, "")
	if rec := app.do(t, "POST", "/api/servers/"+itoa(id)+"/steam-cache/clear", nil, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("no install path: got %d, want 400", rec.Code)
	}

	// A path with no steamapps/ or steam/packages/ is a mis-set path, not a
	// successful no-op.
	id = installServer(t, app, t.TempDir())
	if rec := app.do(t, "POST", "/api/servers/"+itoa(id)+"/steam-cache/clear", nil, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("wrong path: got %d, want 400", rec.Code)
	}
}

func TestClearSteamCacheNeedsPowerPermission(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := installServer(t, app, fakeInstallDir(t))
	path := "/api/servers/" + itoa(id) + "/steam-cache/clear"

	app.createUser(t, admin, "viewer", "viewerpassword1", "user", nil)
	viewer := app.login(t, "viewer", "viewerpassword1")
	if rec := app.do(t, "POST", path, nil, viewer); rec.Code != http.StatusForbidden {
		t.Errorf("without power: got %d, want 403", rec.Code)
	}

	app.createUser(t, admin, "operator", "operatorpassword1", "user", []string{store.PermPower})
	operator := app.login(t, "operator", "operatorpassword1")
	if rec := app.do(t, "POST", path, nil, operator); rec.Code != http.StatusOK {
		t.Errorf("with power: got %d (body %s), want 200", rec.Code, rec.Body)
	}
}
