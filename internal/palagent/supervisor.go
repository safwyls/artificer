package palagent

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/safwyls/palcon/internal/palconfig"
)

// supervisor runs PalServer as a child process — supervisor mode's core
// (docs/sidecar-agent.md phase 3). It owns what docker's restart policy
// and exit codes provided in companion mode: graceful stop (SIGTERM to
// the process group, then SIGKILL after the grace period), crash
// auto-restart with backoff, and the game's output in a ring buffer.
//
// Desired state ("should the game be running?") persists in the install
// volume, so an agent container recreated mid-flight resumes what the
// operator last asked for — docker's unless-stopped semantics, one level
// down.

const (
	// gameLogTail is how many output lines the supervisor retains; the
	// dashboard's log viewer asks for slices of this.
	gameLogTail = 2000
	// backoffCeiling caps crash-restart delays.
	backoffCeiling = 5 * time.Minute
	// stableRuntime is how long the game must stay up before the crash
	// counter resets.
	stableRuntime = 10 * time.Minute
)

var errJobInFlight = errors.New("a job is running — wait for it to finish before starting the game")

// GameStatus is the API view of the supervised process.
type GameStatus struct {
	// State is stopped | running | crashed. "crashed" means the last exit
	// was unclean and the supervisor is between restart attempts.
	State     string    `json:"state"`
	PID       int       `json:"pid,omitempty"`
	StartedAt time.Time `json:"startedAt,omitzero"`
	// Restarts counts automatic crash-restarts since the agent booted.
	Restarts     int       `json:"restarts"`
	LastExitCode *int      `json:"lastExitCode,omitempty"`
	LastExitAt   time.Time `json:"lastExitAt,omitzero"`
}

type supervisor struct {
	installDir    string
	command       string
	args          []string
	adminPassword string
	grace         time.Duration
	// backoffFloor is the first crash-restart delay (doubles per failure);
	// tests shrink it.
	backoffFloor time.Duration
	logger       *slog.Logger
	// jobsBusy reports whether a SteamCMD job is running — the game must
	// not start mid-update.
	jobsBusy func() bool

	mu        sync.Mutex
	cmd       *exec.Cmd
	done      chan struct{} // closed when the current process exits
	state     string
	startedAt time.Time
	stopping  bool
	desired   string // running | stopped, persisted
	restarts  int
	failures  int // consecutive unclean exits, for backoff
	lastExit  *exitInfo
	log       []string
}

type exitInfo struct {
	code int
	at   time.Time
}

func newSupervisor(cfg Config, jobsBusy func() bool) *supervisor {
	args := cfg.GameArgs
	if len(args) == 0 {
		// The flags every serious dedicated-server setup passes.
		args = []string{"-useperfthreads", "-NoAsyncLoadingThread", "-UseMultithreadForDS"}
	}
	grace := cfg.StopGrace
	if grace <= 0 {
		grace = 30 * time.Second
	}
	backoff := cfg.RestartBackoffFloor
	if backoff <= 0 {
		backoff = 5 * time.Second
	}
	command := cfg.GameCommand
	if command == "" {
		command = "./PalServer.sh"
	}
	return &supervisor{
		installDir:    cfg.InstallDir,
		command:       command,
		args:          args,
		adminPassword: cfg.AdminPassword,
		grace:         grace,
		backoffFloor: backoff,
		logger:       cfg.Logger,
		jobsBusy:     jobsBusy,
		state:        "stopped",
		desired:      "stopped",
	}
}

// desiredPath is where the operator's intent survives agent recreation.
func (s *supervisor) desiredPath() string {
	return filepath.Join(s.installDir, ".palagent", "desired")
}

func (s *supervisor) loadDesired(fallback string) string {
	data, err := os.ReadFile(s.desiredPath())
	if err != nil {
		return fallback
	}
	if v := strings.TrimSpace(string(data)); v == "running" || v == "stopped" {
		return v
	}
	return fallback
}

func (s *supervisor) persistDesired(v string) {
	s.desired = v
	if err := os.MkdirAll(filepath.Dir(s.desiredPath()), 0o755); err == nil {
		_ = os.WriteFile(s.desiredPath(), []byte(v+"\n"), 0o644)
	}
}

// Installed reports whether the game files exist yet.
func (s *supervisor) Installed() bool {
	_, err := os.Stat(filepath.Join(s.installDir, s.command))
	return err == nil
}

// Running reports whether the game process is currently alive.
func (s *supervisor) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == "running"
}

// Start launches the game (idempotent while running). It refuses during a
// SteamCMD job: starting a server whose files are being rewritten is how
// installs corrupt.
func (s *supervisor) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startLocked()
}

func (s *supervisor) startLocked() error {
	if s.state == "running" {
		s.persistDesired("running")
		return nil
	}
	if s.jobsBusy != nil && s.jobsBusy() {
		return errJobInFlight
	}
	if !s.Installed() {
		return fmt.Errorf("game is not installed under %s — run an update first", s.installDir)
	}
	s.prepareRuntime()

	cmd := exec.Command(filepath.Join(s.installDir, s.command), s.args...)
	cmd.Dir = s.installDir
	setProcessGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		cmd.Stderr = cmd.Stdout
		err = cmd.Start()
	}
	if err != nil {
		return fmt.Errorf("starting game: %w", err)
	}

	s.cmd = cmd
	s.done = make(chan struct{})
	s.state = "running"
	s.startedAt = time.Now().UTC()
	s.persistDesired("running")
	s.logger.Info("game started", "pid", cmd.Process.Pid)

	go s.wait(cmd, stdout, s.done)
	return nil
}

// pump mirrors game output into the ring buffer and the agent's own
// stdout — in supervisor mode the agent container's log IS the game log.
func (s *supervisor) pump(r interface{ Read([]byte) (int, error) }) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(ansiEscape.ReplaceAllString(scanner.Text(), ""), "\r")
		fmt.Fprintln(os.Stdout, line)
		s.mu.Lock()
		s.log = append(s.log, line)
		if len(s.log) > gameLogTail {
			s.log = s.log[len(s.log)-gameLogTail:]
		}
		s.mu.Unlock()
	}
}

// wait watches one process generation to its exit and applies the restart
// policy. Output is drained before Wait — Wait closes the pipe, and the
// game's last lines (usually the interesting ones) must not be lost to
// that race.
func (s *supervisor) wait(cmd *exec.Cmd, stdout interface{ Read([]byte) (int, error) }, done chan struct{}) {
	s.pump(stdout)
	err := cmd.Wait()
	close(done)

	code := 0
	if err != nil {
		code = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		}
	}

	s.mu.Lock()
	uptime := time.Since(s.startedAt)
	s.lastExit = &exitInfo{code: code, at: time.Now().UTC()}
	clean := code == 0
	if clean {
		s.state = "stopped"
		s.failures = 0
	} else {
		s.state = "crashed"
		if uptime >= stableRuntime {
			s.failures = 0
		}
		s.failures++
	}
	stopping, desired := s.stopping, s.desired
	failures := s.failures
	s.cmd = nil
	s.mu.Unlock()

	s.logger.Info("game exited", "code", code, "uptime", uptime.Round(time.Second), "desired", desired)
	if stopping || desired != "running" {
		return
	}

	// Restart policy: any exit while desired=running comes back — that's
	// what makes palcon's in-game shutdown double as a restart, exactly
	// like docker's unless-stopped did in companion mode. Unclean exits
	// back off so a boot-loop can't spin the CPU.
	delay := time.Duration(0)
	if !clean {
		delay = min(s.backoffFloor<<max(failures-1, 0), backoffCeiling)
		s.logger.Warn("game crashed; restarting", "attempt", failures, "delay", delay)
	}
	time.Sleep(delay)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.desired != "running" || s.state == "running" || s.stopping {
		return
	}
	s.restarts++
	if err := s.startLocked(); err != nil {
		s.logger.Error("restart failed", "error", err)
	}
}

// Stop asks the game to exit (SIGTERM to the process group — PalServer.sh
// wraps the real binary, so signaling only the script leaves the game
// running) and kills it after the grace period. Persists desired=stopped
// first so a concurrent crash-restart can't resurrect it.
func (s *supervisor) Stop() error {
	s.mu.Lock()
	s.persistDesired("stopped")
	if s.state != "running" || s.cmd == nil {
		s.state = "stopped"
		s.mu.Unlock()
		return nil
	}
	s.stopping = true
	pid := s.cmd.Process.Pid
	done := s.done
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.stopping = false
		s.state = "stopped"
		s.mu.Unlock()
	}()

	signalGroup(pid, false)
	select {
	case <-done:
		s.logger.Info("game stopped gracefully")
		return nil
	case <-time.After(s.grace):
	}

	s.logger.Warn("game ignored SIGTERM; killing", "grace", s.grace)
	signalGroup(pid, true)
	select {
	case <-done:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("game process did not die after SIGKILL")
	}
}

func (s *supervisor) Restart() error {
	if err := s.Stop(); err != nil {
		return err
	}
	return s.Start()
}

func (s *supervisor) Status() GameStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := GameStatus{State: s.state, Restarts: s.restarts}
	if s.state == "running" && s.cmd != nil {
		st.PID = s.cmd.Process.Pid
		st.StartedAt = s.startedAt
	}
	if s.lastExit != nil {
		code := s.lastExit.code
		st.LastExitCode = &code
		st.LastExitAt = s.lastExit.at
	}
	return st
}

func (s *supervisor) Logs(tail int) []string {
	if tail < 1 {
		tail = 200
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tail > len(s.log) {
		tail = len(s.log)
	}
	return append([]string(nil), s.log[len(s.log)-tail:]...)
}

// prepareRuntime covers what server images do before first launch: seed
// PalWorldSettings.ini from the game's shipped defaults, enforce the
// management interfaces, and put steamclient.so where the game looks for
// it.
func (s *supervisor) prepareRuntime() {
	iniDir := filepath.Join(s.installDir, "Pal", "Saved", "Config", "LinuxServer")
	ini := filepath.Join(iniDir, "PalWorldSettings.ini")
	if info, err := os.Stat(ini); err != nil || info.Size() == 0 {
		def := filepath.Join(s.installDir, "DefaultPalWorldSettings.ini")
		if data, err := os.ReadFile(def); err == nil {
			if os.MkdirAll(iniDir, 0o755) == nil {
				if os.WriteFile(ini, data, 0o644) == nil {
					s.logger.Info("seeded PalWorldSettings.ini from defaults")
				}
			}
		}
	}

	// Palworld ships with REST and RCON disabled — a supervised server
	// left that way runs fine but is deaf to the dashboard. With an
	// admin password configured, manageability is enforced every start.
	if s.adminPassword != "" {
		err := palconfig.Write(ini, map[string]string{
			"AdminPassword":  s.adminPassword,
			"RCONEnabled":    "True",
			"RESTAPIEnabled": "True",
		})
		if err != nil {
			s.logger.Warn("could not enforce management settings; the dashboard may not reach this server", "error", err)
		} else {
			s.logger.Info("enforced RCON/REST enabled with the configured admin password")
		}
	}

	// The game loads ~/.steam/sdk64/steamclient.so; SteamCMD keeps its
	// copy wherever it bootstrapped. Best-effort: absence only matters to
	// Steam-networking features, and the game logs it loudly itself.
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	sdk := filepath.Join(home, ".steam", "sdk64", "steamclient.so")
	if _, err := os.Stat(sdk); err == nil {
		return
	}
	candidates, _ := filepath.Glob(filepath.Join(home, "*", "share", "Steam", "steamcmd", "linux64", "steamclient.so"))
	more, _ := filepath.Glob(filepath.Join(home, ".local", "share", "Steam", "steamcmd", "linux64", "steamclient.so"))
	candidates = append(candidates, more...)
	candidates = append(candidates, "/usr/lib/steamcmd/linux64/steamclient.so")
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			if os.MkdirAll(filepath.Dir(sdk), 0o755) == nil && os.Symlink(c, sdk) == nil {
				s.logger.Info("linked steamclient.so", "from", c)
			}
			return
		}
	}
}
