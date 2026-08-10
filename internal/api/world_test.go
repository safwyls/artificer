package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/safwyls/wildskeeper/internal/games/dragonwilds/dwsave"
	"github.com/safwyls/wildskeeper/internal/savecache"
	"github.com/safwyls/wildskeeper/internal/store"
)

// realSaveDir is a directory holding the genuine Dragonwilds capture, as a
// bind-mounted SaveGames directory would.
func realSaveDir(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../games/dragonwilds/testdata/world-empty.sav")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "World-75058.sav"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func worldServer(t *testing.T, app *testApp, savePath string) string {
	t.Helper()
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "wilds", Host: "10.0.0.5", RCONPort: 25575, RESTPort: 8212, Enabled: true,
		SavePath: savePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	return "/api/servers/" + itoa(id) + "/world"
}

func TestWorldEndpointAdminOnly(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	path := worldServer(t, app, realSaveDir(t))
	app.createUser(t, admin, "peon", "peonpassword1", "user", nil)
	peon := app.login(t, "peon", "peonpassword1")

	if rec := app.do(t, "GET", path, nil, peon); rec.Code != http.StatusForbidden {
		t.Errorf("as non-admin: got %d, want 403", rec.Code)
	}
}

func TestWorldEndpoint(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	app.api.Worlds = savecache.New[dwsave.World](dwsave.Source{})
	path := worldServer(t, app, realSaveDir(t))

	rec := app.do(t, "GET", path, nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	var out struct {
		Available bool          `json:"available"`
		World     *dwsave.World `json:"world"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if !out.Available || out.World == nil {
		t.Fatalf("world unavailable: %s", rec.Body)
	}
	if out.World.WorldName != "World-75058" {
		t.Errorf("worldName = %q", out.World.WorldName)
	}
	if out.World.SaveGuid != "CA220B254BB44040A0666FB7646ED7FA" {
		t.Errorf("saveGuid = %q", out.World.SaveGuid)
	}
	if out.World.File != "World-75058.sav" {
		t.Errorf("file = %q", out.World.File)
	}
}

// TestWorldEndpointUnavailable covers the calm-absence shapes: no reader
// wired (the pre-Phase-3 default), and no save path configured.
func TestWorldEndpointUnavailable(t *testing.T) {
	check := func(t *testing.T, app *testApp, admin []*http.Cookie, path string) {
		t.Helper()
		rec := app.do(t, "GET", path, nil, admin)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d: %s", rec.Code, rec.Body)
		}
		var out struct {
			Available bool `json:"available"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Available {
			t.Errorf("available = true, want false: %s", rec.Body)
		}
	}

	t.Run("no reader", func(t *testing.T) {
		app, admin := newTestAppWithAdmin(t)
		check(t, app, admin, worldServer(t, app, realSaveDir(t)))
	})
	t.Run("no save path", func(t *testing.T) {
		app, admin := newTestAppWithAdmin(t)
		app.api.Worlds = savecache.New[dwsave.World](dwsave.Source{})
		check(t, app, admin, worldServer(t, app, ""))
	})
}

// TestWorldEndpointBadSave: a save that exists but does not parse is an
// error the operator should see, not a silent "unavailable".
func TestWorldEndpointBadSave(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	app.api.Worlds = savecache.New[dwsave.World](dwsave.Source{})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "World-1.sav"), []byte("GVAS not a spud save"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := worldServer(t, app, dir)

	rec := app.do(t, "GET", path, nil, admin)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500: %s", rec.Code, rec.Body)
	}
}
