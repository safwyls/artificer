package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Config is what survives restarts: where the console is, the companion
// token its admin handed out, an optional save-directory override, and
// the save-sync side (the personal token and the hold this machine has).
// The tokens are credentials, so the file is written 0600.
type Config struct {
	// ConsoleURL is the wildskeeper console's base URL, e.g.
	// "https://wilds.example.com". Empty = local-only mode.
	ConsoleURL string `json:"consoleUrl"`
	// Token is the server's companion token, minted by a console admin on
	// the Adventurers page.
	Token string `json:"token"`
	// SaveDir overrides the auto-detected SaveCharacters directory.
	SaveDir string `json:"saveDir,omitempty"`
	// Sync is the save-sync custody side (sync.go); zero value = off.
	Sync SyncConfig `json:"sync,omitempty"`
}

// SyncConfig is the world-custody configuration and the one piece of
// custody state that must survive a restart: which hold is ours.
type SyncConfig struct {
	// Token is the player's personal sync token from the console's
	// Worlds page — per person, unlike the shared companion token.
	Token string `json:"token,omitempty"`
	// WorldID is the shared world this machine hosts.
	WorldID int64 `json:"worldId,omitempty"`
	// WorldDir is where the hosted world's save lives on this machine.
	// Deliberately not auto-detected: the player-hosted save location is
	// unverified recon (docs/save-sync-architecture.md, phase 0), and a
	// wrong guess here would sync the wrong directory.
	WorldDir string `json:"worldDir,omitempty"`
	// SessionID is the hold this machine currently has (0 = none) and
	// BaseVersion the version it delivered — what a check-in reports
	// against.
	SessionID   int64 `json:"sessionId,omitempty"`
	BaseVersion int64 `json:"baseVersion,omitempty"`
}

func (sc SyncConfig) configured() bool { return sc.Token != "" && sc.WorldDir != "" }

func configPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "artificer-companion", "config.json"), nil
}

// legacyConfigPath is where wkcompanion kept its config before the app
// became the Artificer Companion. Read as a fallback so players' pasted
// tokens survive the upgrade; the next save writes the new location.
func legacyConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "wkcompanion", "config.json"), nil
}

func loadConfig() (Config, string, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// The wkcompanion-era config, if one exists. The old file is left
		// in place: an old binary still runs, and a stale config is
		// cheaper than a migration that deletes someone's credential.
		if legacy, lerr := legacyConfigPath(); lerr == nil {
			if ldata, lerr := os.ReadFile(legacy); lerr == nil {
				var cfg Config
				if json.Unmarshal(ldata, &cfg) == nil {
					return cfg, path, nil
				}
			}
		}
		return Config{}, path, nil
	}
	if err != nil {
		return Config{}, path, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, path, err
	}
	return cfg, path, nil
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

// resolveSaveDir picks the directory to watch: the explicit override, or
// the game's own location. The game stores characters per player under
// LocalAppData on Windows; other platforms have no game client, so the
// override is the only path there.
func (c Config) resolveSaveDir() string {
	if c.SaveDir != "" {
		return c.SaveDir
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "RSDragonwilds", "Saved", "SaveCharacters")
	}
	return ""
}

// normalizeConsoleURL trims the shapes people paste: trailing slashes and
// an accidental /api suffix.
func normalizeConsoleURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimRight(u, "/")
	u = strings.TrimSuffix(u, "/api")
	return u
}
