package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"gopkg.in/yaml.v3"
)

// The generated stack is pasted into `docker compose` verbatim, so it has
// to parse — and operator-supplied strings are interpolated into it, so
// the ones that survive must not be able to add anything of their own.
func TestGeneratedStackIsValidYAML(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
		"name": "Grimwood Bastion", "host": "10.0.0.9", "dataPath": "/mnt/pool/dw",
		// Quotes are legal in these; they must survive as data.
		"ownerId": `P-88F2"weird`, "serverName": `Quote"Name`, "worldName": "Ashenfall-Prime",
		"gamePort": 7877, "agentPort": 9811,
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provision: %d %s", rec.Code, rec.Body)
	}
	var res struct {
		Stack string `json:"stack"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Services map[string]struct {
			Image   string            `yaml:"image"`
			Env     map[string]string `yaml:"environment"`
			Ports   []string          `yaml:"ports"`
			Volumes []string          `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(res.Stack), &parsed); err != nil {
		t.Fatalf("generated stack is not valid YAML: %v\n%s", err, res.Stack)
	}
	svc, ok := parsed.Services["wkagent"]
	if !ok {
		t.Fatalf("no wkagent service in:\n%s", res.Stack)
	}
	if svc.Env["WKAGENT_OWNER_ID"] != `P-88F2"weird` {
		t.Errorf("owner id did not survive quoting: %q", svc.Env["WKAGENT_OWNER_ID"])
	}
	if svc.Env["WKAGENT_SERVER_NAME"] != `Quote"Name` {
		t.Errorf("server name did not survive quoting: %q", svc.Env["WKAGENT_SERVER_NAME"])
	}
	if len(svc.Ports) != 3 {
		t.Errorf("ports = %v, want the game pair plus the agent", svc.Ports)
	}
	if len(svc.Volumes) != 1 || svc.Volumes[0] != "/mnt/pool/dw:/dragonwilds" {
		t.Errorf("volumes = %v", svc.Volumes)
	}
}

// A display name reaches the stack's header comment, where YAML quoting
// isn't available — so a name carrying a newline could append services
// that run on the host when the stack is pasted. Refused at the boundary.
func TestProvisionRefusesStackInjection(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	fields := []string{"name", "host", "ownerId", "serverName", "worldName", "dataPath"}
	for _, field := range fields {
		body := map[string]any{
			"name": "Fine", "host": "10.0.0.9", "dataPath": "/mnt/pool/dw", "ownerId": "owner-abc",
		}
		body[field] = "x\nservices:\n  injected:\n    image: evil"
		rec := app.do(t, "POST", "/api/servers/provision", body, admin)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s with a newline: %d, want 400 (body %s)", field, rec.Code, rec.Body)
		}
	}
	servers, err := app.store.ListServers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Errorf("a refused provision registered %d server(s)", len(servers))
	}
}
