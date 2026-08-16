package api

// The game-facing surface: what a game module's contributed routes
// (Server.GameRoutes) may use. Deliberately small — a game handler that
// needs more than this is probably reaching for something that belongs
// in core or in the game's own packages.

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/safwyls/sampo/core/agentctl"
	"github.com/safwyls/sampo/core/agentfiles"
	"github.com/safwyls/sampo/core/store"
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
