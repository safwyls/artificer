package agent_test

import (
	"archive/tar"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// seedConfigJSON is a minimal but valid gametest.json for the
// config round-trip tests.
const seedConfigJSON = `{"name":"Grimwood","queryPort":15637,"slotCount":16}`

// seedWorld lays a save + config under the agent's install dir, mirroring
// the dedicated server's real layout: extensionless hex-named blobs plus
// the -index sidecar under savegame/, and the json at the install root.
func seedWorld(t *testing.T, install string) {
	t.Helper()
	world := filepath.Join(install, "savegame")
	if err := os.MkdirAll(filepath.Join(world, "backup"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(world, "3ad85aea"):           "world-bytes",
		filepath.Join(world, "3ad85aea-1"):         "rolling-slot",
		filepath.Join(world, "3ad85aea-index"):     `{"latest":1}`,
		filepath.Join(world, "backup", "3ad85aea"): "the game's own backup",
		filepath.Join(install, "gametest.json"):    seedConfigJSON,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func tarNames(t *testing.T, body []byte) map[string]string {
	t.Helper()
	out := map[string]string{}
	tr := tar.NewReader(bytes.NewReader(body))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(tr)
		out[hdr.Name] = string(data)
	}
}

func rawGet(t *testing.T, srv string, path, etag string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest("GET", srv+path, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

func TestAgentSaveBundle(t *testing.T) {
	srv, install := newTestAgent(t, "exit 0")
	seedWorld(t, install)

	resp, body := rawGet(t, srv.URL, "/v1/files/save", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save: got %d %s", resp.StatusCode, body)
	}
	files := tarNames(t, body)
	// Every regular file in savegame/ belongs to the world — the blobs are
	// extensionless, so there is no extension to filter on.
	if files["3ad85aea"] != "world-bytes" || files["3ad85aea-1"] != "rolling-slot" || files["3ad85aea-index"] != `{"latest":1}` {
		t.Errorf("bundle contents wrong: %v", files)
	}
	if _, ok := files["backup/3ad85aea"]; ok {
		t.Error("bundle includes the game's own backup folder")
	}

	// Headers carry each file's real mtime: the console's mirror restores
	// them, so its save cache and "last written" reporting see when the
	// game saved, not when the sync ran.
	onDisk, err := os.Stat(filepath.Join(install, "savegame", "3ad85aea"))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(body))
	for {
		hdr, err := tr.Next()
		if err != nil {
			t.Fatal("3ad85aea not found in bundle headers")
		}
		if hdr.Name == "3ad85aea" {
			if !hdr.ModTime.Equal(onDisk.ModTime()) {
				t.Errorf("bundle mtime = %v, want the file's %v", hdr.ModTime, onDisk.ModTime())
			}
			break
		}
	}

	// Unchanged: 304 on the returned etag, no body.
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on bundle response")
	}
	resp2, _ := rawGet(t, srv.URL, "/v1/files/save", etag)
	if resp2.StatusCode != http.StatusNotModified {
		t.Errorf("unchanged bundle: got %d, want 304", resp2.StatusCode)
	}

	// A rewritten save changes the etag.
	world := filepath.Join(install, "savegame")
	if err := os.WriteFile(filepath.Join(world, "3ad85aea"), []byte("world-bytes-v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp3, _ := rawGet(t, srv.URL, "/v1/files/save", etag)
	if resp3.StatusCode != http.StatusOK {
		t.Errorf("changed bundle: got %d, want 200", resp3.StatusCode)
	}
}

func TestAgentSaveBundleNoWorld(t *testing.T) {
	srv, _ := newTestAgent(t, "exit 0")
	resp, _ := rawGet(t, srv.URL, "/v1/files/save", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("no world: got %d, want 404", resp.StatusCode)
	}
}

func putConfig(t *testing.T, srv, body string) int {
	t.Helper()
	req, _ := http.NewRequest("PUT", srv+"/v1/files/config", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestAgentConfigRoundTrip(t *testing.T) {
	srv, install := newTestAgent(t, "exit 0")
	seedWorld(t, install)
	cfgPath := filepath.Join(install, "gametest.json")

	resp, body := rawGet(t, srv.URL, "/v1/files/config", "")
	if resp.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"name":"Grimwood"`)) {
		t.Fatalf("get config: %d %q", resp.StatusCode, body)
	}

	newCfg := `{"name":"Renamed","queryPort":15637,"slotCount":16}`
	if code := putConfig(t, srv.URL, newCfg); code != http.StatusNoContent {
		t.Fatalf("put config: got %d", code)
	}
	onDisk, err := os.ReadFile(cfgPath)
	if err != nil || string(onDisk) != newCfg {
		t.Errorf("config on disk = %q, %v", onDisk, err)
	}
}

// A malformed upload must never reach disk: a json the game can't parse is
// regenerated with open, password-less defaults on boot — a far worse
// failure than a rejected edit.
func TestAgentConfigPutValidates(t *testing.T) {
	srv, install := newTestAgent(t, "exit 0")
	seedWorld(t, install)

	if code := putConfig(t, srv.URL, `{"name":"Grimwood",`); code != http.StatusBadRequest {
		t.Errorf("put truncated json: got %d, want 400", code)
	}
	if code := putConfig(t, srv.URL, `{"name":"Grimwood","queryPort":123456}`); code != http.StatusBadRequest {
		t.Errorf("put out-of-range queryPort: got %d, want 400", code)
	}
	// The seeded file is untouched by either refusal.
	onDisk, err := os.ReadFile(filepath.Join(install, "gametest.json"))
	if err != nil || string(onDisk) != seedConfigJSON {
		t.Errorf("config on disk = %q, %v; a rejected put must not write", onDisk, err)
	}
}

func TestAgentConfigPutRefusesWhenMissing(t *testing.T) {
	srv, _ := newTestAgent(t, "exit 0") // no seedWorld: no config exists
	if code := putConfig(t, srv.URL, `{"name":"x"}`); code != http.StatusNotFound {
		t.Errorf("put with no config: got %d, want 404 (never conjure a config)", code)
	}
}
