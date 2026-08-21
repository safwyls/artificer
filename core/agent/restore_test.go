package agent_test

// The restore pair: HEAD states the bundle ETag, PUT replaces the save
// behind If-Match — the one deliberate widening of the fixed-verb file
// surface, so its gates get their own file.

import (
	"archive/tar"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func bundleBytes(t *testing.T, files map[string]string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), ModTime: time.Now(), Format: tar.FormatPAX}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func saveRequest(t *testing.T, srv *httptest.Server, method string, body io.Reader, ifMatch string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+"/v1/files/save", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestRestoreSave(t *testing.T) {
	srv, install := newTestAgent(t, "exit 0")
	saveDir := filepath.Join(install, "savegame")
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(saveDir, "World.sav"), []byte("the-original-world-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	head := saveRequest(t, srv, http.MethodHead, nil, "")
	if head.StatusCode != http.StatusNoContent || head.Header.Get("ETag") == "" {
		t.Fatalf("HEAD: got %d, etag %q", head.StatusCode, head.Header.Get("ETag"))
	}
	etag := head.Header.Get("ETag")

	// No precondition, no restore: the caller must state what it
	// believes it is replacing.
	if resp := saveRequest(t, srv, http.MethodPut, bundleBytes(t, map[string]string{"World.sav": "x"}), ""); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("PUT without If-Match: got %d, want 400", resp.StatusCode)
	}
	// A stale belief is a 412 carrying the current truth.
	resp := saveRequest(t, srv, http.MethodPut, bundleBytes(t, map[string]string{"World.sav": "x"}), `"stale"`)
	if resp.StatusCode != http.StatusPreconditionFailed || resp.Header.Get("ETag") != etag {
		t.Errorf("stale PUT: got %d, etag %q, want 412 with current etag", resp.StatusCode, resp.Header.Get("ETag"))
	}
	// Garbage that matches the precondition is still refused, and the
	// live save is untouched.
	if resp := saveRequest(t, srv, http.MethodPut, bytes.NewReader([]byte("not a tar")), etag); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("garbage PUT: got %d, want 400", resp.StatusCode)
	}
	if data, _ := os.ReadFile(filepath.Join(saveDir, "World.sav")); string(data) != "the-original-world-content" {
		t.Fatalf("refused restore touched the live save: %q", data)
	}

	// The real restore: swapped in atomically, the old save kept as .bak.
	resp = saveRequest(t, srv, http.MethodPut, bundleBytes(t, map[string]string{"World.sav": "the-restored-world-content"}), etag)
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("restore: got %d (%s)", resp.StatusCode, body)
	}
	if data, _ := os.ReadFile(filepath.Join(saveDir, "World.sav")); string(data) != "the-restored-world-content" {
		t.Errorf("restore did not land: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(saveDir+".bak", "World.sav")); string(data) != "the-original-world-content" {
		t.Errorf(".bak does not hold the replaced save: %q", data)
	}
	if newETag := resp.Header.Get("ETag"); newETag == "" || newETag == etag {
		t.Errorf("restore answered etag %q, want a fresh one", newETag)
	}
}

func TestRestoreIntoFreshInstall(t *testing.T) {
	srv, install := newTestAgent(t, "exit 0")
	// No savegame dir at all: HEAD answers the empty set's ETag, and a
	// restore against it creates the game's declared location.
	head := saveRequest(t, srv, http.MethodHead, nil, "")
	etag := head.Header.Get("ETag")
	if head.StatusCode != http.StatusNoContent || etag == "" {
		t.Fatalf("HEAD on empty install: got %d, etag %q", head.StatusCode, etag)
	}
	resp := saveRequest(t, srv, http.MethodPut, bundleBytes(t, map[string]string{"World.sav": "the-seeded-world"}), etag)
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("seed restore: got %d (%s)", resp.StatusCode, body)
	}
	if data, _ := os.ReadFile(filepath.Join(install, "savegame", "World.sav")); string(data) != "the-seeded-world" {
		t.Errorf("seed restore did not land: %q", data)
	}
}
