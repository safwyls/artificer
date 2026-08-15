package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/safwyls/flamekeeper/internal/agentctl"
	"github.com/safwyls/flamekeeper/internal/flameagent"
	"github.com/safwyls/flamekeeper/internal/store"
)

// fakeProvisioner implements api.Provisioner with configurable answers —
// the wizard and delete paths are tested against the interface the Ilmari
// adapter fills in production (the adapter's own translation has its own
// tests in provisioner_ilmari_test.go).
type fakeProvisioner struct {
	mu           sync.Mutex
	health       *agentctl.Health
	healthErr    error
	provisionReq *flameagent.ProvisionRequest
	provisionRes *agentctl.ProvisionResult
	provisionErr error
	discovered   []agentctl.DiscoveredServer
	discoverErr  error
	adoptRes     *agentctl.AdoptResult
	adoptErr     error
	destroyRes   *agentctl.DestroyResult
	destroyErr   error
	destroyCalls int
}

func (f *fakeProvisioner) BaseURL() string { return "http://ilmari:8410" }

func (f *fakeProvisioner) Health(ctx context.Context) (*agentctl.Health, error) {
	if f.healthErr != nil {
		return nil, f.healthErr
	}
	if f.health != nil {
		return f.health, nil
	}
	return &agentctl.Health{Agent: "ilmari", Mode: "provisioner"}, nil
}

func (f *fakeProvisioner) Provision(ctx context.Context, req flameagent.ProvisionRequest) (*agentctl.ProvisionResult, error) {
	f.mu.Lock()
	f.provisionReq = &req
	f.mu.Unlock()
	if f.provisionErr != nil {
		return nil, f.provisionErr
	}
	return f.provisionRes, nil
}

func (f *fakeProvisioner) Discover(ctx context.Context) ([]agentctl.DiscoveredServer, error) {
	return f.discovered, f.discoverErr
}

func (f *fakeProvisioner) Adopt(ctx context.Context, container string) (*agentctl.AdoptResult, error) {
	return f.adoptRes, f.adoptErr
}

func (f *fakeProvisioner) RecreateAgent(ctx context.Context, container, imageTag string) (*flameagent.RecreateResult, error) {
	return nil, errors.New("not in these tests")
}

func (f *fakeProvisioner) Destroy(ctx context.Context, container string) (*agentctl.DestroyResult, error) {
	f.mu.Lock()
	f.destroyCalls++
	f.mu.Unlock()
	if f.destroyErr != nil {
		return nil, f.destroyErr
	}
	if f.destroyRes != nil {
		return f.destroyRes, nil
	}
	return &agentctl.DestroyResult{Container: container}, nil
}

func TestProvisionServer(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "Grimwood II", "host": "10.0.0.9", "dataPath": "/mnt/pool/apps/es-g2",
		"gamePort": 25637, "agentPort": 9811, "joinPassword": "sesame-open",
		"imageTag": "beta",
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provision: %d (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Server struct {
			ID            int64  `json:"id"`
			Host          string `json:"host"`
			GamePort      int    `json:"gamePort"`
			AgentURL      string `json:"agentUrl"`
			HasAgentToken bool   `json:"hasAgentToken"`
		} `json:"server"`
		AdminPassword string `json:"adminPassword"`
		AgentToken    string `json:"agentToken"`
		Stack         string `json:"stack"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}

	// The row is wired for the only channel this game has: the agent.
	if res.Server.Host != "10.0.0.9" || res.Server.GamePort != 25637 ||
		res.Server.AgentURL != "http://10.0.0.9:9811" || !res.Server.HasAgentToken {
		t.Errorf("server row = %+v", res.Server)
	}
	srv, err := app.store.GetServer(t.Context(), res.Server.ID)
	if err != nil || srv.AgentToken != res.AgentToken {
		t.Errorf("stored credentials don't match the response (err %v)", err)
	}
	// No dead transport ports: the game has neither RCON nor REST.
	if srv.RCONPort != 0 || srv.RESTPort != 0 {
		t.Errorf("row carries phantom transport ports: rcon %d rest %d", srv.RCONPort, srv.RESTPort)
	}
	if len(res.AdminPassword) < 16 || len(res.AgentToken) < 32 {
		t.Errorf("weak generated credentials: pw %d chars, token %d", len(res.AdminPassword), len(res.AgentToken))
	}

	// The stack carries everything the agent needs, on the beta channel.
	for _, want := range []string{
		"ghcr.io/safwyls/flameagent:beta",
		`user: "568:568"`,
		"HOME: /tmp",
		"FLAMEAGENT_MODE: supervisor",
		"FLAMEAGENT_TOKEN: " + res.AgentToken,
		"FLAMEAGENT_ADMIN_PASSWORD: " + res.AdminPassword,
		`FLAMEAGENT_SERVER_NAME: "Grimwood II"`,
		`FLAMEAGENT_JOIN_PASSWORD: "sesame-open"`,
		`"25637:15637/udp"`, `"9811:8811"`,
		"/mnt/pool/apps/es-g2:/enshrouded",
	} {
		if !strings.Contains(res.Stack, want) {
			t.Errorf("stack missing %q:\n%s", want, res.Stack)
		}
	}
	// Enshrouded binds one UDP port, and its config has no owner concept —
	// a second port line or an owner env var would be Dragonwilds leaking
	// through the template.
	for _, reject := range []string{"25638", "FLAMEAGENT_OWNER_ID", "FLAMEAGENT_WORLD_NAME"} {
		if strings.Contains(res.Stack, reject) {
			t.Errorf("stack carries %q, which this game has no use for:\n%s", reject, res.Stack)
		}
	}
}

// One-click: with a provisioner configured, provisioning also deploys.
func TestProvisionOneClickDeploy(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	dataRoot := t.TempDir()
	fake := &fakeProvisioner{
		health: &agentctl.Health{
			Agent: "ilmari", Mode: "provisioner",
			Provision: &flameagent.ProvisionDefaults{DataRoot: dataRoot, RunAs: "568:568", ImageTag: "latest"},
		},
		provisionRes: &agentctl.ProvisionResult{
			Container: "flameagent-one-click", DataDir: filepath.Join(dataRoot, "one-click"),
		},
	}
	app.api.Provisioner = fake

	// No dataPath: the one-click wizard doesn't ask — the provisioner's
	// data root decides, and the reference stack must reflect it.
	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "One Click", "host": "10.0.0.9", "joinPassword": "sesame-open",
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provision: %d (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Deployed bool   `json:"deployed"`
		DataDir  string `json:"dataDir"`
		Stack    string `json:"stack"`
		Server   struct {
			GamePort      int    `json:"gamePort"`
			ContainerName string `json:"containerName"`
		} `json:"server"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Deployed || res.DataDir != filepath.Join(dataRoot, "one-click") {
		t.Errorf("result = %+v, want deployed into one-click", res)
	}
	if !strings.Contains(res.Stack, filepath.Join(dataRoot, "one-click")+":/enshrouded") {
		t.Errorf("stack volume line missing the resolved data dir:\n%s", res.Stack)
	}
	if res.Server.GamePort != flameagent.DefaultGamePort {
		t.Errorf("gamePort = %d, want the default %d", res.Server.GamePort, flameagent.DefaultGamePort)
	}
	// The deploy request carried the wizard's identity settings through to
	// the provisioner, slug included.
	if fake.provisionReq == nil {
		t.Fatal("the provisioner was never asked to deploy")
	}
	if fake.provisionReq.Slug != "one-click" || fake.provisionReq.JoinPassword != "sesame-open" ||
		fake.provisionReq.ServerName != "One Click" {
		t.Errorf("provision request = %+v", fake.provisionReq)
	}
	// The row records the container the provisioner made — without it the
	// destroy path has no name to pass back, and the logs viewer and
	// watchdog stay dark for the one server flamekeeper knows the name of.
	if res.Server.ContainerName != "flameagent-one-click" {
		t.Errorf("containerName = %q, want flameagent-one-click", res.Server.ContainerName)
	}
}

// Deleting a server destroys its container only when asked, and only
// through the provisioner that created it. The data directory survives
// and is reported back, since removing a container is not consent to
// delete a world.
func TestDeleteServerDestroysContainerWhenAsked(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	dataRoot := t.TempDir()
	fake := &fakeProvisioner{
		destroyRes: &agentctl.DestroyResult{
			Container: "flameagent-doomed", DataDir: filepath.Join(dataRoot, "doomed"),
		},
	}
	app.api.Provisioner = fake

	newServer := func(t *testing.T, container string) string {
		t.Helper()
		rec := app.do(t, "POST", "/api/servers", map[string]any{
			"name": "Doomed", "host": "10.0.0.9", "enabled": true, "containerName": container,
		}, admin)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create server: %d (body %s)", rec.Code, rec.Body)
		}
		var created struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		return itoa(created.ID)
	}

	// Without the flag nothing on the host is touched — the long-standing
	// promise that removing a server only removes the row.
	id := newServer(t, "flameagent-doomed")
	if rec := app.do(t, "DELETE", "/api/servers/"+id, nil, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("plain delete: %d (body %s)", rec.Code, rec.Body)
	}
	if fake.destroyCalls != 0 {
		t.Errorf("a plain delete reached the provisioner: %d destroy calls", fake.destroyCalls)
	}

	// With it, the container is destroyed, and the world's directory comes
	// back so the operator knows what survived.
	id = newServer(t, "flameagent-doomed")
	rec := app.do(t, "DELETE", "/api/servers/"+id+"?removeContainer=true", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("destroying delete: %d (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Destroyed string `json:"destroyed"`
		DataDir   string `json:"dataDir"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Destroyed != "flameagent-doomed" || res.DataDir != filepath.Join(dataRoot, "doomed") {
		t.Errorf("result = %+v", res)
	}
	if fake.destroyCalls != 1 {
		t.Errorf("destroy calls = %d, want 1", fake.destroyCalls)
	}
	if rec := app.do(t, "GET", "/api/servers/"+id, nil, admin); rec.Code != http.StatusNotFound {
		t.Errorf("row survived the destroy: %d", rec.Code)
	}
}

// A destroy the provisioner refuses must leave the row alone: the
// operator still needs the card (and its credentials) to deal with the
// container by hand.
func TestDeleteServerKeepsRowWhenDestroyRefused(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	// A container deployed by hand, so the provisioner won't unmake it.
	fake := &fakeProvisioner{
		destroyErr: fmt.Errorf("%w: that container was not created by this console", agentctl.ErrRejected),
	}
	app.api.Provisioner = fake

	rec := app.do(t, "POST", "/api/servers", map[string]any{
		"name": "By Hand", "host": "10.0.0.9", "enabled": true, "containerName": "flameagent-byhand",
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create server: %d (body %s)", rec.Code, rec.Body)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := itoa(created.ID)

	if rec := app.do(t, "DELETE", "/api/servers/"+id+"?removeContainer=true", nil, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("refused destroy: %d (body %s), want 400", rec.Code, rec.Body)
	}
	if rec := app.do(t, "GET", "/api/servers/"+id, nil, admin); rec.Code != http.StatusOK {
		t.Errorf("row deleted despite the refused destroy: %d", rec.Code)
	}
}

// Asking to destroy with no provisioner configured is refused before the
// row goes, rather than silently degrading to a plain delete.
func TestDeleteServerRefusesDestroyWithoutProvisioner(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := itoa(createTestServer(t, app))

	if rec := app.do(t, "DELETE", "/api/servers/"+id+"?removeContainer=true", nil, admin); rec.Code != http.StatusBadRequest {
		t.Fatalf("destroy without a provisioner: %d (body %s), want 400", rec.Code, rec.Body)
	}
	if rec := app.do(t, "GET", "/api/servers/"+id, nil, admin); rec.Code != http.StatusOK {
		t.Errorf("row deleted anyway: %d", rec.Code)
	}
}

// A name whose container already exists on the host must be refused
// outright. The regression this guards: the deploy failed, but the row had
// already been written, leaving a server registered with credentials the
// running container has never seen — visible in the rail, unreachable
// forever.
func TestProvisionNameConflictRegistersNothing(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	fake := &fakeProvisioner{
		discovered: []agentctl.DiscoveredServer{
			{Name: "flameagent-taken", Image: "ghcr.io/safwyls/flameagent:latest", Running: true, AgentPort: 8811},
		},
	}
	app.api.Provisioner = fake

	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "Taken", "host": "10.0.0.9", "dataPath": "/mnt/pool/apps/taken",
	}, admin)
	if rec.Code != http.StatusConflict {
		t.Fatalf("provision onto a taken name: %d, want 409 (body %s)", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "flameagent-taken") {
		t.Errorf("error should name the container that's in the way: %s", rec.Body)
	}
	if fake.provisionReq != nil {
		t.Error("a deploy was attempted despite the name being visibly taken")
	}
	servers, err := app.store.ListServers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Errorf("a refused provision registered %d server(s): %+v", len(servers), servers)
	}
}

// An unreachable provisioner is the other half of the same decision: there
// is nothing to conflict with, the generated stack is still deployable by
// hand, so the row stays and the wizard falls back to pasting.
func TestProvisionKeepsRowWhenProvisionerUnreachable(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	unreachable := errors.New("ilmari unreachable: connection refused")
	app.api.Provisioner = &fakeProvisioner{
		healthErr: unreachable, discoverErr: unreachable, provisionErr: unreachable,
	}

	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "Fallback", "host": "10.0.0.9", "dataPath": "/mnt/pool/apps/fallback",
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provision: %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Deployed    bool   `json:"deployed"`
		DeployError string `json:"deployError"`
		Stack       string `json:"stack"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Deployed || res.DeployError == "" || res.Stack == "" {
		t.Errorf("want an undeployed row with a paste fallback, got %+v", res)
	}
	servers, err := app.store.ListServers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 {
		t.Errorf("registered %d servers, want the one waiting for a manual deploy", len(servers))
	}
}

func TestProvisionDefaultsAndDiscover(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	fake := &fakeProvisioner{
		health: &agentctl.Health{
			Agent: "ilmari", Mode: "provisioner",
			Provision: &flameagent.ProvisionDefaults{
				DataRoot: "/data", PublicHost: "10.99.0.5", RunAs: "568:568", ImageTag: "latest",
			},
		},
		discovered: []agentctl.DiscoveredServer{
			{Name: "flameagent-adopted", Image: "ghcr.io/safwyls/flameagent:beta", Running: true, GamePort: 25637, AgentPort: 9811},
			{Name: "flameagent-orphan", Image: "ghcr.io/safwyls/flameagent:beta", Running: false, GamePort: 25638, AgentPort: 9911},
		},
		adoptRes: &agentctl.AdoptResult{
			Name: "flameagent-orphan", Mode: "supervisor", ServerName: "Orphaned World",
			Token: "adopted-token-0123456789abcdef", AdminPassword: "adopted-pw",
			GamePort: 25638, AgentPort: 9911,
		},
	}
	app.api.Provisioner = fake

	// An existing row holding the default port set (and the "adopted"
	// candidate's agent port) forces the proposal to a free offset and
	// marks the candidate registered.
	if _, err := app.store.CreateServer(t.Context(), &store.Server{
		Name: "existing", Host: "10.99.0.5", GamePort: flameagent.DefaultGamePort,
		Enabled: true, AgentURL: "http://10.99.0.5:9811", AgentToken: agentToken,
	}); err != nil {
		t.Fatal(err)
	}

	rec := app.do(t, "GET", "/api/servers/provision/defaults", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("defaults: %d (body %s)", rec.Code, rec.Body)
	}
	var defs struct {
		Available bool           `json:"available"`
		Host      string         `json:"host"`
		RunAs     string         `json:"runAs"`
		Ports     map[string]int `json:"ports"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &defs); err != nil {
		t.Fatal(err)
	}
	if !defs.Available || defs.Host != "10.99.0.5" || defs.RunAs != "568:568" {
		t.Errorf("defaults = %+v, want declared host + run-as", defs)
	}
	// Rows AND containers hold ports — the orphan container on 9911 has no
	// row, and the proposal must still avoid it.
	used := map[int]bool{flameagent.DefaultGamePort: true, 25637: true, 25638: true, 9811: true, 9911: true}
	for _, p := range defs.Ports {
		if used[p] {
			t.Errorf("proposed ports collide with tracked/container ones: %v", defs.Ports)
		}
	}

	rec = app.do(t, "GET", "/api/servers/provision/discover", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("discover: %d (body %s)", rec.Code, rec.Body)
	}
	var disc struct {
		Servers []struct {
			Name       string `json:"name"`
			Registered bool   `json:"registered"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &disc); err != nil {
		t.Fatal(err)
	}
	byName := map[string]bool{}
	for _, s := range disc.Servers {
		byName[s.Name] = s.Registered
	}
	if !byName["flameagent-adopted"] || byName["flameagent-orphan"] {
		t.Errorf("registered flags wrong: %v", disc.Servers)
	}

	// Adopt the orphan: one call recreates a fully wired row with the
	// container's own secrets and the declared host.
	rec = app.do(t, "POST", "/api/servers/adopt", map[string]string{"container": "flameagent-orphan"}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("adopt: %d (body %s)", rec.Code, rec.Body)
	}
	var adopted struct {
		Server struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Host     string `json:"host"`
			AgentURL string `json:"agentUrl"`
		} `json:"server"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &adopted); err != nil {
		t.Fatal(err)
	}
	if adopted.Server.Name != "Orphaned World" || adopted.Server.Host != "10.99.0.5" ||
		adopted.Server.AgentURL != "http://10.99.0.5:9911" {
		t.Errorf("adopted row = %+v", adopted.Server)
	}
	row, err := app.store.GetServer(t.Context(), adopted.Server.ID)
	if err != nil || row.AgentToken != "adopted-token-0123456789abcdef" {
		t.Errorf("adopted credentials wrong (err %v)", err)
	}
}

func TestProvisionValidation(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	cases := []map[string]any{
		{"host": "h", "dataPath": "/x"},                                                  // no name
		{"name": "n", "dataPath": "/x"},                                                  // no host
		{"name": "n", "host": "h"},                                                       // no data path, no provisioner
		{"name": "n", "host": "h", "dataPath": "relative/path"},                          // non-absolute path
		{"name": "n", "host": "h", "dataPath": "/x", "gamePort": 8811},                   // game port collides with the agent port
		{"name": "n\nevil", "host": "h", "dataPath": "/x"},                               // control chars in the name
		{"name": "n", "host": "h", "dataPath": "/x", "imageTag": "beta\n    evil: true"}, // yaml injection via tag
		{"name": "n", "host": "h", "dataPath": "/x", "imageTag": "beta beta"},            // not a docker tag
	}
	for i, body := range cases {
		if rec := app.do(t, "POST", "/api/servers/provision", body, admin); rec.Code != http.StatusBadRequest {
			t.Errorf("case %d: got %d, want 400 (body %s)", i, rec.Code, rec.Body)
		}
	}
}

func TestProvisionAdminOnly(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	app.createUser(t, admin, "operator", "operatorpassword1", "user", []string{store.PermPower})
	operator := app.login(t, "operator", "operatorpassword1")
	if rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "n", "host": "h", "dataPath": "/x",
	}, operator); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin provision: got %d, want 403", rec.Code)
	}
}
