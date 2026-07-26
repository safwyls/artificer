package dockerctl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewUnconfigured(t *testing.T) {
	if _, err := New(""); err != ErrNotConfigured {
		t.Errorf("New(\"\") = %v, want ErrNotConfigured", err)
	}
	if _, err := New("ftp://nope"); err == nil {
		t.Error("New(ftp://) should reject the scheme")
	}
}

// The client-side request timeout must exceed the stop grace period by a
// clear margin, or a stop that legitimately uses its full grace period
// reports failure for an action that succeeded.
func TestStopTimeoutExceedsGrace(t *testing.T) {
	if requestTimeout <= stopGrace {
		t.Errorf("requestTimeout (%v) must exceed stopGrace (%v)", requestTimeout, stopGrace)
	}
}

func newFakeEngine(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestInspect(t *testing.T) {
	c := newFakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/palworld/json" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"Name":"/palworld","State":{"Status":"exited","Running":false,"StartedAt":"2026-07-01T00:00:00Z","ExitCode":137}}`))
	})

	state, err := c.Inspect(context.Background(), "palworld")
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	// The leading slash Docker puts on names must be trimmed.
	if state.Name != "palworld" || state.Status != "exited" || state.Running || state.ExitCode != 137 {
		t.Errorf("state = %+v", state)
	}
}

// A 403 from the socket proxy nearly always means a missing grant; the
// error must say how to fix it rather than just "forbidden".
func TestForbiddenExplainsProxyGrants(t *testing.T) {
	c := newFakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	err := c.Start(context.Background(), "palworld")
	if err == nil || !strings.Contains(err.Error(), "POST=1 and CONTAINERS=1") {
		t.Errorf("want proxy-grant hint, got %v", err)
	}
}

func TestNotFound(t *testing.T) {
	c := newFakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	err := c.Start(context.Background(), "nope")
	if err == nil || !strings.Contains(err.Error(), "container not found") {
		t.Errorf("want not-found message, got %v", err)
	}
}

// 304 means "already in the requested state" — not a failure.
func TestNotModifiedIsSuccess(t *testing.T) {
	c := newFakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	})

	if err := c.Start(context.Background(), "palworld"); err != nil {
		t.Errorf("start on running container: %v", err)
	}
}

func TestStopSendsGracePeriod(t *testing.T) {
	var gotQuery string
	c := newFakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.Stop(context.Background(), "palworld"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if gotQuery != "t=30" {
		t.Errorf("stop query = %q, want t=30", gotQuery)
	}
}

func TestLogsDemuxesFramedStream(t *testing.T) {
	// Two stdout frames and a stderr frame, in Docker's 8-byte header
	// framing (stream id, 3 zero bytes, big-endian length).
	frame := func(stream byte, s string) []byte {
		b := []byte{stream, 0, 0, 0, 0, 0, 0, byte(len(s))}
		return append(b, s...)
	}
	var payload []byte
	payload = append(payload, frame(1, "[Game] server started\n")...)
	payload = append(payload, frame(2, "warning: low tps\n")...)
	payload = append(payload, frame(1, "[Game] player joined\n")...)

	c := newFakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/palworld/logs" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if q := r.URL.Query(); q.Get("stdout") != "1" || q.Get("stderr") != "1" || q.Get("tail") != "300" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		w.Write(payload)
	})

	out, err := c.Logs(context.Background(), "palworld", 300)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	want := "[Game] server started\nwarning: low tps\n[Game] player joined\n"
	if out != want {
		t.Errorf("logs = %q, want %q", out, want)
	}
}

// A TTY container streams raw bytes with no framing; they must pass
// through untouched instead of being parsed as bogus headers.
func TestLogsPassesRawStreamThrough(t *testing.T) {
	raw := "[Game] plain tty log line\nanother line\n"
	c := newFakeEngine(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(raw))
	})
	out, err := c.Logs(context.Background(), "palworld", 200)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if out != raw {
		t.Errorf("logs = %q, want raw passthrough", out)
	}
}
