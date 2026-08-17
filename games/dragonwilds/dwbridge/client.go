package dwbridge

// The console-side half: dwbridge verbs over the core agent client
// (drift ledger, agentctl.go row — the four Dragonwilds methods relocate
// here as an extension embedding the core client rather than forking it).

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/safwyls/artificer/core/agent"
	"github.com/safwyls/artificer/core/agentctl"
)

// AgentClient extends the core agent client with the dwbridge verbs.
type AgentClient struct {
	*agentctl.Client
}

// Wrap builds the extension over an already-configured core client.
func Wrap(c *agentctl.Client) *AgentClient { return &AgentClient{Client: c} }

// BridgeCommand relays a dwbridge command to the agent, returning the
// mod's data payload (which may be nil). The error vocabulary is the
// shared one: ErrRejected when the mod doesn't implement the command
// (the agent's 400), and a plain error carrying the agent's message on
// 503 when the bridge is down. The timeout is generous because the far
// end is a game tick, not an HTTP handler — the agent applies its own
// tighter bound on the mod round trip and this only needs to outlast it.
func (c *AgentClient) BridgeCommand(ctx context.Context, command string, args map[string]string) (json.RawMessage, error) {
	var res struct {
		Data json.RawMessage `json:"data"`
	}
	body := map[string]any{"command": command, "args": args}
	if err := c.Do(ctx, http.MethodPost, "/v1/bridge/command", body, &res, 30*time.Second); err != nil {
		return nil, err
	}
	return res.Data, nil
}

// BridgeState fetches the live telemetry the dwbridge mod publishes
// (player roster with positions, world clock). Available=false is a
// normal answer on a modless or stopped server, not an error.
func (c *AgentClient) BridgeState(ctx context.Context) (*BridgeState, error) {
	var out BridgeState
	if err := c.Do(ctx, http.MethodGet, "/v1/bridge/state", nil, &out, 10*time.Second); err != nil {
		return nil, err
	}
	return &out, nil
}

// InstallBridgeKit asks the agent to lay its baked-in UE4SS+dwbridge kit
// next to the server exe (Wine image only; the plain image answers 501).
// RestartRequired is true when the game was running, since the mod only
// loads at process start.
func (c *AgentClient) InstallBridgeKit(ctx context.Context) (restartRequired bool, err error) {
	var out struct {
		RestartRequired bool `json:"restartRequired"`
	}
	// The kit is a few hundred files; give a slow volume more than the
	// default verb timeout.
	if err := c.Do(ctx, http.MethodPost, "/v1/bridge/install", nil, &out, 60*time.Second); err != nil {
		return false, err
	}
	return out.RestartRequired, nil
}

// SetLaunchProfile chooses which of the game's builds the agent starts
// next — native Linux, or the Windows build under Wine that can carry
// the dwbridge mod. It applies at the next start by design: the two
// builds come from different Steam depots, so switching is a re-install
// rather than a restart, and the agent refuses to decide that timing
// for anyone.
func (c *AgentClient) SetLaunchProfile(ctx context.Context, profile string) (*agent.LaunchStatus, error) {
	var out agent.LaunchStatus
	body := map[string]string{"profile": profile}
	if err := c.Do(ctx, http.MethodPut, "/v1/launch", body, &out, 15*time.Second); err != nil {
		return nil, err
	}
	return &out, nil
}

// StatusFromHealth recovers the bridge status a dwagent merged into the
// health payload (Health.Extra round-trips it through typed relays).
// Nil means no bridge directory exists at all — the common no-mod case.
func StatusFromHealth(h *agent.Health) *BridgeStatus {
	if h == nil || h.Extra == nil {
		return nil
	}
	raw, ok := h.Extra["bridge"]
	if !ok || raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var st BridgeStatus
	if err := json.Unmarshal(data, &st); err != nil {
		return nil
	}
	return &st
}
