package flameagent

// The provisioning wire vocabulary.
//
// The agent no longer provisions anything — placing containers is Ilmari's
// job (github.com/safwyls/ilmari), and this console holds no Docker rights
// at all. What remains here are the shapes the Raise-a-server wizard and
// the Ilmari adapter speak to each other (internal/api/provisioner.go
// translates them into Ilmari's spec at the boundary). They live in this
// package because they describe what a provisioned *agent* container is
// made of, and because agentctl re-exports them as its own client
// vocabulary.

// ProvisionRequest is everything the wizard collects to raise one server.
// Every field is data for a fixed template — none of it changes what kind
// of container is made.
type ProvisionRequest struct {
	// Slug names the container (flameagent-<slug>) and the data directory
	// (<data root>/<slug>).
	Slug string `json:"slug"`
	// ImageTag selects the flameagent channel for the new server.
	ImageTag string `json:"imageTag"`
	// Token is the new agent's bearer token (flametender generated it).
	Token string `json:"token"`
	// AdminPassword becomes the password of the admin role group in
	// enshrouded_server.json (FLAMEAGENT_ADMIN_PASSWORD), enforced on every
	// game start.
	AdminPassword string `json:"adminPassword"`
	// JoinPassword becomes the password of the default player role group.
	// Empty means an open server — anyone who finds it can join.
	JoinPassword string `json:"joinPassword,omitempty"`
	ServerName   string `json:"serverName"`
	// RunAs is uid:gid for the container ("" = the image's own user,
	// flameagent/uid 1000).
	RunAs string `json:"runAs"`
	// GamePort is the published UDP port — Enshrouded's single queryPort,
	// which carries game traffic and the Steam query both.
	GamePort  int `json:"gamePort"`
	AgentPort int `json:"agentPort"`
}

// ProvisionDefaults is what the wizard can infer instead of asking: the
// provisioner's own configuration is the source of truth for where and
// how servers land on its host.
type ProvisionDefaults struct {
	DataRoot string `json:"dataRoot"`
	// PublicHost is the address flametender (and players) reach the host on.
	// Inside containers "localhost" means the container itself, so this
	// must be declared, not guessed.
	PublicHost string `json:"publicHost,omitempty"`
	RunAs      string `json:"runAs"`
	ImageTag   string `json:"imageTag"`
}

// DiscoveredServer is one flameagent-shaped container found on the host.
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

// AdoptResult carries everything flametender needs to re-register an
// existing flameagent container — including its secrets. Deliberately: the
// console's own provisioning injected those secrets in the first place, so
// returning them to the (token-authenticated) control plane stays within
// the same trust boundary, and Ilmari filters the env to this console's
// registered FLAMEAGENT_ prefix.
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
