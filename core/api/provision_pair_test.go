package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/safwyls/artificer/core/agent"
	"github.com/safwyls/artificer/core/agentctl"
	"github.com/safwyls/artificer/core/api"
	"github.com/safwyls/artificer/core/store"
)

func pairHealth() *agentctl.Health {
	return &agentctl.Health{
		Agent: "anvil", Mode: "provisioner",
		Provision: &agent.ProvisionDefaults{
			DataRoot: "/data", PublicHost: "10.99.0.5", RunAs: "568:568", ImageTag: "latest",
		},
	}
}

// pairProfile is a two-port game (Dragonwilds' shape): the game binds
// GamePort and GamePort+1, and refuses to start without an owner. These
// are the wizard behaviors wildskeeper carried that a single-port
// profile never exercises (drift ledger, seam 4 / stack rows).
var pairProfile = &api.ProvisionProfile{
	AgentName:       "gtagent",
	ImageRepo:       "ghcr.io/example/gtagent",
	EnvPrefix:       "GTAGENT",
	DefaultGamePort: 7777,
	MountPath:       "/gametest",
	SlugFallback:    "gametest",
	StackHeadline:   "pair test game supervised by gtagent",
	GamePortComment: "game port (first of the pair)",
	GamePortCount:   2,
	OwnerIDRequired: true,
	OwnerIDHelp:     `in-game: Settings, bottom-left "My Player ID"`,
}

func pairApp(t *testing.T) (*testApp, []*http.Cookie) {
	app, admin := newTestAppWithAdmin(t)
	app.api.Provision = pairProfile
	return app, admin
}

func TestPairGameRequiresAnOwner(t *testing.T) {
	app, admin := pairApp(t)
	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "Ashenfall", "host": "10.0.0.9", "dataPath": "/mnt/pool/dw",
	}, admin)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "My Player ID") {
		t.Fatalf("ownerless provision: %d %s, want 400 naming where the id lives", rec.Code, rec.Body)
	}
}

func TestPairMustFitBelowThePortCeiling(t *testing.T) {
	app, admin := pairApp(t)
	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "Ashenfall", "host": "10.0.0.9", "dataPath": "/mnt/pool/dw",
		"ownerId": "owner-1", "gamePort": 65535,
	}, admin)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "1-65534") {
		t.Fatalf("port 65535 with a pair: %d %s, want the 65534 cap", rec.Code, rec.Body)
	}
}

func TestAgentPortCannotSitInsideThePair(t *testing.T) {
	app, admin := pairApp(t)
	// The neighbour, not the game port itself — the collision W's pair
	// logic exists to catch and a single-port check would miss.
	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "Ashenfall", "host": "10.0.0.9", "dataPath": "/mnt/pool/dw",
		"ownerId": "owner-1", "gamePort": 9777, "agentPort": 9778,
	}, admin)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "port range") {
		t.Fatalf("agent on the pair's neighbour: %d %s, want a range collision", rec.Code, rec.Body)
	}
}

func TestPairStackCarriesBothPortsAndTheIdentity(t *testing.T) {
	app, admin := pairApp(t)
	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "Ashenfall", "host": "10.0.0.9", "dataPath": "/mnt/pool/dw",
		"ownerId": "owner-1", "worldName": "Grimwood", "gamePort": 9777, "agentPort": 9811,
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provision: %d %s", rec.Code, rec.Body)
	}
	stack, _ := decodeMap(t, rec)["stack"].(string)
	for _, want := range []string{
		`"9777:7777/udp"`, `"9778:7778/udp"`,
		`GTAGENT_OWNER_ID: "owner-1"`, `GTAGENT_WORLD_NAME: "Grimwood"`,
	} {
		if !strings.Contains(stack, want) {
			t.Errorf("stack missing %s:\n%s", want, stack)
		}
	}
}

func TestPairProposalStridesAndReservesNeighbours(t *testing.T) {
	app, admin := pairApp(t)
	app.api.Provisioner = &fakeProvisioner{health: pairHealth()}
	// A registered server on the default pair: the proposal must skip
	// both its ports — there is no room for a pair at 7777 or 7778.
	if _, err := app.store.CreateServer(t.Context(), &store.Server{
		Name: "existing", Host: "10.99.0.5", GamePort: 7777,
		Enabled: true, AgentURL: "http://10.99.0.5:8811", AgentToken: agentToken,
	}); err != nil {
		t.Fatal(err)
	}
	rec := app.do(t, "GET", "/api/servers/provision/defaults", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("defaults: %d %s", rec.Code, rec.Body)
	}
	ports, _ := decodeMap(t, rec)["ports"].(map[string]any)
	game, _ := ports["game"].(float64)
	if int(game) != 7779 {
		t.Errorf("proposed game port = %v, want 7779 (stride of two past the taken pair)", ports["game"])
	}
}
