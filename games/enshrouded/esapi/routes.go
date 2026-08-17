package esapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/artificer/core/api"
	"github.com/safwyls/artificer/core/store"
	"github.com/safwyls/artificer/games/enshrouded/banqueue"
	"github.com/safwyls/artificer/games/enshrouded/esconfig"
)

// Mount builds Enshrouded's contributed per-server routes for
// api.Server.GameRoutes: the role groups behind PermSettings (each one
// carries its join password in the clear, so reading the list is reading
// the server's credentials) and the ban list behind PermModerate (no
// credentials — a moderator must be able to lift a ban without being
// handed every password).
func Mount(s *api.Server) func(chi.Router) {
	h := &handlers{s: s}
	return func(r chi.Router) {
		r.With(s.RequirePermission(store.PermSettings)).Get("/config/roles", h.handleGetRoles)
		r.With(s.RequirePermission(store.PermSettings)).Put("/config/roles", h.handleUpdateRoles)
		r.With(s.RequirePermission(store.PermModerate)).Get("/bans", h.handleGetBans)
		r.With(s.RequirePermission(store.PermModerate)).Put("/bans", h.handleUpdateBans)
	}
}

type handlers struct{ s *api.Server }

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

func (h *handlers) handleGetRoles(w http.ResponseWriter, r *http.Request) {
	srv, ok := h.s.LoadServer(w, r)
	if !ok {
		return
	}
	path, viaAgent, ok := h.s.ResolveConfigPath(w, r, srv)
	if !ok {
		return
	}
	res, err := esconfig.ReadGroups(path)
	if errors.Is(err, esconfig.ErrNotConfigured) {
		api.WriteError(w, http.StatusBadRequest, "no config path configured")
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		api.WriteError(w, http.StatusNotFound, "enshrouded_server.json not found at the configured path")
		return
	}
	if err != nil {
		h.s.LoggerHandle().Error("reading role groups failed", "server", srv.ID, "error", err)
		api.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, rolesPayload(res, viaAgent))
}

// handleUpdateRoles replaces the whole userGroups list.
//
// Whole-list rather than per-group patches because the order is part of
// the data and deletion has to be expressible; esconfig.ValidateGroups is
// what stops that generality from being a footgun.
func (h *handlers) handleUpdateRoles(w http.ResponseWriter, r *http.Request) {
	srv, ok := h.s.LoadServer(w, r)
	if !ok {
		return
	}
	var req struct {
		Groups []esconfig.Group `json:"groups"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !h.s.EditConfigFile(w, r, srv, esconfig.ErrNotConfigured, func(path string) error {
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
	h.s.Audit(r, srv.ID, "roles-update", strings.Join(names, ", "))

	path, viaAgent, err := h.s.FilesHandle().ConfigPath(r.Context(), srv)
	if err == nil {
		if res, rerr := esconfig.ReadGroups(path); rerr == nil {
			api.WriteJSON(w, http.StatusOK, rolesPayload(res, viaAgent))
			return
		}
	}
	api.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
	// Running means the game is up and holding this list. An edit made now
	// is queued rather than trusted to the file — see Pending.
	Running bool `json:"running"`
	// Pending are the console's edits the file does not reflect yet. They
	// are written into the config immediately before the next start.
	Pending []store.PendingBanEdit `json:"pending"`
	// Reverted is the diagnosis, not a warning: these edits *were* written
	// into the config with the game stopped, and the file no longer agrees.
	// Only the game can have done that, which means bans made outside the
	// game don't stick on this build.
	Reverted []store.PendingBanEdit `json:"reverted"`
}

// banListRunning reports whether the game is up, for the overwrite
// warning. Best-effort: an agent that can't be reached is reported as not
// running rather than failing the read, because a ban list you can't see
// is worse than one whose warning is missing.
func (h *handlers) banListRunning(r *http.Request, srv *store.Server) bool {
	_, health := h.s.AgentSupervisor(r.Context(), srv)
	return health != nil && health.Game != nil && health.Game.State == "running"
}

// bansPayload assembles the response, reconciling the queued edits against
// what the file actually says on the way. Reading is where retirement
// happens: a queued edit the file now agrees with is done and goes away,
// and one that was applied to a stopped server and *still* disagrees is
// the game having overwritten it.
func (h *handlers) bansPayload(r *http.Request, srv *store.Server, res *esconfig.BanList, viaAgent bool) bansResponse {
	path := res.Path
	if viaAgent {
		path = "enshrouded_server.json · synced via flameagent"
	}
	running := h.banListRunning(r, srv)
	out := bansResponse{
		Bans:        res.Bans,
		Path:        path,
		Writable:    res.Writable,
		ObjectShape: res.ObjectShape,
		Unreadable:  res.Unreadable,
		Running:     running,
		Pending:     []store.PendingBanEdit{},
		Reverted:    []store.PendingBanEdit{},
	}

	ids := make([]string, 0, len(res.Bans))
	for _, b := range res.Bans {
		ids = append(ids, b.ID)
	}
	outstanding, err := h.s.StoreHandle().ReconcilePendingBans(r.Context(), srv.ID, ids, running)
	if err != nil {
		// A bookkeeping failure must not cost the operator the ban list
		// itself; the queue is reported empty and retried next read.
		h.s.LoggerHandle().Error("reconciling pending bans failed", "server", srv.ID, "error", err)
		return out
	}
	for _, e := range outstanding {
		if e.Applied {
			// Wanted, written, and the game threw it away. Deliberately not
			// folded into Bans below: showing it as banned would repeat the
			// original lie in a new place.
			out.Reverted = append(out.Reverted, e)
			continue
		}
		out.Pending = append(out.Pending, e)
		out.Bans = banqueue.Apply1(out.Bans, e)
	}
	return out
}

// effectiveBans is the ban list as the console presents it: what the file
// holds, plus the edits waiting for the next restart. It is the state a
// client's next write is a diff against, which is why it exists rather
// than each caller re-deriving it.
func (h *handlers) effectiveBans(ctx context.Context, srv *store.Server) []esconfig.Ban {
	path, _, err := h.s.FilesHandle().ConfigPath(ctx, srv)
	if err != nil {
		return nil
	}
	res, err := esconfig.ReadBans(path)
	if err != nil {
		return nil
	}
	pending, err := h.s.StoreHandle().PendingBans(ctx, srv.ID)
	if err != nil {
		return res.Bans
	}
	bans := res.Bans
	for _, e := range pending {
		bans = banqueue.Apply1(bans, e)
	}
	return bans
}

func (h *handlers) handleGetBans(w http.ResponseWriter, r *http.Request) {
	srv, ok := h.s.LoadServer(w, r)
	if !ok {
		return
	}
	path, viaAgent, ok := h.s.ResolveConfigPath(w, r, srv)
	if !ok {
		return
	}
	res, err := esconfig.ReadBans(path)
	if errors.Is(err, esconfig.ErrNotConfigured) {
		api.WriteError(w, http.StatusBadRequest, "no config path configured")
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		api.WriteError(w, http.StatusNotFound, "enshrouded_server.json not found at the configured path")
		return
	}
	if err != nil {
		h.s.LoggerHandle().Error("reading the ban list failed", "server", srv.ID, "error", err)
		api.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, h.bansPayload(r, srv, res, viaAgent))
}

func (h *handlers) handleUpdateBans(w http.ResponseWriter, r *http.Request) {
	srv, ok := h.s.LoadServer(w, r)
	if !ok {
		return
	}
	var req struct {
		Bans []esconfig.Ban `json:"bans"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Diff against the list the client was shown — the file *with the
	// queued edits applied* — not the raw file. They differ whenever
	// something is waiting for a restart, and diffing against the file
	// there would read "no change" for exactly the edits that undo a
	// queued one: removing a pending ban before it ever reached the game
	// would silently leave it queued.
	before := h.effectiveBans(r.Context(), srv)

	// Who holds the file decides how the edit is made. With the game
	// stopped the console can simply write it. With the game up it cannot:
	// the game keeps this array in memory and writes it back out when it
	// stops, so a write now is erased at the next stop — which is the bug
	// a real deployment hit on 2026-08-16. Writing anyway would be worse
	// than useless; it would put the change in the file, let the read-back
	// confirm it, and lose it hours later with nothing to show the
	// operator. So while the game is up the edit is only queued, and
	// internal/banqueue writes it during the next restart.
	running := h.banListRunning(r, srv)
	if !running {
		if !h.s.EditConfigFile(w, r, srv, esconfig.ErrNotConfigured, func(path string) error {
			return esconfig.WriteBans(path, req.Bans)
		}) {
			return
		}
	} else if err := esconfig.ValidateBans(req.Bans); err != nil {
		// The write is what normally validates. Queued edits skip it, so
		// the check has to happen here or a junk id reaches the queue.
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	added, removed := banDelta(before, req.Bans)
	for _, id := range added {
		if err := h.s.StoreHandle().QueueBanEdit(r.Context(), srv.ID, id, store.PendingBan); err != nil {
			h.s.LoggerHandle().Error("queueing a ban failed", "server", srv.ID, "error", err)
		}
	}
	for _, id := range removed {
		if err := h.s.StoreHandle().QueueBanEdit(r.Context(), srv.ID, id, store.PendingLift); err != nil {
			h.s.LoggerHandle().Error("queueing a lift failed", "server", srv.ID, "error", err)
		}
	}
	// A write made with the game down is already in the config, so the
	// queued rows are marked applied: if the file later disagrees, that is
	// the game having overwritten them, and the panel says so instead of
	// silently showing the ban gone again.
	if !running {
		if err := h.s.StoreHandle().MarkPendingBansApplied(r.Context(), srv.ID, time.Now()); err != nil {
			h.s.LoggerHandle().Error("marking ban edits applied failed", "server", srv.ID, "error", err)
		}
	}
	if detail := banDiffDetail(added, removed); detail != "" {
		h.s.Audit(r, srv.ID, "bans-update", detail)
	}

	path, viaAgent, err := h.s.FilesHandle().ConfigPath(r.Context(), srv)
	if err == nil {
		if res, rerr := esconfig.ReadBans(path); rerr == nil {
			api.WriteJSON(w, http.StatusOK, h.bansPayload(r, srv, res, viaAgent))
			return
		}
	}
	api.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// banDelta reports what changed between the file and the request. It
// drives both the queue and the audit line, so the two can't disagree
// about what the operator asked for.
func banDelta(before, after []esconfig.Ban) (added, removed []string) {
	had := map[string]bool{}
	for _, b := range before {
		had[strings.TrimSpace(b.ID)] = true
	}
	now := map[string]bool{}
	for _, b := range after {
		now[strings.TrimSpace(b.ID)] = true
	}
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
	return added, removed
}

// banDiffDetail renders the change for the audit trail. Account ids are
// the moderation record itself, so unlike passwords they belong in it.
func banDiffDetail(added, removed []string) string {
	var parts []string
	if len(added) > 0 {
		parts = append(parts, "banned "+strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		parts = append(parts, "unbanned "+strings.Join(removed, ", "))
	}
	return strings.Join(parts, "; ")
}
