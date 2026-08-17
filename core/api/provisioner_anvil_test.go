package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/safwyls/artificer/core/agent"
	"github.com/safwyls/artificer/core/anvilclient"
)

// fakeAnvil records what the adapter sends, and answers with Anvil's real
// wire shapes.
type fakeAnvil struct {
	provisionBody map[string]any
	adoptEnv      map[string]string
}

// anvilTestProfile mirrors api_test's testProfile for this internal
// package — the adapter is asserted against the same fake game shape.
var anvilTestProfile = &ProvisionProfile{
	AgentName:       "gtagent",
	ImageRepo:       "ghcr.io/example/gtagent",
	EnvPrefix:       "GTAGENT",
	DefaultGamePort: 25600,
	MountPath:       "/gametest",
	SlugFallback:    "gametest",
}

func newFakeAnvil(t *testing.T) (*fakeAnvil, *AnvilProvisioner) {
	t.Helper()
	f := &fakeAnvil{adoptEnv: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/health":
			json.NewEncoder(w).Encode(map[string]any{
				"service": "anvil", "version": "test", "apiVersion": 1,
				"client": "flametender", "dataRoot": "/mnt/tank/apps/dragonwilds-servers",
				"publicHost": "192.168.1.9", "runAs": "568:568", "dockerOk": true,
			})
		case "/v1/provision":
			json.NewDecoder(r.Body).Decode(&f.provisionBody)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{
				"container": f.provisionBody["name"], "dataDir": "/mnt/tank/apps/dragonwilds-servers/x", "image": f.provisionBody["image"],
			})
		case "/v1/discover":
			// Anvil reports every console's containers on the host —
			// including another console's agent family, which the adapter
			// must filter out of this console's list.
			json.NewEncoder(w).Encode(map[string]any{"servers": []map[string]any{{
				"name": "gtagent-ashenfall", "image": "ghcr.io/example/gtagent:latest", "running": true, "managed": true,
				"ports": []map[string]any{
					{"host": 9777, "container": 25600, "proto": "udp"},
					{"host": 9811, "container": 8811, "proto": "tcp"},
				},
			}, {
				"name": "otheragent-keep", "image": "ghcr.io/example/otheragent:latest", "running": true, "managed": true,
				"ports": []map[string]any{
					{"host": 15637, "container": 15637, "proto": "udp"},
					{"host": 8812, "container": 8811, "proto": "tcp"},
				},
			}, {
				// A hand-deployed stack: the agent image under a name a
				// compose file chose, not the wizard's convention.
				"name": "ix-palhalla-agent-1", "image": "ghcr.io/example/gtagent:v2", "running": false, "managed": false,
				"ports": []map[string]any{
					{"host": 25600, "container": 25600, "proto": "udp"},
					{"host": 8811, "container": 8811, "proto": "tcp"},
				},
			}, {
				// An image whose repo merely shares the prefix string —
				// must not be claimed by the family check.
				"name": "lookalike", "image": "ghcr.io/example/gtagentx:latest", "running": true, "managed": false,
				"ports": []map[string]any{},
			}}})
		case "/v1/adopt":
			// Echo the requested name, as the real Anvil does — with the
			// image that container would really carry, since the adapter's
			// family check reads it.
			var req struct {
				Container string `json:"container"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			image := "ghcr.io/example/gtagent:latest"
			if strings.HasPrefix(req.Container, "otheragent") {
				image = "ghcr.io/example/otheragent:latest"
			}
			json.NewEncoder(w).Encode(map[string]any{
				"name": req.Container, "image": image, "running": true,
				"ports": []map[string]any{
					{"host": 9777, "container": 25600, "proto": "udp"},
					{"host": 9811, "container": 8811, "proto": "tcp"},
				},
				"env": f.adoptEnv,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := anvilclient.New(srv.URL, "test-token-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return f, NewAnvilProvisioner(client, anvilTestProfile)
}

// The adapter is where the profile's provisioning knowledge lives, so this
// asserts the exact translation it performs: the prefixed environment, the
// game port plus the agent port, the container name, the image channel and
// the data mount. Any drift here provisions a container that looks right
// and boots wrong. (Each game module re-runs this assertion with its own
// profile values in its port phase — see the drift ledger's stack rows.)
func TestAnvilProvisionCarriesTheGameShape(t *testing.T) {
	f, p := newFakeAnvil(t)

	res, err := p.Provision(context.Background(), agent.ProvisionRequest{
		Slug: "ashenfall", ImageTag: "latest-wine",
		Token: "agent-token-0123456789abcdef", AdminPassword: "pw12345",
		JoinPassword: "friends-only", ServerName: "Ashenfall",
		RunAs: "568:568", GamePort: 9777, AgentPort: 9811,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Container != "gtagent-ashenfall" {
		t.Errorf("container = %q", res.Container)
	}

	body := f.provisionBody
	if body["name"] != "gtagent-ashenfall" || body["slug"] != "ashenfall" {
		t.Errorf("name/slug = %v/%v", body["name"], body["slug"])
	}
	if body["image"] != "ghcr.io/example/gtagent:latest-wine" {
		t.Errorf("image = %v", body["image"])
	}
	if body["dataMount"] != "/gametest" {
		t.Errorf("dataMount = %v", body["dataMount"])
	}
	env, _ := body["env"].(map[string]any)
	for key, want := range map[string]string{
		"GTAGENT_MODE": "supervisor", "GTAGENT_TOKEN": "agent-token-0123456789abcdef",
		"GTAGENT_ADMIN_PASSWORD": "pw12345", "GTAGENT_JOIN_PASSWORD": "friends-only",
		"GTAGENT_SERVER_NAME": "Ashenfall",
	} {
		if env[key] != want {
			t.Errorf("env[%s] = %v, want %q", key, env[key], want)
		}
	}
	// The port pair: the game's single UDP port and the agent's TCP port.
	// Getting the UDP mapping wrong is the silent kind of broken — the
	// server boots and nobody can join.
	ports, _ := json.Marshal(body["ports"])
	for _, want := range []string{`"host":9777`, `"container":25600`, `"host":9811`, `"container":8811`} {
		if !strings.Contains(string(ports), want) {
			t.Errorf("ports missing %s: %s", want, ports)
		}
	}
	if strings.Contains(string(ports), `"container":25601`) || strings.Contains(string(ports), `"host":9778`) {
		t.Errorf("ports carry a phantom second game port: %s", ports)
	}
}

// Health synthesizes the legacy shape the wizard reads: data root, public
// host and runAs come from this console's Anvil registration.
func TestAnvilHealthFeedsTheWizardDefaults(t *testing.T) {
	_, p := newFakeAnvil(t)
	h, err := p.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.Provision == nil {
		t.Fatal("no provision defaults — the wizard would demand a data path it shouldn't need")
	}
	if h.Provision.DataRoot != "/mnt/tank/apps/dragonwilds-servers" || h.Provision.PublicHost != "192.168.1.9" {
		t.Errorf("defaults = %+v", h.Provision)
	}
}

// Discover and adopt translate Anvil's generic port lists back into the
// well-known ports the wizard shape names.
func TestAnvilDiscoverAndAdoptMapPorts(t *testing.T) {
	f, p := newFakeAnvil(t)
	f.adoptEnv = map[string]string{
		"GTAGENT_MODE": "supervisor", "GTAGENT_TOKEN": "recovered-token",
		"GTAGENT_ADMIN_PASSWORD": "recovered-pw", "GTAGENT_SERVER_NAME": "Ashenfall",
	}

	found, err := p.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Exactly two: the wizard-named container and the hand-deployed stack
	// running this console's agent image. The foreign otheragent-* row and
	// the prefix lookalike never appear — a shared Anvil host serves
	// every console, and the adapter keeps only its own family.
	names := map[string]bool{}
	for _, d := range found {
		names[d.Name] = true
	}
	if len(found) != 2 || !names["gtagent-ashenfall"] || !names["ix-palhalla-agent-1"] {
		t.Errorf("discover = %+v, want the wizard-named and hand-deployed stacks only", found)
	}
	if found[0].Name != "gtagent-ashenfall" || found[0].GamePort != 9777 || found[0].AgentPort != 9811 {
		t.Errorf("discover[0] = %+v", found[0])
	}

	// And adopting a foreign one by name anyway is refused with the reason.
	if _, err := p.Adopt(context.Background(), "otheragent-keep"); err == nil || !strings.Contains(err.Error(), "another console") {
		t.Errorf("foreign adopt err = %v, want a refusal naming the family", err)
	}

	adopted, err := p.Adopt(context.Background(), "gtagent-ashenfall")
	if err != nil {
		t.Fatal(err)
	}
	if adopted.Token != "recovered-token" || adopted.AdminPassword != "recovered-pw" ||
		adopted.Mode != "supervisor" || adopted.GamePort != 9777 || adopted.AgentPort != 9811 {
		t.Errorf("adopted = %+v", adopted)
	}

	// The hand-deployed stack adopts by image family despite its name —
	// losing this orphans every pre-wizard deployment.
	if _, err := p.Adopt(context.Background(), "ix-palhalla-agent-1"); err != nil {
		t.Errorf("adopting a hand-deployed stack with our agent image: %v", err)
	}
}

// The legacy provisioner container is discoverable under Anvil (flameagent
// image, no labels) but must not be adoptable as a game server — the old
// provisioner filtered it out of discovery; the refusal now lives here.
func TestAnvilAdoptRefusesAProvisionerContainer(t *testing.T) {
	f, p := newFakeAnvil(t)
	f.adoptEnv = map[string]string{"GTAGENT_MODE": "provisioner", "GTAGENT_TOKEN": "x"}

	_, err := p.Adopt(context.Background(), "wkprovisioner")
	if err == nil || !strings.Contains(err.Error(), "provisioner") {
		t.Errorf("adopting the old provisioner: err = %v, want a refusal naming what it is", err)
	}
}
