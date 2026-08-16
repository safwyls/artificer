package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/safwyls/flametender/internal/games/enshrouded/esconfig"
	"github.com/safwyls/flametender/internal/store"
)

// Role groups: the structured half of enshrouded_server.json.
//
// Behind PermSettings rather than PermModerate, even though what these
// grant is moderation. The reason is on the wire: a group carries its
// join password in the clear, so reading the list *is* reading the
// server's credentials, and anyone who can read it can join as an admin.
// That is the settings permission's boundary, and it is why the config
// endpoints are gated even for reads.

// rolesResponse is the wire shape. Restart is the fact the console has to
// state every time: the game reads userGroups at boot and matches a
// joining player against the copy it loaded then, so an edit made now
// governs the next start, not the session in progress.
type rolesResponse struct {
	Groups   []esconfig.Group `json:"groups"`
	Path     string           `json:"path"`
	Writable bool             `json:"writable"`
	// RestartRequired is always true here. It ships as a field rather than
	// as frontend copy so that if a game update ever makes roles reloadable,
	// one place stops saying it.
	RestartRequired bool `json:"restartRequired"`
}

func rolesPayload(res *esconfig.Groups, viaAgent bool) rolesResponse {
	path := res.Path
	if viaAgent {
		path = "enshrouded_server.json · synced via flameagent"
	}
	return rolesResponse{Groups: res.Groups, Path: path, Writable: res.Writable, RestartRequired: true}
}

func (s *Server) handleGetRoles(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	path, viaAgent, ok := s.resolveConfigPath(w, r, srv)
	if !ok {
		return
	}
	res, err := esconfig.ReadGroups(path)
	if errors.Is(err, esconfig.ErrNotConfigured) {
		writeError(w, http.StatusBadRequest, "no config path configured")
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "enshrouded_server.json not found at the configured path")
		return
	}
	if err != nil {
		s.logger.Error("reading role groups failed", "server", srv.ID, "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rolesPayload(res, viaAgent))
}

// handleUpdateRoles replaces the whole userGroups list.
//
// Whole-list rather than per-group patches because the order is part of
// the data and deletion has to be expressible; esconfig.ValidateGroups is
// what stops that generality from being a footgun.
func (s *Server) handleUpdateRoles(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	var req struct {
		Groups []esconfig.Group `json:"groups"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !s.editConfigFile(w, r, srv, esconfig.ErrNotConfigured, func(path string) error {
		return esconfig.WriteGroups(path, req.Groups)
	}) {
		return
	}

	// Audited by name and capability, never by password — the audit trail
	// is read by more people than the config is.
	names := make([]string, 0, len(req.Groups))
	for _, g := range req.Groups {
		name := strings.TrimSpace(g.Name)
		if g.CanKickBan {
			name += " (kick/ban)"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	s.audit(r, srv.ID, "roles-update", strings.Join(names, ", "))

	path, viaAgent, err := s.files.ConfigPath(r.Context(), srv)
	if err == nil {
		if res, rerr := esconfig.ReadGroups(path); rerr == nil {
			writeJSON(w, http.StatusOK, rolesPayload(res, viaAgent))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Bans.
//
// Gated on PermModerate: unlike the role groups, the ban list holds no
// credentials, and keeping a moderator out of it would mean the only
// people who could lift a ban are the people who can also read every
// password.
//
// What this cannot do is worth being precise about. Enshrouded has no
// RCON and no admin API, so nothing here ejects anyone: adding an id
// writes it to the file the game reads at start. The live half of
// moderation is the in-game player list, and it stays there.

type bansResponse struct {
	Bans     []esconfig.Ban `json:"bans"`
	Path     string         `json:"path"`
	Writable bool           `json:"writable"`
	// ObjectShape and Unreadable pass esconfig's reading of the file's
	// element format straight through; see internal/games/enshrouded/
	// esconfig/bans.go and the recon doc's open ledger row.
	ObjectShape bool `json:"objectShape"`
	Unreadable  int  `json:"unreadable"`
	// Running is why the console warns before an edit: the game owns this
	// list while it is up (the in-game ban UI writes it), so a file edit
	// under a live server can be overwritten when the game next persists.
	Running bool `json:"running"`
}

// banListRunning reports whether the game is up, for the overwrite
// warning. Best-effort: an agent that can't be reached is reported as not
// running rather than failing the read, because a ban list you can't see
// is worse than one whose warning is missing.
func (s *Server) banListRunning(r *http.Request, srv *store.Server) bool {
	_, health := s.agentSupervisor(r.Context(), srv)
	return health != nil && health.Game != nil && health.Game.State == "running"
}

func (s *Server) bansPayload(r *http.Request, srv *store.Server, res *esconfig.BanList, viaAgent bool) bansResponse {
	path := res.Path
	if viaAgent {
		path = "enshrouded_server.json · synced via flameagent"
	}
	return bansResponse{
		Bans:        res.Bans,
		Path:        path,
		Writable:    res.Writable,
		ObjectShape: res.ObjectShape,
		Unreadable:  res.Unreadable,
		Running:     s.banListRunning(r, srv),
	}
}

func (s *Server) handleGetBans(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	path, viaAgent, ok := s.resolveConfigPath(w, r, srv)
	if !ok {
		return
	}
	res, err := esconfig.ReadBans(path)
	if errors.Is(err, esconfig.ErrNotConfigured) {
		writeError(w, http.StatusBadRequest, "no config path configured")
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "enshrouded_server.json not found at the configured path")
		return
	}
	if err != nil {
		s.logger.Error("reading the ban list failed", "server", srv.ID, "error", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.bansPayload(r, srv, res, viaAgent))
}

func (s *Server) handleUpdateBans(w http.ResponseWriter, r *http.Request) {
	srv, ok := s.loadServer(w, r)
	if !ok {
		return
	}
	var req struct {
		Bans []esconfig.Ban `json:"bans"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Diff against what's on disk before writing, so the audit line names
	// who was banned and who was let back in rather than the list's size.
	var before []esconfig.Ban
	if path, _, err := s.files.ConfigPath(r.Context(), srv); err == nil {
		if res, rerr := esconfig.ReadBans(path); rerr == nil {
			before = res.Bans
		}
	}

	if !s.editConfigFile(w, r, srv, esconfig.ErrNotConfigured, func(path string) error {
		return esconfig.WriteBans(path, req.Bans)
	}) {
		return
	}
	if detail := banDiff(before, req.Bans); detail != "" {
		s.audit(r, srv.ID, "bans-update", detail)
	}

	path, viaAgent, err := s.files.ConfigPath(r.Context(), srv)
	if err == nil {
		if res, rerr := esconfig.ReadBans(path); rerr == nil {
			writeJSON(w, http.StatusOK, s.bansPayload(r, srv, res, viaAgent))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// banDiff renders the change for the audit trail. Account ids are the
// moderation record itself, so unlike passwords they belong in it.
func banDiff(before, after []esconfig.Ban) string {
	had := map[string]bool{}
	for _, b := range before {
		had[strings.TrimSpace(b.ID)] = true
	}
	now := map[string]bool{}
	for _, b := range after {
		now[strings.TrimSpace(b.ID)] = true
	}
	var added, removed []string
	for id := range now {
		if !had[id] {
			added = append(added, id)
		}
	}
	for id := range had {
		if !now[id] {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)

	var parts []string
	if len(added) > 0 {
		parts = append(parts, "banned "+strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		parts = append(parts, "unbanned "+strings.Join(removed, ", "))
	}
	return strings.Join(parts, "; ")
}
