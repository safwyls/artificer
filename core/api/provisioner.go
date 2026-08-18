package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/safwyls/artificer/core/agent"
	"github.com/safwyls/artificer/core/agentctl"
	"github.com/safwyls/artificer/core/anvilclient"
)

// Provisioner is what the API layer needs from whatever places containers
// on the host. Exactly one implementation exists — Anvil, the shared host
// service — but the seam stays an interface: tests fake it, and the wizard
// speaks these shapes rather than Anvil's wire types.
type Provisioner interface {
	BaseURL() string
	Health(ctx context.Context) (*agentctl.Health, error)
	Provision(ctx context.Context, req agent.ProvisionRequest) (*agentctl.ProvisionResult, error)
	Discover(ctx context.Context) ([]agentctl.DiscoveredServer, error)
	Adopt(ctx context.Context, container string) (*agentctl.AdoptResult, error)
	RecreateAgent(ctx context.Context, container, imageTag string) (*agent.RecreateResult, error)
	Destroy(ctx context.Context, container string) (*agentctl.DestroyResult, error)
	// HostPorts reports every published port on the host, whoever holds
	// it. Separate from Discover because Discover is deliberately scoped
	// to this console's own servers, and a port proposal must not be.
	HostPorts(ctx context.Context) ([]int, error)
}

// Interface satisfaction is a compile-time fact, not a hope.
var _ Provisioner = (*AnvilProvisioner)(nil)

// AnvilProvisioner adapts the Anvil host service to the Provisioner
// interface.
//
// The game knowledge that used to live in each agent's provisioner mode
// lives in the ProvisionProfile this adapter carries: which env vars
// configure the sidecar, which ports the game claims, which image family
// to deploy. Anvil itself knows none of it — that is the contract — so
// anything game-shaped in a provisioning flow belongs in the profile and
// nowhere further down. (The stale note this comment once carried about
// "a Dragonwilds sidecar" in an Enshrouded console was drift-ledger
// evidence; it dies with the parameterization.)
type AnvilProvisioner struct {
	c *anvilclient.Client
	p *ProvisionProfile
}

func NewAnvilProvisioner(c *anvilclient.Client, p *ProvisionProfile) *AnvilProvisioner {
	return &AnvilProvisioner{c: c, p: p}
}

func (p *AnvilProvisioner) BaseURL() string { return p.c.BaseURL() }

// translate maps Anvil's error vocabulary onto agentctl's, which is what
// the API layer branches on.
//
// This is the whole reason the adapter exists in error terms. The console
// asks "is this already gone?" (delete the row and move on) or "was I
// refused?" (change nothing, say why) using agentctl's sentinels, because
// those questions predate Anvil and are asked identically of a sidecar
// agent. Without the translation every Anvil failure arrived as an
// untyped error: a destroy of a container someone had already removed by
// hand trapped the operator in a retry that could never succeed, and a
// refused deploy — name taken, port held, image not allowed — was treated
// as a soft failure that left a server row behind for a server that was
// never created.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, anvilclient.ErrNotFound):
		return &translated{agentctl.ErrNotFound, err}
	case errors.Is(err, anvilclient.ErrRejected):
		return &translated{agentctl.ErrRejected, err}
	case errors.Is(err, anvilclient.ErrConflict):
		// ErrBusy is the API layer's "the host already holds this" branch.
		return &translated{agentctl.ErrBusy, err}
	}
	return err
}

// translated answers to two vocabularies at once: the API layer's
// sentinel and Anvil's own error, which still carries the conflict reason
// and the sentence worth showing an operator. Wrapping with fmt.Errorf
// could only keep one of them — an earlier cut wrapped the sentinel and
// silently dropped the reason, so a port collision was reported with the
// advice for a name collision.
type translated struct {
	sentinel error
	inner    error
}

func (t *translated) Error() string   { return t.inner.Error() }
func (t *translated) Unwrap() []error { return []error{t.sentinel, t.inner} }

// ours reports whether a container belongs to this console's agent
// family: named by the wizard's convention, or running this console's
// agent image under any tag. The image check is what keeps hand-deployed
// stacks adoptable — their names were chosen by a compose file or a
// TrueNAS app, not the wizard, and the legacy consoles discovered by
// image for exactly that reason. Cross-console isolation holds either
// way: another console's agent runs a different image family.
func (p *AnvilProvisioner) ours(name, image string) bool {
	if strings.HasPrefix(name, p.p.AgentName+"-") {
		return true
	}
	repo := p.p.ImageRepo
	return image == repo || strings.HasPrefix(image, repo+":") || strings.HasPrefix(image, repo+"@")
}

// agentContainerPort is the one container-side port fact that is the
// protocol's, not a game's: every sidecar agent serves its API on 8811.
const agentContainerPort = 8811

// adminPortMaps renders the named TCP admin transports as Anvil
// mappings.
func adminPortMaps(p *ProvisionProfile, req agent.ProvisionRequest) []anvilclient.PortMap {
	out := make([]anvilclient.PortMap, 0, len(p.AdminPorts))
	for _, ap := range p.AdminPorts {
		host := 0
		switch ap.Key {
		case "rest":
			host = req.RESTPort
		case "rcon":
			host = req.RCONPort
		}
		if host != 0 {
			out = append(out, anvilclient.PortMap{Host: host, Container: ap.Container, Proto: "tcp"})
		}
	}
	return out
}

// gamePortMaps renders the game's contiguous port run as Anvil mappings.
func gamePortMaps(p *ProvisionProfile, gamePort int) []anvilclient.PortMap {
	out := make([]anvilclient.PortMap, 0, p.portCount())
	for i := 0; i < p.portCount(); i++ {
		out = append(out, anvilclient.PortMap{Host: gamePort + i, Container: p.DefaultGamePort + i, Proto: "udp"})
	}
	return out
}

// Health synthesizes the legacy health shape from Anvil's. The wizard
// reads Provision.DataRoot to place data directories and PublicHost to
// prefill the join address; both come straight from this console's Anvil
// registration. ImageTag has no Anvil-side counterpart — the channel is
// the console's own default.
func (p *AnvilProvisioner) Health(ctx context.Context) (*agentctl.Health, error) {
	h, err := p.c.Health(ctx)
	if err != nil {
		return nil, translate(err)
	}
	return &agentctl.Health{
		Agent:      "anvil",
		Version:    h.Version,
		APIVersion: agent.APIVersion,
		Mode:       "provisioner",
		Provision: &agent.ProvisionDefaults{
			DataRoot:   h.DataRoot,
			PublicHost: h.PublicHost,
			RunAs:      h.RunAs,
			ImageTag:   "latest",
		},
	}, nil
}

// Provision translates the game-shaped request into Anvil's spec — the
// same assembly flameagent's own handler performs, kept in step with it until
// that handler is retired.
func (p *AnvilProvisioner) Provision(ctx context.Context, req agent.ProvisionRequest) (*agentctl.ProvisionResult, error) {
	env := map[string]string{
		"HOME":                            "/tmp",
		p.p.EnvPrefix + "_MODE":           "supervisor",
		p.p.EnvPrefix + "_TOKEN":          req.Token,
		p.p.EnvPrefix + "_ADMIN_PASSWORD": req.AdminPassword,
	}
	if req.JoinPassword != "" {
		env[p.p.EnvPrefix+"_JOIN_PASSWORD"] = req.JoinPassword
	}
	if req.ServerName != "" {
		env[p.p.EnvPrefix+"_SERVER_NAME"] = req.ServerName
	}
	if req.OwnerID != "" {
		env[p.p.EnvPrefix+"_OWNER_ID"] = req.OwnerID
	}
	if req.WorldName != "" {
		env[p.p.EnvPrefix+"_WORLD_NAME"] = req.WorldName
	}
	if req.ServerDesc != "" {
		env[p.p.EnvPrefix+"_SERVER_DESC"] = req.ServerDesc
	}
	tag := req.ImageTag
	if tag == "" {
		tag = "latest"
	}
	res, err := p.c.Provision(ctx, anvilclient.Spec{
		Name:  p.p.AgentName + "-" + req.Slug,
		Slug:  req.Slug,
		Image: p.p.ImageRepo + ":" + tag,
		User:  req.RunAs,
		Env:   env,
		Ports: append(append(gamePortMaps(p.p, req.GamePort), adminPortMaps(p.p, req)...),
			anvilclient.PortMap{Host: req.AgentPort, Container: agentContainerPort, Proto: "tcp"}),
		DataMount: p.p.MountPath,
	})
	if err != nil {
		return nil, translate(err)
	}
	return &agentctl.ProvisionResult{Container: res.Container, DataDir: res.DataDir}, nil
}

// Discover maps Anvil's candidates into the legacy shape. Mode is honest
// emptiness: Anvil does not read container environments for discovery, so
// unlike the old provisioner it cannot say supervisor/companion — and it
// cannot filter out a still-running legacy provisioner container either,
// which will appear in the list until Phase 4 removes it. Adopt refuses it
// with a clear message if selected.
func (p *AnvilProvisioner) Discover(ctx context.Context) ([]agentctl.DiscoveredServer, error) {
	found, err := p.c.Discover(ctx)
	if err != nil {
		return nil, translate(err)
	}
	out := make([]agentctl.DiscoveredServer, 0, len(found))
	for _, f := range found {
		// Anvil sees every console's containers on the host and cannot
		// tell them apart — that is its contract. The adapter can: only
		// this console's agent family belongs in its discovery list, or
		// wildskeeper offers to adopt flametender's servers (and a
		// mistaken adopt is a row whose game client can only fail).
		if !p.ours(f.Name, f.Image) {
			continue
		}
		d := agentctl.DiscoveredServer{
			Name:      f.Name,
			Image:     f.Image,
			Running:   f.Running,
			GamePort:  hostPortFor(f.Ports, p.p.DefaultGamePort, "udp"),
			AgentPort: hostPortFor(f.Ports, agentContainerPort, "tcp"),
		}
		if ap := p.p.adminPort("rest"); ap != nil {
			d.RESTPort = hostPortFor(f.Ports, ap.Container, "tcp")
		}
		if ap := p.p.adminPort("rcon"); ap != nil {
			d.RCONPort = hostPortFor(f.Ports, ap.Container, "tcp")
		}
		out = append(out, d)
	}
	return out, nil
}

// Adopt recovers a registration from the env Anvil returns — already
// filtered to this console's registered env prefix, so reading it here
// is reading our own writes back.
func (p *AnvilProvisioner) Adopt(ctx context.Context, container string) (*agentctl.AdoptResult, error) {
	a, err := p.c.Adopt(ctx, container)
	if err != nil {
		return nil, translate(err)
	}
	if !p.ours(a.Name, a.Image) {
		return nil, fmt.Errorf("%s is another console's server (this console adopts %s-* containers, or anything running the %s image)", container, p.p.AgentName, p.p.ImageRepo)
	}
	mode := a.Env[p.p.EnvPrefix+"_MODE"]
	if mode == "" {
		mode = "companion" // the agent's default when unset
	}
	if mode == "provisioner" {
		// The legacy provisioner container is discoverable (it runs the
		// agent image, unlabelled) but is not a game server. The old
		// provisioner filtered it out of discovery; Anvil cannot, so the
		// refusal lands here instead.
		return nil, fmt.Errorf("%s is a provisioner, not a game server — it is retired in the last step of the Anvil migration", container)
	}
	res := &agentctl.AdoptResult{
		Name:          a.Name,
		Mode:          mode,
		ServerName:    a.Env[p.p.EnvPrefix+"_SERVER_NAME"],
		Token:         a.Env[p.p.EnvPrefix+"_TOKEN"],
		AdminPassword: a.Env[p.p.EnvPrefix+"_ADMIN_PASSWORD"],
		GamePort:      hostPortFor(a.Ports, p.p.DefaultGamePort, "udp"),
		AgentPort:     hostPortFor(a.Ports, agentContainerPort, "tcp"),
	}
	if ap := p.p.adminPort("rest"); ap != nil {
		res.RESTPort = hostPortFor(a.Ports, ap.Container, "tcp")
	}
	if ap := p.p.adminPort("rcon"); ap != nil {
		res.RCONPort = hostPortFor(a.Ports, ap.Container, "tcp")
	}
	return res, nil
}

func (p *AnvilProvisioner) RecreateAgent(ctx context.Context, container, imageTag string) (*agent.RecreateResult, error) {
	res, err := p.c.Recreate(ctx, container, p.p.ImageRepo+":"+imageTag)
	if err != nil {
		return nil, translate(err)
	}
	return &agent.RecreateResult{Container: res.Container, Image: res.Image, Previous: res.Previous}, nil
}

func (p *AnvilProvisioner) Destroy(ctx context.Context, container string) (*agentctl.DestroyResult, error) {
	res, err := p.c.Destroy(ctx, container)
	if err != nil {
		return nil, translate(err)
	}
	return &agentctl.DestroyResult{Container: res.Container, DataDir: res.DataDir}, nil
}

// HostPorts asks Anvil what the machine already publishes. Every port,
// not just this console's: the wizard proposes a port run for a container
// that has to coexist with everything else on the box, and the collision
// this service exists to prevent is precisely one console proposing a port
// another console holds.
func (p *AnvilProvisioner) HostPorts(ctx context.Context) ([]int, error) {
	taken, err := p.c.Ports(ctx)
	if err != nil {
		return nil, translate(err)
	}
	out := make([]int, 0, len(taken))
	for _, t := range taken {
		out = append(out, t.Port)
	}
	return out, nil
}

// hostPortFor finds the published host port for a container-side port, the
// way the old provisioner read its well-known ports.
func hostPortFor(ports []anvilclient.PortMap, containerPort int, proto string) int {
	for _, p := range ports {
		if p.Container == containerPort && (p.Proto == proto || p.Proto == "") {
			return p.Host
		}
	}
	return 0
}
