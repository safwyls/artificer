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
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".sav") {
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
	var ack struct {
		Server string `json:"server"`
	}
	_ = json.Unmarshal(body, &ack)

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
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("unexpected answer — is that URL a wildskeeper console?")
	}
	return out.Server, nil
}
