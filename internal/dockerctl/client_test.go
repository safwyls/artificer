package dockerctl_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/safwyls/flamekeeper/internal/dockerctl"
)

// dockerSpy records the requests a client makes and answers with canned
// bodies, so the wire format the daemon actually sees is asserted.
type dockerSpy struct {
	mu       sync.Mutex
	requests []string
	status   int
	body     string
}

func newSpy(t *testing.T, body string) (*dockerSpy, *dockerctl.Client) {
	t.Helper()
	spy := &dockerSpy{status: http.StatusOK, body: body}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spy.mu.Lock()
		spy.requests = append(spy.requests, r.Method+" "+r.URL.RequestURI())
		status, body := spy.status, spy.body
		spy.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	client, err := dockerctl.New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return spy, client
}

func (d *dockerSpy) last() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.requests) == 0 {
		return ""
	}
	return d.requests[len(d.requests)-1]
}

func (d *dockerSpy) set(status int, body string) {
	d.mu.Lock()
	d.status, d.body = status, body
	d.mu.Unlock()
}

func TestRestartSendsTheStopGrace(t *testing.T) {
	spy, client := newSpy(t, "")
	spy.set(http.StatusNoContent, "")

	if err := client.Restart(context.Background(), "flameagent-main"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	// The grace period is what gives Palworld time to flush its world
	// instead of taking a SIGKILL.
	if got := spy.last(); !strings.Contains(got, "/containers/flameagent-main/restart?t=") {
		t.Errorf("restart request = %q", got)
	}
}

func TestProxyPermissionErrorsExplainThemselves(t *testing.T) {
	spy, client := newSpy(t, "")
	spy.set(http.StatusForbidden, "")

	err := client.Restart(context.Background(), "x")
	if err == nil {
		t.Fatal("a 403 reported success")
	}
	if !strings.Contains(err.Error(), "POST=1") {
		t.Errorf("a proxy 403 should name the missing permission: %v", err)
	}
}
