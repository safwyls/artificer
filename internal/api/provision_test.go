package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/safwyls/palcon/internal/agentctl"
	"github.com/safwyls/palcon/internal/palagent"
	"github.com/safwyls/palcon/internal/store"
)

func TestProvisionServer(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "Palhalla II", "host": "10.0.0.9", "dataPath": "/mnt/pool/apps/palworld-p2",
		"gamePort": 9211, "restPort": 9212, "rconPort": 9575, "agentPort": 9811,
		"imageTag": "beta",
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provision: %d (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Server struct {
			ID            int64  `json:"id"`
			Host          string `json:"host"`
			RESTPort      int    `json:"restPort"`
			AgentURL      string `json:"agentUrl"`
			HasAgentToken bool   `json:"hasAgentToken"`
			UseREST       bool   `json:"useRest"`
		} `json:"server"`
		AdminPassword string `json:"adminPassword"`
		AgentToken    string `json:"agentToken"`
		Stack         string `json:"stack"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}

	// The row is fully wired: reachable host/ports, agent URL + token,
	// REST password = the admin password.
	if res.Server.Host != "10.0.0.9" || res.Server.RESTPort != 9212 ||
		res.Server.AgentURL != "http://10.0.0.9:9811" || !res.Server.HasAgentToken || !res.Server.UseREST {
		t.Errorf("server row = %+v", res.Server)
	}
	srv, err := app.store.GetServer(t.Context(), res.Server.ID)
	if err != nil || srv.RESTPassword != res.AdminPassword || srv.AgentToken != res.AgentToken {
		t.Errorf("stored credentials don't match the response (err %v)", err)
	}
	if len(res.AdminPassword) < 16 || len(res.AgentToken) < 32 {
		t.Errorf("weak generated credentials: pw %d chars, token %d", len(res.AdminPassword), len(res.AgentToken))
	}

	// The stack carries everything the agent needs, on the beta channel.
	for _, want := range []string{
		"ghcr.io/safwyls/palagent:beta",
		`user: "568:568"`,
		"HOME: /tmp",
		"PALAGENT_MODE: supervisor",
		"PALAGENT_TOKEN: " + res.AgentToken,
		"PALAGENT_ADMIN_PASSWORD: " + res.AdminPassword,
		`"9211:8211/udp"`, `"9212:8212"`, `"9575:25575"`, `"9811:8811"`,
		"/mnt/pool/apps/palworld-p2:/palworld",
	} {
		if !strings.Contains(res.Stack, want) {
			t.Errorf("stack missing %q:\n%s", want, res.Stack)
		}
	}
}

// One-click: with a provisioner configured, provisioning also deploys.
// The provisioner here is a real provisioner-mode palagent over a fake
// docker API — the full palcon → provisioner → docker chain.
func TestProvisionOneClickDeploy(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	var dockerCalls []string
	dockerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dockerCalls = append(dockerCalls, r.URL.Path)
		switch {
		case r.URL.Path == "/images/create":
			w.Write([]byte(`{"status":"done"}` + "\n"))
		case r.URL.Path == "/containers/create":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"Id":"cafe"}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(dockerSrv.Close)

	dataRoot := t.TempDir()
	provAgent, err := palagent.New(palagent.Config{
		Token: agentToken, InstallDir: t.TempDir(), Version: "test",
		Mode: "provisioner", DockerHost: dockerSrv.URL, DataRoot: dataRoot,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	provSrv := httptest.NewServer(provAgent.Handler())
	t.Cleanup(provSrv.Close)
	app.api.Provisioner, err = agentctl.New(provSrv.URL, agentToken)
	if err != nil {
		t.Fatal(err)
	}

	// No dataPath: the one-click wizard doesn't ask — the provisioner's
	// data root decides, and the reference stack must reflect it.
	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "One Click", "host": "10.0.0.9",
		"serverDesc": "motd here",
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provision: %d (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Deployed bool   `json:"deployed"`
		DataDir  string `json:"dataDir"`
		Stack    string `json:"stack"`
		Server   struct {
			GamePort int `json:"gamePort"`
		} `json:"server"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Deployed || res.DataDir != filepath.Join(dataRoot, "one-click") {
		t.Errorf("result = %+v, want deployed into one-click", res)
	}
	if !strings.Contains(res.Stack, filepath.Join(dataRoot, "one-click")+":/palworld") {
		t.Errorf("stack volume line missing the resolved data dir:\n%s", res.Stack)
	}
	if res.Server.GamePort != 8211 {
		t.Errorf("gamePort = %d, want default 8211", res.Server.GamePort)
	}
	joined := strings.Join(dockerCalls, " ")
	if !strings.Contains(joined, "/containers/create") || !strings.Contains(joined, "/start") {
		t.Errorf("docker never created/started: %v", dockerCalls)
	}
}

func TestProvisionDefaultsAndDiscover(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	dockerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/containers/json":
			w.Write([]byte(`[
			  {"Id":"c1","Names":["/palagent-adopted"],"Image":"ghcr.io/safwyls/palagent:beta","State":"running",
			   "Ports":[{"PrivatePort":8811,"PublicPort":9811,"Type":"tcp"},{"PrivatePort":8212,"PublicPort":9212,"Type":"tcp"}]},
			  {"Id":"c2","Names":["/palagent-orphan"],"Image":"ghcr.io/safwyls/palagent:beta","State":"exited",
			   "Ports":[{"PrivatePort":8811,"PublicPort":9911,"Type":"tcp"}]}
			]`))
		case r.URL.Path == "/containers/c1/json", r.URL.Path == "/containers/c2/json":
			w.Write([]byte(`{"Config":{"Env":["PALAGENT_MODE=supervisor","PALAGENT_TOKEN=adopted-token-0123456789abcdef","PALAGENT_ADMIN_PASSWORD=adopted-pw","PALAGENT_SERVER_NAME=Orphaned World"]}}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(dockerSrv.Close)

	provAgent, err := palagent.New(palagent.Config{
		Token: agentToken, InstallDir: t.TempDir(), Version: "test",
		Mode: "provisioner", DockerHost: dockerSrv.URL, DataRoot: t.TempDir(),
		PublicHost: "10.99.0.5",
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	provSrv := httptest.NewServer(provAgent.Handler())
	t.Cleanup(provSrv.Close)
	app.api.Provisioner, err = agentctl.New(provSrv.URL, agentToken)
	if err != nil {
		t.Fatal(err)
	}

	// An existing row holding the default port set (and the "adopted"
	// candidate's agent port) forces the proposal to a free offset and
	// marks the candidate registered.
	if _, err := app.store.CreateServer(t.Context(), &store.Server{
		Name: "existing", Host: "10.99.0.5", RCONPort: 25575, RESTPort: 8212, GamePort: 8211,
		UseREST: true, Enabled: true, AgentURL: "http://10.99.0.5:9811", AgentToken: agentToken,
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
	// Rows AND containers hold ports — the ghost container on 9911 has no
	// row, and the proposal must still avoid it.
	used := map[int]bool{8211: true, 8212: true, 25575: true, 9811: true, 9212: true, 9911: true}
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
	if !byName["palagent-adopted"] || byName["palagent-orphan"] {
		t.Errorf("registered flags wrong: %v", disc.Servers)
	}

	// Adopt the orphan: one call recreates a fully wired row with the
	// container's own secrets and the declared host.
	rec = app.do(t, "POST", "/api/servers/adopt", map[string]string{"container": "palagent-orphan"}, admin)
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
	if err != nil || row.AgentToken != "adopted-token-0123456789abcdef" || row.RESTPassword != "adopted-pw" {
		t.Errorf("adopted credentials wrong (err %v)", err)
	}
}

func TestProvisionValidation(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	cases := []map[string]any{
		{"host": "h", "dataPath": "/x"},                                             // no name
		{"name": "n", "dataPath": "/x"},                                             // no host
		{"name": "n", "host": "h"},                                                 // no data path, no provisioner
		{"name": "n", "host": "h", "dataPath": "relative/path"},                     // non-absolute path
		{"name": "n", "host": "h", "dataPath": "/x", "gamePort": 80, "restPort": 80},          // duplicate ports
		{"name": "n", "host": "h", "dataPath": "/x", "imageTag": "beta\n    evil: true"},      // yaml injection via tag
		{"name": "n", "host": "h", "dataPath": "/x", "imageTag": "beta beta"},                 // not a docker tag
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
