package palworld_test

// The save-backed views over the vendored fixture — home from palcon's
// pals_save_test, now run through the shared apitest harness with
// Palworld's contributed routes.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/sampo/core/api"
	"github.com/safwyls/sampo/core/api/apitest"
	"github.com/safwyls/sampo/core/store"
	"github.com/safwyls/sampo/games/palworld"
	"github.com/safwyls/sampo/games/palworld/palapi"
	"github.com/safwyls/sampo/games/palworld/palsave"
)

// hasPython reports whether the save extractor's dependencies are present.
// Following the palsave package's own convention, save-backed tests skip
// rather than fail on a machine without them.
func hasPython(module string) bool {
	return exec.Command("python3", "-c", "import "+module).Run() == nil
}

// copySaveFixture lays the vendored newlayout save into a fresh directory
// so a test can't mutate the checked-in fixture.
func copySaveFixture(t *testing.T) string {
	t.Helper()
	src := filepath.Join("palsave", "testdata", "newlayout")
	dst := t.TempDir()
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("reading the save fixture: %v", err)
	}
	var copyTree func(from, to string)
	copyTree = func(from, to string) {
		items, err := os.ReadDir(from)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			fromPath := filepath.Join(from, item.Name())
			toPath := filepath.Join(to, item.Name())
			if item.IsDir() {
				if err := os.MkdirAll(toPath, 0o755); err != nil {
					t.Fatal(err)
				}
				copyTree(fromPath, toPath)
				continue
			}
			data, err := os.ReadFile(fromPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(toPath, data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	copyTree(src, dst)
	return dst
}

// newTestAppWithSave is newTestAppWithAdmin plus a server whose save path
// points at a copy of the fixture, which the save-backed views need.
func newTestAppWithSave(t *testing.T) (*apitest.App, []*http.Cookie, int64) {
	t.Helper()
	if !hasPython("palworld_save_tools") {
		t.Skip("python3 with palworld-save-tools not available")
	}

	reader, err := palsave.NewReader(t.TempDir())
	if err != nil {
		t.Fatalf("building the save reader: %v", err)
	}
	app, admin := apitest.NewWithAdmin(t, apitest.Options{
		Provision: palworld.ProvisionProfile(),
		GameRoutes: func(s *api.Server) func(chi.Router) {
			return palapi.Mount(s, reader)
		},
	})
	app.API.Roster = &palworld.Roster{Reader: reader}

	id, err := app.Store.CreateServer(context.Background(), &store.Server{
		Name: "saved", Host: "10.0.0.5", Enabled: true,
		RCONPort: 25575, RESTPort: 8212,
		SavePath: copySaveFixture(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	return app, admin, id
}

func TestSaveBackedViewsServeTheFixture(t *testing.T) {
	app, admin, id := newTestAppWithSave(t)
	base := "/api/servers/" + itoa(id)

	for _, path := range []string{"/pals", "/guilds", "/inventory", "/storage", "/achievements"} {
		rec := app.Do(t, "GET", base+path, nil, admin)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: %d (body %s)", path, rec.Code, rec.Body)
		}
	}
}

func TestPalsPayloadCarriesPlayers(t *testing.T) {
	app, admin, id := newTestAppWithSave(t)

	rec := app.Do(t, "GET", "/api/servers/"+itoa(id)+"/pals", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("pals: %d (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Players []struct {
			UID      string `json:"uid"`
			Nickname string `json:"nickname"`
		} `json:"players"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Players) == 0 {
		t.Fatalf("the fixture save has players but none came back: %s", rec.Body)
	}
	for _, p := range res.Players {
		if p.UID == "" {
			t.Errorf("a player came back with no uid: %+v", p)
		}
	}
}

// A server with no save configured is a setup problem the frontend can
// explain, not a server error.
func TestSaveViewsWithoutASavePath(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	newServerForVisibility(t, app, admin)
	base := "/api/servers/1"

	for _, path := range []string{"/pals", "/guilds", "/inventory", "/storage", "/achievements"} {
		rec := app.Do(t, "GET", base+path, nil, admin)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s with no save: %d, want 400", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "save path") {
			t.Errorf("GET %s should explain the missing save: %s", path, rec.Body)
		}
	}
}

// Switching a view off hides it from ordinary users while admins keep it —
// the point of the feature is honouring a privacy request without blinding
// the person who has to moderate.
func TestSaveViewsRespectVisibilitySwitches(t *testing.T) {
	app, admin, id := newTestAppWithSave(t)
	base := "/api/servers/" + itoa(id)

	if rec := app.Do(t, "PUT", base+"/visibility", map[string]any{
		"hiddenFeatures": []string{"pals", "paldex", "calculators", "inventory"},
	}, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("hiding views: %d (body %s)", rec.Code, rec.Body)
	}

	app.CreateUser(t, admin, "viewer", "viewerpass123", "user", nil)
	viewer := app.Login(t, "viewer", "viewerpass123")

	for _, path := range []string{"/pals", "/inventory"} {
		if rec := app.Do(t, "GET", base+path, nil, viewer); rec.Code != http.StatusForbidden {
			t.Errorf("GET %s as a viewer with the view off: %d, want 403", path, rec.Code)
		}
		if rec := app.Do(t, "GET", base+path, nil, admin); rec.Code != http.StatusOK {
			t.Errorf("GET %s as an admin with the view off: %d, want 200", path, rec.Code)
		}
	}
}
