package companion

// The daemon-shaped half of the local server (server.go's
// ServerOptions): the bearer check cmd/companiond runs behind, the
// liveness route a shell polls before it has a token, the SSE stream
// that replaces the in-process Subscribe() for an out-of-process shell,
// and the raise route that answers with a reason where there is no
// window. The last guard here is the one that matters most: the browser
// build passes no options and must get exactly the surface it had.

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func daemonApp(t *testing.T) *App {
	t.Helper()
	return NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
}

// With a token set, everything but liveness needs it.
func TestTokenIsRequiredExceptForHealthz(t *testing.T) {
	h := daemonApp(t).RoutesWithOptions(ServerOptions{Token: "sekrit"})

	cases := []struct {
		name   string
		header string
		path   string
		want   int
	}{
		{"state without a token", "", "/api/state", http.StatusUnauthorized},
		{"state with the wrong token", "Bearer nope", "/api/state", http.StatusUnauthorized},
		{"state with a bare token, not a bearer", "sekrit", "/api/state", http.StatusUnauthorized},
		{"state with the token", "Bearer sekrit", "/api/state", http.StatusOK},
		{"healthz without a token", "", "/healthz", http.StatusOK},
		{"healthz with the token", "Bearer sekrit", "/healthz", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("%s %s = %d, want %d (%s)", req.Method, tc.path, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// A rejection says what is wrong in the same JSON shape as every other
// error, and never leaks the token back.
func TestUnauthorizedBodyIsTheOrdinaryErrorShape(t *testing.T) {
	h := daemonApp(t).RoutesWithOptions(ServerOptions{Token: "sekrit"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/state", nil))

	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if out.OK || out.Error == "" {
		t.Errorf("401 body = %+v", out)
	}
	if strings.Contains(rec.Body.String(), "sekrit") {
		t.Errorf("the rejection echoed the token back: %s", rec.Body.String())
	}
}

// Liveness says only that this process is answering.
func TestHealthz(t *testing.T) {
	defer stubVersion("v1.2.3")()
	rec := httptest.NewRecorder()
	daemonApp(t).Routes().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz: %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if out["ok"] != true || out["version"] != "v1.2.3" {
		t.Errorf("healthz body = %v", out)
	}
	// No custody, no config: this is the one route answered before the
	// caller has proven anything.
	for _, leak := range []string{"token", "serverUrl", "links"} {
		if _, ok := out[leak]; ok {
			t.Errorf("healthz reports %q", leak)
		}
	}
}

// The stream opens with a frame that proves it is live, then carries one
// nudge per change — the HTTP face of Subscribe().
func TestEventsStreamsAReadyFrameThenChanges(t *testing.T) {
	a := daemonApp(t)
	srv := httptest.NewServer(a.Routes())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type = %q", ct)
	}

	lines := bufio.NewScanner(resp.Body)
	nextEvent := func(t *testing.T) string {
		t.Helper()
		for lines.Scan() {
			if ev, ok := strings.CutPrefix(lines.Text(), "event: "); ok {
				return ev
			}
		}
		t.Fatalf("stream ended before the next event: %v", lines.Err())
		return ""
	}

	if ev := nextEvent(t); ev != "ready" {
		t.Fatalf("first event = %q, want ready", ev)
	}
	a.changed()
	if ev := nextEvent(t); ev != "changed" {
		t.Fatalf("second event = %q, want changed", ev)
	}
}

// No window here: the route says so, and says where the ability lives,
// rather than answering 404 or a silent ok.
func TestRaiseWithoutAWindowIsA501WithAReason(t *testing.T) {
	rec := httptest.NewRecorder()
	daemonApp(t).Routes().ServeHTTP(rec, httptest.NewRequest("POST", "/api/raise", nil))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("raise: %d", rec.Code)
	}
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if out.OK || out.Error == "" {
		t.Errorf("raise body = %+v", out)
	}
}

// A shell that owns a window gets the raise it asked for, and a raise
// that fails is reported rather than swallowed.
func TestRaiseCallsTheShell(t *testing.T) {
	called := 0
	h := daemonApp(t).RoutesWithOptions(ServerOptions{Raise: func() error { called++; return nil }})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/raise", nil))
	if rec.Code != http.StatusOK || called != 1 {
		t.Fatalf("raise: %d, called %d (%s)", rec.Code, called, rec.Body.String())
	}

	h = daemonApp(t).RoutesWithOptions(ServerOptions{Raise: func() error { return errNoWindow }})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/raise", nil))
	if !strings.Contains(rec.Body.String(), errNoWindow.Error()) {
		t.Errorf("a failed raise was swallowed: %s", rec.Body.String())
	}
}

var errNoWindow = errStr("the window is gone")

type errStr string

func (e errStr) Error() string { return string(e) }

// The browser build passes no options, and its surface is unchanged by
// the daemon work: no auth, and the page's own routes still answer.
func TestBrowserShapeNeedsNoToken(t *testing.T) {
	h := daemonApp(t).Routes()
	for _, path := range []string{"/api/state", "/healthz", "/api/savehints"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("%s asks the browser build for a token", path)
		}
	}
}

// The snapshot's queue is empty, not absent: the renderer renders
// "nothing queued" from it rather than inventing a manifest (sync.go).
func TestSnapshotQueueIsEmptyNotNull(t *testing.T) {
	snap, err := json.Marshal(daemonApp(t).Snapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out struct {
		Sync struct {
			Queue *[]QueuedWork `json:"queue"`
		} `json:"sync"`
	}
	if err := json.Unmarshal(snap, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Sync.Queue == nil {
		t.Fatalf("sync.queue is null in %s", snap)
	}
	if len(*out.Sync.Queue) != 0 {
		t.Errorf("sync.queue = %v, want empty (nothing produces queued work yet)", *out.Sync.Queue)
	}
}
