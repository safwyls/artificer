package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// steamCacheDirs are the directories, relative to the Palworld install root,
// whose contents SteamCMD rebuilds from scratch: appmanifest files and
// partial downloads under steamapps/, and cached package payloads under
// steam/packages/. A game update sometimes leaves both corrupted, after
// which the container's updater fails on every start until they're wiped.
var steamCacheDirs = []string{"steamapps", filepath.Join("steam", "packages")}

// handleClearSteamCache empties the SteamCMD cache directories under the
// server's install path — the equivalent of `rm -rf ./steamapps/*
// ./steam/packages/*` in the install root. Deliberately scoped: only the
// contents of the two well-known cache directories are removed, never the
// directories themselves (they can be mount points) and never game or save
// files. Gated on the power permission by the router: this exists to get a
// broken container updating again, and is useless without restart rights.
func (s *Server) handleClearSteamCache(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	if srv.InstallPath == "" {
		writeError(w, http.StatusBadRequest, "no install path configured for this server")
		return
	}

	user, _ := userFromContext(r.Context())
	actor := "unknown"
	if user != nil {
		actor = user.Username
	}

	removed := 0
	found := false
	for _, rel := range steamCacheDirs {
		dir := filepath.Join(srv.InstallPath, rel)
		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue // absent cache dir is fine; the other may still exist
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("reading %s: %v", rel, err))
			return
		}
		found = true
		for _, e := range entries {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				s.logger.Error("steam cache clear failed",
					"installPath", srv.InstallPath, "entry", filepath.Join(rel, e.Name()), "user", actor, "error", err)
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("deleting %s: %v", filepath.Join(rel, e.Name()), err))
				return
			}
			removed++
		}
	}
	// Neither directory existing means the path isn't a Palworld install
	// root (or isn't mounted) — tell the user rather than reporting a no-op
	// success that leaves their real cache corrupted.
	if !found {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("neither steamapps/ nor steam/packages/ exists under %s — check the install path", srv.InstallPath))
		return
	}

	s.logger.Info("steam cache cleared", "installPath", srv.InstallPath, "removed", removed, "user", actor)
	s.audit(r, srv.ID, "steam-cache-clear", fmt.Sprintf("%d entries", removed))
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
}
