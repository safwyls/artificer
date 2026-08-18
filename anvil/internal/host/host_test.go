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

	"github.com/safwyls/anvil/internal/host"
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
	created map[string]any
	calls   []string
	// requests keeps the query string too, which is where the promises
	// about what a remove does and does not take live (v=0, force=0).
	requests   []string
	containers string
	images     string
}

func (f *fakeDocker) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		f.requests = append(f.requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
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
		case r.URL.Path == "/images/json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(f.images))
		case strings.HasPrefix(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json"):
			// Inspect serves two callers: adopt reads the env (mixed
			// namespaces on purpose, so the scoping has something to scope
			// away), and recreate reads everything a rebuild must carry
			// over — binds, ports, labels, restart policy, networks.
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
			  "Name":"/wkagent-ashenfall",
			  "Config":{
			    "Image":"ghcr.io/safwyls/wkagent:latest","User":"568:568",
			    "Env":["WKAGENT_MODE=supervisor","WKAGENT_TOKEN=wk-secret","PALAGENT_TOKEN=pal-secret","HOME=/tmp"],
			    "Labels":{"anvil.managed":"true","anvil.owner":"wildskeeper","anvil.slug":"ashenfall"}},
			  "HostConfig":{
			    "Binds":["/mnt/tank/dw/ashenfall:/dragonwilds"],
			    "RestartPolicy":{"Name":"unless-stopped"},
			    "PortBindings":{"7777/udp":[{"HostPort":"7777"}],"7778/udp":[{"HostPort":"7778"}],"8811/tcp":[{"HostPort":"8811"}]}},
			  "NetworkSettings":{"Networks":{"bridge":{},"wildskeeper-net":{}}}}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
}

// Two consoles' containers plus something unrelated — the situation this
// service exists to make visible.
const twoConsoles = `[
  {"Id":"c1","Names":["/wkagent-ashenfall"],"Image":"ghcr.io/safwyls/wkagent:latest","ImageID":"sha256:wk1",
   "State":"running","Status":"Up 3 hours","Created":1755400000,
   "Labels":{"wildskeeper.provisioned":"true","wildskeeper.slug":"ashenfall"},
   "Ports":[{"PrivatePort":7777,"PublicPort":7777,"Type":"udp"},{"PrivatePort":8811,"PublicPort":8811,"Type":"tcp"}]},
  {"Id":"c2","Names":["/palagent-palhalla"],"Image":"ghcr.io/safwyls/palagent:latest","ImageID":"sha256:pal1",
   "State":"exited","Status":"Exited (137) 2 days ago","Created":1755300000,
   "Labels":{"palcon.provisioned":"true","palcon.slug":"palhalla"},
   "Ports":[{"PrivatePort":8211,"PublicPort":8211,"Type":"udp"}]},
  {"Id":"c3","Names":["/nginx"],"Image":"nginx:latest","ImageID":"sha256:ng1","State":"running","Status":"Up 5 days",
   "Ports":[{"PrivatePort":80,"PublicPort":9080,"Type":"tcp"}]}
]`

func newService(t *testing.T) (*httptest.Server, *fakeDocker, string) {
	t.Helper()
	fake := &fakeDocker{containers: `[]`, images: `[]`}
	dockerSrv := httptest.NewServer(fake.handler())
	t.Cleanup(dockerSrv.Close)
	dataRoot := t.TempDir()
	svc, err := host.New(host.Config{
		Clients: []host.ClientConfig{
			{ID: "wildskeeper", Token: wkToken, DataRoot: dataRoot, EnvPrefix: "WKAGENT_"},
			{ID: "palcon", Token: palToken, DataRoot: filepath.Join(dataRoot, "pal"), EnvPrefix: "PALAGENT_"},
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
// existed, are still ours — otherwise adopting Anvil would orphan every
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
// legacy containers too — palagent-palhalla predates Anvil, and its
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

// The fleet row says where a container is in its lifecycle, not just
// running-or-not: an exited server's exit code and age (docker's Status
// sentence) are the difference between "stopped on purpose" and "crashed
// two days ago and nobody noticed".
func TestFleetRowsCarryLifecycleState(t *testing.T) {
	srv, fake, _ := newService(t)
	fake.containers = twoConsoles

	_, m := do(t, srv, "GET", "/v1/containers", nil)
	rows, _ := m["containers"].([]any)
	byName := map[string]map[string]any{}
	for _, r := range rows {
		row, _ := r.(map[string]any)
		byName[fmt.Sprint(row["name"])] = row
	}
	crashed := byName["palagent-palhalla"]
	if crashed["state"] != "exited" || crashed["running"] != false {
		t.Errorf("exited container mislabelled: %v", crashed)
	}
	if crashed["status"] != "Exited (137) 2 days ago" {
		t.Errorf("status sentence lost: %v", crashed)
	}
	if crashed["created"] != float64(1755300000) {
		t.Errorf("created lost: %v", crashed)
	}
	up := byName["wkagent-ashenfall"]
	if up["state"] != "running" || up["status"] != "Up 3 hours" {
		t.Errorf("running container mislabelled: %v", up)
	}
}

// Images are as shared as ports: every console sees all of them, joined to
// the containers using them, with dangling ones visible because they are
// pure disk cost.
func TestImagesReportUseAndDangling(t *testing.T) {
	srv, fake, _ := newService(t)
	fake.containers = twoConsoles
	fake.images = `[
	  {"Id":"sha256:wk1","RepoTags":["ghcr.io/safwyls/wkagent:latest"],"Size":900000000,"Created":1755000000},
	  {"Id":"sha256:pal1","RepoTags":["ghcr.io/safwyls/palagent:latest"],"Size":400000000,"Created":1755100000},
	  {"Id":"sha256:old1","RepoTags":["<none>:<none>"],"Size":870000000,"Created":1754000000},
	  {"Id":"sha256:ng1","RepoTags":["nginx:latest"],"Size":100000000,"Created":1754500000}
	]`

	resp, m := do(t, srv, "GET", "/v1/images", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("images: %d %v", resp.StatusCode, m)
	}
	rows, _ := m["images"].([]any)
	if len(rows) != 4 {
		t.Fatalf("got %d images, want all 4: %v", len(rows), rows)
	}
	// Sorted biggest first — the list answers "what is the disk spent on".
	first, _ := rows[0].(map[string]any)
	if first["id"] != "sha256:wk1" {
		t.Errorf("biggest image should lead: %v", first)
	}
	byID := map[string]map[string]any{}
	for _, r := range rows {
		row, _ := r.(map[string]any)
		byID[fmt.Sprint(row["id"])] = row
	}
	if got := fmt.Sprint(byID["sha256:wk1"]["containers"]); got != "[wkagent-ashenfall]" {
		t.Errorf("wkagent image should name its container: %v", got)
	}
	// The exited container still pins its image — created-from, not running-on.
	if got := fmt.Sprint(byID["sha256:pal1"]["containers"]); got != "[palagent-palhalla]" {
		t.Errorf("an exited container still uses its image: %v", got)
	}
	dangling := byID["sha256:old1"]
	if tags, _ := dangling["tags"].([]any); len(tags) != 0 {
		t.Errorf("a dangling image should carry no tags: %v", dangling)
	}
	if got := fmt.Sprint(dangling["containers"]); got != "[]" {
		t.Errorf("nothing uses the dangling image: %v", got)
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

// Discovery is scoped like everything else: a console sees its own
// containers (by label, legacy included) plus unmanaged containers whose
// image falls under its allowlist — the paste-flow deploys that carry no
// label at all. Another console's servers never appear, and neither does
// anything unrelated.
func TestDiscoverIsScopedToTheCaller(t *testing.T) {
	srv, fake, _ := newService(t)
	// twoConsoles plus a hand-deployed wkagent stack (no labels): the
	// paste-flow case discovery exists for.
	fake.containers = strings.TrimSuffix(twoConsoles, "\n]") + `,
  {"Id":"c4","Names":["/wkagent-byhand"],"Image":"ghcr.io/safwyls/wkagent:latest","State":"running",
   "Ports":[{"PrivatePort":7777,"PublicPort":9777,"Type":"udp"}]}
]`

	_, m := doAs(t, srv, wkToken, "GET", "/v1/discover", nil)
	rows, _ := m["servers"].([]any)
	names := map[string]bool{}
	for _, r := range rows {
		row, _ := r.(map[string]any)
		names[fmt.Sprint(row["name"])] = true
	}
	if !names["wkagent-ashenfall"] || !names["wkagent-byhand"] {
		t.Errorf("own and hand-deployed containers should be discoverable: %v", names)
	}
	if names["palagent-palhalla"] {
		t.Error("another console's server was discoverable")
	}
	if names["nginx"] {
		t.Error("an unrelated container was discoverable")
	}
}

// Adopt returns the environment a console's own provisioner injected — and
// only the caller's namespace of it. The trust argument is that the console
// supplied these values in the first place; the scoping is what keeps that
// argument honest when two consoles share a host.
func TestAdoptReturnsOnlyTheCallersEnvNamespace(t *testing.T) {
	srv, fake, _ := newService(t)
	fake.containers = twoConsoles

	resp, m := doAs(t, srv, wkToken, "POST", "/v1/adopt", map[string]any{"container": "wkagent-ashenfall"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("adopt: %d %v", resp.StatusCode, m)
	}
	env, _ := m["env"].(map[string]any)
	if env["WKAGENT_TOKEN"] != "wk-secret" || env["WKAGENT_MODE"] != "supervisor" {
		t.Errorf("the caller's own namespace should come back: %v", env)
	}
	// The fake's env deliberately carries another console's variable and an
	// unrelated one; neither may cross.
	if _, leaked := env["PALAGENT_TOKEN"]; leaked {
		t.Errorf("another console's env crossed the boundary: %v", env)
	}
	if _, leaked := env["HOME"]; leaked {
		t.Errorf("unrelated env crossed: %v", env)
	}
}

// Adoption across the console boundary is the same 403 as destroy and
// rebuild — a foreign server is not recoverable with the wrong token, which
// is most of the point of tokens: adopt returns secrets.
func TestAdoptRefusesForeignContainers(t *testing.T) {
	srv, fake, _ := newService(t)
	fake.containers = twoConsoles

	resp, _ := doAs(t, srv, wkToken, "POST", "/v1/adopt", map[string]any{"container": "palagent-palhalla"})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("adopting a foreign container: status = %d, want 403", resp.StatusCode)
	}
	resp, _ = doAs(t, srv, wkToken, "POST", "/v1/adopt", map[string]any{"container": "nginx"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("adopting an unrelated container: status = %d, want 400", resp.StatusCode)
	}
}

// The service half of the missing-versus-refused invariant. Its client
// half is core/anvilclient's TestMissingIsDistinctFromRefused; a console
// deletes the server row on one of these answers and must not on the
// other, so the two statuses have to stay different at both ends.
func TestDestroyingAMissingContainerIsNotFound(t *testing.T) {
	srv, fake, _ := newService(t)
	fake.containers = twoConsoles

	resp, m := do(t, srv, "POST", "/v1/provision/destroy", map[string]any{"container": "no-such-container"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("destroying a container that isn't here: %d, want 404: %v", resp.StatusCode, m)
	}
	// The refusals are the other answers, and none of them may be reused
	// for "it isn't here": a console reads 403 as "leave everything alone"
	// and 404 as "the job is done".
	resp, _ = do(t, srv, "POST", "/v1/provision/destroy", map[string]any{"container": "palagent-palhalla"})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("another console's container = %d, want 403", resp.StatusCode)
	}
	resp, _ = do(t, srv, "POST", "/v1/provision/destroy", map[string]any{"container": "nginx"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an unmanaged container = %d, want 400", resp.StatusCode)
	}
}

// Unmaking a container is not consent to delete what it was holding. This
// is the promise the whole destroy path exists to keep — a world save
// lives in the data directory, and the console offers this button next to
// "delete server".
//
// Guarded at both levels on purpose: dockerctl's
// TestContainerRemoveKeepsTheVolume asserts the v=0 on the wire, and this
// asserts the endpoint an operator actually reaches leaves the files
// where they are and says where they still live.
func TestDestroyKeepsTheDataDirectory(t *testing.T) {
	srv, fake, dataRoot := newService(t)

	resp, m := do(t, srv, "POST", "/v1/provision", map[string]any{
		"name": "wkagent-ashenfall", "slug": "ashenfall",
		"image": "ghcr.io/safwyls/wkagent:latest", "dataMount": "/dragonwilds",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("provision: %d %v", resp.StatusCode, m)
	}
	dataDir := filepath.Join(dataRoot, "ashenfall")
	world := filepath.Join(dataDir, "world.sav")
	if err := os.WriteFile(world, []byte("a world someone played in"), 0o644); err != nil {
		t.Fatal(err)
	}

	fake.containers = `[{"Id":"c1","Names":["/wkagent-ashenfall"],"Image":"ghcr.io/safwyls/wkagent:latest","State":"running",
	  "Labels":{"anvil.managed":"true","anvil.owner":"wildskeeper","anvil.slug":"ashenfall"}}]`
	resp, m = do(t, srv, "POST", "/v1/provision/destroy", map[string]any{"container": "wkagent-ashenfall"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("destroy: %d %v", resp.StatusCode, m)
	}

	// The answer has to name the directory, or an operator cannot tell
	// what survived without going to look.
	if m["dataDir"] != dataDir {
		t.Errorf("dataDir = %v, want %s", m["dataDir"], dataDir)
	}
	data, err := os.ReadFile(world)
	if err != nil {
		t.Fatalf("the world was deleted along with the container: %v", err)
	}
	if string(data) != "a world someone played in" {
		t.Errorf("world contents changed: %q", data)
	}
	// And on the wire: no volume removal, no SIGKILL that would skip the
	// game's chance to flush.
	removed := ""
	for _, req := range fake.requests {
		if strings.HasPrefix(req, "DELETE /containers/") {
			removed = req
		}
	}
	if removed == "" {
		t.Fatalf("no container removal was sent: %v", fake.requests)
	}
	if !strings.Contains(removed, "v=0") || !strings.Contains(removed, "force=0") {
		t.Errorf("remove request = %q, want v=0 and force=0", removed)
	}
	// The stop comes first, so whatever is inside gets its grace period.
	stopped := false
	for _, req := range fake.requests {
		if strings.Contains(req, "/stop") {
			stopped = true
		}
		if strings.HasPrefix(req, "DELETE /containers/") && !stopped {
			t.Error("the container was removed before it was stopped")
		}
	}
}

// Swapping an image must change the image and nothing else. Every field
// the rebuild forgets is a live server that comes back subtly wrong:
// without its data mount it starts a fresh world, without its ports it is
// unreachable, without its network it cannot resolve what it talks to,
// without its ownership labels it is orphaned from this service entirely.
func TestRecreateKeepsEverythingButTheImage(t *testing.T) {
	srv, fake, _ := newService(t)
	fake.containers = twoConsoles

	resp, m := do(t, srv, "POST", "/v1/provision/recreate", map[string]any{
		"container": "wkagent-ashenfall", "image": "ghcr.io/safwyls/wkagent:beta",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recreate: %d %v", resp.StatusCode, m)
	}
	if m["image"] != "ghcr.io/safwyls/wkagent:beta" || m["previousImage"] != "ghcr.io/safwyls/wkagent:latest" {
		t.Errorf("result = %v", m)
	}

	created := fake.created
	if created == nil {
		t.Fatal("nothing was created")
	}
	if created["Image"] != "ghcr.io/safwyls/wkagent:beta" {
		t.Errorf("image = %v", created["Image"])
	}
	if created["User"] != "568:568" {
		t.Errorf("run-as user lost: %v", created["User"])
	}
	if env := fmt.Sprint(created["Env"]); !strings.Contains(env, "WKAGENT_TOKEN=wk-secret") {
		t.Errorf("agent token lost — the console could not authenticate to the rebuilt agent: %v", env)
	}
	labels, _ := created["Labels"].(map[string]any)
	if labels[host.LabelOwner] != "wildskeeper" || labels[host.LabelSlug] != "ashenfall" {
		t.Errorf("ownership labels lost: %v — the container would be unmanageable after the rebuild", labels)
	}
	hc, _ := created["HostConfig"].(map[string]any)
	if !strings.Contains(fmt.Sprint(hc["Binds"]), "/dragonwilds") {
		t.Errorf("data mount lost: %v — the rebuilt server would start an empty world", hc["Binds"])
	}
	if hc["RestartPolicy"].(map[string]any)["Name"] != "unless-stopped" {
		t.Errorf("restart policy lost: %v", hc["RestartPolicy"])
	}
	bindings, _ := hc["PortBindings"].(map[string]any)
	for _, want := range []string{"7777/udp", "7778/udp", "8811/tcp"} {
		if bindings[want] == nil {
			t.Errorf("port %s lost: %v", want, bindings)
		}
	}
	networking, _ := created["NetworkingConfig"].(map[string]any)
	if networking == nil {
		t.Fatalf("no NetworkingConfig: the rebuilt container is off wildskeeper-net: %v", created)
	}
	endpoints, _ := networking["EndpointsConfig"].(map[string]any)
	if _, ok := endpoints["wildskeeper-net"]; !ok {
		t.Errorf("user-defined network lost: %v", endpoints)
	}
	if _, ok := endpoints["bridge"]; ok {
		t.Error("bridge re-declared explicitly; docker rejects that")
	}

	// Order matters as much as content: the image has to be on the host
	// before the running container is removed, or a bad tag leaves the
	// server destroyed with nothing to put back.
	pulled, removed := -1, -1
	for i, call := range fake.calls {
		switch {
		case strings.Contains(call, "/images/create"):
			pulled = i
		case strings.HasPrefix(call, "DELETE /containers/"):
			removed = i
		}
	}
	if pulled == -1 || removed == -1 || pulled > removed {
		t.Errorf("pull/remove order = %v", fake.calls)
	}
}

// Recreating onto the image it already runs is a no-op, not a rebuild.
// Tearing a live server down to put back exactly what was there is pure
// downtime for nothing.
func TestRecreateOntoTheSameImageChangesNothing(t *testing.T) {
	srv, fake, _ := newService(t)
	fake.containers = twoConsoles

	resp, m := do(t, srv, "POST", "/v1/provision/recreate", map[string]any{
		"container": "wkagent-ashenfall", "image": "ghcr.io/safwyls/wkagent:latest",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("recreate: %d %v", resp.StatusCode, m)
	}
	if fake.created != nil {
		t.Error("the container was rebuilt onto the image it was already running")
	}
	for _, call := range fake.calls {
		if strings.HasPrefix(call, "DELETE /containers/") {
			t.Errorf("a no-op recreate removed the container: %v", fake.calls)
		}
	}
}
