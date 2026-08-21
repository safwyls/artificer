package api

// The dedicated server as a holder (docs/save-sync-architecture.md,
// phase 4): a world with a linked server can be GIVEN to it (the head
// restored onto the server, the server holding the world while the
// group plays on it) and TAKEN back (the server's save committed as the
// new head, the hold returned). Both ride the agent's restore pair —
// HEAD for the precondition, PUT If-Match for the swap — inside the
// stopped-server window, because a save read or written under a running
// game is the torn-save failure this repo keeps re-learning.
//
// The same PUT verb is the backup-restore ability the roadmap carried
// as Shared item 2; handleRestoreBackup below is that button.

import (
	"archive/tar"
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/artificer/core/agentctl"
	"github.com/safwyls/artificer/core/store"
)

// syncServerAgent resolves the world's own agent link — the standalone
// service knows agents, not console server rows — and proves the game
// behind it is not holding its save open. Only a supervisor agent can
// answer that from its own process state; anything less is a refusal,
// not a shrug, because restoring under a running game corrupts.
func (s *Server) syncServerAgent(w http.ResponseWriter, r *http.Request, world *store.SyncWorld) (*agentctl.Client, bool) {
	if world.AgentURL == "" {
		writeError(w, http.StatusConflict, "this world has no dedicated-server agent — set one in its settings")
		return nil, false
	}
	client, health := agentctl.Supervisor(r.Context(), world.AgentURL, world.AgentToken)
	if client == nil {
		writeError(w, http.StatusConflict, "the world's agent is unreachable or not a supervisor — only a supervisor agent can prove the game is stopped")
		return nil, false
	}
	if health.Game != nil && health.Game.State == "running" {
		writeError(w, http.StatusConflict, "the server is running — stop it first")
		return nil, false
	}
	return client, true
}

// handleSyncServerGive hands the world to the dedicated server: the
// server becomes the holder and the head is restored onto it.
func (s *Server) handleSyncServerGive(w http.ResponseWriter, r *http.Request) {
	world, ok := s.loadSyncWorld(w, r)
	if !ok {
		return
	}
	user, uok := userFromContext(r.Context())
	if !uok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if world.HeadVersion == nil {
		writeError(w, http.StatusConflict, "this world has no versions yet — import or check one in first")
		return
	}
	client, ok := s.syncServerAgent(w, r, world)
	if !ok {
		return
	}

	ss, err := s.SaveSync.Checkout(r.Context(), world.ID, user, true, false)
	if err != nil {
		writeSyncError(w, err)
		return
	}
	// The hold is acquired; from here every failure releases it so the
	// world cannot end up "held by the server" with nothing delivered.
	fail := func(status int, msg string) {
		s.SaveSync.Release(r.Context(), ss, user)
		writeError(w, status, msg)
	}

	path, _, err := s.SaveSync.VersionPath(r.Context(), world.ID, *world.HeadVersion)
	if err != nil {
		fail(http.StatusInternalServerError, err.Error())
		return
	}
	etag, err := client.SaveETag(r.Context())
	if err != nil {
		fail(http.StatusBadGateway, "asking the agent for its save state: "+err.Error())
		return
	}
	f, err := os.Open(path)
	if err != nil {
		fail(http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()
	if err := client.RestoreSave(r.Context(), f, etag); err != nil {
		fail(http.StatusBadGateway, "restoring onto the server: "+err.Error())
		return
	}
	s.syncAudit(r, world, "sync-server-give", fmt.Sprintf("version %d", *world.HeadVersion))
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted": true,
		"session":  ss,
		"note":     "the server holds the world — start it when ready",
	})
}

// handleSyncServerTake commits the server's save as the new head and
// returns the hold.
func (s *Server) handleSyncServerTake(w http.ResponseWriter, r *http.Request) {
	world, ok := s.loadSyncWorld(w, r)
	if !ok {
		return
	}
	user, uok := userFromContext(r.Context())
	if !uok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	ss, err := s.store.ActiveSyncSession(r.Context(), world.ID)
	if err != nil {
		writeError(w, http.StatusConflict, "the server does not hold this world")
		return
	}
	if !ss.ServerHeld {
		writeError(w, http.StatusConflict, "this world is held by a player, not the server — they check it in themselves")
		return
	}
	client, ok := s.syncServerAgent(w, r, world)
	if !ok {
		return
	}
	body, _, cancel, err := client.OpenSave(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "reading the server's save: "+err.Error())
		return
	}
	defer cancel()
	defer body.Close()
	v, err := s.SaveSync.Checkin(r.Context(), ss, user, body, store.SyncKindCheckin)
	if err != nil {
		writeSyncError(w, err)
		return
	}
	s.syncAudit(r, world, "sync-server-take", fmt.Sprintf("version %d", v.ID))
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "version": v})
}

// serverStoppedForRestore proves a console server row's game is not
// holding its save open. Supervisor agents answer from their own process
// state; companion agents fall back to the container. A state that
// cannot be proven is a refusal, not a shrug.
func (s *Server) serverStoppedForRestore(ctx context.Context, srv *store.Server) error {
	if _, health := s.agentSupervisor(ctx, srv); health != nil {
		if health.Game != nil && health.Game.State == "running" {
			return errors.New("the server is running — stop it first")
		}
		return nil
	}
	if s.docker != nil && srv.ContainerName != "" {
		st, err := s.docker.Inspect(ctx, srv.ContainerName)
		if err != nil {
			return fmt.Errorf("cannot verify the server is stopped: %v", err)
		}
		if st.Running {
			return errors.New("the server's container is running — stop it first")
		}
		return nil
	}
	return errors.New("cannot verify the server is stopped (no supervisor agent and no docker control)")
}

// handleRestoreBackup places a snapshot back onto the server — the
// restore verb the backups page was waiting for (roadmap, Shared 2).
// Admin-only like the rest of the backup surface; server stopped;
// agent-only for the same read-only-mount reason as the sync flows.
func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	if s.backups == nil {
		writeError(w, http.StatusInternalServerError, "backups are not running")
		return
	}
	name := chi.URLParam(r, "name")
	path, err := s.backups.Path(srv.ID, name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if srv.AgentURL == "" {
		writeError(w, http.StatusConflict, "restore needs the server's sidecar agent — the console's save mount is read-only")
		return
	}
	client, err := agentctl.New(srv.AgentURL, srv.AgentToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.serverStoppedForRestore(r.Context(), srv); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	etag, err := client.SaveETag(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "asking the agent for its save state: "+err.Error())
		return
	}
	// Snapshots are zips (core/backup); the agent speaks tar bundles.
	// Convert in flight rather than storing a second format.
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(zipToBundle(path, pw)) }()
	if err := client.RestoreSave(r.Context(), pr, etag); err != nil {
		writeError(w, http.StatusBadGateway, "restoring the snapshot: "+err.Error())
		return
	}
	s.audit(r, srv.ID, "backup-restore", name)
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "restored": name})
}

// zipToBundle rewrites a backup zip as the agent's tar bundle shape:
// relative regular files, PAX mtimes.
func zipToBundle(zipPath string, w io.Writer) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	tw := tar.NewWriter(w)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		mod := f.Modified
		if mod.IsZero() {
			mod = time.Now()
		}
		hdr := &tar.Header{Name: f.Name, Mode: 0o644, Size: int64(f.UncompressedSize64), ModTime: mod, Format: tar.FormatPAX}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		_, err = io.CopyN(tw, rc, int64(f.UncompressedSize64))
		rc.Close()
		if err != nil {
			return err
		}
	}
	return tw.Close()
}
