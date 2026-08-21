package api_test

// The backups page's restore button, end to end: a snapshot the runner
// wrote is converted to the agent's tar bundle and PUT with the ETag the
// agent just quoted. Driven against an agent that enforces the same
// If-Match precondition the real one does.

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/artificer/core/store"
)

// fakeRestoreAgent speaks the restore pair with the real agent's
// semantics: HEAD quotes the current bundle ETag, PUT refuses anything
// that does not name it.
type fakeRestoreAgent struct {
	mu sync.Mutex
	// etag is the current bundle fingerprint; bumpOnHead rotates it the
	// moment it is quoted, which is the race the precondition exists for
	// — a game that wrote between the caller's look and its PUT.
	etag       string
	bumpOnHead bool
	ifMatch    string
	restored   []byte
}

func (f *fakeRestoreAgent) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"agent": "fakeagent", "mode": "supervisor", "apiVersion": 3,
			"game": map[string]any{"state": "stopped"},
		})
	})
	mux.HandleFunc("HEAD /v1/files/save", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("ETag", f.etag)
		if f.bumpOnHead {
			f.etag = `"the-game-wrote-again"`
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /v1/files/save", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		want := f.etag
		f.ifMatch = r.Header.Get("If-Match")
		f.mu.Unlock()
		if r.Header.Get("If-Match") != want {
			w.Header().Set("ETag", want)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPreconditionFailed)
			json.NewEncoder(w).Encode(map[string]string{"error": "the save changed since you last looked"})
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.restored = data
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func TestRestoreBackupOntoAgent(t *testing.T) {
	fake := &fakeRestoreAgent{etag: `"live-save-etag"`}
	agentSrv := httptest.NewServer(fake.handler())
	defer agentSrv.Close()

	app, admin := newTestAppWithAdmin(t)
	save := fakeSaveDir(t)
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "main", Host: "10.0.0.5", RCONPort: 25575, RESTPort: 8212, UseREST: true, Enabled: true,
		SavePath: save, AgentURL: agentSrv.URL, AgentToken: agentToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/servers/" + itoa(id)

	// A world with a nested level chunk, so the bundle has to carry
	// relative paths, not just top-level names.
	if err := os.MkdirAll(filepath.Join(save, "Levels"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(save, "Levels", "chunk-0.sav"), bytes.Repeat([]byte("c"), 64), 0o644); err != nil {
		t.Fatal(err)
	}

	if rec := app.do(t, "POST", base+"/backups/run", nil, admin); rec.Code != http.StatusAccepted {
		t.Fatalf("run: got %d (body %s)", rec.Code, rec.Body)
	}
	var name string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := app.do(t, "GET", base+"/backups", nil, admin)
		var res struct {
			Snapshots []struct {
				Name  string `json:"name"`
				Bytes int64  `json:"bytes"`
			} `json:"snapshots"`
		}
		json.Unmarshal(rec.Body.Bytes(), &res)
		if len(res.Snapshots) == 1 && res.Snapshots[0].Bytes > 0 {
			name = res.Snapshots[0].Name
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if name == "" {
		t.Fatal("snapshot never appeared")
	}

	rec := app.do(t, "POST", base+"/backups/"+name+"/restore", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore: got %d (body %s)", rec.Code, rec.Body)
	}

	fake.mu.Lock()
	restored, ifMatch := fake.restored, fake.ifMatch
	fake.mu.Unlock()
	if ifMatch != `"live-save-etag"` {
		t.Errorf("If-Match = %q, want the ETag the agent quoted", ifMatch)
	}
	// The bundle must be a well-formed tar carrying every archived file.
	got := map[string]int64{}
	tr := tar.NewReader(bytes.NewReader(restored))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("bundle is not a well-formed tar: %v", err)
		}
		got[hdr.Name] = hdr.Size
	}
	if _, ok := got["Ashenfall.sav"]; !ok {
		t.Errorf("bundle missing the world file; got %v", got)
	}
	if _, ok := got["Levels/chunk-0.sav"]; !ok {
		t.Errorf("bundle missing the nested level chunk; got %v", got)
	}
}

// An agent built before the restore pair still serves GET /v1/files/save,
// so chi answers HEAD and PUT on it with 405 — not the 404 the client's
// old-agent mapping watched for. The console must name that cause: this
// is the failure the wildskeeper restore button hit on a live server the
// day the verb shipped, and all it could say was "agent returned 405:".
func TestRestoreBackupAgainstAgentWithoutTheVerb(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			json.NewEncoder(w).Encode(map[string]any{
				"agent": "oldagent", "mode": "supervisor", "apiVersion": 3,
				"game": map[string]any{"state": "stopped"},
			})
		})
		r.Get("/files/save", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("tar")) })
	})
	agentSrv := httptest.NewServer(r)
	defer agentSrv.Close()

	app, admin := newTestAppWithAdmin(t)
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "main", Host: "10.0.0.5", RCONPort: 25575, RESTPort: 8212, UseREST: true, Enabled: true,
		SavePath: fakeSaveDir(t), AgentURL: agentSrv.URL, AgentToken: agentToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/servers/" + itoa(id)

	if rec := app.do(t, "POST", base+"/backups/run", nil, admin); rec.Code != http.StatusAccepted {
		t.Fatalf("run: got %d (body %s)", rec.Code, rec.Body)
	}
	var name string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := app.do(t, "GET", base+"/backups", nil, admin)
		var res struct {
			Snapshots []struct {
				Name  string `json:"name"`
				Bytes int64  `json:"bytes"`
			} `json:"snapshots"`
		}
		json.Unmarshal(rec.Body.Bytes(), &res)
		if len(res.Snapshots) == 1 && res.Snapshots[0].Bytes > 0 {
			name = res.Snapshots[0].Name
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if name == "" {
		t.Fatal("snapshot never appeared")
	}

	rec := app.do(t, "POST", base+"/backups/"+name+"/restore", nil, admin)
	if rec.Code != http.StatusConflict {
		t.Errorf("restore against an agent without the verb: got %d, want 409", rec.Code)
	}
	msg := decodeMap(t, rec)["error"].(string)
	if !strings.Contains(msg, "update its agent image") {
		t.Errorf("error = %q; it must name the fix, not the status code", msg)
	}
}

// A save that moved under the caller is the precondition doing its job,
// not the agent breaking — the operator can look again, so it must not
// arrive as a gateway failure.
func TestRestoreBackupWhenTheSaveMoved(t *testing.T) {
	fake := &fakeRestoreAgent{etag: `"live-save-etag"`}
	agentSrv := httptest.NewServer(fake.handler())
	defer agentSrv.Close()

	app, admin := newTestAppWithAdmin(t)
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "main", Host: "10.0.0.5", RCONPort: 25575, RESTPort: 8212, UseREST: true, Enabled: true,
		SavePath: fakeSaveDir(t), AgentURL: agentSrv.URL, AgentToken: agentToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/servers/" + itoa(id)

	if rec := app.do(t, "POST", base+"/backups/run", nil, admin); rec.Code != http.StatusAccepted {
		t.Fatalf("run: got %d (body %s)", rec.Code, rec.Body)
	}
	var name string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := app.do(t, "GET", base+"/backups", nil, admin)
		var res struct {
			Snapshots []struct {
				Name  string `json:"name"`
				Bytes int64  `json:"bytes"`
			} `json:"snapshots"`
		}
		json.Unmarshal(rec.Body.Bytes(), &res)
		if len(res.Snapshots) == 1 && res.Snapshots[0].Bytes > 0 {
			name = res.Snapshots[0].Name
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if name == "" {
		t.Fatal("snapshot never appeared")
	}

	// The world is written again between the ETag and the PUT.
	fake.mu.Lock()
	fake.bumpOnHead = true
	fake.mu.Unlock()

	rec := app.do(t, "POST", base+"/backups/"+name+"/restore", nil, admin)
	if rec.Code != http.StatusConflict {
		t.Errorf("restore over a moved save: got %d, want 409", rec.Code)
	}
	if msg := decodeMap(t, rec)["error"].(string); !strings.Contains(msg, "changed since") {
		t.Errorf("error = %q, want the precondition's own explanation", msg)
	}
}
