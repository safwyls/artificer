package palagent_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/safwyls/palcon/internal/palagent"
)

// fakeDockerAPI records the provisioner's docker calls.
type fakeDockerAPI struct {
	calls  []string
	create map[string]any
}

func (f *fakeDockerAPI) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/images/create":
			w.Write([]byte(`{"status":"done"}` + "\n"))
		case r.URL.Path == "/containers/create":
			json.NewDecoder(r.Body).Decode(&f.create)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"Id":"deadbeef"}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
}

func newProvisioner(t *testing.T) (*httptest.Server, *fakeDockerAPI, string) {
	t.Helper()
	fake := &fakeDockerAPI{}
	dockerSrv := httptest.NewServer(fake.handler())
	t.Cleanup(dockerSrv.Close)
	dataRoot := t.TempDir()
	agent, err := palagent.New(palagent.Config{
		Token: testToken, InstallDir: t.TempDir(), Version: "test",
		Mode: "provisioner", DockerHost: dockerSrv.URL, DataRoot: dataRoot,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	return srv, fake, dataRoot
}

func TestProvisionerCreatesServer(t *testing.T) {
	srv, fake, dataRoot := newProvisioner(t)

	resp, m := do(t, srv, "POST", "/v1/provision", testToken, map[string]any{
		"slug": "palhalla-2", "imageTag": "beta",
		"token": "new-agent-token-0123456789abcdef", "adminPassword": "pw12345",
		"serverName": "Palhalla II", "serverDesc": "chill server", "runAs": "568:568",
		"gamePort": 9211, "restPort": 9212, "rconPort": 9575, "agentPort": 9811,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("provision: %d %v", resp.StatusCode, m)
	}
	if m["container"] != "palagent-palhalla-2" || m["dataDir"] != filepath.Join(dataRoot, "palhalla-2") {
		t.Errorf("response = %v", m)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "palhalla-2")); err != nil {
		t.Errorf("data dir not created: %v", err)
	}

	// pull → create → start, template locked to the palagent image.
	joined := strings.Join(fake.calls, " | ")
	for _, want := range []string{"/images/create", "/containers/create", "/start"} {
		if !strings.Contains(joined, want) {
			t.Errorf("docker calls missing %s: %s", want, joined)
		}
	}
	if fake.create["Image"] != "ghcr.io/safwyls/palagent:beta" || fake.create["User"] != "568:568" {
		t.Errorf("create = image %v user %v", fake.create["Image"], fake.create["User"])
	}
	env := strings.Join(toStrings(fake.create["Env"].([]any)), " ")
	for _, want := range []string{"PALAGENT_MODE=supervisor", "PALAGENT_TOKEN=new-agent-token", "PALAGENT_ADMIN_PASSWORD=pw12345", "PALAGENT_SERVER_NAME=Palhalla II", "HOME=/tmp"} {
		if !strings.Contains(env, want) {
			t.Errorf("env missing %s: %s", want, env)
		}
	}
}

func toStrings(in []any) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = v.(string)
	}
	return out
}

func TestProvisionerValidation(t *testing.T) {
	srv, _, _ := newProvisioner(t)
	cases := []map[string]any{
		{"slug": "../evil", "token": "long-enough-token-123456", "adminPassword": "x", "gamePort": 1, "restPort": 2, "rconPort": 3, "agentPort": 4},
		{"slug": "ok", "token": "short", "adminPassword": "x", "gamePort": 1, "restPort": 2, "rconPort": 3, "agentPort": 4},
		{"slug": "ok", "token": "long-enough-token-123456", "adminPassword": "", "gamePort": 1, "restPort": 2, "rconPort": 3, "agentPort": 4},
		{"slug": "ok", "token": "long-enough-token-123456", "adminPassword": "x", "runAs": "steam", "gamePort": 1, "restPort": 2, "rconPort": 3, "agentPort": 4},
		{"slug": "ok", "token": "long-enough-token-123456", "adminPassword": "x", "gamePort": 5, "restPort": 5, "rconPort": 3, "agentPort": 4},
		{"slug": "ok", "token": "long-enough-token-123456", "adminPassword": "x", "imageTag": "beta@sha256:junk", "gamePort": 1, "restPort": 2, "rconPort": 3, "agentPort": 4},
	}
	for i, body := range cases {
		if resp, m := do(t, srv, "POST", "/v1/provision", testToken, body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("case %d: got %d %v, want 400", i, resp.StatusCode, m)
		}
	}
}

func TestNonProvisionerRefusesProvision(t *testing.T) {
	srv, _ := newTestAgent(t, "exit 0") // companion
	if resp, _ := do(t, srv, "POST", "/v1/provision", testToken, map[string]any{"slug": "x"}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("companion provision: %d, want 400", resp.StatusCode)
	}
}
