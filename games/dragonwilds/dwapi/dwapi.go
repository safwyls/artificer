// Package dwapi is Dragonwilds' contributed console routes
// (api.Server.GameRoutes): the world-save metadata endpoint, the launch
// chooser's write half, and the one-click mod install. The read half of
// launch (GET /launch) is core's — every agent reports how it starts the
// game; only choosing is game-shaped.
package dwapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/artificer/core/agentctl"
	"github.com/safwyls/artificer/core/agentfiles"
	"github.com/safwyls/artificer/core/api"
	"github.com/safwyls/artificer/core/savecache"
	"github.com/safwyls/artificer/core/store"
	"github.com/safwyls/artificer/games/dragonwilds/dwbridge"
	"github.com/safwyls/artificer/games/dragonwilds/dwsave"
)

type handlers struct {
	s      *api.Server
	worlds *savecache.Cache[dwsave.World]
	// charNames resolves a server's log-learned character guid → name
	// pairings (dragonwilds.CharacterNames). Nil is fine: the world's
	// transform records then stay name-less, which is what the save alone
	// can say.
	charNames func(agentURL string) map[string]string
	// companion holds character records players relayed via the
	// wkcompanion app — see companion.go.
	companion *companionInbox
	// companionExe is the path of the bundled wkcompanion.exe the console
	// hands out (companion.go); empty when this deployment ships none.
	companionExe string
}

// API is Dragonwilds' contributed route sets: the authenticated
// per-server routes and the token-gated public ones share the companion
// inbox, which is why both hang off one constructor.
type API struct {
	h *handlers
}

// New builds the contributed API. worlds is the dwsave parse cache the
// console main constructs (mtime-keyed, stale-serving); charNames is the
// log-derived identity lookup (dragonwilds.CharacterNames), nil when a
// caller has none.
func New(s *api.Server, worlds *savecache.Cache[dwsave.World], charNames func(agentURL string) map[string]string) *API {
	return &API{h: &handlers{s: s, worlds: worlds, charNames: charNames, companion: newCompanionInbox()}}
}

// Routes mounts the authenticated per-server endpoints
// (api.Server.GameRoutes).
func (a *API) Routes() func(chi.Router) {
	h := a.h
	s := h.s
	return func(r chi.Router) {
		// Admin-only for the same reason backups are: the payload names
		// the world owner's player id and carries every character's
		// inventory and last position, and the pages it feeds already are.
		r.With(s.RequireAdmin).Get("/world", h.handleServerWorld)
		r.With(s.RequireAdmin).Get("/companion", h.handleGetCompanion)
		r.With(s.RequireAdmin).Put("/companion", h.handleSetCompanion)
		r.With(s.RequirePermission(store.PermPower)).Put("/launch", h.handleSetLaunch)
		r.With(s.RequirePermission(store.PermPower)).Post("/bridge/install", h.handleInstallBridge)
	}
}

// SetCompanionExe names the bundled wkcompanion.exe to hand out; empty
// (the default) makes the download answer honestly that this deployment
// ships none.
func (a *API) SetCompanionExe(path string) { a.h.companionExe = path }

// PublicRoutes mounts the token-gated companion endpoints
// (api.Server.PublicGameRoutes) — see companion.go for the trust model.
func (a *API) PublicRoutes() func(chi.Router) {
	h := a.h
	return func(r chi.Router) {
		r.Get("/companion/{token}", h.handleCompanionPing)
		r.Get("/companion/{token}/download", h.handleCompanionDownload)
		r.Post("/companion/{token}/character", h.handleCompanionPush)
	}
}

// agentFor builds the dwbridge-extended agent client for a server; nil
// when no agent is configured.
func (h *handlers) agentFor(srv *store.Server) *dwbridge.AgentClient {
	client, err := agentctl.New(srv.AgentURL, srv.AgentToken)
	if err != nil {
		return nil
	}
	return dwbridge.Wrap(client)
}

// handleServerWorld returns the parsed world for a server, stale-
// tolerant: a page load gets the cached parse immediately while any
// re-parse runs behind it. "available": false is the calm shape for
// every way a world can be legitimately absent; errors are reserved for
// a save that should have parsed and didn't.
func (h *handlers) handleServerWorld(w http.ResponseWriter, r *http.Request) {
	srv, ok := h.s.LoadServer(w, r)
	if !ok {
		return
	}
	if h.worlds == nil || !agentfiles.SaveConfigured(srv) {
		api.WriteJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	savePath, err := h.s.FilesHandle().SavePath(r.Context(), srv)
	if errors.Is(err, agentfiles.ErrNotConfigured) {
		api.WriteJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusBadGateway, "save files unreachable: "+err.Error())
		return
	}
	world, err := h.worlds.ReadServeStale(r.Context(), savePath)
	if errors.Is(err, savecache.ErrNotConfigured) {
		api.WriteJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	if err != nil {
		// Surfaced, not hidden behind a generic message: "no .sav file
		// in …" or a parse error names exactly what an operator must fix.
		h.s.LoggerHandle().Warn("world read failed", "server", srv.Name, "error", err)
		api.WriteError(w, http.StatusInternalServerError, "reading world save: "+err.Error())
		return
	}
	enriched := h.withCompanionRecords(srv, h.withCharNames(srv, world))
	api.WriteJSON(w, http.StatusOK, map[string]any{"available": true, "world": enriched})
}

// withCharNames overlays log-learned names onto the world's name-less
// transform records. The cached world is shared between requests, so a
// name overlay works on a copy — the cache stays exactly what the save
// said. A record that already carries a name (an older-build save's
// embedded character record) keeps it.
func (h *handlers) withCharNames(srv *store.Server, world *dwsave.World) *dwsave.World {
	if h.charNames == nil || world == nil {
		return world
	}
	names := h.charNames(srv.AgentURL)
	if len(names) == 0 {
		return world
	}
	out := *world
	out.Players = make([]dwsave.PlayerCharacter, len(world.Players))
	copy(out.Players, world.Players)
	for i := range out.Players {
		p := &out.Players[i]
		if p.CharName != "" {
			continue
		}
		if name, ok := names[strings.ToUpper(p.CharGuid)]; ok {
			p.CharName = name
		}
	}
	return &out
}

// handleSetLaunch chooses which build the agent starts next.
func (h *handlers) handleSetLaunch(w http.ResponseWriter, r *http.Request) {
	srv, ok := h.s.LoadServer(w, r)
	if !ok {
		return
	}
	var req struct {
		Profile string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	agent := h.agentFor(srv)
	if agent == nil {
		api.WriteError(w, http.StatusBadRequest, "no agent configured for this server")
		return
	}
	status, err := agent.SetLaunchProfile(r.Context(), req.Profile)
	if err != nil {
		api.WriteAgentError(w, err)
		return
	}
	// Worth auditing: it changes which build runs, and therefore whether
	// this server can be saved on demand at all.
	h.s.Audit(r, srv.ID, "launch-profile", req.Profile)
	api.WriteJSON(w, http.StatusOK, status)
}

// handleInstallBridge forwards the one-click mod install to the agent.
// The agent owns every precondition (kit present, Windows build selected
// and installed, nothing already there) and answers with statuses the UI
// maps to honest copy — this handler only adds auth and audit.
func (h *handlers) handleInstallBridge(w http.ResponseWriter, r *http.Request) {
	srv, ok := h.s.LoadServer(w, r)
	if !ok {
		return
	}
	agent := h.agentFor(srv)
	if agent == nil {
		api.WriteError(w, http.StatusBadRequest, "no agent configured for this server")
		return
	}
	restart, err := agent.InstallBridgeKit(r.Context())
	if err != nil {
		api.WriteAgentError(w, err)
		return
	}
	// Worth auditing: it changes what the next game start loads.
	h.s.Audit(r, srv.ID, "bridge-install", "ue4ss kit")
	api.WriteJSON(w, http.StatusOK, map[string]any{"installed": true, "restartRequired": restart})
}
