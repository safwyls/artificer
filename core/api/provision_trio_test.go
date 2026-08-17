package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/safwyls/artificer/core/api"
	"github.com/safwyls/artificer/core/store"
)

// trioProfile is a REST+RCON game (Palworld's shape): one UDP game port
// plus two named TCP admin transports, all four distinct — the carried
// palcon wizard behaviors (drift ledger, seam 4 / provision rows).
var trioProfile = &api.ProvisionProfile{
	AgentName:       "gtagent",
	ImageRepo:       "ghcr.io/example/gtagent",
	EnvPrefix:       "GTAGENT",
	DefaultGamePort: 8211,
	MountPath:       "/gametest",
	SlugFallback:    "gametest",
	StackHeadline:   "trio test game supervised by gtagent",
	GamePortComment: "game",
	AdminPorts: []api.AdminPort{
		{Key: "rest", Container: 8212, Default: 8212, Comment: "REST (dashboard)"},
		{Key: "rcon", Container: 25575, Default: 25575, Comment: "RCON (dashboard fallback)"},
	},
}

func trioApp(t *testing.T) (*testApp, []*http.Cookie) {
	app, admin := newTestAppWithAdmin(t)
	app.api.Provision = trioProfile
	return app, admin
}

func TestTrioPortsMustAllBeDistinct(t *testing.T) {
	app, admin := trioApp(t)
	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "Palhalla", "host": "10.0.0.9", "dataPath": "/mnt/pool/pal",
		"gamePort": 9000, "restPort": 9000,
	}, admin)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "collides") {
		t.Fatalf("duplicate ports: %d %s, want a collision refusal", rec.Code, rec.Body)
	}
}

func TestTrioStackCarriesAllFourPortsAndWiresTheRow(t *testing.T) {
	app, admin := trioApp(t)
	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "Palhalla", "host": "10.0.0.9", "dataPath": "/mnt/pool/pal",
		"serverDesc": "A cosy island",
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provision: %d %s", rec.Code, rec.Body)
	}
	body := decodeMap(t, rec)
	stack, _ := body["stack"].(string)
	for _, want := range []string{
		`"8211:8211/udp"`, `"8212:8212"`, `"25575:25575"`, `"8811:8811"`,
		`GTAGENT_SERVER_DESC: "A cosy island"`,
	} {
		if !strings.Contains(stack, want) {
			t.Errorf("stack missing %s:\n%s", want, stack)
		}
	}
	srv, _ := body["server"].(map[string]any)
	if rest, _ := srv["restPort"].(float64); int(rest) != 8212 {
		t.Errorf("row restPort = %v, want the wizard's — the dashboard must speak REST immediately", srv["restPort"])
	}
	if rcon, _ := srv["rconPort"].(float64); int(rcon) != 25575 {
		t.Errorf("row rconPort = %v, want the wizard's", srv["rconPort"])
	}
}

func TestTrioProposalMovesAllFourTogether(t *testing.T) {
	app, admin := trioApp(t)
	app.api.Provisioner = &fakeProvisioner{health: pairHealth()}
	// Take the default REST port: the whole proposal must shift.
	if _, err := app.store.CreateServer(t.Context(), &store.Server{
		Name: "existing", Host: "10.99.0.5", RESTPort: 8212, GamePort: 7000,
		Enabled: true, AgentURL: "http://10.99.0.5:8811", AgentToken: agentToken,
	}); err != nil {
		t.Fatal(err)
	}
	rec := app.do(t, "GET", "/api/servers/provision/defaults", nil, admin)
	ports, _ := decodeMap(t, rec)["ports"].(map[string]any)
	game, _ := ports["game"].(float64)
	rest, _ := ports["rest"].(float64)
	rcon, _ := ports["rcon"].(float64)
	if int(rest) == 8212 || int(game) != int(rest)-1 || int(rcon) != 25575+(int(game)-8211) {
		t.Errorf("proposal = %v, want the four moving together off the taken REST port", ports)
	}
}
