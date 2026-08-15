package flameagent_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/safwyls/flamekeeper/internal/flameagent"
)

const testToken = "test-token-0123456789abcdef"

// newTestAgent builds an agent over a fresh install dir with cache
// contents, using the given script (a shell body) as its fake steamcmd.
func newTestAgent(t *testing.T, script string) (*httptest.Server, string) {
	t.Helper()
	install := t.TempDir()
	for _, d := range []string{"steamapps/downloading", "steam/packages"} {
		if err := os.MkdirAll(filepath.Join(install, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(install, "steamapps", "appmanifest_2278520.acf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	steamcmd := filepath.Join(t.TempDir(), "steamcmd")
	if err := os.WriteFile(steamcmd, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	agent, err := flameagent.New(flameagent.Config{
		Token:      testToken,
		InstallDir: install,
		SteamCmd:   steamcmd,
		Version:    "test",
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	return srv, install
}

func do(t *testing.T, srv *httptest.Server, method, path, token string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, srv.URL+path, buf)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp, m
}

func TestAgentRejectsShortToken(t *testing.T) {
	_, err := flameagent.New(flameagent.Config{Token: "short", InstallDir: t.TempDir()})
	if err == nil {
		t.Fatal("agent accepted a sub-minimum token")
	}
}

// Provisioner mode is gone on purpose — placing containers is Ilmari's job
// — so an agent configured with the retired mode must refuse to start
// rather than silently run as something else.
func TestAgentRejectsRetiredProvisionerMode(t *testing.T) {
	_, err := flameagent.New(flameagent.Config{
		Token: testToken, InstallDir: t.TempDir(), Mode: "provisioner",
	})
	if err == nil || !strings.Contains(err.Error(), "companion or supervisor") {
		t.Fatalf("provisioner mode: err = %v, want a refusal naming the valid modes", err)
	}
}

func TestAgentAuth(t *testing.T) {
	srv, _ := newTestAgent(t, "exit 0")

	for _, token := range []string{"", "wrong-token-0123456789abcdef"} {
		if resp, _ := do(t, srv, "GET", "/v1/health", token, nil); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("token %q: got %d, want 401", token, resp.StatusCode)
		}
	}

	// healthz is the unauthenticated container healthcheck.
	if resp, _ := do(t, srv, "GET", "/healthz", "", nil); resp.StatusCode != http.StatusNoContent {
		t.Errorf("healthz: got %d, want 204", resp.StatusCode)
	}

	resp, health := do(t, srv, "GET", "/v1/health", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: got %d", resp.StatusCode)
	}
	if health["apiVersion"] != float64(flameagent.APIVersion) || health["mode"] != "companion" || health["installDirOk"] != true {
		t.Errorf("health = %v", health)
	}
}

func TestAgentClearCache(t *testing.T) {
	srv, install := newTestAgent(t, "exit 0")

	resp, m := do(t, srv, "POST", "/v1/steam/clear-cache", testToken, nil)
	if resp.StatusCode != http.StatusOK || m["removed"] != float64(2) {
		t.Fatalf("clear: got %d %v, want 200 removed=2", resp.StatusCode, m)
	}
	entries, err := os.ReadDir(filepath.Join(install, "steamapps"))
	if err != nil || len(entries) != 0 {
		t.Errorf("steamapps not emptied: %v %d", err, len(entries))
	}
}

// waitForJob polls the job endpoint until it leaves the running state.
func waitForJob(t *testing.T, srv *httptest.Server, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, m := do(t, srv, "GET", "/v1/jobs/"+id, testToken, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("job: got %d", resp.StatusCode)
		}
		job := m["job"].(map[string]any)
		if job["state"] != "running" {
			return job
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job never finished")
	return nil
}

func TestAgentUpdateJob(t *testing.T) {
	srv, _ := newTestAgent(t, `echo "Update state (0x61) downloading"; echo "Success! App '2278520' fully installed."`)

	resp, m := do(t, srv, "POST", "/v1/steam/update", testToken, map[string]bool{"validate": true})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("update: got %d %v", resp.StatusCode, m)
	}
	id := m["job"].(map[string]any)["id"].(string)

	job := waitForJob(t, srv, id)
	if job["state"] != "done" {
		t.Fatalf("job = %v, want done", job)
	}
	log := job["log"].([]any)
	if len(log) != 2 {
		t.Errorf("log = %v, want the script's 2 lines", log)
	}

	// The finished job is discoverable via health (flamekeeper-restart path).
	_, health := do(t, srv, "GET", "/v1/health", testToken, nil)
	if health["job"].(map[string]any)["id"] != id {
		t.Error("health does not report the last job")
	}
}

func TestAgentUpdateFailures(t *testing.T) {
	// Zero exit but a SteamCMD error line: the classic 0x602 lie.
	srv, _ := newTestAgent(t, `echo "Error! App '2278520' state is 0x602 after update job."; exit 0`)
	_, m := do(t, srv, "POST", "/v1/steam/update", testToken, nil)
	job := waitForJob(t, srv, m["job"].(map[string]any)["id"].(string))
	if job["state"] != "failed" || job["error"] == "" {
		t.Errorf("zero-exit steam error: job = %v, want failed with error", job)
	}

	// Non-zero exit.
	srv2, _ := newTestAgent(t, "exit 8")
	_, m2 := do(t, srv2, "POST", "/v1/steam/update", testToken, nil)
	if job := waitForJob(t, srv2, m2["job"].(map[string]any)["id"].(string)); job["state"] != "failed" {
		t.Errorf("nonzero exit: job = %v, want failed", job)
	}
}

// The "Missing configuration" failure a cold SteamCMD bootstrap throws
// once must be retried automatically — and succeed when the second run
// does. The script fails on its first invocation only, tracked by a
// sentinel file beside it.
func TestAgentUpdateRetriesColdBootstrapFlake(t *testing.T) {
	srv, _ := newTestAgent(t, `sentinel="$(dirname "$0")/ran-once"
if [ ! -f "$sentinel" ]; then
  touch "$sentinel"
  echo "ERROR! Failed to install app '2278520' (Missing configuration)"
  exit 0
fi
echo "Success! App '2278520' fully installed."`)

	_, m := do(t, srv, "POST", "/v1/steam/update", testToken, nil)
	job := waitForJob(t, srv, m["job"].(map[string]any)["id"].(string))
	if job["state"] != "done" {
		t.Fatalf("job = %v, want done after retry", job)
	}
	joined := ""
	for _, l := range job["log"].([]any) {
		joined += l.(string) + "\n"
	}
	if !strings.Contains(joined, "flameagent: retrying (2/2)") {
		t.Errorf("log missing retry marker:\n%s", joined)
	}
}

// Uppercase ERROR! lines (SteamCMD uses both spellings) must fail the job
// even on exit 0, and only one retry is attempted for a persistent error.
func TestAgentUpdatePersistentErrorFailsOnce(t *testing.T) {
	srv, _ := newTestAgent(t, `echo "ERROR! Failed to install app '2278520' (Missing configuration)"; exit 0`)
	_, m := do(t, srv, "POST", "/v1/steam/update", testToken, nil)
	job := waitForJob(t, srv, m["job"].(map[string]any)["id"].(string))
	if job["state"] != "failed" || !strings.Contains(job["error"].(string), "Missing configuration") {
		t.Errorf("job = %v, want failed with the ERROR! line", job)
	}
}

// ANSI color codes in SteamCMD output must not reach the stored log.
func TestAgentJobLogStripsANSI(t *testing.T) {
	srv, _ := newTestAgent(t, `printf 'Loading Steam API...\033[0mOK\n'`)
	_, m := do(t, srv, "POST", "/v1/steam/update", testToken, nil)
	job := waitForJob(t, srv, m["job"].(map[string]any)["id"].(string))
	if log := job["log"].([]any); log[0] != "Loading Steam API...OK" {
		t.Errorf("log[0] = %q, want ANSI stripped", log[0])
	}
}

func TestAgentJobLogCapped(t *testing.T) {
	// 500 lines of output; the job must retain only the newest 400.
	srv, _ := newTestAgent(t, `i=1; while [ $i -le 500 ]; do echo "line $i"; i=$((i+1)); done`)
	_, m := do(t, srv, "POST", "/v1/steam/update", testToken, nil)
	job := waitForJob(t, srv, m["job"].(map[string]any)["id"].(string))
	log := job["log"].([]any)
	if len(log) != 400 || log[len(log)-1] != "line 500" || log[0] != "line 101" {
		t.Errorf("log len=%d first=%v last=%v, want 400 lines ending at 500", len(log), log[0], log[len(log)-1])
	}
}

func TestAgentOneJobAtATime(t *testing.T) {
	srv, _ := newTestAgent(t, "sleep 2")

	if resp, _ := do(t, srv, "POST", "/v1/steam/update", testToken, nil); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("first update: got %d", resp.StatusCode)
	}
	if resp, _ := do(t, srv, "POST", "/v1/steam/update", testToken, nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("second update: got %d, want 409", resp.StatusCode)
	}
}
