// Package flameagent is the sidecar agent that runs (or sits beside) one
// Enshrouded game server, holding the install volume and the SteamCMD
// tooling so the control plane can stay pure. See docs/sidecar-agent.md.
//
// For this game the agent is not an optional convenience: Enshrouded has
// no RCON, no REST API and no server console, so the agent's health and
// log endpoints are the only source the dashboard has for whether a
// server is up and who is on it (its Steam A2S query is roadmap Phase 2).
//
// The API is a fixed set of dashboard-shaped verbs — never a generic exec
// or an arbitrary path parameter — so a compromised control plane (or a
// leaked token) can repair one game server and nothing else. Long-running
// work runs as a job: POST starts it and returns immediately, the caller
// polls; a control-plane restart mid-job orphans nothing.
package flameagent

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/flametender/internal/steamcmd"
)

// APIVersion is reported in /v1/health so the control plane can refuse to
// drive an agent it doesn't understand (and vice versa) instead of failing weirdly.
// 1 = steam verbs; 2 adds the file verbs (save bundle, config); 3 adds
// supervisor mode's power verbs and game status.
const APIVersion = 3

// DefaultAppID is the Steam app the agent updates when none is configured:
// the Enshrouded dedicated server tool (the game client is 1203620). Set
// FLAMEAGENT_APP_ID to point the agent at a different one.
//
// Spelled out here rather than read from internal/games/enshrouded, on
// purpose. The agent is a thin sidecar — file access, process control,
// SteamCMD — and importing a game package for one integer would link the
// game registry and its client into every agent binary, and run a
// registration nothing here queries. A Steam app id is a fixed number, so
// the duplication cannot drift (an agreement test enforces it anyway).
const DefaultAppID = 2278520

// DefaultGamePort is Enshrouded's single UDP port — the json's queryPort,
// which carries game traffic and the Steam A2S query both. There has been
// no separate game port since mid-2024 (docs/enshrouded-recon.md, "Ports
// & protocols").
const DefaultGamePort = 15637

// minTokenLen is the floor for the shared token; the agent refuses to
// start below it rather than run guessably authenticated.
const minTokenLen = 16

type Config struct {
	// Token is the shared bearer token the control plane presents. Required.
	Token string
	// InstallDir is the game install root (the directory holding
	// steamapps/), shared with the game server container via the volume.
	InstallDir string
	// SteamCmd is the steamcmd binary to exec for update jobs.
	SteamCmd string
	// AppID is the Steam app to update; defaults to DefaultAppID.
	AppID int
	// Mode is "companion" (default: the game runs in its own container)
	// or "supervisor" (this agent runs the game as a child process and
	// owns its lifecycle — docs/sidecar-agent.md phase 3).
	Mode string
	// GameCommand is the launcher relative to InstallDir. Setting it opts
	// out of launch profiles entirely (see ProfileCustom). Supervisor mode
	// only.
	GameCommand string
	// GameArgs are the launcher's flags; defaults to -log plus the
	// configured port. Supervisor mode only.
	GameArgs []string
	// Launch selects and tunes the launch profile — which of the game's two
	// builds to run. Supervisor mode only; see launch.go.
	Launch LaunchConfig
	// StopGrace is how long a SIGTERM'd game gets before SIGKILL;
	// defaults to 30s.
	StopGrace time.Duration
	// GamePort is the UDP port the game binds inside the container —
	// enforced into enshrouded_server.json's queryPort before each start
	// (the game has no port argument). Normally left at DefaultGamePort
	// and remapped on publish; this exists for host-network deployments.
	// Supervisor mode only.
	GamePort int
	// AdminPassword, when set, is enforced into the config's first
	// kick/ban-capable role group before every game start, so the
	// password the dashboard shows is the one that grants admin at the
	// join screen. Authoritative: an edit to that group's password is
	// re-applied from here on the next start. Supervisor mode only.
	AdminPassword string
	// JoinPassword, when set, is enforced the same way onto the first
	// non-admin role group — the password friends type to join. Unset
	// leaves whatever the file has (including an open server). Supervisor
	// mode only.
	JoinPassword string
	// ServerName seeds and then enforces the server-browser name.
	// Supervisor mode only.
	ServerName string
	// Autostart starts the game on agent boot when no persisted desired
	// state exists yet (a fresh provision). Defaults true in supervisor
	// mode; a persisted "stopped" always wins.
	Autostart *bool
	// RestartBackoffFloor is the first crash-restart delay (doubling per
	// consecutive failure); defaults to 5s. Tests shrink it.
	RestartBackoffFloor time.Duration
	// Version is the agent build version, reported in /v1/health.
	Version string
	Logger  *slog.Logger
}

type Agent struct {
	cfg  Config
	jobs *jobRunner
	// game is non-nil only in supervisor mode.
	game *supervisor
}

// New validates the config and builds the agent. It does not listen;
// callers mount Handler() wherever they like (main, or a test server).
// In supervisor mode, call Run to kick off install/autostart.
func New(cfg Config) (*Agent, error) {
	if len(cfg.Token) < minTokenLen {
		return nil, fmt.Errorf("agent token must be at least %d characters", minTokenLen)
	}
	if cfg.InstallDir == "" {
		return nil, errors.New("install dir is required")
	}
	if cfg.SteamCmd == "" {
		cfg.SteamCmd = "steamcmd"
	}
	if cfg.AppID == 0 {
		cfg.AppID = DefaultAppID
	}
	if cfg.Mode == "" {
		cfg.Mode = "companion"
	}
	if cfg.Mode != "companion" && cfg.Mode != "supervisor" {
		// Provisioner mode is gone on purpose: placing containers is
		// Ilmari's job (github.com/safwyls/ilmari), and an agent that also
		// held Docker rights would be a second socket-holder on the host —
		// the exact blindness Ilmari exists to end.
		return nil, fmt.Errorf("unknown mode %q: use companion or supervisor", cfg.Mode)
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	a := &Agent{cfg: cfg, jobs: newJobRunner(cfg.Logger)}
	if cfg.Mode == "supervisor" {
		a.game = newSupervisor(cfg, func() bool {
			cur := a.jobs.current()
			return cur != nil && cur.State == "running"
		})
	}
	return a, nil
}

// Run performs supervisor-mode boot: install the game if it's missing
// (visible as a normal job), then start it unless the operator last asked
// for stopped. A no-op in companion mode. Blocks only while polling an
// install job, so callers run it in a goroutine.
func (a *Agent) Run() {
	if a.game == nil {
		return
	}
	if !a.game.Installed() {
		a.cfg.Logger.Info("game not installed; installing", "dir", a.cfg.InstallDir)
		args := steamcmd.UpdateArgsFor(a.cfg.InstallDir, a.cfg.AppID, true, a.steamPlatform())
		job, err := a.jobs.start("steam-install", a.cfg.SteamCmd, args)
		if err != nil {
			a.cfg.Logger.Error("install could not start", "error", err)
			return
		}
		for {
			time.Sleep(2 * time.Second)
			j := a.jobs.get(job.ID)
			if j.State == "failed" {
				a.cfg.Logger.Error("install failed; not starting the game", "error", j.Error)
				return
			}
			if j.State == "done" {
				break
			}
		}
	}

	autostart := a.cfg.Autostart == nil || *a.cfg.Autostart
	fallback := "stopped"
	if autostart {
		fallback = "running"
	}
	if a.game.loadDesired(fallback) == "running" {
		if err := a.game.Start(); err != nil {
			a.cfg.Logger.Error("boot start failed", "error", err)
		}
	} else {
		a.cfg.Logger.Info("game stays stopped (persisted desired state)")
	}
}

func (a *Agent) Handler() http.Handler {
	r := chi.NewRouter()
	// Bare liveness for container healthchecks: no auth, no body, no
	// information.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	r.Route("/v1", func(r chi.Router) {
		r.Use(a.requireToken)
		r.Get("/health", a.handleHealth)
		r.Post("/steam/clear-cache", a.handleClearCache)
		r.Post("/steam/update", a.handleStartUpdate)
		r.Get("/jobs/{jobID}", a.handleGetJob)
		// Phase 2 file verbs — fixed locations only, never a path
		// parameter (docs/sidecar-agent.md).
		r.Get("/files/save", a.handleGetSave)
		r.Get("/files/config", a.handleGetConfig)
		r.Put("/files/config", a.handlePutConfig)
		// Phase 3 power verbs — supervisor mode only; companion agents
		// answer 400 so flametender falls back to the docker proxy.
		r.Post("/power/{action}", a.handlePower)
		r.Get("/power/logs", a.handleGameLogs)
		// Which launch profile the next start uses. Reading is part of
		// health; this is the write side, and it applies at the next
		// start rather than disturbing a running game.
		r.Put("/launch", a.handleSetLaunchProfile)
	})
	return r
}

// requireToken checks the bearer token in constant time. Hashing first
// makes the comparison length-independent, so a mismatched length doesn't
// return early.
func (a *Agent) requireToken(next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(a.cfg.Token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		gotHash := sha256.Sum256([]byte(got))
		if subtle.ConstantTimeCompare(want[:], gotHash[:]) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid agent token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Health is the /v1/health payload — everything flametender needs to decide
// what this agent can do and whether work is in flight.
type Health struct {
	Agent        string `json:"agent"`
	Version      string `json:"version"`
	APIVersion   int    `json:"apiVersion"`
	Mode         string `json:"mode"`
	InstallDir   string `json:"installDir"`
	InstallDirOk bool   `json:"installDirOk"`
	// SaveFound/ConfigFound report whether the phase 2 file verbs have
	// anything to serve, so flametender can distinguish "not synced yet" from
	// "this install has no world".
	SaveFound     bool   `json:"saveFound"`
	ConfigFound   bool   `json:"configFound"`
	DiskFreeBytes uint64 `json:"diskFreeBytes"`
	// Game is the supervised process's state; nil in companion mode.
	Game *GameStatus `json:"game,omitempty"`
	// Launch reports how this agent starts the game, and what
	// else it could start. Nil outside supervisor mode, where nothing is
	// launched at all.
	Launch *LaunchStatus `json:"launch,omitempty"`
	// Provision carries the wizard defaults. The agent itself never sets
	// it — it survives in the Health shape because the console's Ilmari
	// adapter synthesizes a Health for the wizard, defaults included.
	Provision *ProvisionDefaults `json:"provision,omitempty"`
	// Job is the running job if there is one, else the most recently
	// finished one, else null. Exposing it here (not only under /jobs)
	// lets flametender rediscover in-flight work after its own restart.
	Job *Job `json:"job"`
}

func (a *Agent) handleHealth(w http.ResponseWriter, _ *http.Request) {
	installOk := false
	if _, err := os.Stat(a.cfg.InstallDir); err == nil {
		installOk = true
	}
	_, saveErr := a.findSaveDir()
	_, configErr := os.Stat(a.configPath())
	h := Health{
		Agent:         "flameagent",
		Version:       a.cfg.Version,
		APIVersion:    APIVersion,
		Mode:          a.cfg.Mode,
		InstallDir:    a.cfg.InstallDir,
		InstallDirOk:  installOk,
		SaveFound:     saveErr == nil,
		ConfigFound:   configErr == nil,
		DiskFreeBytes: diskFree(a.cfg.InstallDir),
		Job:           a.jobs.current(),
	}
	if a.game != nil {
		st := a.game.Status()
		h.Game = &st
		h.Launch = a.launchStatus()
	}
	writeJSON(w, http.StatusOK, h)
}

func (a *Agent) handleClearCache(w http.ResponseWriter, _ *http.Request) {
	removed, err := steamcmd.ClearCache(a.cfg.InstallDir)
	if err != nil {
		if errors.Is(err, steamcmd.ErrNotInstallRoot) {
			writeError(w, http.StatusBadRequest, err.Error()+" (agent install dir: "+a.cfg.InstallDir+")")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.cfg.Logger.Info("steam cache cleared", "removed", removed)
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
}

func (a *Agent) handleStartUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Validate bool `json:"validate"`
	}
	// An empty body means default options, not a malformed request.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// The install dir is a volume mount in any real deployment; its absence
	// means the agent is pointed somewhere wrong, and force_install_dir
	// would silently create a fresh install there.
	if _, err := os.Stat(a.cfg.InstallDir); err != nil {
		writeError(w, http.StatusBadRequest, "install dir does not exist: "+a.cfg.InstallDir)
		return
	}
	// In supervisor mode the agent knows the game's state first-hand:
	// SteamCMD must never rewrite files under a live server.
	if a.game != nil && a.game.Running() {
		writeError(w, http.StatusConflict, "stop the server before updating")
		return
	}

	args := steamcmd.UpdateArgsFor(a.cfg.InstallDir, a.cfg.AppID, req.Validate, a.steamPlatform())
	job, err := a.jobs.start("steam-update", a.cfg.SteamCmd, args)
	if errors.Is(err, errJobRunning) {
		writeError(w, http.StatusConflict, "a job is already running")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.cfg.Logger.Info("steam update started", "job", job.ID, "validate", req.Validate)
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

func (a *Agent) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job := a.jobs.get(chi.URLParam(r, "jobID"))
	if job == nil {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}

// maxGracefulWait caps how long a caller may ask the supervisor to wait
// for the game to exit on its own, so a bad value can't wedge a stop.
const maxGracefulWait = 2 * time.Minute

// parseGraceful reads the ?graceful= duration, collapsing anything absent,
// malformed, or negative to "don't wait".
func parseGraceful(v string) time.Duration {
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0
	}
	return min(d, maxGracefulWait)
}

// handlePower starts/stops/restarts the supervised game. The response is
// the post-action status, so flametender needs no follow-up read.
func (a *Agent) handlePower(w http.ResponseWriter, r *http.Request) {
	if a.game == nil {
		writeError(w, http.StatusBadRequest, "agent is in companion mode — the game runs in its own container")
		return
	}
	// graceful is how long an in-game shutdown the caller already requested
	// gets to finish before the supervisor signals the process. Flametender sets
	// it after its REST /shutdown courtesy is accepted; absent, stops
	// escalate immediately as before.
	graceful := parseGraceful(r.URL.Query().Get("graceful"))
	var err error
	switch action := chi.URLParam(r, "action"); action {
	case "start":
		err = a.game.Start()
	case "stop":
		err = a.game.Stop(graceful)
	case "restart":
		err = a.game.Restart(graceful)
	default:
		writeError(w, http.StatusBadRequest, "unknown action")
		return
	}
	if errors.Is(err, errJobInFlight) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"game": a.game.Status()})
}

func (a *Agent) handleGameLogs(w http.ResponseWriter, r *http.Request) {
	if a.game == nil {
		writeError(w, http.StatusBadRequest, "agent is in companion mode — read the game container's logs instead")
		return
	}
	tail := 300
	if n, err := strconv.Atoi(r.URL.Query().Get("tail")); err == nil && n > 0 {
		tail = min(n, gameLogTail)
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": a.game.Logs(tail)})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func newJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// LaunchStatus is the /v1/health launch field: how the agent runs the
// game, and whether the running process is still what was selected.
type LaunchStatus struct {
	Profile string `json:"profile"`
	Label   string `json:"label"`
	// Installed reports whether the game's files are present.
	Installed bool `json:"installed"`
	// Runnable reports whether the profile's launcher exists on this
	// agent — false for the wine profile on an image with no Wine in it,
	// a different failure from "the game isn't installed", and one only a
	// redeploy of the agent itself can fix.
	Runnable bool `json:"runnable"`
	// Available are the profiles the console may select. Empty when the
	// agent runs an explicit command (ProfileCustom), which the console
	// must not silently replace.
	Available []string `json:"available"`
	// PendingRestart reports that the selection has changed since the
	// running process started.
	PendingRestart bool `json:"pendingRestart"`
	// ConfigPath is where enshrouded_server.json lives, relative to the
	// install root.
	ConfigPath string `json:"configPath"`
}

// steamPlatform is the depot the selected build needs, so an install or
// update fetches the build the agent is actually going to run rather than
// whatever matches the host.
func (a *Agent) steamPlatform() string {
	if a.game == nil {
		return ""
	}
	return a.game.Profile().SteamPlatform
}

func (a *Agent) launchStatus() *LaunchStatus {
	p := a.game.Profile()
	st := &LaunchStatus{
		Profile:    p.Name,
		Label:      p.Label,
		Installed:  p.installed(a.cfg.InstallDir),
		Runnable:   p.runnable(a.cfg.InstallDir),
		ConfigPath: p.ConfigRel,
	}
	if p.Name != ProfileCustom {
		st.Available = SelectableProfiles
	}
	st.PendingRestart = a.game.profileChangedSinceStart()
	return st
}

// handleSetLaunchProfile selects the build the next start will launch.
//
// It does not stop, start or restart anything. Switching build is not a
// restart: the two come from different Steam depots, so the new one has to
// be installed before it can run, and doing that behind an operator's back
// during what looked like a settings change would be the worst possible
// moment to surprise them.
func (a *Agent) handleSetLaunchProfile(w http.ResponseWriter, r *http.Request) {
	if a.game == nil {
		writeError(w, http.StatusBadRequest, "agent is not supervising a game — launch profiles are supervisor mode only")
		return
	}
	var in struct {
		Profile string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := a.game.SetProfile(in.Profile); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.launchStatus())
}
