package api

// Cover-art configuration: the IGDB credential pair, and the diagnostics
// that say whether it works.
//
// Two sources, resolved the way the advisor key is: a pair saved through
// the admin UI wins, the environment's is the fallback, and removing the
// saved one falls back rather than turning artwork off. The reason the
// UI can hold credentials at all is that a deployment's owner is not
// always the person who can edit its compose file — asking someone to
// redeploy a container to try a Twitch key is a bad trade for a feature
// whose whole job is decoration.
//
// The diagnostics exist because the first cut had none: every IGDB
// failure degraded to "no cover", which is right for a player and
// useless for whoever configured it. A wrong secret and a game IGDB has
// never heard of produced the same blank tile.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/safwyls/artificer/core/igdb"
	"github.com/safwyls/artificer/core/store"
)

// UseEnvArtwork installs the environment-configured credentials and
// remembers them, so a later delete of the UI-saved pair reverts here.
// Always safe to call: an empty pair yields a client that answers
// nothing until one is saved through the UI.
func (s *Server) UseEnvArtwork(clientID, clientSecret string) {
	s.artworkEnvID = strings.TrimSpace(clientID)
	s.artworkEnvSecret = strings.TrimSpace(clientSecret)
	if s.Artwork == nil {
		s.Artwork = igdb.New(s.artworkEnvID, s.artworkEnvSecret)
		return
	}
	s.Artwork.SetCredentials(s.artworkEnvID, s.artworkEnvSecret, "env")
}

// LoadStoredArtwork applies a pair saved through the UI over whatever
// the environment gave. Called once at startup; an unreadable row (a
// rotated ENCRYPTION_KEY) is for main to log, never to die on — covers
// are not worth refusing to start over.
func (s *Server) LoadStoredArtwork(ctx context.Context) (bool, error) {
	if s.Artwork == nil {
		s.Artwork = igdb.New("", "")
	}
	creds, err := s.store.IGDBCredentials(ctx)
	if err != nil || creds == nil {
		return false, err
	}
	s.Artwork.SetCredentials(creds.ClientID, creds.ClientSecret, "settings")
	return true, nil
}

// artworkStatus is what the admin panel reads: the client's own view,
// plus which sources exist. The secret is never part of it.
func (s *Server) artworkStatus(r *http.Request) map[string]any {
	status := s.Artwork.Status()
	stored := false
	if creds, err := s.store.IGDBCredentials(r.Context()); err == nil && creds != nil {
		stored = true
	}
	return map[string]any{
		"status":        status,
		"stored":        stored,
		"envConfigured": s.artworkEnvID != "" && s.artworkEnvSecret != "",
	}
}

func (s *Server) handleArtworkSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.artworkStatus(r))
}

// handleSetArtworkSettings saves the pair and immediately proves it. A
// credential that is merely stored tells nobody anything; the answer
// carries the result of one real call to IGDB, which is the whole point
// of typing it in.
func (s *Server) handleSetArtworkSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ClientID = strings.TrimSpace(req.ClientID)
	req.ClientSecret = strings.TrimSpace(req.ClientSecret)
	if req.ClientID == "" || req.ClientSecret == "" {
		writeError(w, http.StatusBadRequest, "both a client id and a client secret are required — IGDB authenticates through a Twitch application, and half a pair cannot")
		return
	}
	if err := s.store.SetIGDBCredentials(r.Context(), store.IGDBCredentials{
		ClientID: req.ClientID, ClientSecret: req.ClientSecret,
	}); err != nil {
		s.logger.Error("saving igdb credentials", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save the credentials")
		return
	}
	if s.Artwork == nil {
		s.Artwork = igdb.New("", "")
	}
	s.Artwork.SetCredentials(req.ClientID, req.ClientSecret, "settings")
	s.logger.Info("igdb credentials saved", "clientId", req.ClientID)

	out := s.artworkStatus(r)
	out["test"] = s.artworkTest(r)
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteArtworkSettings drops the saved pair; the environment's,
// if any, takes over.
func (s *Server) handleDeleteArtworkSettings(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteIGDBCredentials(r.Context()); err != nil {
		s.logger.Error("removing igdb credentials", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to remove the credentials")
		return
	}
	s.UseEnvArtwork(s.artworkEnvID, s.artworkEnvSecret)
	s.logger.Info("igdb credentials removed")
	writeJSON(w, http.StatusOK, s.artworkStatus(r))
}

func (s *Server) handleTestArtwork(w http.ResponseWriter, r *http.Request) {
	out := s.artworkStatus(r)
	out["test"] = s.artworkTest(r)
	writeJSON(w, http.StatusOK, out)
}

// artworkTest runs one real lookup and reports it plainly. A failure
// here is a 200 with ok:false, not an HTTP error: the caller asked "does
// this work", and "no, because <IGDB's own words>" is a successful
// answer to that question.
func (s *Server) artworkTest(r *http.Request) map[string]any {
	if err := s.Artwork.Test(r.Context()); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}
