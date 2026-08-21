// The companion inbox: character records relayed by the Artificer
// Companion app (born wkcompanion) running on players' own machines.
//
// The game keeps each character's record on that player's machine and
// caches it server-side only while they are connected (recon, "Where
// player state lives"), so the world save carries a full sheet for
// whoever is online and nothing for whoever is not. The console covers
// the gap two ways: it remembers the sheets it saw (records.go), and it
// accepts what players choose to share here — which is the only source
// for a character who has not been on since this console started, or who
// plays elsewhere entirely.
//
// The inbox is in-memory by design: the companion re-pushes on every
// change and on a steady heartbeat, so a console restart heals itself
// within minutes, and nothing a player shared outlives the console
// process without them still running the app that shares it. The token
// is the same trust shape as the public status page — a secret in the
// URL, minted and revoked by an admin — but write-scoped, so revoking it
// also drops everything it delivered.
package dwapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/artificer/core/api"
	"github.com/safwyls/artificer/games/dragonwilds/dwsave"
)

const (
	// maxCompanionBody comfortably fits a character record (tens of KB —
	// ~100 inventory slots, skills, progress lists) while refusing
	// anything that plainly isn't one.
	maxCompanionBody = 256 << 10
	// maxCompanionRecords is per server: a Dragonwilds world holds six
	// players; headroom covers retired characters without letting a leaked
	// token grow memory unbounded.
	maxCompanionRecords = 16
)

// companionRecord is one shared character, kept raw: the record is
// re-parsed against the current world's guid at merge time, so playtime
// resolves correctly even if the console's world changes.
type companionRecord struct {
	raw        []byte
	receivedAt time.Time
}

type companionInbox struct {
	mu       sync.Mutex
	byServer map[int64]map[string]companionRecord
}

func newCompanionInbox() *companionInbox {
	return &companionInbox{byServer: map[int64]map[string]companionRecord{}}
}

// put stores a record under its canonical guid. ok=false means the
// per-server cap is full and this guid is new.
func (in *companionInbox) put(serverID int64, guid string, raw []byte) bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	recs := in.byServer[serverID]
	if recs == nil {
		recs = map[string]companionRecord{}
		in.byServer[serverID] = recs
	}
	if _, exists := recs[guid]; !exists && len(recs) >= maxCompanionRecords {
		return false
	}
	recs[guid] = companionRecord{raw: append([]byte(nil), raw...), receivedAt: time.Now()}
	return true
}

func (in *companionInbox) drop(serverID int64) {
	in.mu.Lock()
	defer in.mu.Unlock()
	delete(in.byServer, serverID)
}

func (in *companionInbox) count(serverID int64) int {
	in.mu.Lock()
	defer in.mu.Unlock()
	return len(in.byServer[serverID])
}

// snapshot returns the records for one server; values are shared, not
// copied — callers only read.
func (in *companionInbox) snapshot(serverID int64) map[string]companionRecord {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := make(map[string]companionRecord, len(in.byServer[serverID]))
	for k, v := range in.byServer[serverID] {
		out[k] = v
	}
	return out
}

// handleCompanionPush accepts one character record from a companion app.
// The token in the path is the whole credential, like the public status
// page; a miss is a 404 with no hint of which part was wrong.
func (h *handlers) handleCompanionPush(w http.ResponseWriter, r *http.Request) {
	srv, err := h.s.StoreHandle().GetServerByCompanionToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxCompanionBody+1))
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "reading body")
		return
	}
	if len(body) > maxCompanionBody {
		api.WriteError(w, http.StatusRequestEntityTooLarge, "character record too large")
		return
	}
	// Parsed here only to validate and key it; the merge re-parses with
	// the world guid in hand.
	p, err := dwsave.ParseCharacterRecord(body, "")
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if p.CharGuid == "" {
		api.WriteError(w, http.StatusBadRequest, "character record has no char_guid")
		return
	}
	if !h.companion.put(srv.ID, dwsave.CanonicalGuid(p.CharGuid), body) {
		api.WriteError(w, http.StatusConflict, "this server already holds its limit of shared characters")
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"accepted":  true,
		"server":    srv.Name,
		"character": p.CharName,
	})
}

// handleCompanionDownload hands out the companion app itself. The console
// image cross-compiles artificer-companion.exe at build time and ships it
// beside the console binary, so the admin's "give players this link" is
// the whole distribution story. Token-gated like the rest of the tier — the exe
// isn't a secret, but an open path invites scraping and the token link is
// the one players are handed anyway.
func (h *handlers) handleCompanionDownload(w http.ResponseWriter, r *http.Request) {
	if _, err := h.s.StoreHandle().GetServerByCompanionToken(r.Context(), chi.URLParam(r, "token")); err != nil {
		api.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if h.companionExe == "" {
		api.WriteError(w, http.StatusNotFound, "this deployment ships without the companion app — build it with: GOOS=windows GOARCH=amd64 go build ./cmd/companion")
		return
	}
	f, err := os.Open(h.companionExe)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "companion app not present in this deployment ("+h.companionExe+")")
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "reading companion app")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.microsoft.portable-executable")
	w.Header().Set("Content-Disposition", `attachment; filename="artificer-companion.exe"`)
	http.ServeContent(w, r, "artificer-companion.exe", fi.ModTime(), f)
}

// handleCompanionPing lets a companion app verify its configuration
// without sending anything: a valid token answers with the server's name.
func (h *handlers) handleCompanionPing(w http.ResponseWriter, r *http.Request) {
	srv, err := h.s.StoreHandle().GetServerByCompanionToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"server": srv.Name})
}

// handleGetCompanion reports the sharing state for the admin UI.
func (h *handlers) handleGetCompanion(w http.ResponseWriter, r *http.Request) {
	srv, ok := h.s.LoadServer(w, r)
	if !ok {
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled": srv.CompanionToken != "",
		"token":   srv.CompanionToken,
		"shared":  h.companion.count(srv.ID),
	})
}

// handleSetCompanion turns character sharing on (minting a fresh token —
// re-enabling after a disable revokes every copy players hold) or off,
// dropping everything the old token delivered.
func (h *handlers) handleSetCompanion(w http.ResponseWriter, r *http.Request) {
	srv, ok := h.s.LoadServer(w, r)
	if !ok {
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	token := ""
	if in.Enabled {
		buf := make([]byte, 16)
		if _, err := rand.Read(buf); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}
		token = hex.EncodeToString(buf)
	}
	if err := h.s.StoreHandle().SetCompanionToken(r.Context(), srv.ID, token); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to update companion sharing")
		return
	}
	if !in.Enabled {
		h.companion.drop(srv.ID)
		// Revoking sharing forgets what players shared, including sheets
		// already folded into the console's memory (records.go).
		h.records.forgetCompanionSourced(srv.ID)
	}
	detail := "off"
	if in.Enabled {
		detail = "on"
	}
	h.s.Audit(r, srv.ID, "companion-sharing", detail)
	api.WriteJSON(w, http.StatusOK, map[string]any{"enabled": in.Enabled, "token": token})
}
