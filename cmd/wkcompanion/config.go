package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Config is what survives restarts: where the console is, the companion
// token its admin handed out, and an optional save-directory override.
// The token is a credential, so the file is written 0600.
type Config struct {
	// ConsoleURL is the wildskeeper console's base URL, e.g.
	// "https://wilds.example.com". Empty = local-only mode.
	ConsoleURL string `json:"consoleUrl"`
	// Token is the server's companion token, minted by a console admin on
	// the Adventurers page.
	Token string `json:"token"`
	// SaveDir overrides the auto-detected SaveCharacters directory.
	SaveDir string `json:"saveDir,omitempty"`
}

func configPath() (string, error) {
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
