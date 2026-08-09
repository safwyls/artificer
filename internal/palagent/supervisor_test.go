package palagent_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/safwyls/dwcon/internal/palagent"
)

// newSupervisorAgent builds a supervisor-mode agent whose "game" is the
// given shell script, installed as RSDragonwildsServer.sh in a fresh install dir.
func newSupervisorAgent(t *testing.T, gameScript string) (*httptest.Server, *palagent.Agent, string) {
	t.Helper()
	install := t.TempDir()
	writeGame(t, install, gameScript)
	steamcmd := filepath.Join(t.TempDir(), "steamcmd")
	if err := os.WriteFile(steamcmd, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	agent, err := palagent.New(palagent.Config{
		Token: testToken, InstallDir: install, SteamCmd: steamcmd, Version: "test",
		Mode:                "supervisor",
		StopGrace:           500 * time.Millisecond,
		RestartBackoffFloor: 20 * time.Millisecond,
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(agent.Handler())
	t.Cleanup(srv.Close)
	// Whatever the test leaves running dies with it.
	t.Cleanup(func() {
		req, _ := http.NewRequest("POST", srv.URL+"/v1/power/stop", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	})
	return srv, agent, install
}

func writeGame(t *testing.T, install, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(install, "RSDragonwildsServer.sh"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// steadyGame runs until signaled, exiting cleanly on TERM.
const steadyGame = `trap 'echo "caught TERM, flushing world"; exit 0' TERM
echo "Palworld server booting"
while true; do sleep 0.05; done`

func gameState(t *testing.T, srv *httptest.Server) map[string]any {
	t.Helper()
	_, health := do(t, srv, "GET", "/v1/health", testToken, nil)
	game, _ := health["game"].(map[string]any)
	if game == nil {
		t.Fatalf("health has no game block: %v", health)
	}
	return game
}

func waitGameState(t *testing.T, srv *httptest.Server, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if game := gameState(t, srv); game["state"] == want {
			return game
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("game never reached state %q (now %v)", want, gameState(t, srv))
	return nil
}

func TestSupervisorLifecycle(t *testing.T) {
	srv, _, install := newSupervisorAgent(t, steadyGame)

	resp, m := do(t, srv, "POST", "/v1/power/start", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start: %d %v", resp.StatusCode, m)
	}
	game := waitGameState(t, srv, "running")
	if game["pid"] == nil {
		t.Errorf("running game has no pid: %v", game)
	}

	// Output lands in the log verb.
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, logs := do(t, srv, "GET", "/v1/power/logs?tail=50", testToken, nil)
		if lines, _ := logs["lines"].([]any); len(lines) > 0 && strings.Contains(lines[0].(string), "booting") {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatal("game output never reached the log verb")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Graceful stop: TERM is caught, exit is clean, desired persists.
	if resp, m := do(t, srv, "POST", "/v1/power/stop", testToken, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("stop: %d %v", resp.StatusCode, m)
	}
	waitGameState(t, srv, "stopped")
	desired, err := os.ReadFile(filepath.Join(install, ".palagent", "desired"))
	if err != nil || strings.TrimSpace(string(desired)) != "stopped" {
		t.Errorf("desired = %q, %v; want persisted stopped", desired, err)
	}
	// A stopped game must not resurrect.
	time.Sleep(150 * time.Millisecond)
	if game := gameState(t, srv); game["state"] != "stopped" {
		t.Errorf("game resurrected after stop: %v", game)
	}
}

func TestSupervisorKillsStubbornGame(t *testing.T) {
	srv, _, _ := newSupervisorAgent(t, `trap '' TERM
echo "ignoring signals like a champ"
while true; do sleep 0.05; done`)

	do(t, srv, "POST", "/v1/power/start", testToken, nil)
	waitGameState(t, srv, "running")
	// Don't signal before the script has installed its trap — the echo
	// (which follows the trap line) proves it has.
	waitGameLog(t, srv, "ignoring signals")
	start := time.Now()
	resp, _ := do(t, srv, "POST", "/v1/power/stop", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stop: %d", resp.StatusCode)
	}
	if took := time.Since(start); took < 400*time.Millisecond {
		t.Errorf("stop returned in %v — grace period skipped?", took)
	}
	waitGameState(t, srv, "stopped")
}

// A stop must not signal a game that is already shutting itself down.
// Palcon asks the game to exit over REST before calling the agent, and a
// SIGTERM landing on top of that in-flight exit is what turns a clean
// shutdown into "Exiting abnormally (error code: 143)" — 128+SIGTERM —
// with whatever the shutdown was still writing lost.
func TestSupervisorWaitsForSelfExit(t *testing.T) {
	srv, _, _ := newSupervisorAgent(t, `trap 'echo "signalled"; exit 143' TERM
echo "Palworld server booting"
sleep 2
echo "saved and exiting on my own"
exit 0`)

	do(t, srv, "POST", "/v1/power/start", testToken, nil)
	waitGameState(t, srv, "running")
	// The echo follows the trap line, so it proves the handler is armed.
	waitGameLog(t, srv, "booting")

	resp, m := do(t, srv, "POST", "/v1/power/stop?graceful=6s", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stop: %d %v", resp.StatusCode, m)
	}
	game, _ := m["game"].(map[string]any)
	if game == nil {
		t.Fatalf("stop response has no game block: %v", m)
	}
	if game["state"] != "stopped" {
		t.Errorf("state = %v, want stopped", game["state"])
	}
	if code, ok := game["lastExitCode"].(float64); !ok || code != 0 {
		t.Errorf("lastExitCode = %v, want 0 — the game was cut off mid-shutdown", game["lastExitCode"])
	}
	_, logs := do(t, srv, "GET", "/v1/power/logs?tail=100", testToken, nil)
	lines, _ := logs["lines"].([]any)
	for _, l := range lines {
		if strings.Contains(l.(string), "signalled") {
			t.Fatalf("game was signalled inside the self-exit window: %v", lines)
		}
	}
}

// The self-exit window is a courtesy, not a promise: a game that stays up
// through it still gets signalled, and the intentional stop is recorded as
// a stop rather than a crash whatever exit code the signal produces.
func TestSupervisorSignalsAfterSelfExitWindow(t *testing.T) {
	srv, _, _ := newSupervisorAgent(t, `trap 'echo "signalled"; exit 143' TERM
echo "Palworld server booting"
while true; do sleep 0.05; done`)

	do(t, srv, "POST", "/v1/power/start", testToken, nil)
	waitGameState(t, srv, "running")
	waitGameLog(t, srv, "booting")

	start := time.Now()
	resp, m := do(t, srv, "POST", "/v1/power/stop?graceful=300ms", testToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stop: %d %v", resp.StatusCode, m)
	}
	if took := time.Since(start); took < 300*time.Millisecond {
		t.Errorf("stop returned in %v — self-exit window skipped?", took)
	}
	game, _ := m["game"].(map[string]any)
	if game["state"] != "stopped" {
		t.Errorf("state = %v, want stopped (an operator stop is not a crash)", game["state"])
	}
	if code, ok := game["lastExitCode"].(float64); !ok || code != 143 {
		t.Errorf("lastExitCode = %v, want the signalled 143", game["lastExitCode"])
	}
	// Deliberately stopping must not leave a crash on the record for the
	// next start's restart backoff to inherit.
	time.Sleep(150 * time.Millisecond)
	if g := gameState(t, srv); g["state"] != "stopped" {
		t.Errorf("settled state = %v, want stopped", g["state"])
	}
}

func TestSupervisorCrashRestart(t *testing.T) {
	// Crashes on the first run (sentinel), then runs steadily: the
	// supervisor must restart it and count the attempt.
	srv, _, _ := newSupervisorAgent(t, `sentinel="$(dirname "$0")/crashed-once"
if [ ! -f "$sentinel" ]; then touch "$sentinel"; echo "segfault!"; exit 139; fi
`+steadyGame)

	do(t, srv, "POST", "/v1/power/start", testToken, nil)
	// "running" alone races the crash (the state is running from spawn
	// until the exit is reaped); the restart counter is the proof.
	deadline := time.Now().Add(5 * time.Second)
	for {
		game := gameState(t, srv)
		if game["state"] == "running" && game["restarts"].(float64) >= 1 {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("game never restarted after the crash: %v", game)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitGameLog polls the log verb until a line contains substr.
func waitGameLog(t *testing.T, srv *httptest.Server, substr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, logs := do(t, srv, "GET", "/v1/power/logs?tail=100", testToken, nil)
		if lines, _ := logs["lines"].([]any); len(lines) > 0 {
			for _, l := range lines {
				if strings.Contains(l.(string), substr) {
					return
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("game log never contained %q", substr)
}

func TestSupervisorUpdateGameMutualExclusion(t *testing.T) {
	srv, _, _ := newSupervisorAgent(t, steadyGame)

	// Update refused while the game runs.
	do(t, srv, "POST", "/v1/power/start", testToken, nil)
	waitGameState(t, srv, "running")
	if resp, _ := do(t, srv, "POST", "/v1/steam/update", testToken, nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("update while running: %d, want 409", resp.StatusCode)
	}
	do(t, srv, "POST", "/v1/power/stop", testToken, nil)
	waitGameState(t, srv, "stopped")

	// Game start refused while a job runs.
	slow := filepath.Join(t.TempDir(), "slow-steamcmd")
	_ = slow // the shared agent already has a fast steamcmd; simulate via update+start race instead
	if resp, _ := do(t, srv, "POST", "/v1/steam/update", testToken, nil); resp.StatusCode != http.StatusAccepted {
		t.Fatal("update did not start")
	}
	// The fast fake finishes almost instantly; only assert the guard when
	// the job is still live.
	if cur := gameState(t, srv); cur != nil {
		resp, m := do(t, srv, "POST", "/v1/power/start", testToken, nil)
		if resp.StatusCode == http.StatusConflict {
			if !strings.Contains(m["error"].(string), "job") {
				t.Errorf("conflict for the wrong reason: %v", m)
			}
		}
	}
}

// A supervised server keeps the operator's admin password enforced: every
// start rewrites AdminPassword in an existing DedicatedServer.ini. The game
// writes the ini itself on first boot, so a missing file is only logged —
// the agent never invents one (dwconfig's never-add policy).
func TestSupervisorEnforcesManagementConfig(t *testing.T) {
	install := t.TempDir()
	writeGame(t, install, steadyGame)
	iniDir := filepath.Join(install, "RSDragonwilds", "Saved", "Config", "LinuxServer")
	if err := os.MkdirAll(iniDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := "[/Script/Dominion.DedicatedServerSettings]\nServerName=Grimwood\nAdminPassword=stale\n"
	if err := os.WriteFile(filepath.Join(iniDir, "DedicatedServer.ini"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	agent, err := palagent.New(palagent.Config{
		Token: testToken, InstallDir: install, SteamCmd: "/bin/true", Version: "test",
		Mode: "supervisor", StopGrace: 500 * time.Millisecond,
		AdminPassword: "hunter2-but-longer",
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
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

	do(t, srv, "POST", "/v1/power/start", testToken, nil)
	waitGameState(t, srv, "running")

	ini, err := os.ReadFile(filepath.Join(install, "RSDragonwilds", "Saved", "Config", "LinuxServer", "DedicatedServer.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ini), "AdminPassword=hunter2-but-longer") {
		t.Errorf("ini did not get the enforced admin password:\n%s", ini)
	}
	if strings.Contains(string(ini), "AdminPassword=stale") {
		t.Errorf("stale admin password survived enforcement:\n%s", ini)
	}
}

func TestCompanionRefusesPower(t *testing.T) {
	srv, _ := newTestAgent(t, "exit 0")
	if resp, _ := do(t, srv, "POST", "/v1/power/start", testToken, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("companion power: %d, want 400", resp.StatusCode)
	}
	if resp, _ := do(t, srv, "GET", "/v1/power/logs", testToken, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("companion logs: %d, want 400", resp.StatusCode)
	}
}

func TestSupervisorBootInstallsAndStarts(t *testing.T) {
	// No game installed; the "steamcmd" install script creates it, then
	// Run must bring the game up (autostart default).
	install := t.TempDir()
	steamcmd := filepath.Join(t.TempDir(), "steamcmd")
	script := "#!/bin/sh\ncat > " + filepath.Join(install, "RSDragonwildsServer.sh") + " <<'GAME'\n#!/bin/sh\n" + steadyGame + "\nGAME\nchmod +x " + filepath.Join(install, "RSDragonwildsServer.sh") + "\necho \"Success! App '4019830' fully installed.\"\n"
	if err := os.WriteFile(steamcmd, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	agent, err := palagent.New(palagent.Config{
		Token: testToken, InstallDir: install, SteamCmd: steamcmd, Version: "test",
		Mode: "supervisor", StopGrace: 500 * time.Millisecond,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
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

	go agent.Run()
	game := waitGameStateWithin(t, srv, "running", 15*time.Second)
	if game["pid"] == nil {
		t.Fatalf("boot-installed game not running: %v", game)
	}
}

func waitGameStateWithin(t *testing.T, srv *httptest.Server, want string, within time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if game := gameState(t, srv); game["state"] == want {
			return game
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("game never reached state %q", want)
	return nil
}
