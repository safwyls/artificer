package agent

import (
	"log/slog"
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
	// FindSaveDir overrides the SaveDirName lookup for games whose save
	// location needs discovery (Dragonwilds globs two spellings). Nil
	// means SaveDirName joined to the install root, required non-empty.
	FindSaveDir func(installDir string) (string, error)
	// StopSignal is the graceful stop signal for the game's process
	// group. Zero means SIGTERM; Enshrouded uses SIGINT, on which it
	// saves the world.
	StopSignal syscall.Signal
	// StopGrace is the default wait between the graceful signal and
	// SIGKILL when Config doesn't override it. Zero means 30s; games
	// that save on the way down want much more.
	StopGrace time.Duration
	// BuildProfile assembles the named launch profile — command, args,
	// env, probe file, steam platform — from wherever the game module
	// reads its tuning. Games with one build ignore name; games with a
	// chooser (Dragonwilds' native/wine) build the one asked for. The
	// custom-command escape hatch (GameCommand set) is handled by the
	// kit and never reaches this hook. args arrives resolved: the
	// operator's GameArgs, else DefaultArgs.
	BuildProfile func(name, installDir string, gamePort int, args []string) Profile
	// Profiles are the selectable profile names, for games that ship
	// more than one build. Empty means one fixed profile (whatever
	// BuildProfile returns for the empty name) and no chooser.
	Profiles []string
	// DefaultProfile is the initial selection when nothing is persisted.
	DefaultProfile string
	// DefaultArgs supplies launcher args when the operator sets none —
	// Dragonwilds derives them from the game port. Nil means none.
	DefaultArgs func(gamePort int) []string
	// HealthExtras and LaunchExtras merge game-specific keys into the
	// /v1/health and launch-status payloads (Dragonwilds' bridge
	// status). Keys are top-level in the JSON, so a console's typed
	// relay round-trips them via the Extra maps on Health/LaunchStatus.
	HealthExtras func(a *Agent) map[string]any
	LaunchExtras func(a *Agent) map[string]any
	// PrepareRuntime, when set, runs before every game start with the
	// resolved runtime facts — the seed/enforce step that makes a fresh
	// install bootable, keeps dashboard-issued settings authoritative,
	// and does whatever else the game needs standing before its process
	// exists (Dragonwilds' bridge dir and steamclient link).
	PrepareRuntime func(env RuntimeEnv)
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
	OwnerID       string
	WorldName     string
	// ServerDesc is the server-browser description/MOTD, for games that
	// have one.
	ServerDesc string
}

// RuntimeEnv is everything PrepareRuntime sees: where the install and
// the profile's settings file live, which profile is about to start,
// and the operator's identity settings.
type RuntimeEnv struct {
	InstallDir string
	ConfigPath string
	Profile    Profile
	Identity   RuntimeIdentity
	Logger     *slog.Logger
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
