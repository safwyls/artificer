package enshrouded

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/safwyls/sampo/core/agentctl"
	"github.com/safwyls/sampo/core/game"
	"github.com/safwyls/sampo/games/enshrouded/eslog"
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
		t = eslog.NewTracker(eslog.RulesV2)
		trackers[agentURL] = t
	}
	return t
}

// logTail is how much of the agent's ring each refresh asks for — the
// whole of it, because the tracker anchors incrementally and the ring is
// capped at the same figure agent-side.
const logTail = 2000

// The Steam query, cached per server.
//
// One dashboard load calls Info, Players and Metrics, and all three want
// the same answer; without a cache that is three UDP round trips through
// the agent for one screen. The window is short enough that "right now"
// stays true — the whole point of asking the game rather than reading the
// log is that it is current.
const queryTTL = 5 * time.Second

type queryState struct {
	mu  sync.Mutex
	at  time.Time
	res *agentctl.QueryResult
	err error
}

var (
	queriesMu sync.Mutex
	queries   = map[string]*queryState{}
)

func queryStateFor(agentURL string) *queryState {
	queriesMu.Lock()
	defer queriesMu.Unlock()
	q, ok := queries[agentURL]
	if !ok {
		q = &queryState{}
		queries[agentURL] = q
	}
	return q
}

// query returns the game's own answer, or an error if it didn't give one.
//
// Both outcomes are ordinary. A server that is still booting, or one
// whose port is firewalled even from the container, simply doesn't
// answer, and every caller here treats that as "fall back to what the log
// knows" rather than as a failure worth showing anyone.
func (c *Client) query(ctx context.Context) (*agentctl.QueryResult, error) {
	if c.agent == nil {
		return nil, errNoAgent
	}
	c.queries.mu.Lock()
	defer c.queries.mu.Unlock()
	if time.Since(c.queries.at) < queryTTL {
		return c.queries.res, c.queries.err
	}
	res, err := c.agent.Query(ctx)
	c.queries.at = time.Now()
	c.queries.res, c.queries.err = res, err
	return res, err
}

// queryInfo is query() narrowed to the reply body, for the callers that
// only want the facts and not the failure.
func (c *Client) queryInfo(ctx context.Context) *agentctl.QueryInfo {
	res, err := c.query(ctx)
	if err != nil || res == nil {
		return nil
	}
	return res.Info
}

// Client derives Enshrouded state through the flameagent sidecar. It
// implements game.Client with the honest subset: Info, Players and
// Metrics work; every command returns game.UnsupportedError, because the
// game offers no channel to carry one (no RCON, no API, no console — see
// docs/enshrouded-recon.md, "Query/admin surface").
type Client struct {
	agent    *agentctl.Client
	agentErr error
	tracker  *eslog.Tracker
	queries  *queryState
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
	return &Client{agent: a, tracker: trackerFor(conn.AgentURL), queries: queryStateFor(conn.AgentURL)}
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

// readyWindow is how long a process gets to log its host-online line
// before its absence stops meaning "still starting".
//
// The marker only exists once in the log, at boot, and the agent's ring
// holds roughly 80 minutes. A console that starts watching an
// already-running server therefore never sees it — and reporting that
// server as "starting" forever would be worse than reporting nothing.
// Past this window, absence is ignorance rather than evidence.
const readyWindow = 15 * time.Minute

// readiness answers the question "running" can't: whether anyone can
// actually join yet. A booting Enshrouded server binds its port and loads
// the world well before it accepts a connection, so this is the
// difference between a friend joining and a friend getting an error.
//
// Only the log's `HostOnline` line is treated as proof. The Steam query
// answering is deliberately *not*: the game and the query share one port,
// so a reply says the socket is up, which is exactly the thing that
// happens too early.
func (c *Client) readiness(st *agentctl.GameStatus) string {
	if c.tracker.Ready() {
		return game.ReadinessReady
	}
	if !st.StartedAt.IsZero() && time.Since(st.StartedAt) < readyWindow {
		return game.ReadinessStarting
	}
	return ""
}

// Info reports liveness, presence and — when the game answers its Steam
// query — the facts only the game itself holds.
//
// The two sources are kept in their lanes. The Steam query owns the
// present: it is the game's own count, and unlike the log it cannot be
// missing a player whose join line has scrolled out of the agent's ring.
// The log owns everything the query can't express — who those players
// are, and whether the server has finished coming up. When the query
// doesn't answer, the log-derived count stands in, which is the old
// behaviour and still the right fallback.
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
	info := &game.ServerInfo{
		PlayerCount: len(c.tracker.Sessions()),
		Transport:   "agent",
		Readiness:   c.readiness(st),
	}
	if q := c.queryInfo(ctx); q != nil {
		info.PlayerCount = q.Players
		// The name is the game's own copy of the config's `name`, which
		// makes it the one place an edit that never reached the game would
		// show — worth more than echoing the row's name back.
		info.ServerName = q.Name
		info.Version = q.Version
		info.Transport = "agent+a2s"
	}
	return info, nil
}

// Players is the log-derived session list. The join line carries the
// SteamID64 and a login line a few lines later carries the display name
// (verified against a real server, 2026-08-15), so both are real here:
// the id is the identity, the name is the label. A session whose name
// line hasn't arrived — or scrolled past before the console started
// watching — falls back to the id, then to the peer index; a roster row
// with a worse label beats a missing player.
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
		name := s.Name
		if name == "" {
			name = uid
		}
		if uid == "" {
			// Nothing identifying survived; the peer index is the only
			// handle left, and it is at least stable for this session.
			uid = fmt.Sprintf("peer-%d", s.Peer)
		}
		if name == "" {
			name = fmt.Sprintf("Peer #%d", s.Peer)
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

// Metrics lets the collector chart player count and process uptime.
//
// MaxPlayerNum is the configured slot count when the Steam query answers
// and the game's hard cap otherwise. That distinction is the whole reason
// the query was worth building for charts: a 3-player evening on a
// 4-slot server is nearly full, and against the 16-slot cap it looks
// empty. FPS and frame time stay zero, which charts read correctly as
// "not reported".
func (c *Client) Metrics(ctx context.Context) (*game.Metrics, error) {
	st, err := c.refresh(ctx)
	if err != nil {
		return nil, err
	}
	if st.State != "running" {
		return nil, fmt.Errorf("server process is %s", st.State)
	}
	m := &game.Metrics{
		CurrentPlayerNum: len(c.tracker.Sessions()),
		MaxPlayerNum:     MaxSlots,
		UptimeSeconds:    int(time.Since(st.StartedAt).Seconds()),
	}
	if q := c.queryInfo(ctx); q != nil {
		m.CurrentPlayerNum = q.Players
		if q.MaxPlayers > 0 {
			m.MaxPlayerNum = q.MaxPlayers
		}
	}
	return m, nil
}

// Settings has no live transport — the game can't be asked for its
// config, only the JSON read at rest, which is the config editor's job
// (esconfig), not this client's.
func (c *Client) Settings(ctx context.Context) (map[string]any, error) {
	return nil, &game.UnsupportedError{Op: "settings", Reason: reasonNoSettings}
}
