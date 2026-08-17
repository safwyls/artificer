package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/artificer/core/collector"
	"github.com/safwyls/artificer/core/sched"
	"github.com/safwyls/artificer/core/store"
)

// handlePublicStatus is the one unauthenticated data endpoint: a read-only
// status snapshot behind a per-server unguessable token. Everything it
// reports comes from Palcon's own database (the collector's latest sample,
// the restart schedules), so public traffic can never add load to — or
// probe — the game server itself. No player names, no host, no ports.
func (s *Server) handlePublicStatus(w http.ResponseWriter, r *http.Request) {
	srv, err := s.store.GetServerByPublicToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load status")
		return
	}

	resp := map[string]any{
		"name":   srv.Name,
		"online": false,
	}

	m, err := s.store.LatestMetric(r.Context(), srv.ID)
	if err == nil && m != nil {
		// Online means the collector heard from the server within the last
		// few sampling intervals; a stale sample is an offline server (or a
		// stopped Palcon, which can't tell anyone anything anyway).
		resp["online"] = time.Since(m.TS) < 3*collector.Interval
		if m.PlayerCount != nil {
			resp["players"] = *m.PlayerCount
		}
		if m.MaxPlayers != nil {
			resp["maxPlayers"] = *m.MaxPlayers
		}
	}

	// The soonest enabled schedule, so the page can answer "when's the
	// next restart" for people without accounts.
	if schedules, err := s.store.ListRestartSchedules(r.Context(), srv.ID); err == nil {
		var next time.Time
		for _, sc := range schedules {
			if !sc.Enabled {
				continue
			}
			if t := sched.NextRun(sc, time.Now()); !t.IsZero() && (next.IsZero() || t.Before(next)) {
				next = t
			}
		}
		if !next.IsZero() {
			resp["nextRestartAt"] = next.UTC().Format(time.RFC3339)
		}
	}

	// Public and cache-friendly: 15s covers a refresh storm from a Discord
	// link without ever being meaningfully stale against 30s sampling.
	w.Header().Set("Cache-Control", "public, max-age=15")
	writeJSON(w, http.StatusOK, resp)
}

// handleUpdatePublicStatus turns the public page on (minting a fresh token
// each time — enabling after a disable revokes old links) or off.
func (s *Server) handleUpdatePublicStatus(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token := ""
	if in.Enabled {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}
		token = hex.EncodeToString(buf)
	}
	if err := s.store.SetPublicToken(r.Context(), srv.ID, token); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update public status")
		return
	}
	detail := "off"
	if in.Enabled {
		detail = "on"
	}
	s.audit(r, srv.ID, "public-status-update", detail)
	writeJSON(w, http.StatusOK, map[string]any{"enabled": in.Enabled, "token": token})
}
