package api

// The vault surface: the route assembly the standalone save-sync
// service (reliquary) mounts — authentication, users and world custody,
// none of the console furniture. Same Server, same handlers, same
// middleware as Routes(); a smaller selection of the same parts, so the
// two assemblies cannot drift on behavior, only on what exists.

import (
	"io/fs"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// CompanionExe (assigned after New, like Provisioner) is the bundled
// Artificer Companion the vault hands out; empty answers the download
// with where to get one instead of a broken link.
//
// It lives on Server rather than reliquary because the token-tier
// download handler does — the companion fetches itself updates through
// the same base URL it syncs against.

// VaultRoutes builds the standalone save-sync service's handler: JSON
// API under /api (auth, users, custody), the embedded UI for everything
// else. SaveSync must be set — a vault without the engine is nothing.
func (s *Server) VaultRoutes(staticFS fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Route("/api", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "not found")
		})
		// Save-bundle transfers, outside the JSON body cap — the same
		// split Routes() makes, for the same reason.
		r.Group(func(r chi.Router) {
			r.Use(maxBodyBytes(maxSyncUpload))
			s.mountSyncUploads(r)
		})
		r.Group(func(r chi.Router) {
			r.Use(maxBodyBytes(1 << 20))
			r.Get("/version", s.handleVersion)
			r.Post("/login", s.handleLogin)
			r.Post("/login/cloudflare", s.handleCloudflareLogin)
			r.Group(func(r chi.Router) {
				r.Use(s.requireAuth)
				r.Post("/logout", s.handleLogout)
				r.Get("/me", s.handleMe)
				r.Post("/me/password", s.handleChangeOwnPassword)
				r.With(s.requireAdmin).Get("/users", s.handleListUsers)
				r.With(s.requireAdmin).Post("/users", s.handleCreateUser)
				r.With(s.requireAdmin).Put("/users/{userID}", s.handleUpdateUser)
				r.With(s.requireAdmin).Delete("/users/{userID}", s.handleDeleteUser)
				s.mountSyncSmall(r)
			})
		})
	})

	r.NotFound(spaHandler(staticFS))
	return r
}

// handleSyncCompanionDownload hands out the bundled companion app,
// token-gated like the rest of the tier: the vault's UI gives each
// player one link that carries their own credential.
func (s *Server) handleSyncCompanionDownload(w http.ResponseWriter, r *http.Request) {
	if s.CompanionExe == "" {
		writeError(w, http.StatusNotFound, "this deployment ships without the companion app — download it from the repo's GitHub releases, or build it: GOOS=windows GOARCH=amd64 go build ./cmd/companion")
		return
	}
	f, err := os.Open(s.CompanionExe)
	if err != nil {
		writeError(w, http.StatusNotFound, "companion app not present in this deployment ("+s.CompanionExe+")")
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reading companion app")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.microsoft.portable-executable")
	w.Header().Set("Content-Disposition", `attachment; filename="artificer-companion.exe"`)
	http.ServeContent(w, r, "artificer-companion.exe", fi.ModTime(), f)
}
