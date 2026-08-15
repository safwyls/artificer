package agentctl

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/safwyls/flamekeeper/internal/flameagent"
)

// GameStatus mirrors the agent's wire type, like Job and Health.
type GameStatus = flameagent.GameStatus

// Power performs start/stop/restart on a supervisor-mode agent's game and
// returns the post-action status. Stop legitimately waits out the game's
// grace period, so the timeout mirrors dockerctl's stop margin.
//
// graceful (stop/restart only) tells the agent the game has already been
// asked to shut itself down in-game and that exit should be given that
// long to finish before the process is signalled; the agent waits it out,
// so it extends this call's budget too. Zero means signal immediately.
func (c *Client) Power(ctx context.Context, action string, graceful time.Duration) (*GameStatus, error) {
	var res struct {
		Game *GameStatus `json:"game"`
	}
	path := "/v1/power/" + action
	if graceful > 0 {
		path += "?graceful=" + graceful.String()
	}
	if err := c.do(ctx, http.MethodPost, path, nil, &res, 90*time.Second+graceful); err != nil {
		return nil, fmt.Errorf("agent %s: %w", action, err)
	}
	return res.Game, nil
}

// GameLogs returns the supervised game's recent output.
func (c *Client) GameLogs(ctx context.Context, tail int) ([]string, error) {
	var res struct {
		Lines []string `json:"lines"`
	}
	path := "/v1/power/logs?tail=" + strconv.Itoa(tail)
	if err := c.do(ctx, http.MethodGet, path, nil, &res, 20*time.Second); err != nil {
		return nil, err
	}
	return res.Lines, nil
}

// The provisioning wire vocabulary, re-exported so the console's
// provisioning code (the wizard, the Ilmari adapter) speaks one set of
// names. The HTTP methods that used to drive a provisioner-mode agent are
// gone with that mode — Ilmari is the only placer of containers now.

// ProvisionResult reports what provisioning created.
type ProvisionResult struct {
	Container string `json:"container"`
	ID        string `json:"id"`
	DataDir   string `json:"dataDir"`
}

// DiscoveredServer mirrors the agent wire type.
type DiscoveredServer = flameagent.DiscoveredServer

// DestroyResult mirrors the agent wire type.
type DestroyResult = flameagent.DestroyResult

// AdoptResult mirrors the agent wire type.
type AdoptResult = flameagent.AdoptResult
