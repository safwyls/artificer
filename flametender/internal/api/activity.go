package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/safwyls/flametender/internal/collector"
)

// audit records a management action against a server. Best-effort by
// design: the action already happened, so a failed audit write is a log
// line, never an error to the user. The request only supplies identity —
// the write itself is detached so it can't be cancelled by a closing tab.
func (s *Server) audit(r *http.Request, serverID int64, action, detail string) {
	username := "unknown"
	if user, ok := userFromContext(r.Context()); ok {
		username = user.Username
	}
	if len(detail) > 300 {
		detail = detail[:297] + "..."
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	if err := s.store.InsertAudit(ctx, serverID, username, action, detail); err != nil {
		s.logger.Error("audit: recording action", "action", action, "server", serverID, "error", err)
	}
}

// handleServerActivity returns the join/leave history the collector has
// recorded. Any signed-in user may read it — it's the same information the
// player list shows, extended backward in time.
func (s *Server) handleServerActivity(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	hours := 48
	if h, err := strconv.Atoi(r.URL.Query().Get("hours")); err == nil && h > 0 {
		hours = min(h, int(collector.ActivityRetention/time.Hour))
	}
	events, err := s.store.ListPlayerEvents(r.Context(), srv.ID, time.Now().Add(-time.Duration(hours)*time.Hour))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load activity")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"hours":  hours,
		// The sampling cadence bounds how precise session edges can be.
		"intervalSeconds": int(collector.Interval / time.Second),
	})
}

// handleServerAudit returns the management-action trail. Admin-only: it
// names which admin did what, which is not the other users' business.
func (s *Server) handleServerAudit(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	limit := 200
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = min(l, 1000)
	}
	entries, err := s.store.ListAudit(r.Context(), srv.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load audit log")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
