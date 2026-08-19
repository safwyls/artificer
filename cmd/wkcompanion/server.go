package main

import (
	"embed"
	"encoding/json"
	"net/http"
	"sort"
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
	mux.HandleFunc("GET /data/", func(w http.ResponseWriter, r *http.Request) {
		data, err := uiFS.ReadFile("ui" + r.URL.Path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
	mux.HandleFunc("GET /api/state", a.handleState)
	mux.HandleFunc("PUT /api/config", a.handleSetConfig)
	mux.HandleFunc("POST /api/push", a.handlePushNow)
	return mux
}

func (a *app) handleState(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	chars := make([]*character, 0, len(a.characters))
	for _, c := range a.characters {
		chars = append(chars, c)
	}
	sort.Slice(chars, func(i, j int) bool { return chars[i].File < chars[j].File })
	out := map[string]any{
		"characters": chars,
		"config": map[string]any{
			"consoleUrl":  a.cfg.ConsoleURL,
			"tokenSet":    a.cfg.Token != "",
			"saveDir":     a.cfg.SaveDir,
			"detectedDir": a.cfg.resolveSaveDir(),
		},
		"relay":   a.relay,
		"scanErr": a.scanErr,
	}
	a.mu.Unlock()
	writeJSON(w, out)
}

func (a *app) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ConsoleURL string `json:"consoleUrl"`
		Token      string `json:"token"`
		SaveDir    string `json:"saveDir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	in.ConsoleURL = normalizeConsoleURL(in.ConsoleURL)

	// A configured relay is verified before it is saved: a typo'd token
	// should fail here, not silently every 15 seconds forever.
	server := ""
	if in.ConsoleURL != "" && in.Token != "" {
		var err error
		if server, err = a.ping(in.ConsoleURL, in.Token); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}

	a.mu.Lock()
	a.cfg.ConsoleURL = in.ConsoleURL
	a.cfg.Token = in.Token
	a.cfg.SaveDir = in.SaveDir
	a.relay = relayStatus{Configured: in.ConsoleURL != "" && in.Token != "", Server: server}
	// Sharing turned off or retargeted: what was pushed no longer counts.
	for _, c := range a.characters {
		c.PushedAt = nil
	}
	cfg, path := a.cfg, a.cfgPath
	a.mu.Unlock()

	if err := saveConfig(path, cfg); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": "saving config: " + err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "server": server})
}

func (a *app) handlePushNow(w http.ResponseWriter, r *http.Request) {
	if !a.relayConfigured() {
		writeJSON(w, map[string]any{"ok": false, "error": "no console configured"})
		return
	}
	a.scan()
	ok := a.pushChanged(true)
	a.mu.Lock()
	rs := a.relay
	a.mu.Unlock()
	writeJSON(w, map[string]any{"ok": ok, "relay": rs})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
