package api

// The game-facing surface: what a game module's contributed routes
// (Server.GameRoutes) may use. Deliberately small — a game handler that
// needs more than this is probably reaching for something that belongs
// in core or in the game's own packages.

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/safwyls/artificer/core/agentctl"
	"github.com/safwyls/artificer/core/agentfiles"
	"github.com/safwyls/artificer/core/store"
)

// WriteJSON writes a JSON response — the same encoder core's handlers use.
func WriteJSON(w http.ResponseWriter, status int, v any) { writeJSON(w, status, v) }

// WriteError writes core's error shape.
func WriteError(w http.ResponseWriter, status int, msg string) { writeError(w, status, msg) }

// LoadServer resolves the {id} server for a game route; ok=false means
// the response is already written.
func (s *Server) LoadServer(w http.ResponseWriter, r *http.Request) (*store.Server, bool) {
	return s.loadServer(w, r)
}

// RequirePermission is core's per-permission gate, for game routes that
// need their own grants.
func (s *Server) RequirePermission(perm string) func(http.Handler) http.Handler {
	return s.requirePermission(perm)
}

// RequireAdmin is core's admin gate, for game routes.
func (s *Server) RequireAdmin(next http.Handler) http.Handler {
	return s.requireAdmin(next)
}

// WriteAgentError maps agentctl's sentinel errors onto responses the way
// core's own agent-backed handlers do.
func WriteAgentError(w http.ResponseWriter, err error) { writeAgentError(w, err) }

// ResolveConfigPath is core's config-mount resolution (local mount or
// agent-synced copy); ok=false means the response is already written.
func (s *Server) ResolveConfigPath(w http.ResponseWriter, r *http.Request, srv *store.Server) (path string, viaAgent bool, ok bool) {
	return s.resolveConfigPath(w, r, srv)
}

// EditConfigFile runs one agent-push-aware settings-file edit, the same
// path core's own config writes take.
func (s *Server) EditConfigFile(w http.ResponseWriter, r *http.Request, srv *store.Server, notConfigured error, edit func(path string) error) bool {
	return s.editConfigFile(w, r, srv, notConfigured, edit)
}

// Audit records an action against a server, exactly as core handlers do.
func (s *Server) Audit(r *http.Request, serverID int64, action, detail string) {
	s.audit(r, serverID, action, detail)
}

// RequireFeature gates a game route on the server's visibility switches
// the way core's own feature-gated routes are; false means the response
// is already written.
func RequireFeature(w http.ResponseWriter, r *http.Request, srv *store.Server, features ...string) bool {
	return requireFeature(w, r, srv, features...)
}

// HiddenPlayers returns the per-player opt-outs that apply to this
// request — empty for an admin, who sees everyone.
func (s *Server) HiddenPlayers(r *http.Request, serverID int64) (store.PlayerVisibility, error) {
	return s.hiddenPlayers(r, serverID)
}

// AgentSupervisor resolves the server's agent when it is a supervisor
// (nil health otherwise) — how game routes ask "is the game up".
func (s *Server) AgentSupervisor(ctx context.Context, srv *store.Server) (*agentctl.Client, *agentctl.Health) {
	return s.agentSupervisor(ctx, srv)
}

// Store, Files, Logger and OfflineWork expose the Server's collaborators
// to game routes.
func (s *Server) StoreHandle() *store.Store       { return s.store }
func (s *Server) FilesHandle() *agentfiles.Syncer { return s.files }
func (s *Server) LoggerHandle() *slog.Logger      { return s.logger }
func (s *Server) OfflineWork() OfflineConfigWork  { return s.bans }
func (s *Server) OfflinePending(ctx context.Context, srv *store.Server) bool {
	return s.bans != nil && s.bans.Pending(ctx, srv)
}
