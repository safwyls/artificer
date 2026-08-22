package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Config is what survives restarts: where the save-sync service
// (reliquary) is, the player's personal token, and the worlds this
// machine syncs. The token is a credential, so the file is written 0600.
type Config struct {
	// ServerURL is the save-sync service's base URL, e.g.
	// "https://vault.example.com". Empty = nothing configured, nothing
	// leaves the machine.
	ServerURL string `json:"serverUrl"`
	// Token is the player's personal sync token from the service's page.
	Token string `json:"token"`
	// Links are the worlds this machine syncs — one per linked game save
	// folder.
	Links []WorldLink `json:"links,omitempty"`
	// SteamDirs are extra places discovery looks for Steam libraries —
	// each entry may be a Steam root, a steamapps folder, or
	// steamapps/common, whatever spelling the player pasted
	// (discover.go normalizes). For the machines the registry and the
	// default locations miss.
	SteamDirs []string `json:"steamDirs,omitempty"`
	// Hidden are shelf entries the player has put away, by the same key
	// the artwork map uses ("app:<id>" or "name:<lowercased>"). A Steam
	// library is full of things that are not games — redistributables,
	// runtimes, controller configs — and a shelf that shows them all is
	// a shelf nobody scans. Hiding is reversible and local: it never
	// touches a link or the service.
	Hidden []string `json:"hidden,omitempty"`
	// LaunchOnCheckout starts the game once a checkout has put the save
	// in place — the second half of what "check this world out" means.
	// A pointer because absent must mean on: this is the behaviour
	// people asked for, and a config written before the setting existed
	// should get it. An explicit false is the player saying they want
	// the save without the game.
	LaunchOnCheckout *bool `json:"launchOnCheckout,omitempty"`
}

// launchOnCheckout is the stored setting with its default applied.
func (c Config) launchOnCheckout() bool {
	return c.LaunchOnCheckout == nil || *c.LaunchOnCheckout
}

// WorldLink ties one world on the service to one save folder here, and
// carries the one piece of custody state that must survive a restart:
// which hold is ours.
type WorldLink struct {
	WorldID   int64  `json:"worldId"`
	GameTitle string `json:"gameTitle,omitempty"`
	Dir       string `json:"dir"`
	// AppID is the Steam app id this link came from, when it came from a
	// discovered game. It is what lets the worlds list show the same
	// cover the shelf does; without it a linked world can still be
	// matched by title, which is why it stays optional.
	AppID string `json:"appId,omitempty"`
	// LaunchTarget overrides what starts this game when the world is
	// checked out (launch.go). Empty means Steam's own run URI, built
	// from AppID; a world with neither is one the companion will not
	// pretend it can start. A path or a URI, never a command line — the
	// OS's opener takes it, so an .exe, a .lnk and another launcher's
	// URI scheme all work without quoting rules.
	LaunchTarget string `json:"launchTarget,omitempty"`
	// SessionID is the hold this machine has on the world (0 = none);
	// BaseVersion the version it delivered.
	SessionID   int64 `json:"sessionId,omitempty"`
	BaseVersion int64 `json:"baseVersion,omitempty"`
}

func (c Config) configured() bool { return c.ServerURL != "" && c.Token != "" }

func (c *Config) link(worldID int64) *WorldLink {
	for i := range c.Links {
		if c.Links[i].WorldID == worldID {
			return &c.Links[i]
		}
	}
	return nil
}

func configPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "artificer-companion", "config.json"), nil
}

// legacyConfigPath is where wkcompanion kept its config. Read as a
// fallback so an upgraded machine keeps what it can; the next save
// writes the new location and the old file stays for old binaries.
func legacyConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "wkcompanion", "config.json"), nil
}

// legacyConfig is both earlier shapes at once: the wkcompanion character
// relay's fields, and the first Artificer Companion cut's nested sync
// block. The character relay is retired, so only the sync side maps
// forward.
type legacyConfig struct {
	ConsoleURL string `json:"consoleUrl"`
	Sync       struct {
		Token       string `json:"token"`
		WorldID     int64  `json:"worldId"`
		WorldDir    string `json:"worldDir"`
		SessionID   int64  `json:"sessionId"`
		BaseVersion int64  `json:"baseVersion"`
	} `json:"sync"`
}

func parseConfig(data []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.ServerURL != "" || len(cfg.Links) > 0 {
		return cfg, nil
	}
	// Nothing in the current shape: try the legacy fields in the same
	// bytes. A wkcompanion-era config with no sync block maps to an
	// empty config — its relay credential has nothing to authenticate
	// any more.
	var old legacyConfig
	if json.Unmarshal(data, &old) != nil || old.Sync.Token == "" {
		return cfg, nil
	}
	cfg.ServerURL = old.ConsoleURL
	cfg.Token = old.Sync.Token
	if old.Sync.WorldID != 0 || old.Sync.WorldDir != "" {
		cfg.Links = []WorldLink{{
			WorldID:     old.Sync.WorldID,
			Dir:         old.Sync.WorldDir,
			SessionID:   old.Sync.SessionID,
			BaseVersion: old.Sync.BaseVersion,
		}}
	}
	return cfg, nil
}

func loadConfig() (Config, string, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if legacy, lerr := legacyConfigPath(); lerr == nil {
			if ldata, lerr := os.ReadFile(legacy); lerr == nil {
				if cfg, perr := parseConfig(ldata); perr == nil {
					return cfg, path, nil
				}
			}
		}
		return Config{}, path, nil
	}
	if err != nil {
		return Config{}, path, err
	}
	cfg, err := parseConfig(data)
	return cfg, path, err
}

func saveConfig(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// normalizeServerURL trims the shapes people paste: trailing slashes and
// an accidental /api suffix.
func normalizeServerURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimRight(u, "/")
	u = strings.TrimSuffix(u, "/api")
	return u
}
