package api

import (
	"context"
	"fmt"

	"github.com/safwyls/sampo/core/agent"
	"github.com/safwyls/sampo/core/agentctl"
	"github.com/safwyls/sampo/core/ilmariclient"
)

// Provisioner is what the API layer needs from whatever places containers
// on the host. Exactly one implementation exists — Ilmari, the shared host
// service — but the seam stays an interface: tests fake it, and the wizard
// speaks these shapes rather than Ilmari's wire types.
type Provisioner interface {
	BaseURL() string
	Health(ctx context.Context) (*agentctl.Health, error)
	Provision(ctx context.Context, req agent.ProvisionRequest) (*agentctl.ProvisionResult, error)
	Discover(ctx context.Context) ([]agentctl.DiscoveredServer, error)
	Adopt(ctx context.Context, container string) (*agentctl.AdoptResult, error)
	RecreateAgent(ctx context.Context, container, imageTag string) (*agent.RecreateResult, error)
	Destroy(ctx context.Context, container string) (*agentctl.DestroyResult, error)
}

// Interface satisfaction is a compile-time fact, not a hope.
var _ Provisioner = (*IlmariProvisioner)(nil)

// IlmariProvisioner adapts the Ilmari host service to the Provisioner
// interface.
//
// The game knowledge that used to live in each agent's provisioner mode
// lives in the ProvisionProfile this adapter carries: which env vars
// configure the sidecar, which ports the game claims, which image family
// to deploy. Ilmari itself knows none of it — that is the contract — so
// anything game-shaped in a provisioning flow belongs in the profile and
// nowhere further down. (The stale note this comment once carried about
// "a Dragonwilds sidecar" in an Enshrouded console was drift-ledger
// evidence; it dies with the parameterization.)
type IlmariProvisioner struct {
	c *ilmariclient.Client
	p *ProvisionProfile
}

func NewIlmariProvisioner(c *ilmariclient.Client, p *ProvisionProfile) *IlmariProvisioner {
	return &IlmariProvisioner{c: c, p: p}
}

func (p *IlmariProvisioner) BaseURL() string { return p.c.BaseURL() }

// agentContainerPort is the one container-side port fact that is the
// protocol's, not a game's: every sidecar agent serves its API on 8811.
const agentContainerPort = 8811

// gamePortMaps renders the game's contiguous port run as Ilmari mappings.
func gamePortMaps(p *ProvisionProfile, gamePort int) []ilmariclient.PortMap {
	out := make([]ilmariclient.PortMap, 0, p.portCount())
	for i := 0; i < p.portCount(); i++ {
		out = append(out, ilmariclient.PortMap{Host: gamePort + i, Container: p.DefaultGamePort + i, Proto: "udp"})
	}
	return out
}

// Health synthesizes the legacy health shape from Ilmari's. The wizard
// reads Provision.DataRoot to place data directories and PublicHost to
// prefill the join address; both come straight from this console's Ilmari
// registration. ImageTag has no Ilmari-side counterpart — the channel is
// the console's own default.
func (p *IlmariProvisioner) Health(ctx context.Context) (*agentctl.Health, error) {
	h, err := p.c.Health(ctx)
	if err != nil {
		return nil, err
	}
	return &agentctl.Health{
		Agent:      "ilmari",
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

// Provision translates the game-shaped request into Ilmari's spec — the
// same assembly flameagent's own handler performs, kept in step with it until
// that handler is retired.
func (p *IlmariProvisioner) Provision(ctx context.Context, req agent.ProvisionRequest) (*agentctl.ProvisionResult, error) {
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
	tag := req.ImageTag
	if tag == "" {
		tag = "latest"
	}
	res, err := p.c.Provision(ctx, ilmariclient.Spec{
		Name:  p.p.AgentName + "-" + req.Slug,
		Slug:  req.Slug,
		Image: p.p.ImageRepo + ":" + tag,
		User:  req.RunAs,
		Env:   env,
		Ports: append(gamePortMaps(p.p, req.GamePort),
			ilmariclient.PortMap{Host: req.AgentPort, Container: agentContainerPort, Proto: "tcp"}),
		DataMount: p.p.MountPath,
	})
	if err != nil {
		return nil, err
	}
	return &agentctl.ProvisionResult{Container: res.Container, DataDir: res.DataDir}, nil
}

// Discover maps Ilmari's candidates into the legacy shape. Mode is honest
// emptiness: Ilmari does not read container environments for discovery, so
// unlike the old provisioner it cannot say supervisor/companion — and it
// cannot filter out a still-running legacy provisioner container either,
// which will appear in the list until Phase 4 removes it. Adopt refuses it
// with a clear message if selected.
func (p *IlmariProvisioner) Discover(ctx context.Context) ([]agentctl.DiscoveredServer, error) {
	found, err := p.c.Discover(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]agentctl.DiscoveredServer, 0, len(found))
	for _, f := range found {
		out = append(out, agentctl.DiscoveredServer{
			Name:      f.Name,
			Image:     f.Image,
			Running:   f.Running,
			GamePort:  hostPortFor(f.Ports, p.p.DefaultGamePort, "udp"),
			AgentPort: hostPortFor(f.Ports, agentContainerPort, "tcp"),
		})
	}
	return out, nil
}

// Adopt recovers a registration from the env Ilmari returns — already
// filtered to this console's registered env prefix, so reading it here
// is reading our own writes back.
func (p *IlmariProvisioner) Adopt(ctx context.Context, container string) (*agentctl.AdoptResult, error) {
	a, err := p.c.Adopt(ctx, container)
	if err != nil {
		return nil, err
	}
	mode := a.Env[p.p.EnvPrefix+"_MODE"]
	if mode == "" {
		mode = "companion" // the agent's default when unset
	}
	if mode == "provisioner" {
		// The legacy provisioner container is discoverable (it runs the
		// agent image, unlabelled) but is not a game server. The old
		// provisioner filtered it out of discovery; Ilmari cannot, so the
		// refusal lands here instead.
		return nil, fmt.Errorf("%s is a provisioner, not a game server — it is retired in the last step of the Ilmari migration", container)
	}
	return &agentctl.AdoptResult{
		Name:          a.Name,
		Mode:          mode,
		ServerName:    a.Env[p.p.EnvPrefix+"_SERVER_NAME"],
		Token:         a.Env[p.p.EnvPrefix+"_TOKEN"],
		AdminPassword: a.Env[p.p.EnvPrefix+"_ADMIN_PASSWORD"],
		GamePort:      hostPortFor(a.Ports, p.p.DefaultGamePort, "udp"),
		AgentPort:     hostPortFor(a.Ports, agentContainerPort, "tcp"),
	}, nil
}

func (p *IlmariProvisioner) RecreateAgent(ctx context.Context, container, imageTag string) (*agent.RecreateResult, error) {
	res, err := p.c.Recreate(ctx, container, p.p.ImageRepo+":"+imageTag)
	if err != nil {
		return nil, err
	}
	return &agent.RecreateResult{Container: res.Container, Image: res.Image, Previous: res.Previous}, nil
}

func (p *IlmariProvisioner) Destroy(ctx context.Context, container string) (*agentctl.DestroyResult, error) {
	res, err := p.c.Destroy(ctx, container)
	if err != nil {
		return nil, err
	}
	return &agentctl.DestroyResult{Container: res.Container, DataDir: res.DataDir}, nil
}

// hostPortFor finds the published host port for a container-side port, the
// way the old provisioner read its well-known ports.
func hostPortFor(ports []ilmariclient.PortMap, containerPort int, proto string) int {
	for _, p := range ports {
		if p.Container == containerPort && (p.Proto == proto || p.Proto == "") {
			return p.Host
		}
	}
	return 0
}
