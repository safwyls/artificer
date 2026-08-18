package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/safwyls/artificer/core/anvilclient"
	"github.com/safwyls/artificer/core/store"
)

// fakeFleet is a fakeProvisioner that also has Anvil's fleet view — the
// shape the host dashboard type-asserts for.
type fakeFleet struct {
	fakeProvisioner
	fleetHealth   *anvilclient.Health
	fleetHealthE  error
	containers    []anvilclient.ManagedContainer
	containersErr error
	images        []anvilclient.HostImage
	imagesErr     error
}

func (f *fakeFleet) FleetHealth(ctx context.Context) (*anvilclient.Health, error) {
	return f.fleetHealth, f.fleetHealthE
}

func (f *fakeFleet) FleetContainers(ctx context.Context) ([]anvilclient.ManagedContainer, error) {
	return f.containers, f.containersErr
}

func (f *fakeFleet) FleetImages(ctx context.Context) ([]anvilclient.HostImage, error) {
	return f.images, f.imagesErr
}

func TestHostOverviewIsAdminOnly(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	app.createUser(t, admin, "viewer", "viewerpass123", "user", nil)
	viewer := app.login(t, "viewer", "viewerpass123")

	if rec := app.do(t, "GET", "/api/host", nil, viewer); rec.Code != http.StatusForbidden {
		t.Errorf("non-admin: got %d, want 403 (body %s)", rec.Code, rec.Body)
	}
	if rec := app.do(t, "GET", "/api/host", nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out: got %d, want 401 (body %s)", rec.Code, rec.Body)
	}
}

// Without an Anvil there is no host view — and the endpoint says so with a
// reason, rather than a 500 or an empty host that looks real. A wizard-only
// Provisioner without the fleet view answers the same way.
func TestHostOverviewSaysWhyItIsUnavailable(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)

	for name, p := range map[string]struct{ set bool }{"no provisioner": {false}, "no fleet view": {true}} {
		app.api.Provisioner = nil
		if p.set {
			app.api.Provisioner = &fakeProvisioner{}
		}
		rec := app.do(t, "GET", "/api/host", nil, admin)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got %d (body %s)", name, rec.Code, rec.Body)
		}
		m := decodeMap(t, rec)
		if m["available"] != false || m["reason"] == nil {
			t.Errorf("%s: want available=false with a reason, got %v", name, m)
		}
	}
}

func TestHostOverviewJoinsContainersToServerRows(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "Ashenfall", Host: "10.0.0.9", ContainerName: "gtagent-ashenfall",
	})
	if err != nil {
		t.Fatal(err)
	}

	app.api.Provisioner = &fakeFleet{
		fleetHealth: &anvilclient.Health{Service: "anvil", Version: "1.2.0", DockerOk: true, DataRoot: "/mnt/tank/gt"},
		containers: []anvilclient.ManagedContainer{
			// Registered: joins to the server row above.
			{Name: "gtagent-ashenfall", Image: "ghcr.io/example/gtagent:latest", Running: true,
				State: "running", Status: "Up 3 hours", Managed: true, Mine: true, Slug: "ashenfall"},
			// Mine but registered nowhere — the orphan the dashboard exists
			// to make visible.
			{Name: "gtagent-lost", Image: "ghcr.io/example/gtagent:latest", Running: false,
				State: "exited", Status: "Exited (137) 2 days ago", Managed: true, Mine: true, Slug: "lost"},
			// Foreign: listed, never joined.
			{Name: "palagent-palhalla", Image: "ghcr.io/safwyls/palagent:latest", Running: true,
				State: "running", Managed: true, Mine: false, Owner: "palcon"},
			// Unmanaged: what an old Anvil (no ?managed=1) sends anyway.
			// The endpoint must drop it — a shared box's unrelated apps are
			// not this console's to relay.
			{Name: "nginx", Image: "nginx:latest", Running: true, State: "running", Managed: false},
		},
		images: []anvilclient.HostImage{{ID: "sha256:gt1", Tags: []string{"ghcr.io/example/gtagent:latest"}, Size: 900, Containers: []string{"gtagent-ashenfall", "gtagent-lost"}}},
	}

	rec := app.do(t, "GET", "/api/host", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d (body %s)", rec.Code, rec.Body)
	}
	m := decodeMap(t, rec)
	if m["available"] != true || m["anvilURL"] != "http://anvil:8410" {
		t.Errorf("overview header: %v", m)
	}
	health, _ := m["health"].(map[string]any)
	if health["dockerOk"] != true {
		t.Errorf("health lost in transit: %v", m["health"])
	}
	rows, _ := m["containers"].([]any)
	if len(rows) != 3 {
		t.Fatalf("containers = %v (unmanaged rows must be dropped, managed ones kept)", m["containers"])
	}
	byName := map[string]map[string]any{}
	for _, r := range rows {
		row, _ := r.(map[string]any)
		byName[fmt.Sprint(row["name"])] = row
	}
	if _, leaked := byName["nginx"]; leaked {
		t.Errorf("an unmanaged container leaked to the browser: %v", rows)
	}
	joined := byName["gtagent-ashenfall"]
	if joined["serverId"] != float64(id) || joined["serverName"] != "Ashenfall" {
		t.Errorf("registered container should join to its server row: %v", joined)
	}
	if joined["status"] != "Up 3 hours" {
		t.Errorf("lifecycle status lost: %v", joined)
	}
	orphan := byName["gtagent-lost"]
	if _, has := orphan["serverId"]; has {
		t.Errorf("an unregistered container must not claim a server row: %v", orphan)
	}
	if _, has := byName["palagent-palhalla"]["serverId"]; has {
		t.Errorf("a foreign container must not join: %v", byName["palagent-palhalla"])
	}
	images, _ := m["images"].([]any)
	if len(images) != 1 {
		t.Errorf("images = %v", m["images"])
	}
}

// Anvil wired but broken: each section carries its own failure, so "health
// answers but the list doesn't" is distinguishable from "anvil is down".
func TestHostOverviewReportsSectionFailuresIndependently(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	app.api.Provisioner = &fakeFleet{
		fleetHealth:   &anvilclient.Health{Service: "anvil", DockerOk: false},
		containersErr: fmt.Errorf("anvil unreachable: connection refused"),
		// An Anvil from before /v1/images answers 404 → ErrNotFound; the
		// dashboard should name the upgrade, not fail the page.
		imagesErr: fmt.Errorf("%w: no route", anvilclient.ErrNotFound),
	}

	rec := app.do(t, "GET", "/api/host", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d (body %s)", rec.Code, rec.Body)
	}
	m := decodeMap(t, rec)
	if m["available"] != true {
		t.Errorf("a wired Anvil is available even when reads fail: %v", m)
	}
	if m["health"] == nil {
		t.Errorf("the section that worked should still answer: %v", m)
	}
	if m["fleetError"] == nil {
		t.Errorf("the failed section should say so: %v", m)
	}
	imagesErr := fmt.Sprint(m["imagesError"])
	if !strings.Contains(imagesErr, "upgrade") {
		t.Errorf("an old Anvil should be named as such: %q", imagesErr)
	}
}
