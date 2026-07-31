package agentctl

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/safwyls/palcon/internal/palagent"
)

// GameStatus mirrors the agent's wire type, like Job and Health.
type GameStatus = palagent.GameStatus

// Power performs start/stop/restart on a supervisor-mode agent's game and
// returns the post-action status. Stop legitimately waits out the game's
// grace period, so the timeout mirrors dockerctl's stop margin.
func (c *Client) Power(ctx context.Context, action string) (*GameStatus, error) {
	var res struct {
		Game *GameStatus `json:"game"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/power/"+action, nil, &res, 90*time.Second); err != nil {
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

// ProvisionResult reports what a provisioner-mode agent created.
type ProvisionResult struct {
	Container string `json:"container"`
	ID        string `json:"id"`
	DataDir   string `json:"dataDir"`
}

// Provision asks a provisioner-mode agent to instantiate the Palworld
// supervisor template. The generous timeout covers the image pull a first
// provision performs.
func (c *Client) Provision(ctx context.Context, req palagent.ProvisionRequest) (*ProvisionResult, error) {
	var res ProvisionResult
	if err := c.do(ctx, http.MethodPost, "/v1/provision", req, &res, 10*time.Minute); err != nil {
		return nil, err
	}
	return &res, nil
}
