package dockerctl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	if err := c.ImagePull(ctx, "ghcr.io/safwyls/palagent:beta"); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if (*calls)[0] != "POST /images/create?fromImage=ghcr.io%2Fsafwyls%2Fpalagent&tag=beta" {
		t.Errorf("pull call = %q", (*calls)[0])
	}

	id, err := c.ContainerCreate(ctx, ContainerSpec{
		Name: "palagent-test", Image: "ghcr.io/safwyls/palagent:beta",
		User: "568:568", Env: []string{"A=b"},
		Binds:                []string{"/data/test:/palworld"},
		Ports:                map[int]string{9211: "8211/udp", 9811: "8811"},
		RestartUnlessStopped: true,
	})
	if err != nil || id != "abc123" {
		t.Fatalf("create: id=%q err=%v", id, err)
	}
	p := *payload
	if p["User"] != "568:568" || p["Image"] != "ghcr.io/safwyls/palagent:beta" {
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
