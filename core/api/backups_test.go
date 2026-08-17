package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/safwyls/artificer/core/store"
)

// fakeSaveDir writes a minimal save with a valid container magic, enough
// for the backup runner to accept it.
func fakeSaveDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	level := append(make([]byte, 8), []byte("GVAS")...)
	level = append(level, make([]byte, 32)...)
	if err := os.WriteFile(filepath.Join(dir, "Ashenfall.sav"), level, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBackupEndpointsAdminOnly(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	id := createTestServer(t, app)
	base := "/api/servers/" + itoa(id)
	app.createUser(t, admin, "peon", "peonpassword1", "user", []string{store.PermPower})
	peon := app.login(t, "peon", "peonpassword1")

	reqs := []struct{ method, path string }{
		{"GET", base + "/backups"},
		{"PUT", base + "/backups/settings"},
		{"POST", base + "/backups/run"},
		{"GET", base + "/backups/20260101-000000.zip/download"},
		{"DELETE", base + "/backups/20260101-000000.zip"},
	}
	for _, rq := range reqs {
		if rec := app.do(t, rq.method, rq.path, map[string]any{}, peon); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as non-admin: got %d, want 403", rq.method, rq.path, rec.Code)
		}
	}
}

func TestBackupLifecycle(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	save := fakeSaveDir(t)
	id, err := app.store.CreateServer(context.Background(), &store.Server{
		Name: "main", Host: "10.0.0.5", RCONPort: 25575, RESTPort: 8212, UseREST: true, Enabled: true,
		SavePath: save,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/servers/" + itoa(id)

	// Settings validation, then a real update.
	if rec := app.do(t, "PUT", base+"/backups/settings", map[string]int{"intervalHours": 999, "keep": 14}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("bad interval: got %d, want 400", rec.Code)
	}
	if rec := app.do(t, "PUT", base+"/backups/settings", map[string]int{"intervalHours": 12, "keep": 0}, admin); rec.Code != http.StatusBadRequest {
		t.Errorf("bad keep: got %d, want 400", rec.Code)
	}
	if rec := app.do(t, "PUT", base+"/backups/settings", map[string]int{"intervalHours": 12, "keep": 7}, admin); rec.Code != http.StatusOK {
		t.Fatalf("settings: got %d (body %s)", rec.Code, rec.Body)
	}

	if rec := app.do(t, "POST", base+"/backups/run", nil, admin); rec.Code != http.StatusAccepted {
		t.Fatalf("run: got %d (body %s)", rec.Code, rec.Body)
	}

	// The snapshot lands asynchronously; a tiny save takes milliseconds.
	var name string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := app.do(t, "GET", base+"/backups", nil, admin)
		if rec.Code != http.StatusOK {
			t.Fatalf("list: got %d", rec.Code)
		}
		var res struct {
			IntervalHours int  `json:"intervalHours"`
			Keep          int  `json:"keep"`
			Available     bool `json:"available"`
			Snapshots     []struct {
				Name  string `json:"name"`
				Bytes int64  `json:"bytes"`
			} `json:"snapshots"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		if !res.Available || res.IntervalHours != 12 || res.Keep != 7 {
			t.Fatalf("list = %+v, want available with saved settings", res)
		}
		if len(res.Snapshots) == 1 && res.Snapshots[0].Bytes > 0 {
			name = res.Snapshots[0].Name
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if name == "" {
		t.Fatal("snapshot never appeared")
	}

	rec := app.do(t, "GET", base+"/backups/"+name+"/download", nil, admin)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("download: code %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec.Body.Len() < 22 { // an empty zip's central directory alone is 22 bytes
		t.Fatalf("download body suspiciously small: %d bytes", rec.Body.Len())
	}

	if rec := app.do(t, "DELETE", base+"/backups/"+name, nil, admin); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d", rec.Code)
	}
	if rec := app.do(t, "GET", base+"/backups/"+name+"/download", nil, admin); rec.Code != http.StatusNotFound {
		t.Errorf("download after delete: got %d, want 404", rec.Code)
	}
}
