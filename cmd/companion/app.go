package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/safwyls/artificer/games/dragonwilds/dwsave"
)

// character is one parsed character save file on this machine.
type character struct {
	File     string                  `json:"file"`
	ModTime  time.Time               `json:"modTime"`
	Player   *dwsave.PlayerCharacter `json:"player"`
	PushedAt *time.Time              `json:"pushedAt,omitempty"`
	raw      []byte
}

// relayStatus is what the page shows about the console connection.
type relayStatus struct {
	Configured bool       `json:"configured"`
	Server     string     `json:"server,omitempty"`
	LastPushAt *time.Time `json:"lastPushAt,omitempty"`
	LastError  string     `json:"lastError,omitempty"`
}

type app struct {
	cfgPath string
	client  *http.Client

	mu         sync.Mutex
	cfg        Config
	characters map[string]*character // keyed by file basename
	relay      relayStatus
	scanErr    string
	// Save-sync custody state (sync.go): the console's world roster and
	// this machine's part in it.
	worldSync      syncState
	lastCheckpoint time.Time
}

func newApp(cfg Config, cfgPath string) *app {
	return &app{
		cfg:        cfg,
		cfgPath:    cfgPath,
		client:     &http.Client{Timeout: 15 * time.Second},
		characters: map[string]*character{},
	}
}

// scan reads the save directory fresh. A file that stops parsing (the
// game writing it mid-scan) keeps its previous good state until the next
// pass; a file that disappears is dropped.
func (a *app) scan() {
	a.mu.Lock()
	dir := a.cfg.resolveSaveDir()
	a.mu.Unlock()

	if dir == "" {
		a.setScanErr("no SaveCharacters directory found — set one in the settings below")
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		a.setScanErr(fmt.Sprintf("cannot read %s: %v", dir, err))
		return
	}

	seen := map[string]bool{}
	for _, e := range entries {
		// Current game builds write character records as <name>.json
		// (verified against a real install, 2026-08-19); .sav is the
		// older spelling community tooling knew. Accept both.
		ext := filepath.Ext(e.Name())
		if e.IsDir() || (!strings.EqualFold(ext, ".sav") && !strings.EqualFold(ext, ".json")) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		seen[e.Name()] = true

		a.mu.Lock()
		prev := a.characters[e.Name()]
		a.mu.Unlock()
		if prev != nil && prev.ModTime.Equal(info.ModTime()) {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		p, err := dwsave.ParseCharacterRecord(raw, "")
		if err != nil {
			continue // not a character record; SaveCharacters can hold other files
		}
		a.mu.Lock()
		a.characters[e.Name()] = &character{File: e.Name(), ModTime: info.ModTime(), Player: p, raw: raw}
		a.mu.Unlock()
	}

	a.mu.Lock()
	for name := range a.characters {
		if !seen[name] {
			delete(a.characters, name)
		}
	}
	a.scanErr = ""
	a.mu.Unlock()
}

func (a *app) setScanErr(msg string) {
	a.mu.Lock()
	a.scanErr = msg
	a.mu.Unlock()
}

func (a *app) relayConfigured() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.ConsoleURL != "" && a.cfg.Token != ""
}

// statusLine is the one-glance sharing state, shown in the tray menu.
func (a *app) statusLine() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.ConsoleURL == "" || a.cfg.Token == "" {
		return "Local-only — nothing is shared"
	}
	name := a.relay.Server
	if name == "" {
		name = "console"
	}
	switch {
	case a.relay.LastError != "":
		return "Sharing error — open the sheet for details"
	case a.relay.LastPushAt != nil:
		return fmt.Sprintf("Sharing with %s · pushed %s", name, a.relay.LastPushAt.Format("15:04"))
	default:
		return fmt.Sprintf("Sharing with %s · waiting for first push", name)
	}
}

// pushChanged relays characters that changed since their last push; force
// re-pushes everything (the heartbeat that heals a restarted console).
// Reports whether every due push succeeded.
func (a *app) pushChanged(force bool) bool {
	a.mu.Lock()
	var due []*character
	for _, c := range a.characters {
		if force || c.PushedAt == nil || c.PushedAt.Before(c.ModTime) {
			due = append(due, c)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].File < due[j].File })
	a.mu.Unlock()

	allOK := true
	for _, c := range due {
		if err := a.pushOne(c); err != nil {
			allOK = false
			a.mu.Lock()
			a.relay.LastError = err.Error()
			a.mu.Unlock()
			continue
		}
	}
	return allOK
}

func (a *app) pushOne(c *character) error {
	a.mu.Lock()
	url := normalizeConsoleURL(a.cfg.ConsoleURL) + "/api/public/companion/" + a.cfg.Token + "/character"
	raw := c.raw
	a.mu.Unlock()

	resp, err := a.client.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("console answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	// A 200 is only a success if it is the console's acknowledgment. An
	// auth layer in front of the console (a Cloudflare Access login page,
	// say) answers 200 with HTML — that must not count as delivered.
	var ack struct {
		Accepted bool   `json:"accepted"`
		Server   string `json:"server"`
	}
	if err := json.Unmarshal(body, &ack); err != nil || !ack.Accepted {
		return fmt.Errorf("%s", interceptedHint(body))
	}

	now := time.Now()
	a.mu.Lock()
	c.PushedAt = &now
	a.relay.LastPushAt = &now
	a.relay.LastError = ""
	if ack.Server != "" {
		a.relay.Server = ack.Server
	}
	a.mu.Unlock()
	return nil
}

// ping verifies a console URL + token pair, returning the server's name.
func (a *app) ping(consoleURL, token string) (string, error) {
	url := normalizeConsoleURL(consoleURL) + "/api/public/companion/" + token
	resp, err := a.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("console answered %d — check the URL and token", resp.StatusCode)
	}
	var out struct {
		Server string `json:"server"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Server == "" {
		return "", fmt.Errorf("%s", interceptedHint(body))
	}
	return out.Server, nil
}

// interceptedHint names the most common shape of a wrong 200: something
// in front of the console (Cloudflare Access, a tunnel's login, a generic
// reverse proxy) answered instead of the console itself. Seen for real on
// 2026-08-19: a console behind Cloudflare Access returned its login page
// with a 200, which read as "unexpected answer" and gave no clue why.
func interceptedHint(body []byte) string {
	if bytes.HasPrefix(bytes.TrimSpace(body), []byte("<")) {
		return "the answer was a web page, not the console API — an auth layer " +
			"(Cloudflare Access, a tunnel login) is intercepting the request. " +
			"Allow /api/public/* to bypass it, or use the console's direct/LAN address"
	}
	return "unexpected answer — is that URL a wildskeeper console with companion sharing enabled?"
}
