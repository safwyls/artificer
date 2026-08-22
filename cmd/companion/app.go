package main

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type app struct {
	cfgPath string
	client  *http.Client

	mu  sync.Mutex
	cfg Config
	// worldSync is the custody state the page shows (sync.go).
	worldSync syncState
	// lastCheckpoint tracks the auto-checkpoint pushes per world.
	lastCheckpoint map[int64]time.Time
	// discovered is the installed-games scan (discover.go), refreshed on
	// demand — a filesystem walk, not something to run every tick.
	discovered discovery
	// art caches cover lookups the service answered, misses included, so
	// a rescan doesn't re-ask for games IGDB has never heard of.
	art map[string]gameArt
	// artError is why the last lookup failed, shown under the shelf.
	// Artwork is decoration and never blocks custody, but a shelf with
	// no covers should still be able to say what went wrong — a silent
	// failure here is what made a working IGDB credential look broken.
	artError string
	// artAsked counts games actually sent to the service, so "nothing
	// was ever asked" is distinguishable from "asked, got nothing".
	artAsked int
	// hints caches the catalogue's save locations per game, misses
	// included (an empty list), so a rescan does not re-ask about games
	// the manifest has never carried.
	hints          map[string][]location
	hintsError     string
	hintsAvailable bool
	// pageSeen is when the companion page last asked for state. While
	// someone is looking, the custody poll runs at the page's pace
	// instead of the background one — a page showing minute-old state
	// looks broken to the person watching it happen.
	pageSeen time.Time
	// refreshing single-flights the status poll: the page asks on every
	// render, and a slow service must not stack requests.
	refreshing bool
}

// rescan re-runs game discovery with the configured Steam folders.
func (a *app) rescan() {
	a.mu.Lock()
	extra := append([]string(nil), a.cfg.SteamDirs...)
	a.mu.Unlock()
	found := discoverGames(extra)
	a.mu.Lock()
	a.discovered = found
	a.mu.Unlock()
}

func newApp(cfg Config, cfgPath string) *app {
	return &app{
		cfg:            cfg,
		cfgPath:        cfgPath,
		client:         &http.Client{Timeout: 15 * time.Second},
		lastCheckpoint: map[int64]time.Time{},
	}
}

// saveCfg persists the current config under the lock's protection.
func (a *app) saveCfg() error {
	a.mu.Lock()
	cfg, path := a.cfg, a.cfgPath
	a.mu.Unlock()
	return saveConfig(path, cfg)
}

// statusLine is the one-glance custody state, shown in the tray menu.
func (a *app) statusLine() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.cfg.configured() {
		return "Not connected — open the page to set up"
	}
	holds := 0
	for _, l := range a.cfg.Links {
		if l.SessionID != 0 {
			holds++
		}
	}
	switch {
	case a.worldSync.LastError != "":
		return "Sync error — open the page for details"
	case holds == 1:
		return "Holding 1 world · " + a.worldSync.LastAction
	case holds > 1:
		return fmt.Sprintf("Holding %d worlds", holds)
	case a.worldSync.LastAction != "":
		return a.worldSync.LastAction
	default:
		return "Connected — no worlds held"
	}
}

// interceptedHint names the most common shape of a wrong 200: something
// in front of the service (Cloudflare Access, a tunnel's login, a generic
// reverse proxy) answered instead of the service itself. Seen for real on
// 2026-08-19 against a console behind Access: a login page with a 200,
// which read as "unexpected answer" and gave no clue why.
func interceptedHint(body []byte) string {
	if bytes.HasPrefix(bytes.TrimSpace(body), []byte("<")) {
		return "the answer was a web page, not the sync API — an auth layer " +
			"(Cloudflare Access, a tunnel login) is intercepting the request. " +
			"Allow /api/public/* to bypass it, or use the service's direct/LAN address"
	}
	return "unexpected answer — is that URL a save-sync (reliquary) service?"
}
