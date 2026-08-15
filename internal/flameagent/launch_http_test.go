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

	"github.com/safwyls/flametender/internal/flameagent"
)

// authed issues a request with the agent's bearer token.
func authed(t *testing.T, method, url string, body any) (*http.Response, []byte) {
	t.Helper()
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

type launchStatus struct {
	Profile        string   `json:"profile"`
	Installed      bool     `json:"installed"`
	Available      []string `json:"available"`
	PendingRestart bool     `json:"pendingRestart"`
	ConfigPath     string   `json:"configPath"`
}

func healthLaunch(t *testing.T, srv string) launchStatus {
	t.Helper()
	_, body := authed(t, http.MethodGet, srv+"/v1/health", nil)
	var h struct {
		Launch launchStatus `json:"launch"`
	}
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("health: %v (%s)", err, body)
	}
	return h.Launch
}

// newWineAgent builds a supervisor agent with no explicit game command, so
// the default wine profile is in charge. The game is never started unless
// the test also plants an exe and a wine stub.
func newWineAgent(t *testing.T, cfg flameagent.Config) (*httptest.Server, string) {
	t.Helper()
	install := t.TempDir()
	cfg.Token = testToken
	cfg.InstallDir = install
	cfg.SteamCmd = "/bin/true"
	cfg.Version = "test"
	cfg.Mode = "supervisor"
	cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	agent, err := flameagent.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		req, _ := http.NewRequest("POST", srv.URL+"/v1/power/stop", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	})
	return srv, install
}

// Enshrouded has exactly one build, so an agent with no explicit command
// must default to the wine profile — and report the one selectable profile
// and the json's fixed location, which is what the console renders.
func TestDefaultProfileIsWine(t *testing.T) {
	srv, _ := newWineAgent(t, flameagent.Config{})

	got := healthLaunch(t, srv.URL)
	if got.Profile != flameagent.ProfileWine {
		t.Fatalf("profile = %q, want wine by default", got.Profile)
	}
	if len(got.Available) != 1 || got.Available[0] != flameagent.ProfileWine {
		t.Errorf("available = %v, want just wine", got.Available)
	}
	if got.ConfigPath != "enshrouded_server.json" {
		t.Errorf("config path = %q, want the json at the install root", got.ConfigPath)
	}
}

func TestUnknownLaunchProfileIsRefused(t *testing.T) {
	srv, _ := newWineAgent(t, flameagent.Config{})
	resp, _ := authed(t, http.MethodPut, srv.URL+"/v1/launch", map[string]string{"profile": "proton"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unknown profile", resp.StatusCode)
	}
}

// An explicit game command is the operator having already said exactly
// what to run. The agent reports it as the custom profile and refuses to
// let the console silently swap it for a known one.
func TestExplicitCommandReportsCustomAndRefusesSelection(t *testing.T) {
	srv, _, _ := newSupervisorAgent(t, steadyGame)

	got := healthLaunch(t, srv.URL)
	if got.Profile != flameagent.ProfileCustom {
		t.Fatalf("profile = %q, want custom for an explicit command", got.Profile)
	}
	if len(got.Available) != 0 {
		t.Errorf("available = %v, want none — the console must not offer to replace the operator's command", got.Available)
	}

	resp, body := authed(t, http.MethodPut, srv.URL+"/v1/launch", map[string]string{"profile": flameagent.ProfileWine})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("selecting over an explicit command: %d %s, want 400", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "explicit game command") {
		t.Errorf("refusal should name the explicit command as the reason: %s", body)
	}
}

// The launch path end to end, without needing real Wine: a stub on PATH
// stands in for the wine binary and records what it was handed. This is the
// only place the profile's environment is proven to actually reach the
// process — every piece of it fails silently in production if it doesn't.
func TestWineProfileLaunchesAndSeedsTheConfig(t *testing.T) {
	// A stub "wine64" that dumps its arguments and environment, then
	// behaves like the game (running until the supervisor's INT).
	binDir := t.TempDir()
	stub := "#!/bin/sh\n" +
		"{ printf '%s\\n' \"argv: $*\" \"WINEPREFIX=$WINEPREFIX\" \"PWD=$(pwd)\"; } > " +
		filepath.Join(binDir, "launched") + "\n" +
		"trap 'exit 0' INT TERM\nwhile true; do sleep 0.05; done\n"
	if err := os.WriteFile(filepath.Join(binDir, "wine64"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	// Before the agent is built: the wine binary is resolved when the
	// profile is assembled, not at exec time.
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	srv, install := newWineAgent(t, flameagent.Config{
		ServerName:    "Grimwood Bastion",
		AdminPassword: "hunter2-but-longer",
		JoinPassword:  "friends-only",
		GamePort:      25637,
		StopGrace:     500 * time.Millisecond,
	})

	// The Windows build's file, so the profile counts as installed.
	if err := os.WriteFile(filepath.Join(install, "enshrouded_server.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := healthLaunch(t, srv.URL); !got.Installed {
		t.Fatal("the wine profile should be installed once the exe exists")
	}
	if resp, body := authed(t, http.MethodPost, srv.URL+"/v1/power/start", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("start under wine: %d %s", resp.StatusCode, body)
	}

	var out []byte
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		// The stub's redirect creates the file before printf fills it, so
		// existence alone can hand back a torn read — wait for the last
		// line it writes.
		if data, err := os.ReadFile(filepath.Join(binDir, "launched")); err == nil && strings.Contains(string(data), "PWD=") {
			out = data
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if out == nil {
		t.Fatal("the wine stub was never executed — the profile's command did not reach exec")
	}
	got := string(out)
	if !strings.Contains(got, "enshrouded_server.exe") {
		t.Errorf("wine was not handed the server exe: %s", got)
	}
	// The port is deliberately not a launch argument — the json owns it,
	// and a stray flag would be silently ignored at best.
	argv := ""
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "argv: ") {
			argv = line
		}
	}
	if strings.Contains(strings.ToLower(argv), "port") {
		t.Errorf("argv carries a port flag; the json owns the port: %s", argv)
	}
	// The prefix lives in the install volume so it survives agent
	// recreation, and the working directory is the install root, where the
	// server's relative ./savegame and ./logs belong.
	if !strings.Contains(got, "WINEPREFIX="+filepath.Join(install, ".wineprefix")) {
		t.Errorf("WINEPREFIX is not the install-volume prefix: %s", got)
	}
	realInstall, err := filepath.EvalSymlinks(install)
	if err != nil {
		t.Fatal(err)
	}
	pwd := ""
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if rest, ok := strings.CutPrefix(line, "PWD="); ok {
			pwd = rest
		}
	}
	if pwd != install && pwd != realInstall {
		t.Errorf("PWD = %q, want the install root %q", pwd, install)
	}

	// Starting also seeded the config: name, port and both role passwords
	// were in place before the game's first boot could write open defaults.
	data, err := os.ReadFile(filepath.Join(install, "enshrouded_server.json"))
	if err != nil {
		t.Fatalf("no enshrouded_server.json was seeded: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("seeded config does not parse: %v\n%s", err, data)
	}
	if doc["name"] != "Grimwood Bastion" {
		t.Errorf("seeded name = %v", doc["name"])
	}
	if port, _ := doc["queryPort"].(float64); port != 25637 {
		t.Errorf("seeded queryPort = %v, want 25637", doc["queryPort"])
	}
	if got := groupPassword(t, doc, true); got != "hunter2-but-longer" {
		t.Errorf("seeded admin password = %q", got)
	}
	if got := groupPassword(t, doc, false); got != "friends-only" {
		t.Errorf("seeded join password = %q", got)
	}

	if resp, body := authed(t, http.MethodPost, srv.URL+"/v1/power/stop", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("stop: %d %s", resp.StatusCode, body)
	}
}
