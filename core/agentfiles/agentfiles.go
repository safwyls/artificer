// Package agentfiles resolves a server's save and config to an effective
// local path. A server with bind mounts keeps using them untouched; a
// server with only an agent (docs/sidecar-agent.md, phase 2) gets its
// files mirrored from the agent into DATA_DIR/agentfiles/<server id>/ and
// hands consumers that cache path instead. Everything downstream —
// the save parser, the backup archiver, the ini editor — keeps working on
// local files and never learns agents exist.
package agentfiles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/safwyls/sampo/core/agentctl"
	"github.com/safwyls/sampo/core/game"
	"github.com/safwyls/sampo/core/store"
)

// ErrNotConfigured means the server has neither a local path nor an agent
// for the requested file — the feature is off, not broken.
var ErrNotConfigured = errors.New("not configured")

// minCheckInterval bounds how often an unchanged-save check hits the
// agent. The check is a conditional GET answered 304 (a directory walk,
// no transfer), so it can stay close to the collector's own poll cadence.
const minCheckInterval = 10 * time.Second

type serverState struct {
	mu        sync.Mutex
	etag      string
	lastCheck time.Time
}

type Syncer struct {
	logger *slog.Logger
	// root is DATA_DIR/agentfiles; per-server caches live in <root>/<id>/.
	root string

	mu    sync.Mutex
	state map[int64]*serverState
}

func New(dataDir string, logger *slog.Logger) *Syncer {
	return &Syncer{
		logger: logger,
		root:   filepath.Join(dataDir, "agentfiles"),
		state:  map[int64]*serverState{},
	}
}

func (s *Syncer) stateFor(id int64) *serverState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state[id] == nil {
		s.state[id] = &serverState{}
	}
	return s.state[id]
}

// SaveConfigured reports whether SavePath can return anything for this
// server — the gate the backup scheduler and availability flags use.
func SaveConfigured(srv *store.Server) bool {
	return srv.SavePath != "" || srv.AgentURL != ""
}

// SavePath returns the local directory holding the server's save files: a
// configured bind mount verbatim, else the agent-synced cache (refreshed
// here if the agent reports changes). ErrNotConfigured when the server has
// neither.
func (s *Syncer) SavePath(ctx context.Context, srv *store.Server) (string, error) {
	if srv.SavePath != "" {
		return srv.SavePath, nil
	}
	client, err := agentctl.New(srv.AgentURL, srv.AgentToken)
	if errors.Is(err, agentctl.ErrNotConfigured) {
		return "", ErrNotConfigured
	}
	if err != nil {
		return "", err
	}

	st := s.stateFor(srv.ID)
	st.mu.Lock()
	defer st.mu.Unlock()

	dir := filepath.Join(s.root, fmt.Sprintf("%d", srv.ID), "save")
	fresh := time.Since(st.lastCheck) < minCheckInterval
	if _, err := os.Stat(dir); err == nil && fresh {
		return dir, nil
	}

	etag, changed, err := client.SyncSave(ctx, dir, st.etag)
	if err != nil {
		// A cached copy beats an error: the parser serves stale anyway,
		// and the agent being briefly down shouldn't blank the pal pages.
		if _, statErr := os.Stat(dir); statErr == nil {
			s.logger.Warn("save sync failed; serving cached copy", "server", srv.Name, "error", err)
			return dir, nil
		}
		return "", err
	}
	st.etag = etag
	st.lastCheck = time.Now()
	if changed {
		s.logger.Info("save synced from agent", "server", srv.Name, "dir", dir)
	}
	return dir, nil
}

// ConfigPath returns a local directory holding the game's settings file
// (named by the game's config codec) for the config editor to read/edit:
// the configured mount verbatim (viaAgent=false),
// else a fresh copy from the agent (viaAgent=true — write-backs must go
// through PushConfig).
func (s *Syncer) ConfigPath(ctx context.Context, srv *store.Server) (path string, viaAgent bool, err error) {
	if srv.ConfigPath != "" {
		return srv.ConfigPath, false, nil
	}
	client, err := agentctl.New(srv.AgentURL, srv.AgentToken)
	if errors.Is(err, agentctl.ErrNotConfigured) {
		return "", false, ErrNotConfigured
	}
	if err != nil {
		return "", false, err
	}

	st := s.stateFor(srv.ID)
	st.mu.Lock()
	defer st.mu.Unlock()

	data, err := client.GetConfig(ctx)
	if err != nil {
		return "", false, err
	}
	dir := filepath.Join(s.root, fmt.Sprintf("%d", srv.ID), "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	name, err := configFilename(srv)
	if err != nil {
		return "", false, err
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		return "", false, err
	}
	return dir, true, nil
}

// configFilename is the game's settings filename, from its codec — the
// one game fact this package needs, looked up rather than known.
func configFilename(srv *store.Server) (string, error) {
	def, ok := game.Get(srv.Game)
	if !ok || def.Config == nil {
		return "", fmt.Errorf("game %q has no config file", srv.Game)
	}
	return def.Config.Filename, nil
}

// PushConfig sends the (locally edited) cached config back to the agent,
// which writes it atomically next to the game.
func (s *Syncer) PushConfig(ctx context.Context, srv *store.Server, dir string) error {
	client, err := agentctl.New(srv.AgentURL, srv.AgentToken)
	if err != nil {
		return err
	}
	name, err := configFilename(srv)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	return client.PutConfig(ctx, data)
}
