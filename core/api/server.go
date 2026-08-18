// Package api wires up the HTTP server: auth, server CRUD, and the
// per-server RCON/REST actions, plus serving the built React SPA.
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/safwyls/artificer/core/advisor"
	"github.com/safwyls/artificer/core/agentfiles"
	"github.com/safwyls/artificer/core/backup"
	"github.com/safwyls/artificer/core/cfaccess"
	"github.com/safwyls/artificer/core/dockerctl"
	"github.com/safwyls/artificer/core/notify"
	"github.com/safwyls/artificer/core/store"
)

// AccessVerifier is what the API needs from Cloudflare Access: turn the
// assertion on a request into an identity, or refuse it. One
// implementation ships (*cfaccess.Verifier); the seam is an interface so
// tests can exercise this layer's account handling without minting real
// tokens, and so the cryptography stays testable on its own terms in
// internal/cfaccess.
type AccessVerifier interface {
	Verify(ctx context.Context, token string) (*cfaccess.Identity, error)
	// LogoutURL ends the Access session itself, which signing out of this
	// console alone would leave running.
	LogoutURL() string
}

type Server struct {
	store     *store.Store
	jwtSecret []byte
	logger    *slog.Logger
	// CookieSecure marks session cookies Secure (set from COOKIE_SECURE
	// for deployments behind TLS; default off for plain-HTTP LAN use).
	CookieSecure bool
	// docker is nil when no DOCKER_HOST is set; power control is then
	// simply unavailable rather than broken.
	docker *dockerctl.Client
	// notifier delivers Discord messages; the API only uses it for the
	// "send a test message" endpoint.
	notifier *notify.Notifier
	// backups runs the snapshot schedule and owns the archive directory.
	backups *backup.Runner
	// files resolves save/config to a local path — a bind mount, or the
	// agent-synced cache (docs/sidecar-agent.md phase 2).
	files *agentfiles.Syncer
	// Provisioner, when set (like CookieSecure, assigned after New), lets
	// the new-server wizard deploy stacks itself instead of handing the
	// operator a file. Exactly one implementation exists: the shared Anvil
	// host service (see provisioner.go). This console deliberately has no
	// built-in provisioner — one host, one Docker-socket holder.
	Provisioner Provisioner
	// Access, when set (assigned after New, like Provisioner), verifies
	// Cloudflare Access assertions so people who signed in at the tunnel
	// don't sign in twice. Nil means the console only knows password
	// login — see internal/cfaccess and docs/cloudflare-access.md.
	Access AccessVerifier
	// AccessAdminEmails hold the admin role whenever they sign in through
	// Access. Lowercased by config; matched case-insensitively.
	AccessAdminEmails []string
	loginLimiter      *loginLimiter
	// bans applies config edits queued while the server was running, in
	// the gap between a stop and the next start. The game module supplies
	// it; nil when the game has no such work.
	bans OfflineConfigWork
	// Provision is the game's provisioning knowledge (assigned after New,
	// like Provisioner). Nil disables the Raise-a-server wizard.
	Provision *ProvisionProfile
	// SessionCookie names the login cookie; empty means the neutral
	// default. Consoles set their brand's name so sessions survive.
	SessionCookie string
	// AdvisorPrompt is the console's advisor system prompt (assigned
	// after New). The text is game payload — the game module owns it;
	// an empty System leaves the advisor disabled even with a key.
	AdvisorPrompt advisor.Prompt
	// DocsFS serves the console's embedded docs for the advisor's
	// docs-search tool; nil means the endpoint reports none.
	DocsFS fs.FS
	// Roster, when set, reads the save-derived player roster for the
	// visibility editor (assigned after New, like Provisioner).
	Roster RosterSource
	// GameRoutes, when set, mounts the game's own per-server endpoints
	// (drift ledger seam 5) inside the authenticated /servers/{id} group.
	// The game module builds the closure over this Server via its Routes
	// helper; core mounts it blind.
	GameRoutes func(r chi.Router)
	// The advisor has two possible sources, resolved in advisor():
	// a key saved through the admin UI (uiAdvisor, encrypted in the store)
	// wins over one from the environment (envAdvisor, set by main). Both
	// nil means the feature is simply absent — see core/advisor.
	advisorMu  sync.RWMutex
	envAdvisor AdvisorClient
	uiAdvisor  AdvisorClient
}

// OfflineConfigWork is config edits that must wait for the window when
// the game is down — see the identical seam in core/sched.
type OfflineConfigWork interface {
	Pending(ctx context.Context, srv *store.Server) bool
	Apply(ctx context.Context, srv *store.Server)
}

func New(st *store.Store, jwtSecret []byte, logger *slog.Logger, docker *dockerctl.Client, notifier *notify.Notifier, backups *backup.Runner, files *agentfiles.Syncer, bans OfflineConfigWork) *Server {
	return &Server{store: st, jwtSecret: jwtSecret, logger: logger, docker: docker, notifier: notifier, backups: backups, files: files,
		loginLimiter: newLoginLimiter(), bans: bans}
}

// Routes builds the full HTTP handler: JSON API under /api, and the built
// frontend (staticFS) for everything else, with an index.html fallback so
// client-side routing works on refresh/deep links.
func (s *Server) Routes(staticFS fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// The pals payload is the largest thing served (tens of MB of JSON on a
	// big world) and compresses ~10x; this also covers the JS bundles.
	r.Use(middleware.Compress(5))

	r.Route("/api", func(r chi.Router) {
		// No endpoint takes a body anywhere near this size; cap it so
		// json.Decode can't be fed an arbitrarily large request.
		r.Use(maxBodyBytes(1 << 20))
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "not found")
		})
		r.Post("/login", s.handleLogin)
		// Sign-in for people Cloudflare Access already authenticated.
		// Unauthenticated by necessity — the assertion on the request
		// *is* the credential, and cfaccess verifies it before anything
		// is read from it. A 404 when Access isn't configured keeps the
		// frontend's silent attempt cheap.
		r.Post("/login/cloudflare", s.handleCloudflareLogin)

		// The only unauthenticated data endpoint: token-gated, read-only,
		// served entirely from Flametender's own database. See public.go.
		r.Get("/public/status/{token}", s.handlePublicStatus)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/logout", s.handleLogout)
			r.Get("/me", s.handleMe)
			r.Post("/me/password", s.handleChangeOwnPassword)

			// Registered flat rather than via r.Route: a subrouter's "/"
			// only matches /users/, so POST /api/users 404s.
			// The host dashboard: what Anvil holds on this machine —
			// containers, ports, images. Read-only; see host.go.
			r.With(s.requireAdmin).Get("/host", s.handleHostOverview)

			r.With(s.requireAdmin).Get("/users", s.handleListUsers)
			r.With(s.requireAdmin).Post("/users", s.handleCreateUser)
			r.With(s.requireAdmin).Put("/users/{userID}", s.handleUpdateUser)
			r.With(s.requireAdmin).Delete("/users/{userID}", s.handleDeleteUser)

			// Advisor key management: process-wide (the advisor is one
			// feature, not a per-server one), admin-only, stored encrypted.
			// A key saved here wins over one from the environment.
			r.With(s.requireAdmin).Put("/advisor/key", s.handleSetAdvisorKey)
			r.With(s.requireAdmin).Delete("/advisor/key", s.handleDeleteAdvisorKey)
			r.With(s.requireAdmin).Put("/advisor/settings", s.handleSetAdvisorSettings)
			// Change which model a saved key runs, without re-entering it.
			r.With(s.requireAdmin).Put("/advisor/key/model", s.handleSetAdvisorKeyModel)
			r.Put("/me/advisor-key/model", s.handleSetUserAdvisorKeyModel)
			// A user's own key, shadowing the shared one for their requests
			// only. Any signed-in user; scoped to their account.
			r.Put("/me/advisor-key", s.handleSetUserAdvisorKey)
			r.Delete("/me/advisor-key", s.handleDeleteUserAdvisorKey)
			// Embedded console docs, for the advisor's docs-search tool.
			r.Get("/docs", s.handleDocs)

			r.Get("/servers", s.handleListServers)
			r.With(s.requireAdmin).Post("/servers", s.handleCreateServer)
			// New-server wizard: registers the row and generates the
			// supervisor stack file for the human to deploy. Defaults and
			// discovery let the wizard prefill from the provisioner's
			// config instead of asking.
			r.With(s.requireAdmin).Post("/servers/provision", s.handleProvisionServer)
			r.With(s.requireAdmin).Get("/servers/provision/defaults", s.handleProvisionDefaults)
			r.With(s.requireAdmin).Get("/servers/provision/discover", s.handleProvisionDiscover)
			r.With(s.requireAdmin).Post("/servers/adopt", s.handleAdoptServer)
			r.Route("/servers/{serverID}", func(r chi.Router) {
				r.Get("/", s.handleGetServer)
				r.With(s.requireAdmin).Put("/", s.handleUpdateServer)
				r.With(s.requireAdmin).Delete("/", s.handleDeleteServer)

				r.Get("/info", s.handleServerInfo)
				r.Get("/players", s.handleServerPlayers)
				// What this server's commands can actually do, asked
				// before offering them rather than discovered by a 501.
				r.Get("/capabilities", s.handleServerCapabilities)
				r.With(s.requirePermission(store.PermBroadcast)).Post("/broadcast", s.handleServerBroadcast)
				r.With(s.requirePermission(store.PermModerate)).Post("/kick", s.handleServerKick)
				r.With(s.requirePermission(store.PermModerate)).Post("/ban", s.handleServerBan)
				r.With(s.requirePermission(store.PermModerate)).Post("/unban", s.handleServerUnban)
				r.With(s.requirePermission(store.PermSave)).Post("/save", s.handleServerSave)
				r.With(s.requirePermission(store.PermShutdown)).Post("/shutdown", s.handleServerShutdown)

				// Container power. Reading state is fine for anyone
				// signed in; changing it needs the grant.
				r.Get("/container", s.handleContainerStatus)
				r.With(s.requirePermission(store.PermPower)).Post("/container/{action}", s.handleContainerAction)
				r.With(s.requirePermission(store.PermPower)).Get("/container/logs", s.handleContainerLogs)
				// SteamCMD repair & update — power territory: they exist
				// to get a broken container updating again. Runs via the
				// server's flameagent when configured, else the local
				// install-path mount (cache clear only).
				// How the agent will start the game. Read-only — one build
				// exists, so there is nothing to select; it answers "can
				// this agent's image actually run it?".
				r.Get("/launch", s.handleGetLaunch)
				// Rebuild this server's agent on another flameagent image.
				// Admin-only: it destroys and recreates a container, which
				// is provisioning, not day-to-day power.
				r.With(s.requireAdmin).Post("/agent/image", s.handleRecreateAgent)
				r.With(s.requirePermission(store.PermPower)).Post("/steam-cache/clear", s.handleClearSteamCache)
				r.With(s.requirePermission(store.PermPower)).Post("/steam/update", s.handleSteamUpdateStart)
				r.With(s.requirePermission(store.PermPower)).Get("/steam/update", s.handleSteamUpdateStatus)
				r.Get("/settings", s.handleServerSettings)

				// Settings editor (enshrouded_server.json here; the codec
				// is per-game, see config.go). Gated even for reading: the
				// file holds the role passwords in the clear.
				r.With(s.requirePermission(store.PermSettings)).Get("/config", s.handleGetConfig)
				r.With(s.requirePermission(store.PermSettings)).Put("/config", s.handleUpdateConfig)
				r.With(s.requirePermission(store.PermSettings)).Post("/config/rotate-admin-password", s.handleRotateAdminPassword)
				// Game-contributed routes (roles, ban lists, save-derived
				// views). The permission-boundary rule they must follow:
				// credential-bearing config behind PermSettings,
				// credential-free moderation behind PermModerate.
				if s.GameRoutes != nil {
					s.GameRoutes(r)
				}

				// Automation: restart schedules are visible to anyone
				// signed in ("when's the next restart?" is player-facing
				// information); changing them, and everything Discord, is
				// admin infrastructure config.
				r.Get("/automation", s.handleGetAutomation)
				r.With(s.requireAdmin).Post("/schedules", s.handleCreateSchedule)
				r.With(s.requireAdmin).Put("/schedules/{scheduleID}", s.handleUpdateSchedule)
				r.With(s.requireAdmin).Delete("/schedules/{scheduleID}", s.handleDeleteSchedule)
				r.With(s.requireAdmin).Put("/discord", s.handleUpdateDiscord)
				r.With(s.requireAdmin).Delete("/discord", s.handleDeleteDiscord)
				r.With(s.requireAdmin).Post("/discord/test", s.handleTestDiscord)
				r.With(s.requireAdmin).Put("/watchdog", s.handleUpdateWatchdog)
				r.With(s.requireAdmin).Put("/public", s.handleUpdatePublicStatus)

				// Save backups: the archive is the whole world, so even
				// listing is admin-only.
				r.With(s.requireAdmin).Get("/backups", s.handleListBackups)
				r.With(s.requireAdmin).Put("/backups/settings", s.handleUpdateBackupSettings)
				r.With(s.requireAdmin).Post("/backups/run", s.handleRunBackup)
				r.With(s.requireAdmin).Get("/backups/{name}/download", s.handleDownloadBackup)
				r.With(s.requireAdmin).Delete("/backups/{name}", s.handleDeleteBackup)

				// Player join/leave history is player-facing; the audit
				// trail names which admin did what and stays admin-only.
				r.Get("/activity", s.handleServerActivity)
				r.With(s.requireAdmin).Get("/audit", s.handleServerAudit)

				r.Get("/metrics", s.handleServerMetrics)
				r.Get("/metrics/history", s.handleServerMetricsHistory)
				// Advisor chat. GET says whether the process has a key at
				// all; POST answers one question.
				r.Get("/advisor", s.handleAdvisorStatus)
				r.Post("/advisor", s.handleAdvisorChat)

				// Who can see what. Admin-only in both directions: the list of
				// players who asked to be hidden is itself the sort of thing
				// the hiding is meant to keep quiet.
				r.With(s.requireAdmin).Get("/visibility", s.handleServerVisibility)
				r.With(s.requireAdmin).Put("/visibility", s.handleUpdateServerVisibility)
			})
		})
	})

	r.NotFound(spaHandler(staticFS))

	return r
}

// maxBodyBytes caps request body reads; exceeding it makes json.Decode
// fail, which handlers already report as a 400.
func maxBodyBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
