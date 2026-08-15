package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/flamekeeper/internal/agentfiles"
	"github.com/safwyls/flamekeeper/internal/backup"
)

// Backups are admin-only end to end: a snapshot is the entire world,
// players' inventories included, and the settings decide disk usage on the
// Palcon host.

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	if s.backups == nil {
		writeError(w, http.StatusInternalServerError, "backups are not running")
		return
	}
	snaps, err := s.backups.List(srv.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list snapshots")
		return
	}
	var total int64
	for _, sn := range snaps {
		total += sn.Bytes
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available":     agentfiles.SaveConfigured(srv),
		"running":       s.backups.Running(srv.ID),
		"intervalHours": srv.BackupIntervalHours,
		"keep":          srv.BackupKeep,
		"snapshots":     snaps,
		"totalBytes":    total,
	})
}

func (s *Server) handleUpdateBackupSettings(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	var in struct {
		IntervalHours int `json:"intervalHours"`
		Keep          int `json:"keep"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.IntervalHours < 0 || in.IntervalHours > 336 {
		writeError(w, http.StatusBadRequest, "interval must be 0 (off) to 336 hours")
		return
	}
	if in.Keep < 1 || in.Keep > 100 {
		writeError(w, http.StatusBadRequest, "keep must be 1 to 100 snapshots")
		return
	}
	if err := s.store.SetBackupSettings(r.Context(), srv.ID, in.IntervalHours, in.Keep); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save backup settings")
		return
	}
	s.audit(r, srv.ID, "backup-settings", fmt.Sprintf("every %dh, keep %d", in.IntervalHours, in.Keep))
	writeJSON(w, http.StatusOK, map[string]int{"intervalHours": in.IntervalHours, "keep": in.Keep})
}

// handleRunBackup kicks a snapshot off in the background — a big world
// takes a while to compress, and the browser shouldn't hold a request open
// for it. The UI polls the list until `running` clears.
func (s *Server) handleRunBackup(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	if s.backups == nil {
		writeError(w, http.StatusInternalServerError, "backups are not running")
		return
	}
	if !agentfiles.SaveConfigured(srv) {
		writeError(w, http.StatusBadRequest, "no save path or agent configured for this server")
		return
	}
	if s.backups.Running(srv.ID) {
		writeError(w, http.StatusConflict, "a backup is already running")
		return
	}
	s.audit(r, srv.ID, "backup-run", "")
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Minute)
		defer cancel()
		if _, err := s.backups.BackupNow(ctx, srv); err != nil && !errors.Is(err, backup.ErrBusy) {
			s.logger.Error("manual backup failed", "server", srv.ID, "error", err)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", srv.Name+"-"+name))
	http.ServeFile(w, r, path)
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	if s.backups == nil {
		writeError(w, http.StatusInternalServerError, "backups are not running")
		return
	}
	name := chi.URLParam(r, "name")
	if err := s.backups.Delete(srv.ID, name); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	s.audit(r, srv.ID, "backup-delete", name)
	w.WriteHeader(http.StatusNoContent)
}
