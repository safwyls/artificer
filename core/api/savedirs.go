package api

// Save locations, from the Ludusavi manifest (core/savedb).
//
// Same shape as the artwork proxy, for the same reason: one deployment
// holds a catalogue every companion needs, so it fetches it once and
// answers batches. What travels back is path *templates* — the manifest
// speaks in placeholders (<winLocalAppData>, <storeUserId>) that only
// the player's own machine can resolve, so expansion happens there and
// the service stays game-blind and OS-blind.
//
// Absent entries mean "the manifest doesn't carry this game", which is
// an ordinary answer: the companion falls back to Steam Cloud paths, its
// built-in catalogue and a name search, exactly as it did before.

import (
	"encoding/json"
	"net/http"

	"github.com/safwyls/artificer/core/savedb"
)

// handleSyncSaveHints resolves save-location templates for a batch of
// games.
func (s *Server) handleSyncSaveHints(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Games []savedb.Query `json:"games"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// The same generous cap the artwork batch uses: a big Steam library
	// is a few hundred games.
	if len(in.Games) > 500 {
		in.Games = in.Games[:500]
	}
	locations := s.SaveDB.Lookup(in.Games)
	if locations == nil {
		locations = map[string][]savedb.Location{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted":  true,
		"available": s.SaveDB.Loaded(),
		"locations": locations,
	})
}

// handleSaveHintsStatus is the admin view: did the catalogue load, how
// big is it, when was it fetched, and what went wrong if it didn't.
func (s *Server) handleSaveHintsStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": s.SaveDB.Status()})
}

// handleRefreshSaveHints re-fetches on demand. It runs on the request's
// own context: a 17MB fetch is slow enough that the admin should see it
// finish (or fail) rather than be told "started" and left guessing.
func (s *Server) handleRefreshSaveHints(w http.ResponseWriter, r *http.Request) {
	if s.SaveDB == nil {
		writeError(w, http.StatusNotFound, "this deployment has no save-location catalogue configured")
		return
	}
	out := map[string]any{}
	if err := s.SaveDB.Refresh(r.Context()); err != nil {
		s.logger.Error("refreshing the save-location manifest", "error", err)
		out["refreshed"] = false
		out["error"] = err.Error()
	} else {
		out["refreshed"] = true
	}
	out["status"] = s.SaveDB.Status()
	writeJSON(w, http.StatusOK, out)
}
