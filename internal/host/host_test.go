package host_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/safwyls/ilmari/internal/host"
)

// Two consoles, two tokens: the tests exercise the boundary between them.
const (
	wkToken  = "wk-token-0123456789abcdef"
	palToken = "pal-token-0123456789abcdef"
)

// testToken is the default caller (wildskeeper) for tests that don't care
// which console is asking.
const testToken = wkToken

// fakeDocker stands in for the daemon and records what it was asked to do.
type fakeDocker struct {
	created    map[string]any
	calls      []string
	containers string
}

func (f *fakeDocker) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/images/create":
			w.Write([]byte(`{"status":"done"}` + "\n"))
		case r.URL.Path == "/containers/create":
			json.NewDecoder(r.Body).Decode(&f.created)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"Id":"new"}`))
		case r.URL.Path == "/containers/json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(f.containers))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
}

// Two consoles' containers plus something unrelated — the situation this
// service exists to make visible.
const twoConsoles = `[
  {"Id":"c1","Names":["/wkagent-ashenfall"],"Image":"ghcr.io/safwyls/wkagent:latest","State":"running",
   "Labels":{"wildskeeper.provisioned":"true","wildskeeper.slug":"ashenfall"},
   "Ports":[{"PrivatePort":7777,"PublicPort":7777,"Type":"udp"},{"PrivatePort":8811,"PublicPort":8811,"Type":"tcp"}]},
  {"Id":"c2","Names":["/palagent-palhalla"],"Image":"ghcr.io/safwyls/palagent:latest","State":"running",
   "Labels":{"palcon.provisioned":"true","palcon.slug":"palhalla"},
   "Ports":[{"PrivatePort":8211,"PublicPort":8211,"Type":"udp"}]},
  {"Id":"c3","Names":["/nginx"],"Image":"nginx:latest","State":"running",
   "Ports":[{"PrivatePort":80,"PublicPort":9080,"Type":"tcp"}]}
]`

func newService(t *testing.T) (*httptest.Server, *fakeDocker, string) {
	t.Helper()
	fake := &fakeDocker{containers: `[]`}
	dockerSrv := httptest.NewServer(fake.handler())
	t.Cleanup(dockerSrv.Close)
	dataRoot := t.TempDir()
	svc, err := host.New(host.Config{
		Clients: []host.ClientConfig{
			{ID: "wildskeeper", Token: wkToken, DataRoot: dataRoot},
			{ID: "palcon", Token: palToken, DataRoot: filepath.Join(dataRoot, "pal")},
		},
		DockerHost: dockerSrv.URL,
		Version:    "test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(svc.Handler())
	t.Cleanup(srv.Close)
	return srv, fake, dataRoot
}

func do(t *testing.T, srv *httptest.Server, method, path string, body any) (*http.Response, map[string]any) {
	return doAs(t, srv, testToken, method, path, body)
}

func doAs(t *testing.T, srv *httptest.Server, token, method, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var payload io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, srv.URL+path, payload)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	return resp, m
}

func TestRequiresToken(t *testing.T) {
	srv, _, _ := newService(t)
	resp, err := http.Get(srv.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 without a token", resp.StatusCode)
	}
}

// The contract: a console sends what its game needs as data, and this
// service places it without understanding any of it. The spec below is
// Palworld-shaped — four ports, a server description — and nothing here
// knows what a server description is.
func TestPlacesAnyConsolesContainer(t *testing.T) {
	srv, fake, dataRoot := newService(t)

	resp, m := doAs(t, srv, palToken, "POST", "/v1/provision", map[string]any{
		"name": "palagent-palhalla", "slug": "palhalla",
		"image": "ghcr.io/safwyls/palagent:latest", "user": "568:568",
		"env":       map[string]string{"PALAGENT_MODE": "supervisor", "PALAGENT_SERVER_DESC": "chill server"},
		"ports":     []map[string]any{{"host": 8211, "container": 8211, "proto": "udp"}, {"host": 25575, "container": 25575}},
		"dataMount": "/palworld",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("provision: %d %v", resp.StatusCode, m)
	}
	// Under *palcon's* data root — the caller's own, not a shared one.
	if _, err := os.Stat(filepath.Join(dataRoot, "pal", "palhalla")); err != nil {
		t.Errorf("data dir not created under the caller's data root: %v", err)
	}
	env := fmt.Sprint(fake.created["Env"])
	if !strings.Contains(env, "PALAGENT_SERVER_DESC=chill server") {
		t.Errorf("the console's env did not survive: %v", env)
	}
	labels, _ := fake.created["Labels"].(map[string]any)
	if labels[host.LabelManaged] != "true" || labels[host.LabelSlug] != "palhalla" || labels[host.LabelOwner] != "palcon" {
		t.Errorf("ownership labels wrong: %v", labels)
	}
	hostCfg, _ := fake.created["HostConfig"].(map[string]any)
	if !strings.Contains(fmt.Sprint(hostCfg["Binds"]), ":/palworld") {
		t.Errorf("data mount not applied: %v", hostCfg["Binds"])
	}
}

// The failure this service was created to prevent: one console proposing a
// port another already holds. Before, the create succeeded and the *start*
// failed, leaving a half-made container to remove by hand.
func TestRefusesAPortAnotherConsoleAlreadyHolds(t *testing.T) {
	srv, fake, _ := newService(t)
	fake.containers = twoConsoles

	resp, m := do(t, srv, "POST", "/v1/provision", map[string]any{
		"name": "wkagent-new", "slug": "new", "image": "ghcr.io/safwyls/wkagent:latest",
		"ports": []map[string]any{{"host": 8211, "container": 7777, "proto": "udp"}},
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for a port another console holds: %v", resp.StatusCode, m)
	}
	// The refusal has to name what holds it, or the operator is back to
	// reading docker ps.
	if !strings.Contains(fmt.Sprint(m["error"]), "palagent-palhalla") {
		t.Errorf("the refusal should name the holder: %v", m["error"])
	}
	if fake.created != nil {
		t.Error("a refused provision still created a container")
	}
}

// A leaked token must not be a way to run anything on the host.
func TestRefusesImagesOutsideTheAllowlist(t *testing.T) {
	srv, fake, _ := newService(t)
	resp, m := do(t, srv, "POST", "/v1/provision", map[string]any{
		"name": "evil", "slug": "evil", "image": "docker.io/library/alpine:latest",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %v", resp.StatusCode, m)
	}
	if fake.created != nil {
		t.Error("a disallowed image reached docker create")
	}
}

// The slug becomes a directory under the data root and must never escape it.
func TestRefusesSlugTraversal(t *testing.T) {
	srv, _, _ := newService(t)
	for _, slug := range []string{"../etc", "a/b", "/abs", ".."} {
		resp, _ := do(t, srv, "POST", "/v1/provision", map[string]any{
			"name": "x", "slug": slug, "image": "ghcr.io/safwyls/wkagent:latest",
		})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("slug %q: status = %d, want 400", slug, resp.StatusCode)
		}
	}
}

// Ownership labels are the gate every destroy and rebuild reads, so a
// caller must not be able to set them.
func TestCallerCannotForgeOwnershipLabels(t *testing.T) {
	srv, fake, _ := newService(t)
	do(t, srv, "POST", "/v1/provision", map[string]any{
		"name": "wkagent-x", "slug": "x", "image": "ghcr.io/safwyls/wkagent:latest",
		"labels": map[string]string{host.LabelSlug: "someone-elses", "mine": "ok"},
	})
	labels, _ := fake.created["Labels"].(map[string]any)
	if labels[host.LabelSlug] != "x" {
		t.Errorf("a caller overwrote an ownership label: %v", labels)
	}
	if labels["mine"] != "ok" {
		t.Errorf("the caller's own labels should survive: %v", labels)
	}
}

// Containers made by a console's own provisioner, before this service
// existed, are still ours — otherwise adopting Ilmari would orphan every
// server already running.
func TestRecognisesContainersFromBeforeThisServiceExisted(t *testing.T) {
	srv, fake, dataRoot := newService(t)
	fake.containers = twoConsoles

	resp, m := do(t, srv, "GET", "/v1/containers", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %v", resp.StatusCode, m)
	}
	rows, _ := m["containers"].([]any)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want every container on the host: %v", len(rows), rows)
	}
	byName := map[string]map[string]any{}
	for _, r := range rows {
		row, _ := r.(map[string]any)
		byName[fmt.Sprint(row["name"])] = row
	}
	for _, name := range []string{"wkagent-ashenfall", "palagent-palhalla"} {
		if byName[name]["managed"] != true {
			t.Errorf("%s should be recognised as managed: %v", name, byName[name])
		}
	}
	// Unmanaged containers are listed too: they hold ports, and leaving
	// them out is precisely the blindness this replaces.
	if byName["nginx"]["managed"] != false {
		t.Errorf("nginx should be listed but not managed: %v", byName["nginx"])
	}
	if got := byName["wkagent-ashenfall"]["dataDir"]; got != filepath.Join(dataRoot, "ashenfall") {
		t.Errorf("data dir = %v", got)
	}
}

// Acting on something this service didn't make is refused, whichever verb
// asks.
func TestRefusesToTouchForeignContainers(t *testing.T) {
	srv, fake, _ := newService(t)
	fake.containers = twoConsoles

	for _, tc := range []struct {
		path string
		body map[string]any
	}{
		{"/v1/provision/destroy", map[string]any{"container": "nginx"}},
		{"/v1/provision/recreate", map[string]any{"container": "nginx", "image": "ghcr.io/safwyls/wkagent:latest"}},
	} {
		resp, m := do(t, srv, "POST", tc.path, tc.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400: %v", tc.path, resp.StatusCode, m)
		}
	}
}

func TestPortsReportsEveryPublishedPort(t *testing.T) {
	srv, fake, _ := newService(t)
	fake.containers = twoConsoles

	_, m := do(t, srv, "GET", "/v1/ports", nil)
	ports, _ := m["ports"].([]any)
	if len(ports) != 4 {
		t.Fatalf("got %d published ports, want all 4 across both consoles and nginx: %v", len(ports), ports)
	}
}

// The boundary the per-console tokens exist for: wildskeeper's token must
// not be able to destroy or rebuild a Palworld server. This holds for
// legacy containers too — palagent-palhalla predates Ilmari, and its
// palcon.provisioned label is what names its owner.
func TestAConsoleCannotActOnAnotherConsolesServers(t *testing.T) {
	srv, fake, _ := newService(t)
	fake.containers = twoConsoles

	for _, tc := range []struct {
		path string
		body map[string]any
	}{
		{"/v1/provision/destroy", map[string]any{"container": "palagent-palhalla"}},
		{"/v1/provision/recreate", map[string]any{"container": "palagent-palhalla", "image": "ghcr.io/safwyls/palagent:beta"}},
	} {
		resp, m := doAs(t, srv, wkToken, "POST", tc.path, tc.body)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s across the console boundary: status = %d, want 403: %v", tc.path, resp.StatusCode, m)
		}
	}
	// And the right console can: same request, palcon's token.
	resp, m := doAs(t, srv, palToken, "POST", "/v1/provision/destroy", map[string]any{"container": "palagent-palhalla"})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("the owning console was refused its own container: %d %v", resp.StatusCode, m)
	}
}

// Foreign rows in the fleet view exist to explain port collisions, and for
// nothing else — no slug, no data directory. What is shared between the
// contracts is deliberately just enough to avoid stepping on each other.
func TestForeignRowsShowPortsButNotPaths(t *testing.T) {
	srv, fake, dataRoot := newService(t)
	fake.containers = twoConsoles

	_, m := doAs(t, srv, wkToken, "GET", "/v1/containers", nil)
	rows, _ := m["containers"].([]any)
	byName := map[string]map[string]any{}
	for _, r := range rows {
		row, _ := r.(map[string]any)
		byName[fmt.Sprint(row["name"])] = row
	}

	mine := byName["wkagent-ashenfall"]
	if mine["mine"] != true || mine["dataDir"] != filepath.Join(dataRoot, "ashenfall") {
		t.Errorf("own row should carry its data dir: %v", mine)
	}
	foreign := byName["palagent-palhalla"]
	if foreign["mine"] != false || foreign["owner"] != "palcon" {
		t.Errorf("foreign row mislabelled: %v", foreign)
	}
	if _, has := foreign["dataDir"]; has {
		t.Errorf("a foreign row leaked its data directory: %v", foreign)
	}
	if _, has := foreign["slug"]; has {
		t.Errorf("a foreign row leaked its slug: %v", foreign)
	}
	// But its ports are visible — that is the part that must be shared.
	if _, has := foreign["ports"]; !has {
		t.Errorf("a foreign row must still show its ports: %v", foreign)
	}
}

// Health answers for the presented token: palcon's token gets palcon's
// data root, which is how a console detects holding the wrong credential
// before anything is placed.
func TestHealthIsPerConsole(t *testing.T) {
	srv, _, dataRoot := newService(t)

	_, m := doAs(t, srv, palToken, "GET", "/v1/health", nil)
	if m["client"] != "palcon" || m["dataRoot"] != filepath.Join(dataRoot, "pal") {
		t.Errorf("health for palcon's token = client %v, dataRoot %v", m["client"], m["dataRoot"])
	}
	_, m = doAs(t, srv, wkToken, "GET", "/v1/health", nil)
	if m["client"] != "wildskeeper" || m["dataRoot"] != dataRoot {
		t.Errorf("health for wildskeeper's token = client %v, dataRoot %v", m["client"], m["dataRoot"])
	}
}

// Two consoles sharing a token would make the caller ambiguous, so it is a
// startup refusal, not a silent last-one-wins.
func TestSharedTokensAreRefusedAtStartup(t *testing.T) {
	_, err := host.New(host.Config{
		Clients: []host.ClientConfig{
			{ID: "wildskeeper", Token: wkToken, DataRoot: "/a"},
			{ID: "palcon", Token: wkToken, DataRoot: "/b"},
		},
		DockerHost: "tcp://127.0.0.1:1",
	})
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Errorf("shared tokens should refuse startup, got %v", err)
	}
}
