package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/safwyls/flamekeeper/internal/flameagent"
	"github.com/safwyls/flamekeeper/internal/ilmari"
)

// fakeIlmari records what the adapter sends, and answers with Ilmari's real
// wire shapes.
type fakeIlmari struct {
	provisionBody map[string]any
	adoptEnv      map[string]string
}

func newFakeIlmari(t *testing.T) (*fakeIlmari, *IlmariProvisioner) {
	t.Helper()
	f := &fakeIlmari{adoptEnv: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/health":
			json.NewEncoder(w).Encode(map[string]any{
				"service": "ilmari", "version": "test", "apiVersion": 1,
				"client": "flamekeeper", "dataRoot": "/mnt/tank/apps/dragonwilds-servers",
				"publicHost": "192.168.1.9", "runAs": "568:568", "dockerOk": true,
			})
		case "/v1/provision":
			json.NewDecoder(r.Body).Decode(&f.provisionBody)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{
				"container": f.provisionBody["name"], "dataDir": "/mnt/tank/apps/dragonwilds-servers/x", "image": f.provisionBody["image"],
			})
		case "/v1/discover":
			json.NewEncoder(w).Encode(map[string]any{"servers": []map[string]any{{
				"name": "flameagent-ashenfall", "image": "ghcr.io/safwyls/flameagent:latest", "running": true, "managed": true,
				"ports": []map[string]any{
					{"host": 9777, "container": 15637, "proto": "udp"},
					{"host": 9811, "container": 8811, "proto": "tcp"},
				},
			}}})
		case "/v1/adopt":
			json.NewEncoder(w).Encode(map[string]any{
				"name": "flameagent-ashenfall", "image": "ghcr.io/safwyls/flameagent:latest", "running": true,
				"ports": []map[string]any{
					{"host": 9777, "container": 15637, "proto": "udp"},
					{"host": 9811, "container": 8811, "proto": "tcp"},
				},
				"env": f.adoptEnv,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := ilmari.New(srv.URL, "test-token-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return f, NewIlmariProvisioner(client)
}

// The adapter is where flameagent's provisioning knowledge lives, so this
// asserts the exact translation it performs: the FLAMEAGENT_* environment,
// the game's single UDP port plus the agent port, the container name, the
// image channel and the data mount. Any drift here provisions a container
// that looks right and boots wrong.
func TestIlmariProvisionCarriesTheGameShape(t *testing.T) {
	f, p := newFakeIlmari(t)

	res, err := p.Provision(context.Background(), flameagent.ProvisionRequest{
		Slug: "ashenfall", ImageTag: "latest-wine",
		Token: "agent-token-0123456789abcdef", AdminPassword: "pw12345",
		JoinPassword: "friends-only", ServerName: "Ashenfall",
		RunAs: "568:568", GamePort: 9777, AgentPort: 9811,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Container != "flameagent-ashenfall" {
		t.Errorf("container = %q", res.Container)
	}

	body := f.provisionBody
	if body["name"] != "flameagent-ashenfall" || body["slug"] != "ashenfall" {
		t.Errorf("name/slug = %v/%v", body["name"], body["slug"])
	}
	if body["image"] != "ghcr.io/safwyls/flameagent:latest-wine" {
		t.Errorf("image = %v", body["image"])
	}
	if body["dataMount"] != "/enshrouded" {
		t.Errorf("dataMount = %v", body["dataMount"])
	}
	env, _ := body["env"].(map[string]any)
	for key, want := range map[string]string{
		"FLAMEAGENT_MODE": "supervisor", "FLAMEAGENT_TOKEN": "agent-token-0123456789abcdef",
		"FLAMEAGENT_ADMIN_PASSWORD": "pw12345", "FLAMEAGENT_JOIN_PASSWORD": "friends-only",
		"FLAMEAGENT_SERVER_NAME": "Ashenfall",
	} {
		if env[key] != want {
			t.Errorf("env[%s] = %v, want %q", key, env[key], want)
		}
	}
	// The port pair: the game's single UDP port and the agent's TCP port.
	// Getting the UDP mapping wrong is the silent kind of broken — the
	// server boots and nobody can join.
	ports, _ := json.Marshal(body["ports"])
	for _, want := range []string{`"host":9777`, `"container":15637`, `"host":9811`, `"container":8811`} {
		if !strings.Contains(string(ports), want) {
			t.Errorf("ports missing %s: %s", want, ports)
		}
	}
	if strings.Contains(string(ports), `"container":15638`) || strings.Contains(string(ports), `"host":9778`) {
		t.Errorf("ports carry a phantom second game port: %s", ports)
	}
}

// Health synthesizes the legacy shape the wizard reads: data root, public
// host and runAs come from this console's Ilmari registration.
func TestIlmariHealthFeedsTheWizardDefaults(t *testing.T) {
	_, p := newFakeIlmari(t)
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

// Discover and adopt translate Ilmari's generic port lists back into the
// well-known ports the wizard shape names.
func TestIlmariDiscoverAndAdoptMapPorts(t *testing.T) {
	f, p := newFakeIlmari(t)
	f.adoptEnv = map[string]string{
		"FLAMEAGENT_MODE": "supervisor", "FLAMEAGENT_TOKEN": "recovered-token",
		"FLAMEAGENT_ADMIN_PASSWORD": "recovered-pw", "FLAMEAGENT_SERVER_NAME": "Ashenfall",
	}

	found, err := p.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].GamePort != 9777 || found[0].AgentPort != 9811 {
		t.Errorf("discover = %+v", found)
	}

	adopted, err := p.Adopt(context.Background(), "flameagent-ashenfall")
	if err != nil {
		t.Fatal(err)
	}
	if adopted.Token != "recovered-token" || adopted.AdminPassword != "recovered-pw" ||
		adopted.Mode != "supervisor" || adopted.GamePort != 9777 || adopted.AgentPort != 9811 {
		t.Errorf("adopted = %+v", adopted)
	}
}

// The legacy provisioner container is discoverable under Ilmari (flameagent
// image, no labels) but must not be adoptable as a game server — the old
// provisioner filtered it out of discovery; the refusal now lives here.
func TestIlmariAdoptRefusesAProvisionerContainer(t *testing.T) {
	f, p := newFakeIlmari(t)
	f.adoptEnv = map[string]string{"FLAMEAGENT_MODE": "provisioner", "FLAMEAGENT_TOKEN": "x"}

	_, err := p.Adopt(context.Background(), "wkprovisioner")
	if err == nil || !strings.Contains(err.Error(), "provisioner") {
		t.Errorf("adopting the old provisioner: err = %v, want a refusal naming what it is", err)
	}
}
