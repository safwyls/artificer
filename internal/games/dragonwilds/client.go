package dragonwilds

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/safwyls/dwcon/internal/agentctl"
	"github.com/safwyls/dwcon/internal/game"
	"github.com/safwyls/dwcon/internal/games/dragonwilds/dwlog"
)

// errNoAgent is the config-level failure: without a sidecar there is no
// transport to derive anything from. It maps to 502 like any other
// unreachable-admin-interface error, which is accurate — the fix is
// configuration, not a missing game capability.
var errNoAgent = errors.New("dragonwilds servers are managed through a palagent sidecar; set the server's agent URL and token")

// trackers is the per-server session state, keyed by agent URL. Clients are
// rebuilt from the row on every API call (store.Server.Client), so the
// state a log-derived player list needs has to outlive them; keying on the
// agent URL keeps one tracker per server without the client knowing row ids.
var (
	trackersMu sync.Mutex
	trackers   = map[string]*dwlog.Tracker{}
)

func trackerFor(agentURL string) *dwlog.Tracker {
	trackersMu.Lock()
	defer trackersMu.Unlock()
	t, ok := trackers[agentURL]
	if !ok {
		t = dwlog.NewTracker(dwlog.RulesV0)
		trackers[agentURL] = t
	}
	return t
}

// logTail is how much of the agent's ring each refresh asks for — the whole
// of it, because the tracker anchors incrementally and the ring is capped
// at the same figure agent-side.
const logTail = 2000

// Client derives Dragonwilds state through the palagent sidecar. It
// implements game.Client with the honest subset: Info and Players work,
// commands return game.UnsupportedError until the dwbridge mod exists.
type Client struct {
	agent    *agentctl.Client
	agentErr error
	tracker  *dwlog.Tracker
}

// New builds the client for one server. A missing or malformed agent URL is
// carried as a deferred error rather than returned, because game.Definition's
// NewClient contract has no error path — the first call reports it instead.
func New(conn game.Conn) game.Client {
	if conn.AgentURL == "" {
		return &Client{agentErr: errNoAgent}
	}
	a, err := agentctl.New(conn.AgentURL, conn.AgentToken)
	if err != nil {
		return &Client{agentErr: fmt.Errorf("agent: %w", err)}
	}
	return &Client{agent: a, tracker: trackerFor(conn.AgentURL)}
}

// refresh polls the agent once and feeds the tracker: health for process
// state (and the restart-reset key), the log ring for events. It returns
// the game status so callers don't re-fetch.
func (c *Client) refresh(ctx context.Context) (*agentctl.GameStatus, error) {
	if c.agentErr != nil {
		return nil, c.agentErr
	}
	h, err := c.agent.Health(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent health: %w", err)
	}
	if h.Game == nil {
		return nil, errors.New("agent is not supervising a game process (companion mode); dragonwilds needs supervisor mode")
	}
	if h.Game.State != "running" {
		return h.Game, nil
	}
	lines, err := c.agent.GameLogs(ctx, logTail)
	if err != nil {
		return nil, fmt.Errorf("agent logs: %w", err)
	}
	c.tracker.Update(h.Game.StartedAt, lines)
	return h.Game, nil
}

// Info reports liveness and the derived player count. ServerName stays
// empty: the log stream doesn't carry it (v0) and inventing it from the row
// would just echo the user's own input back at them.
func (c *Client) Info(ctx context.Context) (*game.ServerInfo, error) {
	st, err := c.refresh(ctx)
	if err != nil {
		return nil, err
	}
	if st.State != "running" {
		// An error, not a zero Info: the dashboard's "unreachable" state is
		// the truthful rendering of a stopped or crashed process, and the
		// power panel (agent-backed) stays available alongside it.
		return nil, fmt.Errorf("server process is %s", st.State)
	}
	return &game.ServerInfo{
		PlayerCount: len(c.tracker.Sessions()),
		Transport:   "agent",
	}, nil
}

// Players is the log-derived session list. v0 identity is the player name
// for all three id fields — the only identity the verified log lines carry.
// The collector keys sessions by UserID, so it must be stable and unique
// per player, which names are on a six-slot friends server; real ids take
// over when a log corpus provides them (recon: open gate 1).
func (c *Client) Players(ctx context.Context) ([]game.Player, error) {
	st, err := c.refresh(ctx)
	if err != nil {
		return nil, err
	}
	if st.State != "running" {
		return nil, fmt.Errorf("server process is %s", st.State)
	}
	sessions := c.tracker.Sessions()
	players := make([]game.Player, 0, len(sessions))
	for _, s := range sessions {
		players = append(players, game.Player{
			Name:      s.Name,
			PlayerUID: s.Name,
			UserID:    s.Name,
		})
	}
	return players, nil
}

// The command tier. Every reason names the real constraint so the 501
// surfaces in the UI as capability truth, not a fault.

func (c *Client) Broadcast(ctx context.Context, message string) error {
	return &game.UnsupportedError{Op: "broadcast", Reason: "the game has no native console; in-game messages need the dwbridge mod"}
}

func (c *Client) Kick(ctx context.Context, playerUID, message string) error {
	return &game.UnsupportedError{Op: "kick", Reason: "the game has no native console; kicking needs the dwbridge mod or the in-game Server Management menu"}
}

func (c *Client) Ban(ctx context.Context, playerUID, message string) error {
	return &game.UnsupportedError{Op: "ban", Reason: "bans are managed in-game via Server Management; no on-disk ban list is known to edit"}
}

func (c *Client) Unban(ctx context.Context, playerUID string) error {
	return &game.UnsupportedError{Op: "unban", Reason: "only the server Owner can unban, in-game via Server Management"}
}

func (c *Client) Save(ctx context.Context) error {
	return &game.UnsupportedError{Op: "save", Reason: "the game exposes no save command; autosave covers running servers and backups snapshot the save directory"}
}

func (c *Client) Shutdown(ctx context.Context, waitSeconds int, message string) error {
	return &game.UnsupportedError{Op: "shutdown", Reason: "no in-game shutdown exists; stop the server through the agent power controls, which allow a grace period"}
}

// Metrics lets the collector chart what the derived view does know: player
// count against the hard six-slot cap, and process uptime. FPS and frame
// time stay zero — nothing reports them — which charts read correctly as
// "not reported".
func (c *Client) Metrics(ctx context.Context) (*game.Metrics, error) {
	st, err := c.refresh(ctx)
	if err != nil {
		return nil, err
	}
	if st.State != "running" {
		return nil, fmt.Errorf("server process is %s", st.State)
	}
	return &game.Metrics{
		CurrentPlayerNum: len(c.tracker.Sessions()),
		MaxPlayerNum:     MaxPlayers,
		UptimeSeconds:    int(time.Since(st.StartedAt).Seconds()),
	}, nil
}

// Settings has no live transport — the game can't be asked for its config,
// only the ini read at rest, which is the config editor's job (dwconfig),
// not this client's.
func (c *Client) Settings(ctx context.Context) (map[string]any, error) {
	return nil, &game.UnsupportedError{Op: "settings", Reason: "the game has no live settings query; edit DedicatedServer.ini from the Configuration view"}
}
