package dragonwilds_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/artificer/core/api"
	"github.com/safwyls/artificer/core/api/apitest"
	"github.com/safwyls/artificer/core/game"
	"github.com/safwyls/artificer/core/savecache"
	"github.com/safwyls/artificer/games/dragonwilds"
	"github.com/safwyls/artificer/games/dragonwilds/dwapi"
	"github.com/safwyls/artificer/games/dragonwilds/dwsave"
)

// itoa mirrors the old harness helper.
func itoa(id int64) string { return strconv.FormatInt(id, 10) }

func init() {
	// The tests run the console as a wildskeeper main would wire it:
	// dragonwilds registered (the import's init) and default.
	game.DefaultID = dragonwilds.Definition.ID
}

// newTestAppWithAdmin builds the app with Dragonwilds' contributions —
// worlds cache, provisioning profile, contributed routes — mirroring
// cmd/wildskeeper.
func newTestAppWithAdmin(t *testing.T) (*apitest.App, []*http.Cookie) {
	worlds := savecache.New[dwsave.World](dwsave.Source{})
	return apitest.NewWithAdmin(t, apitest.Options{
		Provision: dragonwilds.ProvisionProfile(),
		GameRoutes: func(s *api.Server) func(chi.Router) {
			return dwapi.Mount(s, worlds, nil)
		},
	})
}

// newTestAppWithAdminNoWorlds wires the routes without a worlds cache,
// for the calm-unavailable case.
func newTestAppWithAdminNoWorlds(t *testing.T) (*apitest.App, []*http.Cookie) {
	return apitest.NewWithAdmin(t, apitest.Options{
		Provision: dragonwilds.ProvisionProfile(),
		GameRoutes: func(s *api.Server) func(chi.Router) {
			return dwapi.Mount(s, nil, nil)
		},
	})
}
