package api_test

// The dedicated server as a holder: give restores the head onto the
// server's agent and leaves the server holding the world; take commits
// the agent's save as the new head and returns the hold. Driven against
// a fake agent that speaks the restore pair.

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/safwyls/artificer/core/store"
)

// fakeSyncAgent is the minimum agent for the server-holder flows: health
// (supervisor, stopped), the bundle ETag, the restore PUT, and the save
// download.
type fakeSyncAgent struct {
	mu       sync.Mutex
	etag     string
	restored []byte
	saveData string
}

func (f *fakeSyncAgent) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"agent": "fakeagent", "mode": "supervisor", "apiVersion": 3,
			"game": map[string]any{"state": "stopped"},
		})
	})
	mux.HandleFunc("HEAD /v1/files/save", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", f.etag)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /v1/files/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Match") != f.etag {
			w.Header().Set("ETag", f.etag)
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		data, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.restored = data
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /v1/files/save", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("ETag", f.etag)
		tw := tar.NewWriter(w)
		tw.WriteHeader(&tar.Header{Name: "World.sav", Mode: 0o644, Size: int64(len(f.saveData)), ModTime: time.Now(), Format: tar.FormatPAX})
		tw.Write([]byte(f.saveData))
		tw.Close()
	})
	return mux
}

func TestSyncServerGiveAndTake(t *testing.T) {
	fake := &fakeSyncAgent{etag: `"fake-etag-1"`, saveData: "the-server-played-world-0123456789"}
	agentSrv := httptest.NewServer(fake.handler())
	defer agentSrv.Close()

	app, admin := newSyncApp(t)
	serverID, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "grimwood", Host: "127.0.0.1", Enabled: true,
		AgentURL: agentSrv.URL, AgentToken: agentToken,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// A world linked to the server, seeded with a head.
	if rec := app.do(t, "POST", "/api/sync/worlds", map[string]string{"name": "midgard"}, admin); rec.Code != http.StatusCreated {
		t.Fatalf("create world: %d", rec.Code)
	}
	if rec := app.do(t, "PUT", "/api/sync/worlds/1", map[string]any{
		"name": "midgard", "serverId": serverID, "leaseHours": 48,
		"maxBytes": 1 << 24, "keepVersions": 5, "checkpoints": true, "webhookUrl": "",
	}, admin); rec.Code != http.StatusOK {
		t.Fatalf("link server: %d (body %s)", rec.Code, rec.Body)
	}
	if rec := app.doTar(t, "/api/sync/worlds/1/import", admin); rec.Code != http.StatusOK {
		t.Fatalf("import head: %d (body %s)", rec.Code, rec.Body)
	}

	// Give: the head lands on the agent and the server holds the world.
	rec := app.do(t, "POST", "/api/sync/worlds/1/server/give", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("give: got %d (body %s)", rec.Code, rec.Body)
	}
	fake.mu.Lock()
	restored := fake.restored
	fake.mu.Unlock()
	if len(restored) == 0 || !bytes.Contains(restored, []byte("World.sav")) {
		t.Error("give did not deliver the head bundle to the agent")
	}
	rec = app.do(t, "GET", "/api/sync/worlds/1", nil, admin)
	detail := decodeMap(t, rec)
	holder := detail["status"].(map[string]any)["holder"]
	if holder == nil || holder.(map[string]any)["serverHeld"] != true {
		t.Fatalf("holder after give = %v, want server-held", holder)
	}

	// A player checkout while the server holds it is a 409 like any
	// other hold.
	app.createUser(t, admin, "alice", "alicepassword", "user", []string{store.PermSync})
	alice := app.login(t, "alice", "alicepassword")
	if rec := app.do(t, "POST", "/api/sync/worlds/1/checkout", nil, alice); rec.Code != http.StatusConflict {
		t.Errorf("checkout during server hold: got %d, want 409", rec.Code)
	}

	// Take: the agent's save becomes the new head, the hold returns.
	rec = app.do(t, "POST", "/api/sync/worlds/1/server/take", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("take: got %d (body %s)", rec.Code, rec.Body)
	}
	var out struct {
		Version struct {
			ID       int64 `json:"id"`
			Conflict bool  `json:"conflict"`
		} `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode take: %v", err)
	}
	if out.Version.Conflict {
		t.Error("take flagged as conflict; a server hold based on the head fast-forwards")
	}
	rec = app.do(t, "GET", "/api/sync/worlds/1", nil, admin)
	detail = decodeMap(t, rec)
	status := detail["status"].(map[string]any)
	if status["holder"] != nil {
		t.Error("world still held after take")
	}
	head := status["head"].(map[string]any)
	if int64(head["id"].(float64)) != out.Version.ID {
		t.Errorf("head = %v, want the taken version %d", head["id"], out.Version.ID)
	}
}
