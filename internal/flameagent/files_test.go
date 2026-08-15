package flameagent_test

import (
	"archive/tar"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// seedWorld lays a save + config under the agent's install dir, mirroring
// the dedicated server's real layout.
func seedWorld(t *testing.T, install string) {
	t.Helper()
	world := filepath.Join(install, "RSDragonwilds", "Saved", "SaveGames")
	for _, d := range []string{filepath.Join(world, "Players"), filepath.Join(world, "backup"), filepath.Join(install, "RSDragonwilds", "Saved", "Config", "LinuxServer")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(world, "Level.sav"):           "level-bytes",
		filepath.Join(world, "LevelMeta.sav"):       "meta",
		filepath.Join(world, "Players", "1111.sav"): "p1",
		filepath.Join(world, "stray.txt"):           "not a save",
		filepath.Join(world, "backup", "old.sav"):   "the game's own backup",
		filepath.Join(install, "RSDragonwilds", "Saved", "Config", "LinuxServer", "DedicatedServer.ini"): "[/Script/Dominion.DedicatedServerSettings]\nServerName=Grimwood\nAdminPassword=old\n",
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
	if files["Level.sav"] != "level-bytes" || files["LevelMeta.sav"] != "meta" || files["Players/1111.sav"] != "p1" {
		t.Errorf("bundle contents wrong: %v", files)
	}
	if _, ok := files["stray.txt"]; ok {
		t.Error("bundle includes non-.sav file")
	}
	if _, ok := files["backup/old.sav"]; ok {
		t.Error("bundle includes the game's own backup folder")
	}

	// Headers carry each file's real mtime: the console's mirror restores
	// them, so its save cache and "last written" reporting see when the
	// game saved, not when the sync ran.
	onDisk, err := os.Stat(filepath.Join(install, "RSDragonwilds", "Saved", "SaveGames", "Level.sav"))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(body))
	for {
		hdr, err := tr.Next()
		if err != nil {
			t.Fatal("Level.sav not found in bundle headers")
		}
		if hdr.Name == "Level.sav" {
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
	world := filepath.Join(install, "RSDragonwilds", "Saved", "SaveGames")
	if err := os.WriteFile(filepath.Join(world, "Level.sav"), []byte("level-bytes-v2"), 0o644); err != nil {
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

func TestAgentConfigRoundTrip(t *testing.T) {
	srv, install := newTestAgent(t, "exit 0")
	seedWorld(t, install)
	iniPath := filepath.Join(install, "RSDragonwilds", "Saved", "Config", "LinuxServer", "DedicatedServer.ini")

	resp, body := rawGet(t, srv.URL, "/v1/files/config", "")
	if resp.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("ServerName=Grimwood")) {
		t.Fatalf("get config: %d %q", resp.StatusCode, body)
	}

	newIni := "[/Script/Dominion.DedicatedServerSettings]\nServerName=Renamed\n"
	req, _ := http.NewRequest("PUT", srv.URL+"/v1/files/config", bytes.NewBufferString(newIni))
	req.Header.Set("Authorization", "Bearer "+testToken)
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusNoContent {
		t.Fatalf("put config: got %d", putResp.StatusCode)
	}
	onDisk, err := os.ReadFile(iniPath)
	if err != nil || string(onDisk) != newIni {
		t.Errorf("config on disk = %q, %v", onDisk, err)
	}
}

func TestAgentConfigPutRefusesWhenMissing(t *testing.T) {
	srv, _ := newTestAgent(t, "exit 0") // no seedWorld: no ini exists
	req, _ := http.NewRequest("PUT", srv.URL+"/v1/files/config", bytes.NewBufferString("x"))
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("put with no ini: got %d, want 404 (never conjure a config)", resp.StatusCode)
	}
}
