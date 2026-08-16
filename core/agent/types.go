// Package agent holds the wire vocabulary of the console↔agent protocol:
// the JSON shapes a game console and its sidecar agent exchange. The
// game-specific agents (palagent, wkagent, flameagent) serve this
// protocol; core/agentctl is its client. The shared agent implementation
// (supervision skeleton, jobs, steam verbs) migrates into this package as
// the agent-kit extraction lands — the types come first because both
// sides already agree on them.
package agent

import "time"

// APIVersion is the protocol version an agent reports in Health. The
// console tolerates older agents per-route (a 404 is "agent too old for
// this verb", not an error); this number is how it can tell at a glance.
const APIVersion = 3

// Job is the API view of one unit of background work. Fields are value
// copies — handlers never hand out a pointer into the runner's mutable
// state.
type Job struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	State      string    `json:"state"` // running | done | failed
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitzero"`
	// Error is the failure summary when State is failed: the exec error,
	// or the SteamCMD "Error!" line that betrayed a zero-exit failure.
	Error string   `json:"error,omitempty"`
	Log   []string `json:"log,omitempty"`
}

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

// LaunchStatus reports how an agent starts the game, and what else it
// could start.
type LaunchStatus struct {
	Profile string `json:"profile"`
	Label   string `json:"label"`
	// Installed reports whether the game's files are present.
	Installed bool `json:"installed"`
	// Runnable reports whether the launcher exists on this agent at all —
	// false on an image missing the runtime the game needs, which is a
	// different failure from "the game isn't installed" and one only a
	// redeploy can fix.
	Runnable bool `json:"runnable"`
	// ConfigPath is where the game's settings file lives, relative to the
	// install root.
	ConfigPath string `json:"configPath"`
}

// Health is the /v1/health payload — everything the console needs to
// decide what an agent can do and whether work is in flight.
type Health struct {
	Agent        string `json:"agent"`
	Version      string `json:"version"`
	APIVersion   int    `json:"apiVersion"`
	Mode         string `json:"mode"`
	InstallDir   string `json:"installDir"`
	InstallDirOk bool   `json:"installDirOk"`
	// SaveFound/ConfigFound report whether the file verbs have anything
	// to serve, so the console can distinguish "not synced yet" from
	// "this install has no world".
	SaveFound     bool   `json:"saveFound"`
	ConfigFound   bool   `json:"configFound"`
	DiskFreeBytes uint64 `json:"diskFreeBytes"`
	// Game is the supervised process's state; nil in companion mode.
	Game *GameStatus `json:"game,omitempty"`
	// Launch reports how this agent starts the game. Nil outside
	// supervisor mode, where nothing is launched at all.
	Launch *LaunchStatus `json:"launch,omitempty"`
	// Provision carries the wizard defaults. An agent itself never sets
	// it — it survives in the Health shape because the console's Ilmari
	// adapter synthesizes a Health for the wizard, defaults included.
	Provision *ProvisionDefaults `json:"provision,omitempty"`
	// Job is the running job if there is one, else the most recently
	// finished one, else null. Exposing it here (not only under /jobs)
	// lets the console rediscover in-flight work after its own restart.
	Job *Job `json:"job"`
}

// The provisioning wire vocabulary: the shapes the Raise-a-server wizard
// and the Ilmari adapter speak to each other. Ilmari itself knows none of
// it — the adapter translates into Ilmari's spec at the boundary.

// ProvisionRequest is everything the wizard collects to raise one server.
// Every field is data for a fixed template — none of it changes what kind
// of container is made. Which env vars the fields become is the game
// module's provisioning profile's call.
type ProvisionRequest struct {
	// Slug names the container (<agent>-<slug>) and the data directory
	// (<data root>/<slug>).
	Slug string `json:"slug"`
	// ImageTag selects the agent image channel for the new server.
	ImageTag string `json:"imageTag"`
	// Token is the new agent's bearer token (the console generated it).
	Token string `json:"token"`
	// AdminPassword is the game's admin credential, enforced by the agent
	// on every game start. What it concretely becomes is per game.
	AdminPassword string `json:"adminPassword"`
	// JoinPassword gates joining, for games that support one. Empty means
	// an open server — anyone who finds it can join.
	JoinPassword string `json:"joinPassword,omitempty"`
	ServerName   string `json:"serverName"`
	// RunAs is uid:gid for the container ("" = the image's own user).
	RunAs string `json:"runAs"`
	// GamePort is the first published game port; how many contiguous
	// ports the game claims from it is a game fact.
	GamePort  int `json:"gamePort"`
	AgentPort int `json:"agentPort"`
}

// ProvisionDefaults is what the wizard can infer instead of asking: the
// provisioner's own configuration is the source of truth for where and
// how servers land on its host.
type ProvisionDefaults struct {
	DataRoot string `json:"dataRoot"`
	// PublicHost is the address the console (and players) reach the host
	// on. Inside containers "localhost" means the container itself, so
	// this must be declared, not guessed.
	PublicHost string `json:"publicHost,omitempty"`
	RunAs      string `json:"runAs"`
	ImageTag   string `json:"imageTag"`
}

// DiscoveredServer is one agent-shaped container found on the host.
// Deliberately free of environment values: a container's env carries its
// token and passwords, and those only cross the boundary through adopt.
type DiscoveredServer struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	Mode    string `json:"mode"` // supervisor | companion | "" (unknown)
	Running bool   `json:"running"`
	// Published host ports for the well-known container ports.
	GamePort  int `json:"gamePort,omitempty"`
	AgentPort int `json:"agentPort,omitempty"`
}

// AdoptResult carries everything the console needs to re-register an
// existing agent container — including its secrets. Deliberately: the
// console's own provisioning injected those secrets in the first place,
// so returning them to the (token-authenticated) control plane stays
// within the same trust boundary, and Ilmari filters the env to this
// console's registered prefix.
type AdoptResult struct {
	Name          string `json:"name"`
	Mode          string `json:"mode"`
	ServerName    string `json:"serverName,omitempty"`
	Token         string `json:"token"`
	AdminPassword string `json:"adminPassword"`
	GamePort      int    `json:"gamePort,omitempty"`
	AgentPort     int    `json:"agentPort,omitempty"`
}

// DestroyResult reports what the destroy verb unmade. DataDir comes back
// so the operator learns where the world still is — destroying a
// container is not consent to delete a save.
type DestroyResult struct {
	Container string `json:"container"`
	DataDir   string `json:"dataDir,omitempty"`
}

// RecreateResult reports a container rebuilt on a different image,
// keeping everything else.
type RecreateResult struct {
	Container string `json:"container"`
	Image     string `json:"image"`
	Previous  string `json:"previousImage"`
}
