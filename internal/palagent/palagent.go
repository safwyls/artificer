// Package palagent is the sidecar agent that sits next to one Palworld
// game server, holding the install volume and the SteamCMD tooling so
// palcon can stay a pure control plane. See docs/sidecar-agent.md.
//
// The API is a fixed set of dashboard-shaped verbs — never a generic exec
// or an arbitrary path parameter — so a compromised palcon (or a leaked
// token) can repair one game server and nothing else. Long-running work
// runs as a job: POST starts it and returns immediately, palcon polls; a
// palcon restart mid-job orphans nothing.
package palagent

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
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/palcon/internal/steamops"
)

// APIVersion is reported in /v1/health so palcon can refuse to drive an
// agent it doesn't understand (and vice versa) instead of failing weirdly.
const APIVersion = 1

// minTokenLen is the floor for the shared token; the agent refuses to
// start below it rather than run guessably authenticated.
const minTokenLen = 16

type Config struct {
	// Token is the shared bearer token palcon presents. Required.
	Token string
	// InstallDir is the Palworld install root (the directory holding
	// steamapps/), shared with the game server container via the volume.
	InstallDir string
	// SteamCmd is the steamcmd binary to exec for update jobs.
	SteamCmd string
	// AppID is the Steam app to update; defaults to the Palworld
	// dedicated server.
	AppID int
	// Version is the agent build version, reported in /v1/health.
	Version string
	Logger  *slog.Logger
}

type Agent struct {
	cfg  Config
	jobs *jobRunner
}

// New validates the config and builds the agent. It does not listen;
// callers mount Handler() wherever they like (main, or a test server).
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
		cfg.AppID = steamops.PalworldAppID
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Agent{cfg: cfg, jobs: newJobRunner(cfg.Logger)}, nil
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

// Health is the /v1/health payload — everything palcon needs to decide
// what this agent can do and whether work is in flight.
type Health struct {
	Agent         string `json:"agent"`
	Version       string `json:"version"`
	APIVersion    int    `json:"apiVersion"`
	Mode          string `json:"mode"`
	InstallDir    string `json:"installDir"`
	InstallDirOk  bool   `json:"installDirOk"`
	DiskFreeBytes uint64 `json:"diskFreeBytes"`
	// Job is the running job if there is one, else the most recently
	// finished one, else null. Exposing it here (not only under /jobs)
	// lets palcon rediscover in-flight work after its own restart.
	Job *Job `json:"job"`
}

func (a *Agent) handleHealth(w http.ResponseWriter, _ *http.Request) {
	installOk := false
	if _, err := os.Stat(a.cfg.InstallDir); err == nil {
		installOk = true
	}
	writeJSON(w, http.StatusOK, Health{
		Agent:         "palagent",
		Version:       a.cfg.Version,
		APIVersion:    APIVersion,
		Mode:          "companion",
		InstallDir:    a.cfg.InstallDir,
		InstallDirOk:  installOk,
		DiskFreeBytes: diskFree(a.cfg.InstallDir),
		Job:           a.jobs.current(),
	})
}

func (a *Agent) handleClearCache(w http.ResponseWriter, _ *http.Request) {
	removed, err := steamops.ClearCache(a.cfg.InstallDir)
	if err != nil {
		if errors.Is(err, steamops.ErrNotInstallRoot) {
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

	args := steamops.UpdateArgs(a.cfg.InstallDir, a.cfg.AppID, req.Validate)
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
