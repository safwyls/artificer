package agentctl

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/safwyls/flametender/internal/games/enshrouded/esquery"
)

// QueryResult is the agent's relay of the game's own Steam query reply.
//
// It goes through the agent rather than being run from here — see
// internal/flameagent/query.go for why — but the payload is the game's,
// unmodified.
type QueryResult struct {
	Info    *esquery.Info         `json:"info"`
	Players []esquery.PlayerEntry `json:"players"`
	// PlayersError is set when the info query answered but the player list
	// didn't. The count in Info is still good; only the names are missing.
	PlayersError string `json:"playersError,omitempty"`
}

// Query asks the agent to run the Steam query against the game.
//
// A booting or unreachable server answers 503 through the agent, which
// arrives here as an ordinary error — callers treat that as "no query
// answer" and fall back to what the log tail knows, rather than as a
// failure worth surfacing.
func (c *Client) Query(ctx context.Context) (*QueryResult, error) {
	var res QueryResult
	// Tighter than the agent's own 3s query budget would suggest, because
	// the agent bounds the UDP wait itself; this only has to cover the
	// hop to it plus that wait.
	//
	// An agent too old to have this route answers 404, which arrives as
	// ErrNotFound and falls back like any other non-answer — a console
	// ahead of its agents loses the query, not the dashboard.
	if err := c.do(ctx, http.MethodGet, "/v1/query", nil, &res, 8*time.Second); err != nil {
		return nil, err
	}
	if res.Info == nil {
		return nil, errors.New("agent returned no query result")
	}
	return &res, nil
}
