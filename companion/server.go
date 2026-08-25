package companion

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"time"

	web "github.com/safwyls/artificer/web/companion"
)

// ui is the built React frontend, embedded in web/companion the way the
// consoles and reliquary embed theirs. It must be embedded rather than
// served from disk: the companion is one exe a player downloads, with no
// installer and nothing beside it.
var ui = func() fs.FS {
	dist, err := web.Dist()
	if err != nil {
		// Only reachable if the binary was built without a frontend
		// build, which the Dockerfile and release workflow both do.
		panic("companion frontend missing from build: " + err.Error())
	}
	return dist
}()

// ServerOptions are the per-shell parts of the local server. The
// browser build passes none of them and gets exactly the surface it
// always had; cmd/companiond fills them in.
type ServerOptions struct {
	// Token, when set, is required as `Authorization: Bearer <token>` on
	// every request except GET /healthz. A loopback listener is reachable
	// by every process on the machine — including a page in the player's
	// browser — so the daemon an Electron shell spawns must not be
	// answerable to anything that did not get the token from that shell.
	//
	// Empty means no auth, which is the browser build's shape: it serves
	// the page itself, to a browser that cannot be handed a secret before
	// the first request.
	Token string
	// Raise focuses whatever window the shell owns, for POST /api/raise.
	// Nil where the shell has no window; the route then answers 501
	// naming where the ability actually lives, per the repo rule that a
	// missing ability answers with a reason rather than hiding.
	Raise func() error
}

// Routes is the local server the browser build serves: no auth, no
// window to raise.
func (a *App) Routes() http.Handler { return a.RoutesWithOptions(ServerOptions{}) }

// RoutesWithOptions is Routes with the shell-specific parts filled in.
func (a *App) RoutesWithOptions(opt ServerOptions) http.Handler {
	mux := http.NewServeMux()
	// Liveness, deliberately outside the token check: a shell polls this
	// to know when the daemon is up, and it must be able to do that
	// before it hands the token to anything. It reports no state beyond
	// "this process is answering" — see docs/companion-api-surface.md.
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	// The push side of the state surface (SSE, matching core's custody
	// stream in core/api/savesync_live.go) and the shell handshake.
	mux.HandleFunc("GET /api/events", a.handleEvents)
	mux.HandleFunc("POST /api/raise", a.handleRaise(opt.Raise))
	// The page and its assets. Everything that is not /api is the
	// frontend; there is no router in it, so index.html is the only
	// document — but the hashed JS/CSS beside it must be served too.
	mux.Handle("GET /", http.FileServerFS(ui))
	mux.HandleFunc("GET /api/state", a.handleState)
	mux.HandleFunc("PUT /api/config", a.handleSetConfig)
	mux.HandleFunc("POST /api/discover", a.handleDiscover)
	mux.HandleFunc("GET /api/artwork", a.handleArtwork)
	// Shelf housekeeping: browsing this machine for a save folder, and
	// putting non-game entries away (browse.go, hidden.go).
	mux.HandleFunc("POST /api/sync/refresh", a.handleSyncNow)
	mux.HandleFunc("GET /api/savehints", a.handleSaveHints)
	mux.HandleFunc("GET /api/history", a.handleHistory)
	mux.HandleFunc("GET /api/browse", a.handleBrowse)
	// The two halves of a save folder (savepath.go): where does this
	// folder split, and where does an existing world live under mine.
	mux.HandleFunc("GET /api/savepath/split", a.handleSplitSavePath)
	mux.HandleFunc("POST /api/savepath/resolve", a.handleResolveSavePath)
	mux.HandleFunc("POST /api/hide", a.handleHide)
	// World links and custody. Local-only like everything here; the real
	// authorization is the sync token these calls carry upstream.
	mux.HandleFunc("POST /api/links", a.handleAddLink)
	mux.HandleFunc("POST /api/links/create", a.handleCreateWorld)
	mux.HandleFunc("PUT /api/links/{worldID}", a.handleUpdateLink)
	mux.HandleFunc("POST /api/links/{worldID}/launch", a.linkAction((*App).Launch))
	// Keeping this build current (update.go). Local-only like the rest;
	// what it reaches out to is GitHub's public release API.
	mux.HandleFunc("POST /api/update/check", a.handleCheckUpdate)
	mux.HandleFunc("POST /api/update/apply", a.handleApplyUpdate)
	mux.HandleFunc("DELETE /api/links/{worldID}", a.linkAction((*App).Unlink))
	mux.HandleFunc("POST /api/links/{worldID}/checkout", a.handleCheckout)
	mux.HandleFunc("POST /api/links/{worldID}/checkin", a.linkAction((*App).Checkin))
	mux.HandleFunc("POST /api/links/{worldID}/checkpoint", a.linkAction((*App).Checkpoint))
	mux.HandleFunc("POST /api/links/{worldID}/renew", a.linkAction((*App).Renew))
	mux.HandleFunc("POST /api/links/{worldID}/claim", a.linkAction((*App).Claim))
	if opt.Token == "" {
		return mux
	}
	return requireToken(opt.Token, mux)
}

// requireToken rejects anything without the bearer token — except
// GET /healthz, which is how a shell learns the daemon is up.
//
// The comparison is constant-time: this is a local secret an unrelated
// process on the same machine could otherwise guess a byte at a time.
func requireToken(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": "missing or wrong bearer token",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleHealthz says only that this process is answering. No custody,
// no config, no version-shaped secrets: it is the one route a shell may
// call before it has proven anything.
func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true, "version": Version})
}

// handleRaise is the single-instance handshake: whatever is already
// running owns the window, and a second launch asks it to come forward
// rather than starting a rival process. The daemon outlives its shell,
// so the route belongs here even though the raising itself does not.
func (a *App) handleRaise(raise func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if raise == nil {
			// A reason, not silence: this build has no window, and the
			// caller is told which one does.
			w.WriteHeader(http.StatusNotImplemented)
			writeJSON(w, map[string]any{
				"ok": false,
				"error": "this companion build has no window to raise — " +
					"the desktop shell (Electron main) owns that; the browser " +
					"build's page is at the address this server prints",
			})
			return
		}
		if err := raise(); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "raised": true})
	}
}

// eventKeepalive paces the comment frames that keep an idle stream from
// being reaped. Same figure and same reason as core's custody stream
// (core/api/savesync_live.go).
const eventKeepalive = 25 * time.Second

// handleEvents streams engine change nudges as server-sent events —
// the HTTP face of the in-process Subscribe() a same-process shell uses
// (facade.go). SSE rather than a WebSocket because that is what core
// already does for custody events and because this is one direction
// with no payload: the client re-reads GET /api/state, which keeps a
// dropped or coalesced event harmless.
//
// A connected stream also counts as someone looking, the way the page's
// poll does — otherwise a renderer that stopped polling because it has
// a live stream would quietly get minute-old custody.
func (a *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]any{"ok": false, "error": "this server cannot stream events"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	nudges, unsubscribe := a.Subscribe()
	defer unsubscribe()

	a.MarkSeen()
	// An opening frame proves the stream is live before anything
	// happens, so the renderer can show "live" instead of waiting for
	// the first change to find out.
	fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(eventKeepalive)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			// Still watching, so keep the custody poll at the page's pace.
			a.MarkSeen()
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case _, ok := <-nudges:
			if !ok {
				return
			}
			fmt.Fprint(w, "event: changed\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	// Someone is looking. Note it, and start a poll if the view has gone
	// stale — in the background, because the page asks every few seconds
	// and must not wait on the service to render. The next ask shows the
	// answer, which is what makes an open page feel live without the
	// background loop having to poll this hard all day.
	a.MarkSeen()
	// One assembly for both shells: this is exactly the value the desktop
	// window renders (facade.go), marshalled.
	writeJSON(w, a.Snapshot())
}

// handleSetConfig saves whichever settings the request carries — the
// connection panel and the discovery panel post independently, so absent
// fields keep their stored values (pointers make absent distinguishable
// from cleared). A completed connection is proven with a status poll —
// a typo'd token should fail here, not silently every minute forever;
// an empty token keeps the saved one. New Steam folders trigger a
// rescan.
func (a *App) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ServerURL        *string   `json:"serverUrl"`
		Token            string    `json:"token"`
		SteamDirs        *[]string `json:"steamDirs"`
		LaunchOnCheckout *bool     `json:"launchOnCheckout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	if err := a.SetConfig(ConfigUpdate{
		ServerURL:        in.ServerURL,
		Token:            in.Token,
		SteamDirs:        in.SteamDirs,
		LaunchOnCheckout: in.LaunchOnCheckout,
	}); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleDiscover(w http.ResponseWriter, r *http.Request) {
	a.Rescan()
	a.mu.Lock()
	found := len(a.discovered.Games)
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "found": found})
}

// handleArtwork answers cover art for the discovered games, resolved
// through the sync service (which holds the IGDB credentials).
func (a *App) handleArtwork(w http.ResponseWriter, r *http.Request) {
	art := a.Artwork()
	asked, failure := a.ArtStatus()
	writeJSON(w, map[string]any{"ok": true, "art": art, "asked": asked, "error": failure})
}

// handleSyncNow polls the service immediately and answers with what
// happened. The page's own poll keeps things fresh while it is open;
// this is for the moment someone wants to be certain rather than
// patient — and for saying plainly when the service cannot be reached,
// which a silent background poll never does.
func (a *App) handleSyncNow(w http.ResponseWriter, r *http.Request) {
	worlds, err := a.SyncNow()
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "worlds": worlds})
}

// handleSaveHints asks the service for the catalogue's locations and
// folds them into the discovered games' candidates. Driven by the page
// when the game set changes, like artwork.
// handleHistory answers both the Activity and the Conflicts views: one
// merged read of every linked world's version list from the vault, which
// is the only thing that can see either. `?refresh=1` skips the cache,
// for the view's own refresh control.
//
// It answers 200 with `ok:false` and a reason rather than an error
// status, the same shape the other read-only panels use — a view that
// cannot load has something to say, and the page draws the reason.
func (a *App) handleHistory(w http.ResponseWriter, r *http.Request) {
	var (
		h   History
		err error
	)
	if r.URL.Query().Get("refresh") != "" {
		h, err = a.RefreshHistory()
	} else {
		h, err = a.History()
	}
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{
		"ok":        true,
		"entries":   h.Entries,
		"failed":    h.Failed,
		"fetchedAt": h.FetchedAt,
		"truncated": h.Truncated,
	})
}

func (a *App) handleSaveHints(w http.ResponseWriter, r *http.Request) {
	available, known, failure := a.SaveHints()
	writeJSON(w, map[string]any{"ok": true, "available": available, "known": known, "error": failure})
}

// handleSplitSavePath reports where a chosen folder divides into the
// part a joining player supplies and the part the world carries with it.
// The page shows the answer before anything is recorded, because a guess
// nobody can see is a guess nobody can correct.
func (a *App) handleSplitSavePath(w http.ResponseWriter, r *http.Request) {
	split, err := a.SplitSavePath(r.URL.Query().Get("dir"), r.URL.Query().Get("appId"), r.URL.Query().Get("name"))
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "split": split})
}

// handleResolveSavePath joins a world's own folder under a root the
// player chose, creating it when asked. This is the join flow: the
// second player to take a world cannot type an opaque id they have never
// seen, so they supply the half they know and the companion makes the
// rest.
func (a *App) handleResolveSavePath(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Root   string `json:"root"`
		Leaf   string `json:"leaf"`
		Create bool   `json:"create"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	dir, exists, err := a.ResolveSavePath(in.Root, in.Leaf, in.Create)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "dir": dir, "exists": exists})
}

func (a *App) handleAddLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		WorldID   int64  `json:"worldId"`
		GameTitle string `json:"gameTitle"`
		Dir       string `json:"dir"`
		Meta      string `json:"meta"`
		AppID     string `json:"appId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.WorldID == 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	if err := a.Link(in.WorldID, in.GameTitle, in.Dir, in.Meta, in.AppID); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleCreateWorld(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		GameTitle string `json:"gameTitle"`
		Dir       string `json:"dir"`
		Meta      string `json:"meta"`
		AppID     string `json:"appId"`
		SavePath  string `json:"savePath"`
		Seed      bool   `json:"seed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	if err := a.CreateWorld(in.Name, in.GameTitle, in.Dir, in.Meta, in.AppID, in.SavePath, in.Seed); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleCheckout takes the world and, when the setting allows and the
// world has something to start, plays it. The answer says which of those
// happened: a save on disk with a game that would not start is a real
// outcome the page has to be able to explain.
func (a *App) handleCheckout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Takeover bool  `json:"takeover"`
		Play     *bool `json:"play"`
	}
	json.NewDecoder(r.Body).Decode(&in) // an empty body is a plain checkout
	id, err := strconv.ParseInt(r.PathValue("worldID"), 10, 64)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid world id"})
		return
	}
	// Play defaults to true — "check out & play" is the existing one-button
	// flow. An explicit false is the plain-checkout button: the save lands
	// on this machine and nothing is launched, no matter what the
	// launch-on-checkout setting says.
	play := in.Play == nil || *in.Play
	out, err := a.Checkout(id, in.Takeover, play)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	body := map[string]any{"ok": true, "launched": out.Launched}
	if out.LaunchError != nil {
		body["launchError"] = out.LaunchError.Error()
	}
	writeJSON(w, body)
}

// handleCheckUpdate asks GitHub now rather than waiting for the timer —
// the same "be certain rather than patient" the sync-now button serves.
func (a *App) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	st := a.CheckUpdate(r.Context())
	writeJSON(w, map[string]any{"ok": st.Error == "", "update": st, "error": st.Error})
}

// handleApplyUpdate replaces this binary and restarts into the new one.
// The answer goes out *before* the restart, because the page is served
// by the process that is about to exit — a reply written afterwards
// would never arrive, and the player would see a failed request for an
// update that actually worked.
func (a *App) handleApplyUpdate(w http.ResponseWriter, r *http.Request) {
	// Not r.Context(): that is cancelled the moment this response is
	// written, and the download outlives it.
	if err := a.ApplyUpdate(context.Background()); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// In installer mode nothing was replaced and there is nothing to
	// restart: an installer is waiting, and running it is the shell's
	// job because it means quitting the app this daemon lives inside.
	// The path goes back so the page can hand it over.
	if staged := a.StagedInstaller(); staged != "" {
		writeJSON(w, map[string]any{"ok": true, "installer": staged})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "restarting": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	// Hand the page a moment to receive that, then swap processes.
	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := a.RestartAfterUpdate(); err != nil {
			log.Printf("update: restarting: %v", err)
		}
	}()
}

// handleUpdateLink edits the parts of a link the player owns: the launch
// target, the local folder it points at, and — since that lives on the
// service, not here — the world's name.
func (a *App) handleUpdateLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("worldID"), 10, 64)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid world id"})
		return
	}
	var in struct {
		LaunchTarget *string `json:"launchTarget"`
		Dir          *string `json:"dir"`
		WorldName    *string `json:"worldName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	if err := a.EditLink(id, LinkEdit{LaunchTarget: in.LaunchTarget, Dir: in.Dir, WorldName: in.WorldName}); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// linkAction adapts a per-world verb into a local handler.
func (a *App) linkAction(fn func(*App, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("worldID"), 10, 64)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "invalid world id"})
			return
		}
		if !a.SyncConfigured() {
			writeJSON(w, map[string]any{"ok": false, "error": "set the server URL and token first"})
			return
		}
		if err := fn(a, id); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
