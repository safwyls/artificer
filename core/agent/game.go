package agent

import (
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
)

// Game is the game-shaped half of a sidecar agent. The agent kit in this
// package is everything the three consoles' agents always shared —
// supervision skeleton, jobs, steam verbs, file verbs, auth — and every
// game fact it needs arrives here, so a concrete agent (flameagent,
// wkagent, palagent) is this package plus one Game and a main.
type Game struct {
	// AgentName labels the agent ("flameagent") — Health's agent field
	// and the container-name prefix convention.
	AgentName string
	// AppID is the game's Steam dedicated-server app. Spelled as data on
	// purpose: the agent is a thin sidecar, and importing a game package
	// for one integer would link the game registry into every agent
	// binary. An agreement test on the game side keeps it honest.
	AppID int
	// DefaultGamePort is the game's own first port, used when Config
	// doesn't override it.
	DefaultGamePort int
	// ConfigRelPath is the settings file relative to the install root.
	ConfigRelPath string
	// ConfigContentType is served on GET /v1/files/config
	// ("application/json; charset=utf-8", "text/plain; charset=utf-8").
	ConfigContentType string
	// ValidateConfig vets an uploaded settings file before it is written;
	// nil accepts anything. Games whose server regenerates permissive
	// defaults over a malformed file (Enshrouded's open, password-less
	// server) should refuse bad uploads here.
	ValidateConfig func(data []byte) error
	// SaveDirName is the world-save directory under the install root. The
	// agent deliberately never follows config-supplied paths.
	SaveDirName string
	// StopSignal is the graceful stop signal for the game's process
	// group. Zero means SIGTERM; Enshrouded uses SIGINT, on which it
	// saves the world.
	StopSignal syscall.Signal
	// StopGrace is the default wait between the graceful signal and
	// SIGKILL when Config doesn't override it. Zero means 30s; games
	// that save on the way down want much more.
	StopGrace time.Duration
	// BuildProfile assembles the game's launch profile — command, args,
	// env, probe file, steam platform — from wherever the game module
	// reads its tuning. The custom-command escape hatch (GameCommand set)
	// is handled by the kit and never reaches this hook.
	BuildProfile func(installDir string, gameArgs []string) Profile
	// PrepareRuntime, when set, runs before every game start with the
	// settings file's path and the identity the operator configured —
	// the seed/enforce step that makes a fresh install bootable and
	// keeps dashboard-issued passwords authoritative.
	PrepareRuntime func(cfgPath string, id RuntimeIdentity)
	// Routes mounts game-specific verbs under the authenticated /v1
	// router (Enshrouded's A2S query relay, Dragonwilds' dwbridge).
	Routes func(r chi.Router, a *Agent)
}

// RuntimeIdentity is what PrepareRuntime may seed or enforce. Empty
// string fields mean "leave the file's value alone".
type RuntimeIdentity struct {
	ServerName    string
	GamePort      int
	AdminPassword string
	JoinPassword  string
}

func (g Game) stopSignal() syscall.Signal {
	if g.StopSignal == 0 {
		return syscall.SIGTERM
	}
	return g.StopSignal
}

func (g Game) stopGrace() time.Duration {
	if g.StopGrace <= 0 {
		return 30 * time.Second
	}
	return g.StopGrace
}
