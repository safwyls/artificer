package agentctl

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// QueryInfo is the console-side view of the game's own query reply (Steam
// A2S for the games that answer it). The agent runs the wire protocol
// with the game module's decoder — the query must originate next to the
// game — and what crosses to the console is this neutral JSON shape.
type QueryInfo struct {
	// Name is the server-browser name — the game's own copy of the
	// config's name, and so the first place a config edit that never
	// reached the game would show up.
	Name string `json:"name"`
	// Map is the world/level name.
	Map string `json:"map"`
	// Players and MaxPlayers are the live count and the configured slot
	// count — the latter often unavailable any other way.
	Players    int `json:"players"`
	MaxPlayers int `json:"maxPlayers"`
	Bots       int `json:"bots"`
	// Version is the game build string as the server reports it.
	Version string `json:"version"`
	// AppID is the Steam app the server claims to be, zero when the reply
	// couldn't say honestly — a wrong id is worse than a missing one for
	// the only check this exists to serve.
	AppID int `json:"appId"`
	// Protocol and VAC are reported for completeness.
	Protocol byte `json:"protocol"`
	VAC      bool `json:"vac"`
}

// QueryPlayer is one row of the player reply. Note what is *not* here: an
// account id. A2S returns names and durations only, so this can say how
// many people are on and how long, but never who they are in a bannable
// way — the log tracker stays the roster's source and this stays the
// presence check.
type QueryPlayer struct {
	Name    string  `json:"name"`
	Score   int32   `json:"score"`
	Seconds float64 `json:"seconds"`
}

// QueryResult is the agent's relay of the game's own query reply.
type QueryResult struct {
	Info    *QueryInfo    `json:"info"`
	Players []QueryPlayer `json:"players"`
	// PlayersError is set when the info query answered but the player list
	// didn't. The count in Info is still good; only the names are missing.
	PlayersError string `json:"playersError,omitempty"`
}

// Query asks the agent to run the game's own query against it.
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
