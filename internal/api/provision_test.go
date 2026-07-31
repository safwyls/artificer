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

	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "One Click", "host": "10.0.0.9", "dataPath": "/mnt/pool/apps/oneclick",
		"serverDesc": "motd here",
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provision: %d (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Deployed bool   `json:"deployed"`
		DataDir  string `json:"dataDir"`
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
	if res.Server.GamePort != 8211 {
		t.Errorf("gamePort = %d, want default 8211", res.Server.GamePort)
	}
	joined := strings.Join(dockerCalls, " ")
	if !strings.Contains(joined, "/containers/create") || !strings.Contains(joined, "/start") {
		t.Errorf("docker never created/started: %v", dockerCalls)
	}
}

func TestProvisionValidation(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	cases := []map[string]any{
		{"host": "h", "dataPath": "/x"},                                             // no name
		{"name": "n", "dataPath": "/x"},                                             // no host
		{"name": "n", "host": "h", "dataPath": "relative/path"},                     // non-absolute path
		{"name": "n", "host": "h", "dataPath": "/x", "gamePort": 80, "restPort": 80}, // duplicate ports
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
