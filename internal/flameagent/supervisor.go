package flameagent

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

	"github.com/safwyls/flametender/internal/games/enshrouded/esconfig"
)

// supervisor runs the game server as a child process — supervisor mode's core
// (docs/sidecar-agent.md phase 3). It owns what docker's restart policy
// and exit codes provided in companion mode: graceful stop (SIGINT to
// the process group — Enshrouded's clean shutdown signal, which also
// saves the world — then SIGKILL after the grace period), crash
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
	installDir string
	// profile is how the game is launched — which build, with what
	// environment, writing its config where. Guarded by mu because the
	// console can change it between starts.
	profile Profile
	// launch is the environment-supplied tuning a profile is rebuilt from
	// when the selection changes.
	launch      LaunchConfig
	gamePort    int
	gameCommand string
	gameArgs    []string
	// adminPassword and joinPassword are enforced into
	// enshrouded_server.json's role groups before every start — see
	// prepareRuntime. Empty means "leave the file's value alone".
	adminPassword string
	joinPassword  string
	serverName    string
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
	// runningProfile is the profile the live process was started with, so a
	// selection made while the game is up can be reported as pending rather
	// than pretended to be in effect.
	runningProfile string
	failures       int // consecutive unclean exits, for backoff
	lastExit       *exitInfo
	log            []string
}

type exitInfo struct {
	code int
	at   time.Time
}

func newSupervisor(cfg Config, jobsBusy func() bool) *supervisor {
	port := cfg.GamePort
	if port <= 0 {
		port = DefaultGamePort
	}
	grace := cfg.StopGrace
	if grace <= 0 {
		// Enshrouded writes the world on the way down; community images
		// budget 60-90s for it. Killing a mid-save server is how worlds
		// corrupt, so the default is generous.
		grace = 120 * time.Second
	}
	backoff := cfg.RestartBackoffFloor
	if backoff <= 0 {
		backoff = 5 * time.Second
	}
	s := &supervisor{
		installDir:    cfg.InstallDir,
		launch:        cfg.Launch,
		gamePort:      port,
		gameCommand:   cfg.GameCommand,
		gameArgs:      cfg.GameArgs,
		adminPassword: cfg.AdminPassword,
		joinPassword:  cfg.JoinPassword,
		serverName:    cfg.ServerName,
		grace:         grace,
		backoffFloor:  backoff,
		logger:        cfg.Logger,
		jobsBusy:      jobsBusy,
		state:         "stopped",
		desired:       "stopped",
	}
	// The selection persists in the install volume for the same reason
	// desired-state does: an agent container recreated mid-flight must come
	// back running the build the operator chose, not the default.
	s.profile = s.buildProfile(s.loadProfileName(cfg.Launch.Profile))
	return s
}

// buildProfile assembles the named profile from this supervisor's config.
// The game port is not part of the launch: Enshrouded reads queryPort
// from its json, which prepareRuntime enforces instead.
func (s *supervisor) buildProfile(name string) Profile {
	return buildProfile(name, s.launch, s.installDir, s.gameCommand, s.gameArgs)
}

// profileNamePath is where the launch selection survives agent recreation.
func (s *supervisor) profileNamePath() string {
	return filepath.Join(s.installDir, ".flameagent", "profile")
}

func (s *supervisor) loadProfileName(fallback string) string {
	if data, err := os.ReadFile(s.profileNamePath()); err == nil {
		if v := strings.TrimSpace(string(data)); validProfile(v) {
			return v
		}
	}
	return fallback
}

// SetProfile changes which build the next start launches. It deliberately
// does not restart anything: switching build is a heavier act than a
// restart (the two are installed from different depots), so the decision to
// bring the game down belongs to whoever asked.
func (s *supervisor) SetProfile(name string) (Profile, error) {
	if !validProfile(name) {
		return Profile{}, fmt.Errorf("unknown launch profile %q", name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.profile.Name == ProfileCustom {
		return s.profile, errors.New("this agent is configured with an explicit game command; unset FLAMEAGENT_GAME_CMD to choose a profile")
	}
	s.profile = s.buildProfile(name)
	if err := os.MkdirAll(filepath.Dir(s.profileNamePath()), 0o755); err == nil {
		_ = os.WriteFile(s.profileNamePath(), []byte(name+"\n"), 0o644)
	}
	s.logger.Info("launch profile selected", "profile", name, "appliesAt", "next start")
	return s.profile, nil
}

// profileChangedSinceStart reports whether the running game is a different
// build from the one now selected.
func (s *supervisor) profileChangedSinceStart() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == "running" && s.runningProfile != "" && s.runningProfile != s.profile.Name
}

// Profile is the active launch profile.
func (s *supervisor) Profile() Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profile
}

// desiredPath is where the operator's intent survives agent recreation.
func (s *supervisor) desiredPath() string {
	return filepath.Join(s.installDir, ".flameagent", "desired")
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

// Installed reports whether the *selected build's* files exist yet. The two
// builds come from different depots, so a native install is not a Wine
// install however full the directory looks.
func (s *supervisor) Installed() bool {
	s.mu.Lock()
	p := s.profile
	s.mu.Unlock()
	return p.installed(s.installDir)
}

// installedLocked is Installed for callers already holding mu.
func (s *supervisor) installedLocked() bool {
	return s.profile.installed(s.installDir)
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
	if !s.installedLocked() {
		return fmt.Errorf("the %s build is not installed under %s — run an update first", s.profile.Name, s.installDir)
	}
	s.prepareRuntime()

	cmd := exec.Command(s.profile.resolveCommand(s.installDir), s.profile.Args...)
	cmd.Dir = filepath.Join(s.installDir, s.profile.Dir)
	if len(s.profile.Env) > 0 {
		cmd.Env = append(os.Environ(), s.profile.Env...)
	}
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
	s.runningProfile = s.profile.Name
	s.startedAt = time.Now().UTC()
	s.persistDesired("running")
	s.logger.Info("game started", "pid", cmd.Process.Pid, "profile", s.profile.Name)

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
	stopping, desired := s.stopping, s.desired
	// An operator-initiated stop is never a crash, whatever the exit code.
	// A game signalled while it was already shutting itself down exits 143
	// (128+SIGTERM) — calling that a crash mislabels the dashboard and
	// poisons the restart backoff for the next real failure.
	clean := code == 0 || stopping
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
	failures := s.failures
	s.cmd = nil
	s.mu.Unlock()

	// Only now: Stop waits on done, and must observe the settled status
	// rather than race this goroutine for the lock. It also means a
	// Restart's Start sees cmd == nil and cannot be clobbered by the
	// outgoing generation's bookkeeping.
	close(done)

	s.logger.Info("game exited", "code", code, "uptime", uptime.Round(time.Second), "desired", desired)
	if stopping || desired != "running" {
		return
	}

	// Restart policy: any exit while desired=running comes back — that's
	// what makes flametender's in-game shutdown double as a restart, exactly
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

// Stop brings the game down and waits for it. Persists desired=stopped
// first, so neither a concurrent crash-restart nor an in-flight self-exit
// can resurrect it.
//
// selfExit is how long a shutdown the game has *already* accepted gets to
// finish before the supervisor signals. Enshrouded has no external
// shutdown request, so callers pass zero today; the parameter survives
// because the agent API carries it and a future channel could use it.
//
// The graceful signal is SIGINT to the process *group* — Enshrouded's
// clean shutdown (the recon doc's images all translate their stops into
// SIGINT on the exe; a SIGTERM to the Wine wrapper is not reliably
// propagated), and the group is what reaches the exe through Wine's own
// processes. The server saves the world on the way down, which is why
// the grace period is generous — then SIGKILL once it expires.
func (s *supervisor) Stop(selfExit time.Duration) error {
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

	// wait closes done only after recording the exit, so the status is
	// already settled on every path out of here; this just clears the flag.
	defer func() {
		s.mu.Lock()
		s.stopping = false
		s.mu.Unlock()
	}()

	if selfExit > 0 {
		select {
		case <-done:
			s.logger.Info("game shut itself down")
			return nil
		case <-time.After(selfExit):
			s.logger.Info("game did not exit on its own; signalling", "waited", selfExit)
		}
	}

	signalGroup(pid, false)
	select {
	case <-done:
		s.logger.Info("game stopped gracefully")
		return nil
	case <-time.After(s.grace):
	}

	s.logger.Warn("game ignored SIGINT; killing", "grace", s.grace)
	signalGroup(pid, true)
	select {
	case <-done:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("game process did not die after SIGKILL")
	}
}

func (s *supervisor) Restart(selfExit time.Duration) error {
	if err := s.Stop(selfExit); err != nil {
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

// prepareRuntime makes a freshly installed server bootable, then keeps
// the operator's identity settings authoritative.
//
// When enshrouded_server.json is absent, the game would generate one with
// defaults on first start — but that default is an *open* server named
// "Enshrouded Server", so the seed writes a complete config first and the
// first start is already named and password-protected. When the file
// exists it belongs to the operator (or the game), and only the
// explicitly configured identity settings are enforced into it: the
// server-browser name, the queryPort the container publishes, and the
// role-group passwords. Everything else in the file is never touched.
func (s *supervisor) prepareRuntime() {
	cfgPath := filepath.Join(s.installDir, s.profile.ConfigRel)

	e := esconfig.Enforcement{ServerName: s.serverName, QueryPort: s.gamePort}
	if s.adminPassword != "" {
		e.AdminPassword = &s.adminPassword
	}
	if s.joinPassword != "" {
		e.JoinPassword = &s.joinPassword
	}

	if _, err := os.Stat(cfgPath); err != nil {
		if err := esconfig.Write(cfgPath, esconfig.Seed(e)); err != nil {
			s.logger.Warn("could not seed enshrouded_server.json", "error", err)
			return
		}
		s.logger.Info("seeded enshrouded_server.json", "path", cfgPath)
		return
	}

	doc, err := esconfig.Load(cfgPath)
	if err != nil {
		// A file the game can't parse either would be regenerated with open
		// defaults on boot; refuse to make it worse and say so loudly.
		s.logger.Warn("enshrouded_server.json is unreadable; leaving it alone", "error", err)
		return
	}
	if esconfig.Enforce(doc, e) {
		if err := esconfig.Write(cfgPath, doc); err != nil {
			s.logger.Warn("could not enforce identity settings", "error", err)
			return
		}
		s.logger.Info("enforced identity settings", "path", cfgPath)
	}
}
