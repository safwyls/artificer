package api

import (
	"errors"
	"net/http"

	"github.com/safwyls/palcon/internal/palsave"
)

// readSaveForRequest resolves the {serverID} route param and returns the
// parsed save data for that server, writing the error response and
// returning ok=false on any failure. 400 with a distinct message when the
// server has no save path configured, so the frontend can show setup
// guidance instead of an error.
func (s *Server) readSaveForRequest(w http.ResponseWriter, r *http.Request) (*palsave.Result, bool) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return nil, false
	}
	result, err := s.palReader.Read(r.Context(), srv.SavePath)
	if errors.Is(err, palsave.ErrNotConfigured) {
		writeError(w, http.StatusBadRequest, "no save path configured")
		return nil, false
	}
	if err != nil {
		s.logger.Error("save extraction failed", "server", srv.ID, "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return nil, false
	}
	return result, true
}

// handleServerPals serves the phase 5 Pal viewer: party/palbox/base pals
// per player, parsed from the server's Level.sav (read-only).
func (s *Server) handleServerPals(w http.ResponseWriter, r *http.Request) {
	result, ok := s.readSaveForRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleServerGuilds serves the guild view. Backed by the same cached save
// read as /pals, so opening both costs one parse.
func (s *Server) handleServerGuilds(w http.ResponseWriter, r *http.Request) {
	result, ok := s.readSaveForRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"guilds":      result.Guilds,
		"players":     result.Players,
		"parsedAt":    result.ParsedAt,
		"saveModTime": result.SaveModTime,
	})
}
