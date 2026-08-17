package palworld_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/sampo/core/api"
	"github.com/safwyls/sampo/core/api/apitest"
	"github.com/safwyls/sampo/core/game"
	"github.com/safwyls/sampo/games/palworld"
	"github.com/safwyls/sampo/games/palworld/palapi"
	"github.com/safwyls/sampo/games/palworld/palsave"
)

// itoa mirrors the old harness helper.
func itoa(id int64) string { return strconv.FormatInt(id, 10) }

func init() {
	// The tests run the console as a palcon main would wire it: palworld
	// registered (the import's init) and default.
	game.DefaultID = palworld.Definition.ID
}

// newTestAppWithAdmin builds the app with Palworld's contributions —
// save reader, contributed routes, roster, provisioning profile —
// mirroring cmd/palcon.
func newTestAppWithAdmin(t *testing.T) (*apitest.App, []*http.Cookie) {
	t.Helper()
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
	return app, admin
}

// makeMember creates a non-admin account and returns its session, for
// checking the half of visibility that admins don't experience.
func makeMember(t *testing.T, app *apitest.App, admin []*http.Cookie) []*http.Cookie {
	t.Helper()
	rec := app.Do(t, "POST", "/api/users", map[string]any{
		"username": "member", "password": "member-password-1", "role": "member",
	}, admin)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create member: got %d (body %s)", rec.Code, rec.Body)
	}
	return app.Login(t, "member", "member-password-1")
}

func newServerForVisibility(t *testing.T, app *apitest.App, admin []*http.Cookie) {
	t.Helper()
	rec := app.Do(t, "POST", "/api/servers", map[string]any{
		"name": "s1", "host": "10.0.0.1", "rconPort": 25575, "rconPassword": "x",
	}, admin)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create server: got %d (body %s)", rec.Code, rec.Body)
	}
}
