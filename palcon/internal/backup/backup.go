// Package backup snapshots Palworld save directories into the data
// dataset. The save mount is read-only by deployment convention, so a
// backup is a pure read: walk the save directory, zip every .sav file,
// write the archive under DATA_DIR/backups/<server id>/, prune old ones.
//
// Restores are deliberately manual (download the zip, unpack it over the
// save with the server stopped): writing into the save mount is exactly
// what Palcon promises never to do.
package backup

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/safwyls/palcon/internal/agentfiles"
	"github.com/safwyls/palcon/internal/notify"
	"github.com/safwyls/palcon/internal/store"
)

const (
	// sweepEvery is how often due backups are checked for; snapshots are
	// hours apart, so a fine-grained timer buys nothing.
	sweepEvery = time.Minute
	// nameFormat is the snapshot filename timestamp (UTC), sortable
	// lexically so retention can sort by name.
	nameFormat = "20060102-150405"
)

// ErrBusy means a backup for this server is already running.
var ErrBusy = errors.New("a backup is already running for this server")

// Snapshot is one archived backup on disk.
type Snapshot struct {
	Name  string    `json:"name"`
	TS    time.Time `json:"ts"`
	Bytes int64     `json:"bytes"`
}

type Runner struct {
	store    *store.Store
	notifier *notify.Notifier
	logger   *slog.Logger
	// files resolves the save to a local directory: the read-only mount,
	// or the agent-synced cache — either way the archive walk below stays
	// a pure local read.
	files *agentfiles.Syncer
	// root is DATA_DIR/backups; per-server snapshots live in <root>/<id>/.
	root string

	mu      sync.Mutex
	running map[int64]bool
	// failing dedupes the Discord failure alert to once per streak.
	failing map[int64]bool
}

func New(st *store.Store, notifier *notify.Notifier, logger *slog.Logger, dataDir string, files *agentfiles.Syncer) *Runner {
	return &Runner{
		store:    st,
		notifier: notifier,
		logger:   logger,
		files:    files,
		root:     filepath.Join(dataDir, "backups"),
		running:  make(map[int64]bool),
		failing:  make(map[int64]bool),
	}
}

// Run sweeps until ctx is cancelled. Intended to be started in a goroutine.
func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(sweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sweep(ctx)
		}
	}
}

func (r *Runner) sweep(ctx context.Context) {
	servers, err := r.store.ListServers(ctx)
	if err != nil {
		r.logger.Error("backup: listing servers", "error", err)
		return
	}
	for _, srv := range servers {
		if !srv.Enabled || !agentfiles.SaveConfigured(srv) || srv.BackupIntervalHours <= 0 {
			continue
		}
		due, err := r.isDue(ctx, srv)
		if err != nil {
			r.logger.Error("backup: checking schedule", "server", srv.ID, "error", err)
			continue
		}
		if !due {
			continue
		}
		if _, err := r.BackupNow(ctx, srv); err != nil {
			if errors.Is(err, ErrBusy) {
				continue
			}
			r.logger.Error("backup: scheduled backup failed", "server", srv.ID, "error", err)
			r.mu.Lock()
			firstFailure := !r.failing[srv.ID]
			r.failing[srv.ID] = true
			r.mu.Unlock()
			if firstFailure && r.notifier != nil {
				r.notifier.BackupFailed(ctx, srv, err)
			}
		} else {
			r.mu.Lock()
			delete(r.failing, srv.ID)
			r.mu.Unlock()
		}
	}
}

// isDue reports whether the newest snapshot is older than the configured
// interval AND the save has actually changed since — a server that's been
// offline for a week doesn't need seven identical archives. The age check
// runs first so agent-backed servers only pay for a sync when a snapshot
// is actually in the window.
func (r *Runner) isDue(ctx context.Context, srv *store.Server) (bool, error) {
	snaps, err := r.List(srv.ID)
	if err != nil {
		return false, err
	}
	if len(snaps) > 0 {
		newest := snaps[0].TS
		if time.Since(newest) < time.Duration(srv.BackupIntervalHours)*time.Hour {
			return false, nil
		}
		savePath, err := r.files.SavePath(ctx, srv)
		if err != nil {
			return false, nil // save unreachable right now; try next sweep
		}
		level, err := os.Stat(levelSavPath(savePath))
		if err != nil {
			return false, nil // no readable Level.sav; nothing to back up
		}
		if !level.ModTime().After(newest) {
			return false, nil
		}
	}
	return true, nil
}

// Running reports whether a backup for the server is in flight, so the UI
// can show it without a job-tracking table.
func (r *Runner) Running(serverID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running[serverID]
}

func levelSavPath(savePath string) string {
	if strings.EqualFold(filepath.Base(savePath), "Level.sav") {
		return savePath
	}
	return filepath.Join(savePath, "Level.sav")
}

// verifySavMagic rejects a Level.sav that doesn't look like a Palworld
// save container — most importantly one caught mid-write, which would
// otherwise archive fine and only reveal itself on restore day.
func verifySavMagic(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 24)
	if _, err := io.ReadFull(f, head); err != nil {
		return fmt.Errorf("save too short to be valid: %w", err)
	}
	magic := string(head[8:11])
	if magic == "PlZ" || magic == "PlM" {
		return nil
	}
	// Xbox-style chunked container: the real header sits 12 bytes in.
	if string(head[8:11]) == "CNK" {
		inner := string(head[20:23])
		if inner == "PlZ" || inner == "PlM" {
			return nil
		}
	}
	return errors.New("Level.sav doesn't look like a Palworld save (mid-write? wrong path?)")
}

// BackupNow snapshots the server's save directory immediately. Safe to call
// concurrently; a second call while one runs returns ErrBusy.
func (r *Runner) BackupNow(ctx context.Context, srv *store.Server) (*Snapshot, error) {
	if !agentfiles.SaveConfigured(srv) {
		return nil, errors.New("no save path or agent configured")
	}
	r.mu.Lock()
	if r.running[srv.ID] {
		r.mu.Unlock()
		return nil, ErrBusy
	}
	r.running[srv.ID] = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.running, srv.ID)
		r.mu.Unlock()
	}()

	// Resolve at snapshot time so an agent-backed backup archives the
	// freshest synced copy, not whatever the last poll happened to pull.
	saveDir, err := r.files.SavePath(ctx, srv)
	if err != nil {
		return nil, fmt.Errorf("resolving save: %w", err)
	}
	if strings.EqualFold(filepath.Base(saveDir), "Level.sav") {
		saveDir = filepath.Dir(saveDir)
	}
	if err := verifySavMagic(levelSavPath(saveDir)); err != nil {
		return nil, err
	}

	dir := filepath.Join(r.root, fmt.Sprintf("%d", srv.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	name := now.Format(nameFormat) + ".zip"
	tmp := filepath.Join(dir, name+".tmp")
	if err := writeArchive(ctx, saveDir, tmp); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	final := filepath.Join(dir, name)
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return nil, err
	}

	info, err := os.Stat(final)
	if err != nil {
		return nil, err
	}
	pruned, err := r.prune(srv.ID, srv.BackupKeep)
	if err != nil {
		r.logger.Error("backup: pruning old snapshots", "server", srv.ID, "error", err)
	}
	r.logger.Info("backup: snapshot written",
		"server", srv.ID, "name", name, "bytes", info.Size(), "pruned", pruned)
	return &Snapshot{Name: name, TS: now, Bytes: info.Size()}, nil
}

// writeArchive zips every .sav under the save directory (Level.sav,
// LevelMeta.sav, WorldOption.sav, Players/, GlobalPalStorage.sav, pal
// storage sidecars), preserving relative paths. Anything else — the game's
// own scratch files, a stray backup folder — stays out.
func writeArchive(ctx context.Context, saveDir, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)

	err = filepath.WalkDir(saveDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			// The game (and some server images) keep their own rolling
			// backups next to the save; archiving archives helps no one.
			if strings.EqualFold(d.Name(), "backup") || strings.EqualFold(d.Name(), "backups") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".sav") {
			return nil
		}
		rel, err := filepath.Rel(saveDir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		f.Close()
		return err
	})
	if err != nil {
		zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return out.Close()
}

var snapshotName = regexp.MustCompile(`^\d{8}-\d{6}\.zip$`)

// List returns the server's snapshots, newest first.
func (r *Runner) List(serverID int64) ([]Snapshot, error) {
	dir := filepath.Join(r.root, fmt.Sprintf("%d", serverID))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Snapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Snapshot{}
	for _, e := range entries {
		if e.IsDir() || !snapshotName.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		ts, err := time.ParseInLocation(nameFormat, strings.TrimSuffix(e.Name(), ".zip"), time.UTC)
		if err != nil {
			ts = info.ModTime().UTC()
		}
		out = append(out, Snapshot{Name: e.Name(), TS: ts, Bytes: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS.After(out[j].TS) })
	return out, nil
}

// Path validates a snapshot name (no traversal — the pattern admits only
// timestamp names) and returns its on-disk location.
func (r *Runner) Path(serverID int64, name string) (string, error) {
	if !snapshotName.MatchString(name) {
		return "", errors.New("invalid snapshot name")
	}
	path := filepath.Join(r.root, fmt.Sprintf("%d", serverID), name)
	if _, err := os.Stat(path); err != nil {
		return "", errors.New("snapshot not found")
	}
	return path, nil
}

func (r *Runner) Delete(serverID int64, name string) error {
	path, err := r.Path(serverID, name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// prune removes snapshots beyond the newest `keep`, returning how many.
func (r *Runner) prune(serverID int64, keep int) (int, error) {
	if keep < 1 {
		keep = 1
	}
	snaps, err := r.List(serverID)
	if err != nil {
		return 0, err
	}
	pruned := 0
	for _, s := range snaps[min(keep, len(snaps)):] {
		if err := os.Remove(filepath.Join(r.root, fmt.Sprintf("%d", serverID), s.Name)); err != nil {
			return pruned, err
		}
		pruned++
	}
	return pruned, nil
}
