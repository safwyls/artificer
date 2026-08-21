package main

import (
	"embed"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

//go:embed ui
var uiFS embed.FS

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data, err := uiFS.ReadFile("ui/index.html")
		if err != nil {
			http.Error(w, "ui missing from build", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
	mux.HandleFunc("GET /api/state", a.handleState)
	mux.HandleFunc("PUT /api/config", a.handleSetConfig)
	mux.HandleFunc("POST /api/discover", a.handleDiscover)
	// World links and custody. Local-only like everything here; the real
	// authorization is the sync token these calls carry upstream.
	mux.HandleFunc("POST /api/links", a.handleAddLink)
	mux.HandleFunc("POST /api/links/create", a.handleCreateWorld)
	mux.HandleFunc("DELETE /api/links/{worldID}", a.linkAction(func(a *app, id int64) error { return a.unlink(id) }))
	mux.HandleFunc("POST /api/links/{worldID}/checkout", a.handleCheckout)
	mux.HandleFunc("POST /api/links/{worldID}/checkin", a.linkAction((*app).syncCheckin))
	mux.HandleFunc("POST /api/links/{worldID}/checkpoint", a.linkAction((*app).syncCheckpointNow))
	mux.HandleFunc("POST /api/links/{worldID}/renew", a.linkAction((*app).syncRenew))
	mux.HandleFunc("POST /api/links/{worldID}/claim", a.linkAction((*app).syncClaim))
	return mux
}

func (a *app) handleState(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	st := a.worldSync
	st.Configured = a.cfg.configured()
	links := append([]WorldLink(nil), a.cfg.Links...)
	discovered := a.discovered
	out := map[string]any{
		"config": map[string]any{
			"serverUrl": a.cfg.ServerURL,
			"tokenSet":  a.cfg.Token != "",
			"steamDirs": append([]string(nil), a.cfg.SteamDirs...),
		},
		"links":      links,
		"discovered": discovered,
		"sync":       st,
	}
	a.mu.Unlock()
	writeJSON(w, out)
}

// handleSetConfig saves whichever settings the request carries — the
// connection panel and the discovery panel post independently, so absent
// fields keep their stored values (pointers make absent distinguishable
// from cleared). A completed connection is proven with a status poll —
// a typo'd token should fail here, not silently every minute forever;
// an empty token keeps the saved one. New Steam folders trigger a
// rescan.
func (a *app) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ServerURL *string   `json:"serverUrl"`
		Token     string    `json:"token"`
		SteamDirs *[]string `json:"steamDirs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	a.mu.Lock()
	if in.ServerURL != nil {
		a.cfg.ServerURL = normalizeServerURL(*in.ServerURL)
	}
	if strings.TrimSpace(in.Token) != "" {
		a.cfg.Token = strings.TrimSpace(in.Token)
	}
	if in.SteamDirs != nil {
		dirs := make([]string, 0, len(*in.SteamDirs))
		for _, d := range *in.SteamDirs {
			if d = strings.TrimSpace(d); d != "" {
				dirs = append(dirs, d)
			}
		}
		a.cfg.SteamDirs = dirs
	}
	a.mu.Unlock()
	if err := a.saveCfg(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "saving config: " + err.Error()})
		return
	}
	if in.SteamDirs != nil {
		a.rescan()
	}
	if in.ServerURL != nil && a.syncConfigured() {
		if err := a.syncRefresh(); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *app) handleDiscover(w http.ResponseWriter, r *http.Request) {
	a.rescan()
	a.mu.Lock()
	found := len(a.discovered.Games)
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": true, "found": found})
}

func (a *app) handleAddLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		WorldID   int64  `json:"worldId"`
		GameTitle string `json:"gameTitle"`
		Dir       string `json:"dir"`
		Meta      string `json:"meta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.WorldID == 0 {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	if err := a.linkWorld(in.WorldID, in.GameTitle, strings.TrimSpace(in.Dir), in.Meta); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *app) handleCreateWorld(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		GameTitle string `json:"gameTitle"`
		Dir       string `json:"dir"`
		Meta      string `json:"meta"`
		Seed      bool   `json:"seed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	if err := a.createWorld(strings.TrimSpace(in.Name), in.GameTitle, strings.TrimSpace(in.Dir), in.Meta, in.Seed); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *app) handleCheckout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Takeover bool `json:"takeover"`
	}
	json.NewDecoder(r.Body).Decode(&in) // an empty body is a plain checkout
	a.linkAction(func(a *app, id int64) error { return a.syncCheckout(id, in.Takeover) })(w, r)
}

// linkAction adapts a per-world verb into a local handler.
func (a *app) linkAction(fn func(*app, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("worldID"), 10, 64)
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": "invalid world id"})
			return
		}
		if !a.syncConfigured() {
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
