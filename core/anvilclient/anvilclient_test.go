package anvilclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/safwyls/artificer/core/anvilclient"
)

const token = "anvil-token-0123456789"

// fakeAnvil answers with a status and body per path, the way the real
// service does. The status codes here are the ones anvil/internal/host
// actually writes — the invariant under test spans two modules, so each
// side guards its half: this file that the client keeps the distinctions,
// and anvil's host_test that the service still makes them.
func fakeAnvil(t *testing.T, routes map[string]struct {
	status int
	body   string
}) *anvilclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		route, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"no route"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(route.status)
		w.Write([]byte(route.body))
	}))
	t.Cleanup(srv.Close)
	c, err := anvilclient.New(srv.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type route = struct {
	status int
	body   string
}

// The invariant this package lost when provisioning moved to Anvil, and
// the reason api/servers.go could no longer clear the row for a container
// someone had already removed by hand.
func TestMissingIsDistinctFromRefused(t *testing.T) {
	ctx := context.Background()

	gone := fakeAnvil(t, map[string]route{
		"/v1/provision/destroy": {http.StatusNotFound, `{"error":"no container with that name"}`},
	})
	_, err := gone.Destroy(ctx, "no-such-container")
	if !errors.Is(err, anvilclient.ErrNotFound) {
		t.Errorf("destroying a missing container: %v, want ErrNotFound", err)
	}
	if errors.Is(err, anvilclient.ErrRejected) {
		t.Errorf("a missing container must not also read as a refusal: %v", err)
	}

	// A container that exists but isn't ours must not be mistaken for
	// "already gone" and allowed to proceed to the row delete.
	foreign := fakeAnvil(t, map[string]route{
		"/v1/provision/destroy": {http.StatusForbidden, `{"error":"that container belongs to a different console"}`},
	})
	_, err = foreign.Destroy(ctx, "palagent-palhalla")
	if !errors.Is(err, anvilclient.ErrRejected) {
		t.Errorf("destroying another console's container: %v, want ErrRejected", err)
	}
	if errors.Is(err, anvilclient.ErrNotFound) {
		t.Errorf("a refused destroy must not read as already-gone: %v", err)
	}
	if !strings.Contains(err.Error(), "different console") {
		t.Errorf("the refusal lost anvil's explanation: %v", err)
	}
}

// A container Anvil never made is a refusal too — the operator has to go
// manage it wherever it was deployed — but it is emphatically not gone.
func TestUnmanagedContainerIsRefusedNotMissing(t *testing.T) {
	c := fakeAnvil(t, map[string]route{
		"/v1/provision/destroy": {http.StatusBadRequest, `{"error":"that container was not created by Anvil — manage it wherever it was deployed"}`},
	})
	_, err := c.Destroy(context.Background(), "nginx")
	if !errors.Is(err, anvilclient.ErrRejected) || errors.Is(err, anvilclient.ErrNotFound) {
		t.Errorf("destroying an unmanaged container: %v, want ErrRejected and not ErrNotFound", err)
	}
}

// 5xx is neither. Treating a broken Anvil as "already gone" would delete
// the row for a server that is still running.
func TestServerErrorsAreNeitherMissingNorRefused(t *testing.T) {
	c := fakeAnvil(t, map[string]route{
		"/v1/provision/destroy": {http.StatusBadGateway, `{"error":"docker endpoint unreachable"}`},
	})
	_, err := c.Destroy(context.Background(), "wkagent-ashenfall")
	if err == nil {
		t.Fatal("a 502 reported success")
	}
	if errors.Is(err, anvilclient.ErrNotFound) || errors.Is(err, anvilclient.ErrRejected) {
		t.Errorf("a 502 must stay untyped: %v", err)
	}
}

func TestConflictCarriesItsReason(t *testing.T) {
	name := fakeAnvil(t, map[string]route{
		"/v1/provision": {http.StatusConflict, `{"error":"a container named wkagent-ashenfall already exists on this host","reason":"name-taken"}`},
	})
	_, err := name.Provision(context.Background(), anvilclient.Spec{Name: "wkagent-ashenfall"})
	if !errors.Is(err, anvilclient.ErrConflict) {
		t.Errorf("name conflict: %v, want ErrConflict", err)
	}
	if got := anvilclient.ConflictReason(err); got != anvilclient.ReasonNameTaken {
		t.Errorf("reason = %q, want %q", got, anvilclient.ReasonNameTaken)
	}

	ports := fakeAnvil(t, map[string]route{
		"/v1/provision": {http.StatusConflict, `{"error":"host port already in use: 8211 (palagent-palhalla)","reason":"ports-in-use"}`},
	})
	_, err = ports.Provision(context.Background(), anvilclient.Spec{Name: "wkagent-new"})
	if got := anvilclient.ConflictReason(err); got != anvilclient.ReasonPortsInUse {
		t.Errorf("reason = %q, want %q", got, anvilclient.ReasonPortsInUse)
	}
	// The name-taken advice ("adopt it instead") is wrong for this one, so
	// the caller has to be able to tell them apart without reading prose.
	if !strings.Contains(err.Error(), "8211") {
		t.Errorf("port conflict lost which port: %v", err)
	}

	// An Anvil too old to send a reason still conflicts, just without one.
	old := fakeAnvil(t, map[string]route{
		"/v1/provision": {http.StatusConflict, `{"error":"a container named wkagent-x already exists on this host"}`},
	})
	_, err = old.Provision(context.Background(), anvilclient.Spec{Name: "wkagent-x"})
	if !errors.Is(err, anvilclient.ErrConflict) || anvilclient.ConflictReason(err) != "" {
		t.Errorf("older anvil conflict = %v, reason %q", err, anvilclient.ConflictReason(err))
	}
}

func TestHealthRefusesAnIncompatibleAnvil(t *testing.T) {
	future := fakeAnvil(t, map[string]route{
		"/v1/health": {http.StatusOK, `{"service":"anvil","apiVersion":99,"client":"wildskeeper"}`},
	})
	if _, err := future.Health(context.Background()); !errors.Is(err, anvilclient.ErrAPIVersion) {
		t.Errorf("health against a v99 anvil: %v, want ErrAPIVersion", err)
	}

	ok := fakeAnvil(t, map[string]route{
		"/v1/health": {http.StatusOK, `{"service":"anvil","apiVersion":1,"client":"wildskeeper","dataRoot":"/data"}`},
	})
	h, err := ok.Health(context.Background())
	if err != nil {
		t.Fatalf("health against a matching anvil: %v", err)
	}
	if h.Client != "wildskeeper" || h.DataRoot != "/data" {
		t.Errorf("health = %+v", h)
	}

	// Silence is not a mismatch: an Anvil from before the field existed
	// should still be usable rather than locked out on a zero value.
	silent := fakeAnvil(t, map[string]route{
		"/v1/health": {http.StatusOK, `{"service":"anvil","client":"wildskeeper"}`},
	})
	if _, err := silent.Health(context.Background()); err != nil {
		t.Errorf("health against an anvil that reports no apiVersion: %v", err)
	}
}

func TestPortsAndContainersReadTheWholeHost(t *testing.T) {
	c := fakeAnvil(t, map[string]route{
		"/v1/ports": {http.StatusOK, `{"ports":[
			{"port":8211,"proto":"udp","container":"palagent-palhalla"},
			{"port":9080,"proto":"tcp","container":"nginx"}]}`},
		"/v1/containers": {http.StatusOK, `{"containers":[
			{"name":"wkagent-ashenfall","image":"ghcr.io/safwyls/wkagent:latest","running":true,
			 "managed":true,"mine":true,"slug":"ashenfall","owner":"wildskeeper","dataDir":"/data/ashenfall",
			 "ports":[{"host":7777,"container":7777,"proto":"udp"}]},
			{"name":"palagent-palhalla","image":"ghcr.io/safwyls/palagent:latest","running":true,
			 "managed":true,"mine":false,"owner":"palcon",
			 "ports":[{"host":8211,"container":8211,"proto":"udp"}]}]}`},
	})
	ctx := context.Background()

	ports, err := c.Ports(ctx)
	if err != nil {
		t.Fatalf("Ports: %v", err)
	}
	// Both rows: a port held by another console counts exactly as much as
	// one held by this one, which is the entire point of asking Anvil.
	if len(ports) != 2 || ports[0].Port != 8211 || ports[1].Container != "nginx" {
		t.Errorf("ports = %+v", ports)
	}

	found, err := c.Containers(ctx)
	if err != nil {
		t.Fatalf("Containers: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("containers = %+v", found)
	}
	if !found[0].Mine || found[0].DataDir != "/data/ashenfall" {
		t.Errorf("own row = %+v", found[0])
	}
	// A foreign row carries enough to explain a collision and nothing more.
	if found[1].Mine || found[1].DataDir != "" || found[1].Slug != "" {
		t.Errorf("foreign row leaked more than ports: %+v", found[1])
	}
	if len(found[1].Ports) != 1 || found[1].Ports[0].Host != 8211 {
		t.Errorf("foreign row lost its ports: %+v", found[1])
	}
}

func TestContainersCarryLifecycleState(t *testing.T) {
	c := fakeAnvil(t, map[string]route{
		"/v1/containers": {http.StatusOK, `{"containers":[
			{"name":"palagent-palhalla","image":"ghcr.io/safwyls/palagent:latest","running":false,
			 "state":"exited","status":"Exited (137) 2 days ago","created":1755300000,
			 "managed":true,"mine":true,"slug":"palhalla"}]}`},
	})
	found, err := c.Containers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := found[0]
	if row.State != "exited" || row.Status != "Exited (137) 2 days ago" || row.Created != 1755300000 {
		t.Errorf("lifecycle fields lost in transit: %+v", row)
	}
}

func TestImagesListsTheHostsDisk(t *testing.T) {
	c := fakeAnvil(t, map[string]route{
		"/v1/images": {http.StatusOK, `{"images":[
			{"id":"sha256:wk1","tags":["ghcr.io/safwyls/wkagent:latest"],"size":900000000,"created":1755000000,
			 "containers":["wkagent-ashenfall"]},
			{"id":"sha256:old1","tags":[],"size":870000000,"created":1754000000,"containers":[]}]}`},
	})
	images, err := c.Images(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 || images[0].Size != 900000000 || images[0].Containers[0] != "wkagent-ashenfall" {
		t.Errorf("images = %+v", images)
	}
	// The dangling image arrives with no tags and no users — its size is
	// the whole story.
	if len(images[1].Tags) != 0 || len(images[1].Containers) != 0 {
		t.Errorf("dangling image = %+v", images[1])
	}
}

// An Anvil from before /v1/images answers 404; the client must surface
// that as ErrNotFound so the console can say "upgrade Anvil to see images"
// instead of showing an empty host.
func TestImagesOnAnOlderAnvilIsNotFound(t *testing.T) {
	c := fakeAnvil(t, map[string]route{})
	_, err := c.Images(context.Background())
	if !errors.Is(err, anvilclient.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
