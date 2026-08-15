package enshrouded

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/safwyls/flametender/internal/agentctl"
	"github.com/safwyls/flametender/internal/game"
	"github.com/safwyls/flametender/internal/games/enshrouded/eslog"
)

// errNoAgent is the config-level failure: without a sidecar there is no
// transport to derive anything from. It maps to 502 like any other
// unreachable-admin-interface error, which is accurate — the fix is
// configuration, not a missing game capability.
var errNoAgent = errors.New("enshrouded servers are managed through a flameagent sidecar; set the server's agent URL and token")

// trackers is the per-server session state, keyed by agent URL. Clients
// are rebuilt from the row on every API call (store.Server.Client), so
// the state a log-derived player list needs has to outlive them; keying
// on the agent URL keeps one tracker per server without the client
// knowing row ids.
var (
	trackersMu sync.Mutex
	trackers   = map[string]*eslog.Tracker{}
)

func trackerFor(agentURL string) *eslog.Tracker {
	trackersMu.Lock()
	defer trackersMu.Unlock()
	t, ok := trackers[agentURL]
	if !ok {
		t = eslog.NewTracker(eslog.RulesV1)
		trackers[agentURL] = t
	}
	return t
}

// logTail is how much of the agent's ring each refresh asks for — the
// whole of it, because the tracker anchors incrementally and the ring is
// capped at the same figure agent-side.
const logTail = 2000

// Client derives Enshrouded state through the flameagent sidecar. It
// implements game.Client with the honest subset: Info, Players and
// Metrics work; every command returns game.UnsupportedError, because the
// game offers no channel to carry one (no RCON, no API, no console — see
// docs/enshrouded-recon.md, "Query/admin surface").
type Client struct {
	agent    *agentctl.Client
	agentErr error
	tracker  *eslog.Tracker
}

// New builds the client for one server. A missing or malformed agent URL
// is carried as a deferred error rather than returned, because
// game.Definition's NewClient contract has no error path — the first
// call reports it instead.
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
		return nil, errors.New("agent is not supervising a game process (companion mode); enshrouded needs supervisor mode")
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
// empty: the log stream doesn't carry it, and inventing it from the row
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

// Players is the log-derived session list. Enshrouded's log identifies
// players by SteamID64 only — names never appear in it — so the id is
// both the identity and, for now, the display name. The A2S player query
// (roadmap Phase 2) is the path to real names.
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
		uid := CanonicalUID(s.SteamID)
		name := uid
		if name == "" {
			// A session whose accepted-id line scrolled past unseen still
			// deserves a row; the peer number is the only handle left.
			name = fmt.Sprintf("Peer #%d", s.Peer)
			uid = name
		}
		players = append(players, game.Player{
			Name:      name,
			PlayerUID: uid,
			UserID:    uid,
		})
	}
	return players, nil
}

// The command tier. Enshrouded has no command transport at all — no RCON,
// no HTTP API, no console, and (unlike Dragonwilds' UE4SS bridge) no
// injection surface on its proprietary engine — so every command returns
// *game.UnsupportedError, which the API maps to 501: capability truth
// ("this game can't"), never a fault. The reasons say where the ability
// actually lives, because most of these exist in-game.

const (
	reasonNoBroadcast = "Enshrouded has no server-to-player messaging channel; announcements have to happen in-game or out of band (the scheduler's Discord notices cover restarts)"
	reasonModeration  = "kick and ban live in the in-game player menu, for anyone who joined with a kick/ban-capable role password; the server has no admin API to do it from outside"
	reasonNoSave      = "the server autosaves every 10 minutes and saves on shutdown; there is no on-demand save trigger"
	reasonNoShutdown  = "no in-game shutdown exists; stop the server through the agent power controls, which allow a grace period (the game saves on the way down)"
	reasonNoSettings  = "the game has no live settings query; edit enshrouded_server.json from the Configuration view (changes apply on restart)"
)

func commandReason(op string) string {
	switch op {
	case "broadcast":
		return reasonNoBroadcast
	case "kick", "ban", "unban":
		return reasonModeration
	case "save":
		return reasonNoSave
	case "shutdown":
		return reasonNoShutdown
	default:
		return "Enshrouded has no command channel (no RCON, no admin API)"
	}
}

// Supports answers the capability question without firing the command, so
// the console can say what a server will do before anyone clicks. For
// Enshrouded the answer is a stable no for every op — this exists so the
// shared layer's "no prober means everything works" default never
// promises what the 501s below would refuse.
func (c *Client) Supports(ctx context.Context, op string) (bool, string) {
	return false, commandReason(op)
}

func unsupported(op string) error {
	return &game.UnsupportedError{Op: op, Reason: commandReason(op)}
}

func (c *Client) Broadcast(ctx context.Context, message string) error {
	return unsupported("broadcast")
}

func (c *Client) Kick(ctx context.Context, playerUID, message string) error {
	return unsupported("kick")
}

func (c *Client) Ban(ctx context.Context, playerUID, message string) error {
	return unsupported("ban")
}

func (c *Client) Unban(ctx context.Context, playerUID string) error {
	return unsupported("unban")
}

func (c *Client) Save(ctx context.Context) error {
	return unsupported("save")
}

// Shutdown stays pointed at the real mechanism: stopping the process is
// the agent's job, and Enshrouded makes that safe — the server writes the
// world on the way down (recon doc, "Runtime behavior"), so the power
// stop's grace period is the clean shutdown.
func (c *Client) Shutdown(ctx context.Context, waitSeconds int, message string) error {
	return unsupported("shutdown")
}

// Metrics lets the collector chart what the derived view does know:
// player count and process uptime. MaxSlots is the game's hard cap, not
// the configured slotCount — the log stream doesn't carry the config, and
// the A2S query (Phase 2) is the honest source for the real number. FPS
// and frame time stay zero, which charts read correctly as "not
// reported".
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
		MaxPlayerNum:     MaxSlots,
		UptimeSeconds:    int(time.Since(st.StartedAt).Seconds()),
	}, nil
}

// Settings has no live transport — the game can't be asked for its
// config, only the JSON read at rest, which is the config editor's job
// (esconfig), not this client's.
func (c *Client) Settings(ctx context.Context) (map[string]any, error) {
	return nil, &game.UnsupportedError{Op: "settings", Reason: reasonNoSettings}
}
