package enshrouded_test

import (
	"log/slog"
	"net/http"
	"testing"

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

const agentToken = "api-test-agent-token-0123456789"

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
