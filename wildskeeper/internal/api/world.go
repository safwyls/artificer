package api

import (
	"errors"
	"net/http"

	"github.com/safwyls/wildskeeper/internal/agentfiles"
	"github.com/safwyls/wildskeeper/internal/game"
	"github.com/safwyls/wildskeeper/internal/savecache"
)

// The world endpoint serves the metadata parsed out of the server's save
// file — a domain endpoint in dwsave's terms, the same way the ini editor
// is in dwconfig's. Admin-only for the same reason backups are: it names
// the world owner's player id, and the page it feeds already is.

// handleServerWorld returns the parsed world for a server, stale-tolerant:
// a page load gets the cached parse immediately while any re-parse runs
// behind it. "available": false is the calm shape for every way a world can
// be legitimately absent (no reader wired, no save path, no game that keeps
// one); errors are reserved for a save that should have parsed and didn't.
func (s *Server) handleServerWorld(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	gameID := srv.Game
	if gameID == "" {
		gameID = game.DefaultID
	}
	if s.Worlds == nil || gameID != "dragonwilds" || !agentfiles.SaveConfigured(srv) {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}

	savePath, err := s.files.SavePath(r.Context(), srv)
	if errors.Is(err, agentfiles.ErrNotConfigured) {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "save files unreachable: "+err.Error())
		return
	}

	world, err := s.Worlds.ReadServeStale(r.Context(), savePath)
	if errors.Is(err, savecache.ErrNotConfigured) {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	if err != nil {
		// Surfaced, not hidden behind a generic message: "no .sav file in
		// …" or a parse error names exactly what an operator must fix.
		s.logger.Warn("world read failed", "server", srv.Name, "error", err)
		writeError(w, http.StatusInternalServerError, "reading world save: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "world": world})
}
