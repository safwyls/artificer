package dockerctl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeDocker implements the three endpoints the provisioner path uses.
func fakeDocker(t *testing.T) (*httptest.Server, *[]string, *map[string]any) {
	t.Helper()
	var calls []string
	var createPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case r.URL.Path == "/images/create":
			// Progress lines, then EOF — the client must consume them all.
			w.Write([]byte(`{"status":"Pulling"}` + "\n" + `{"status":"Download complete"}` + "\n"))
		case r.URL.Path == "/containers/create":
			json.NewDecoder(r.Body).Decode(&createPayload)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"Id":"abc123"}`))
		default:
			w.WriteHeader(http.StatusNoContent) // start
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &createPayload
}

func TestImagePullAndCreate(t *testing.T) {
	srv, calls, payload := fakeDocker(t)
	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if err := c.ImagePull(ctx, "ghcr.io/safwyls/wkagent:beta"); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if (*calls)[0] != "POST /images/create?fromImage=ghcr.io%2Fsafwyls%2Fwkagent&tag=beta" {
		t.Errorf("pull call = %q", (*calls)[0])
	}

	id, err := c.ContainerCreate(ctx, ContainerSpec{
		Name: "wkagent-test", Image: "ghcr.io/safwyls/wkagent:beta",
		User: "568:568", Env: []string{"A=b"},
		Binds:                []string{"/data/test:/palworld"},
		Ports:                map[int]string{9211: "8211/udp", 9811: "8811"},
		RestartUnlessStopped: true,
	})
	if err != nil || id != "abc123" {
		t.Fatalf("create: id=%q err=%v", id, err)
	}
	p := *payload
	if p["User"] != "568:568" || p["Image"] != "ghcr.io/safwyls/wkagent:beta" {
		t.Errorf("payload user/image = %v %v", p["User"], p["Image"])
	}
	hc := p["HostConfig"].(map[string]any)
	if hc["RestartPolicy"].(map[string]any)["Name"] != "unless-stopped" {
		t.Errorf("restart policy = %v", hc["RestartPolicy"])
	}
	bindings := hc["PortBindings"].(map[string]any)
	if _, ok := bindings["8211/udp"]; !ok {
		t.Errorf("missing udp binding: %v", bindings)
	}
	if _, ok := bindings["8811/tcp"]; !ok {
		t.Errorf("protoless port not normalized to tcp: %v", bindings)
	}
}

func TestImagePullSurfacesStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"status":"Pulling"}` + "\n" + `{"error":"manifest unknown"}` + "\n"))
	}))
	t.Cleanup(srv.Close)
	c, _ := New(srv.URL)
	if err := c.ImagePull(context.Background(), "ghcr.io/x/y:z"); err == nil {
		t.Fatal("mid-stream pull error not surfaced")
	}
}

// InspectSpec is what makes a rebuild faithful rather than a fresh
// container that happens to share a name. Docker has no "change the
// image", so recreate reads everything back and builds it again — and
// anything this drops is silently gone from the rebuilt container. These
// are the cases where the docker payload does not say what it appears to.
func TestInspectSpecReadsBackWhatRecreateMustKeep(t *testing.T) {
	const inspect = `{
	  "Name": "/wkagent-ashenfall",
	  "Config": {
	    "Image": "ghcr.io/safwyls/wkagent:latest",
	    "User": "568:568",
	    "Env": ["WKAGENT_MODE=supervisor","WKAGENT_TOKEN=secret"],
	    "Labels": {"anvil.managed":"true","anvil.slug":"ashenfall"}
	  },
	  "HostConfig": {
	    "Binds": ["/mnt/tank/apps/dw/ashenfall:/dragonwilds"],
	    "RestartPolicy": {"Name": "unless-stopped"},
	    "PortBindings": {
	      "7777/udp": [{"HostPort": "7777"}],
	      "7778/udp": [{"HostPort": "7778"}],
	      "8811/tcp": [{"HostPort": "8811"}],
	      "9999/tcp": [{"HostPort": ""}]
	    }
	  },
	  "NetworkSettings": {"Networks": {"bridge": {}, "wildskeeper-net": {}, "anvil-net": {}}}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(inspect))
	}))
	t.Cleanup(srv.Close)
	c, _ := New(srv.URL)

	spec, err := c.InspectSpec(context.Background(), "wkagent-ashenfall")
	if err != nil {
		t.Fatalf("InspectSpec: %v", err)
	}
	if spec.Name != "wkagent-ashenfall" {
		t.Errorf("name = %q — docker's leading slash must be trimmed, or the rebuild is refused", spec.Name)
	}
	if spec.Image != "ghcr.io/safwyls/wkagent:latest" || spec.User != "568:568" {
		t.Errorf("image/user = %q %q", spec.Image, spec.User)
	}
	if len(spec.Env) != 2 || spec.Env[1] != "WKAGENT_TOKEN=secret" {
		// Losing the env means a rebuilt agent the console can't
		// authenticate to — a server that goes dark on an image bump.
		t.Errorf("env = %v", spec.Env)
	}
	if len(spec.Binds) != 1 || spec.Binds[0] != "/mnt/tank/apps/dw/ashenfall:/dragonwilds" {
		t.Errorf("binds = %v — the world lives there", spec.Binds)
	}
	if !spec.RestartUnlessStopped {
		t.Error("restart policy lost: the server would not come back after a host reboot")
	}
	if spec.Labels["anvil.slug"] != "ashenfall" {
		// Ownership labels are what every later destroy and rebuild
		// checks; a container that loses them is orphaned.
		t.Errorf("labels = %v", spec.Labels)
	}

	// The game's whole UDP run, with protocols intact — a port rebuilt as
	// tcp is a port the game is not listening on.
	want := map[int]string{7777: "7777/udp", 7778: "7778/udp", 8811: "8811/tcp"}
	if len(spec.Ports) != len(want) {
		t.Fatalf("ports = %v, want %v (the empty HostPort is not reproducible and must be dropped)", spec.Ports, want)
	}
	for host, container := range want {
		if spec.Ports[host] != container {
			t.Errorf("port %d = %q, want %q", host, spec.Ports[host], container)
		}
	}

	// bridge is what a container gets with no networks declared, and
	// docker rejects it as an explicit endpoint; the user-defined ones
	// must come back, or the rebuilt container silently loses everything
	// it reached by service name.
	if len(spec.Networks) != 2 || spec.Networks[0] != "anvil-net" || spec.Networks[1] != "wildskeeper-net" {
		t.Errorf("networks = %v, want the two user-defined ones, sorted, without bridge", spec.Networks)
	}
}

// A container on the default bridge alone has no networks to re-declare —
// and an empty list must stay empty rather than becoming a NetworkingConfig
// that names nothing.
func TestInspectSpecOnTheDefaultBridgeDeclaresNoNetworks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"Name":"/x","Config":{"Image":"i"},"HostConfig":{},"NetworkSettings":{"Networks":{"bridge":{}}}}`))
	}))
	t.Cleanup(srv.Close)
	c, _ := New(srv.URL)

	spec, err := c.InspectSpec(context.Background(), "x")
	if err != nil {
		t.Fatalf("InspectSpec: %v", err)
	}
	if len(spec.Networks) != 0 {
		t.Errorf("networks = %v, want none", spec.Networks)
	}
	if len(spec.Ports) != 0 {
		t.Errorf("ports = %v, want an empty map rather than nil-shaped surprises", spec.Ports)
	}
}

// The round trip is the actual promise: what InspectSpec reads has to be
// what ContainerCreate sends. Asserting the two halves separately leaves
// room for a field that survives the read and never reaches the payload —
// which is how a rebuilt container quietly loses its network.
func TestRecreateRoundTripPutsTheNetworksAndPortsBack(t *testing.T) {
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/json") {
			w.Write([]byte(`{
			  "Name":"/wkagent-ashenfall",
			  "Config":{"Image":"ghcr.io/safwyls/wkagent:latest","User":"568:568","Env":["A=b"],
			            "Labels":{"anvil.managed":"true"}},
			  "HostConfig":{"Binds":["/data/ashenfall:/dragonwilds"],
			                "RestartPolicy":{"Name":"unless-stopped"},
			                "PortBindings":{"7777/udp":[{"HostPort":"7777"}],"8811/tcp":[{"HostPort":"8811"}]}},
			  "NetworkSettings":{"Networks":{"bridge":{},"wildskeeper-net":{}}}}`))
			return
		}
		json.NewDecoder(r.Body).Decode(&payload)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"Id":"rebuilt"}`))
	}))
	t.Cleanup(srv.Close)
	c, _ := New(srv.URL)
	ctx := context.Background()

	spec, err := c.InspectSpec(ctx, "wkagent-ashenfall")
	if err != nil {
		t.Fatal(err)
	}
	spec.Image = "ghcr.io/safwyls/wkagent:beta" // the one field recreate changes
	if _, err := c.ContainerCreate(ctx, *spec); err != nil {
		t.Fatalf("ContainerCreate: %v", err)
	}

	if payload["Image"] != "ghcr.io/safwyls/wkagent:beta" || payload["User"] != "568:568" {
		t.Errorf("image/user = %v %v", payload["Image"], payload["User"])
	}
	networking, ok := payload["NetworkingConfig"].(map[string]any)
	if !ok {
		t.Fatalf("no NetworkingConfig — the rebuilt container is cut off from wildskeeper-net: %v", payload)
	}
	endpoints, _ := networking["EndpointsConfig"].(map[string]any)
	if _, ok := endpoints["wildskeeper-net"]; !ok {
		t.Errorf("endpoints = %v, want wildskeeper-net", endpoints)
	}
	if _, ok := endpoints["bridge"]; ok {
		t.Error("bridge was re-declared; docker rejects it as an explicit endpoint")
	}
	hc, _ := payload["HostConfig"].(map[string]any)
	bindings, _ := hc["PortBindings"].(map[string]any)
	if len(bindings) != 2 || bindings["7777/udp"] == nil || bindings["8811/tcp"] == nil {
		t.Errorf("port bindings = %v", bindings)
	}
	if fmt.Sprint(hc["Binds"]) != "[/data/ashenfall:/dragonwilds]" {
		t.Errorf("binds = %v", hc["Binds"])
	}
	if hc["RestartPolicy"].(map[string]any)["Name"] != "unless-stopped" {
		t.Errorf("restart policy = %v", hc["RestartPolicy"])
	}
}
