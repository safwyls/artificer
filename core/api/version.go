package api

// The build this binary came from, reported so a bug report can name it.
//
// Two UIs and one service move save data between them, and the first
// question about any misbehavior is which halves are talking. Stamped at
// link time (-X main.version), handed to the Server, and shown in the
// page footer; the companion reads the service's from its own status
// call and shows both.

import "net/http"

// version is the stamped build, or "dev" for a binary built without one.
func (s *Server) version() string {
	if s.Version == "" {
		return "dev"
	}
	return s.Version
}

// handleVersion answers unauthenticated on purpose: the login page shows
// it too, and a build number is not a secret.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": s.version()})
}
