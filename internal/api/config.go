package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/safwyls/flametender/internal/agentfiles"
	"github.com/safwyls/flametender/internal/games/enshrouded/esconfig"
	"github.com/safwyls/flametender/internal/store"
)

// configCodec is one game's config reader/writer behind a common wire
// shape. The sibling consoles' ini codecs and esconfig deliberately share
// their policy (never add or remove keys, type-validate, .bak, atomic
// swap) but not their types; this is the seam that keeps the handlers
// below game-blind.
type configCodec struct {
	// filename names the file for user-facing labels and 404s.
	filename      string
	notConfigured error
	read          func(path string) (*configPayload, error)
	write         func(path string, changes map[string]string) error
	// rotateAdminPassword is nil for games whose admin access isn't a
	// password session the ini controls.
	rotateAdminPassword func(path, newPassword string) error
}

// configPayload matches both packages' Result JSON shape.
type configPayload struct {
	Settings any    `json:"settings"`
	Path     string `json:"path"`
	Writable bool   `json:"writable"`
}

var enshroudedCodec = &configCodec{
	filename:      "enshrouded_server.json",
	notConfigured: esconfig.ErrNotConfigured,
	read: func(path string) (*configPayload, error) {
		res, err := esconfig.Read(path)
		if err != nil {
			return nil, err
		}
		return &configPayload{Settings: res.Settings, Path: res.Path, Writable: res.Writable}, nil
	},
	write:               esconfig.WriteChanges,
	rotateAdminPassword: esconfig.RotateAdminPassword,
}

// codecFor picks the config codec for a server's game. One game today;
// the seam stays so a second game slots in as a codec, not a rewrite.
func codecFor(srv *store.Server) *configCodec {
	return enshroudedCodec
}

// resolveConfigPath yields the local directory palconfig operates on: the
// configured mount, or a fresh copy pulled from the server's agent. When
// viaAgent, edits must be pushed back with s.files.PushConfig. Errors are
// written to w; ok=false means the response is already sent.
func (s *Server) resolveConfigPath(w http.ResponseWriter, r *http.Request, srv *store.Server) (path string, viaAgent, ok bool) {
	path, viaAgent, err := s.files.ConfigPath(r.Context(), srv)
	if errors.Is(err, agentfiles.ErrNotConfigured) {
		writeError(w, http.StatusBadRequest, "no config path configured")
		return "", false, false
	}
	if err != nil {
		s.logger.Error("fetching config from agent failed", "server", srv.ID, "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return "", false, false
	}
	return path, viaAgent, true
}

// handleGetConfig returns the parsed settings ini for the settings editor.
// This reads the file on the config mount — the source of truth for what the
// server boots with — not the live REST /settings, which reflects the
// currently-running config and can't be written back.
//
// 400 with a distinct message when the server has no config path, so the
// frontend can show setup guidance instead of an error.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	codec := codecFor(srv)
	path, viaAgent, ok := s.resolveConfigPath(w, r, srv)
	if !ok {
		return
	}
	res, err := codec.read(path)
	if errors.Is(err, codec.notConfigured) {
		writeError(w, http.StatusBadRequest, "no config path configured")
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, codec.filename+" not found at the configured path")
		return
	}
	if err != nil {
		s.logger.Error("reading server config failed", "server", srv.ID, "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	// The cache path is an implementation detail; what the user should
	// see is where the file actually lives.
	if viaAgent {
		res.Path = codec.filename + " · synced via flameagent"
	}
	writeJSON(w, http.StatusOK, res)
}

// handleUpdateConfig writes changed settings back to the settings ini.
// Only existing keys can be changed and each value is validated against its
// type, so a bad edit is rejected whole rather than half-writing the file.
// Changes take effect when the server next restarts.
func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	var req struct {
		Changes map[string]string `json:"changes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	codec := codecFor(srv)
	if !s.writeConfig(w, r, srv, codec, req.Changes) {
		return
	}

	// Record which keys changed — never the values, which include the
	// admin/join passwords.
	keys := make([]string, 0, len(req.Changes))
	for k := range req.Changes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	s.audit(r, srv.ID, "config-update", strings.Join(keys, ", "))
	s.respondFreshConfig(w, r, srv, codec)
}

// editConfigFile runs one edit against the server's config file and, for
// agent-backed servers, ships the result back afterwards.
//
// Every writer of enshrouded_server.json goes through here — the flat
// settings editor, the role groups, the ban list, the password rotation —
// because the part that is easy to get wrong is the same for all of them:
// an edit that lands only on the local cache copy looks like a success
// and changes nothing the game will ever read. Errors are written to w;
// false means the response is already sent.
func (s *Server) editConfigFile(w http.ResponseWriter, r *http.Request, srv *store.Server, notConfigured error, edit func(path string) error) bool {
	path, viaAgent, ok := s.resolveConfigPath(w, r, srv)
	if !ok {
		return false
	}
	err := edit(path)
	if errors.Is(err, notConfigured) {
		writeError(w, http.StatusBadRequest, "no config path configured")
		return false
	}
	if errors.Is(err, os.ErrPermission) {
		writeError(w, http.StatusBadGateway, "config file is read-only — mount it read-write to edit settings")
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "enshrouded_server.json not found at the configured path")
		return false
	}
	if err != nil {
		// Validation failures (unknown key, wrong type, a role group that
		// would lock everyone out) read as bad requests; they're the
		// caller's input, not a server fault.
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	// Agent-backed edits land on a local cache copy; ship it back to the
	// game server before reporting success, so a failed push fails loudly
	// instead of silently editing a file nothing reads.
	if viaAgent {
		if err := s.files.PushConfig(r.Context(), srv, path); err != nil {
			s.logger.Error("pushing config to agent failed", "server", srv.ID, "error", err)
			writeError(w, http.StatusBadGateway, "saving to the agent failed: "+err.Error())
			return false
		}
	}
	return true
}

// writeConfig runs one validated flat-settings write.
func (s *Server) writeConfig(w http.ResponseWriter, r *http.Request, srv *store.Server, codec *configCodec, changes map[string]string) bool {
	return s.editConfigFile(w, r, srv, codec.notConfigured, func(path string) error {
		return codec.write(path, changes)
	})
}

// respondFreshConfig returns the freshly-read settings so the client
// re-syncs to what's actually on disk.
func (s *Server) respondFreshConfig(w http.ResponseWriter, r *http.Request, srv *store.Server, codec *configCodec) {
	path, viaAgent, err := s.files.ConfigPath(r.Context(), srv)
	if err == nil {
		if res, rerr := codec.read(path); rerr == nil {
			if viaAgent {
				res.Path = codec.filename + " · synced via flameagent"
			}
			writeJSON(w, http.StatusOK, res)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRotateAdminPassword generates a fresh admin-role password and
// writes it into enshrouded_server.json. For Enshrouded this is the one
// real remote-admin lever: whoever holds it can join with kick/ban
// rights, and rotating it locks out the previous holders at their next
// join. The new password is returned exactly once and never logged or
// audited by value.
func (s *Server) handleRotateAdminPassword(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	codec := codecFor(srv)
	if codec.rotateAdminPassword == nil {
		writeError(w, http.StatusNotImplemented, "admin-password rotation isn't wired for this game")
		return
	}
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		writeError(w, http.StatusInternalServerError, "generating a password failed")
		return
	}
	password := hex.EncodeToString(buf)

	ok = s.editConfigFile(w, r, srv, codec.notConfigured, func(path string) error {
		return codec.rotateAdminPassword(path, password)
	})
	if !ok {
		return
	}
	s.audit(r, srv.ID, "config-rotate-admin-password", "")
	writeJSON(w, http.StatusOK, map[string]string{"password": password})
}
