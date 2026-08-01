package api

import (
	"errors"
	"net/http"

	"github.com/safwyls/palcon/internal/agentfiles"
	"github.com/safwyls/palcon/internal/palsave"
	"github.com/safwyls/palcon/internal/store"
)

// readSaveForRequest resolves the {serverID} route param and returns the
// parsed save data for that server alongside the server itself, writing the
// error response and returning ok=false on any failure. 400 with a distinct
// message when the server has no save path configured, so the frontend can
// show setup guidance instead of an error.
//
// features are the views this endpoint's payload backs; the read is refused
// only when every one of them is switched off.
func (s *Server) readSaveForRequest(w http.ResponseWriter, r *http.Request, features ...string) (*palsave.Result, *store.Server, bool) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return nil, nil, false
	}
	if !requireFeature(w, r, srv, features...) {
		return nil, nil, false
	}
	savePath, err := s.files.SavePath(r.Context(), srv)
	if errors.Is(err, agentfiles.ErrNotConfigured) {
		writeError(w, http.StatusBadRequest, "no save path configured")
		return nil, nil, false
	}
	if err != nil {
		s.logger.Error("save sync from agent failed", "server", srv.ID, "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return nil, nil, false
	}
	// Serve-stale: a save that changed since the last parse returns the old
	// parse immediately (with its SaveModTime telling on itself) while a
	// re-parse runs in the background; only a never-parsed save blocks.
	result, err := s.palReader.ReadServeStale(r.Context(), savePath)
	if errors.Is(err, palsave.ErrNotConfigured) {
		writeError(w, http.StatusBadRequest, "no save path configured")
		return nil, nil, false
	}
	if err != nil {
		s.logger.Error("save extraction failed", "server", srv.ID, "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return nil, nil, false
	}
	return result, srv, true
}

// handleServerPals serves the phase 5 Pal viewer: party/palbox/base pals
// per player, parsed from the server's Level.sav (read-only).
func (s *Server) handleServerPals(w http.ResponseWriter, r *http.Request) {
	// One payload, three views: Player pals, Paldex and Calculators all read
	// it, so it answers while any of them is on.
	result, srv, ok := s.readSaveForRequest(w, r, store.FeaturePals, store.FeaturePaldex, store.FeatureCalculators)
	if !ok {
		return
	}
	hidden, err := s.hiddenPlayers(r, srv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"players":     toPalsPlayers(visiblePlayers(result.Players, hidden, store.StreamPals)),
		"guilds":      result.Guilds,
		"parsedAt":    result.ParsedAt,
		"saveModTime": result.SaveModTime,
	})
}

// visiblePlayers drops the players withheld from this stream. Returns the
// input untouched when nothing is hidden, which is the usual case.
func visiblePlayers(players []palsave.PlayerPals, hidden store.PlayerVisibility, stream string) []palsave.PlayerPals {
	if len(hidden) == 0 {
		return players
	}
	out := make([]palsave.PlayerPals, 0, len(players))
	for _, p := range players {
		if !hidden.HiddenFor(p.UID, stream) {
			out = append(out, p)
		}
	}
	return out
}

// palsPlayer is what /pals and /guilds serve for each player.
//
// Spelled out rather than serving palsave.PlayerPals directly, and that is the
// whole point: PlayerPals is the extractor's struct, and everything added to it
// used to appear on these two endpoints for free. Inventory and Character did
// exactly that — the Inventory view was gated and its player-level hides were
// honoured, while the same bytes stayed one fetch away on /pals. Adding a field
// here has to be a decision now, not a side effect. TestPalsPayloadFields
// fails if this drifts.
//
// json:"-" wouldn't have worked: the same struct is unmarshalled *from* the
// extractor, so hiding a field from the response hides it from the parse too.
type palsPlayer struct {
	UID              string         `json:"uid"`
	Nickname         string         `json:"nickname"`
	Level            int            `json:"level"`
	Party            []palsave.Pal  `json:"party"`
	Palbox           []palsave.Pal  `json:"palbox"`
	Base             []palsave.Pal  `json:"base"`
	Storage          []palsave.Pal  `json:"storage"`
	LastOnline       int64          `json:"lastOnline"`
	LastX            *float64       `json:"lastX"`
	LastY            *float64       `json:"lastY"`
	Platform         string         `json:"platform"`
	TechnologyPoints int            `json:"technologyPoints"`
	Paldeck          []string       `json:"paldeck"`
	Captures         map[string]int `json:"captures"`
}

func toPalsPlayers(players []palsave.PlayerPals) []palsPlayer {
	out := make([]palsPlayer, 0, len(players))
	for _, p := range players {
		out = append(out, palsPlayer{
			UID:              p.UID,
			Nickname:         p.Nickname,
			Level:            p.Level,
			Party:            p.Party,
			Palbox:           p.Palbox,
			Base:             p.Base,
			Storage:          p.Storage,
			LastOnline:       p.LastOnline,
			LastX:            p.LastX,
			LastY:            p.LastY,
			Platform:         p.Platform,
			TechnologyPoints: p.TechnologyPoints,
			Paldeck:          p.Paldeck,
			Captures:         p.Captures,
		})
	}
	return out
}

// inventoryPlayer is the /inventory projection of a player: who they are, plus
// their bags. Deliberately not the whole PlayerPals — the pals payload runs to
// tens of MB on a large world, and none of it is on screen here.
type inventoryPlayer struct {
	UID       string             `json:"uid"`
	Nickname  string             `json:"nickname"`
	Level     int                `json:"level"`
	Inventory palsave.Inventory  `json:"inventory"`
	Character *palsave.Character `json:"character,omitempty"`
	// Unix seconds; 0 when the save recorded none. Enough to caption the
	// sheet with how stale a look at this player is.
	LastOnline int64  `json:"lastOnline"`
	Platform   string `json:"platform"`
}

// handleServerInventory serves the item viewer: every player's containers,
// parsed from the server's Level.sav (read-only). Backed by the same cached
// save read as /pals and /guilds, so opening any of them costs one parse.
func (s *Server) handleServerInventory(w http.ResponseWriter, r *http.Request) {
	result, srv, ok := s.readSaveForRequest(w, r, store.FeatureInventory)
	if !ok {
		return
	}
	hidden, err := s.hiddenPlayers(r, srv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	players := make([]inventoryPlayer, 0, len(result.Players))
	for _, p := range visiblePlayers(result.Players, hidden, store.StreamInventory) {
		// A player with no containers at all has nothing to show; skipping
		// them keeps the page from listing empty sections for characters
		// that only exist as a guild membership.
		if len(p.Inventory) == 0 {
			continue
		}
		players = append(players, inventoryPlayer{
			UID:        p.UID,
			Nickname:   p.Nickname,
			Level:      p.Level,
			Inventory:  p.Inventory,
			Character:  p.Character,
			LastOnline: p.LastOnline,
			Platform:   p.Platform,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"players":     players,
		"parsedAt":    result.ParsedAt,
		"saveModTime": result.SaveModTime,
	})
}

// handleServerGuilds serves the guild view. Backed by the same cached save
// read as /pals, so opening both costs one parse.
//
// The live map reads this too — it's where offline players' last-known
// positions and the guild base coordinates come from — so it answers while
// either view is on.
func (s *Server) handleServerGuilds(w http.ResponseWriter, r *http.Request) {
	result, srv, ok := s.readSaveForRequest(w, r, store.FeatureGuilds, store.FeatureMap)
	if !ok {
		return
	}
	hidden, err := s.hiddenPlayers(r, srv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Two different hides land on one payload: a player withheld from pals
	// drops out entirely (and out of their guild's rollups with them), while
	// one withheld from the map keeps their guild standing but loses the
	// last-known position, which is the only part the map plots.
	players := withoutPositions(visiblePlayers(result.Players, hidden, store.StreamPals), hidden)
	writeJSON(w, http.StatusOK, map[string]any{
		"guilds":      result.Guilds,
		"players":     toPalsPlayers(players),
		"parsedAt":    result.ParsedAt,
		"saveModTime": result.SaveModTime,
	})
}

// withoutPositions blanks the last-known coordinates of players withheld from
// the map stream.
//
// Copies before writing: the slice it's given belongs to the reader's parse
// cache, which is shared by every request for this save, so editing in place
// would hide the player from everyone until the save changed.
func withoutPositions(players []palsave.PlayerPals, hidden store.PlayerVisibility) []palsave.PlayerPals {
	any := false
	for _, p := range players {
		if hidden.HiddenFor(p.UID, store.StreamMap) {
			any = true
			break
		}
	}
	if !any {
		return players
	}
	out := make([]palsave.PlayerPals, len(players))
	copy(out, players)
	for i := range out {
		if hidden.HiddenFor(out[i].UID, store.StreamMap) {
			out[i].LastX, out[i].LastY = nil, nil
		}
	}
	return out
}
