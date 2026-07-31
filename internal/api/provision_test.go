package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

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
