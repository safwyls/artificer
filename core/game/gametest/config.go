package gametest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/safwyls/sampo/core/game"
)

// ConfigFilename is the fake game's settings file, for tests that build
// fixtures on disk.
const ConfigFilename = "gametest.json"

// ErrNotConfigured mirrors the real codecs' sentinel.
var ErrNotConfigured = errors.New("gametest: no config path configured")

// configCodec is a real (if tiny) codec over a flat JSON object, so the
// shared config handlers are exercised against the same policy the game
// codecs share: read returns key/value rows, writes may only change
// existing keys, and a rotate is just a write to a well-known key.
var configCodec = &game.ConfigCodec{
	Filename:      ConfigFilename,
	NotConfigured: ErrNotConfigured,
	Read:          readConfig,
	Write:         writeChanges,
	RotateAdminPassword: func(path, newPassword string) error {
		return writeChanges(path, map[string]string{"adminPassword": newPassword})
	},
}

func configFile(path string) string { return filepath.Join(path, ConfigFilename) }

func load(path string) (map[string]string, error) {
	if path == "" {
		return nil, ErrNotConfigured
	}
	data, err := os.ReadFile(configFile(path))
	if err != nil {
		return nil, err
	}
	var settings map[string]string
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("gametest config: %w", err)
	}
	return settings, nil
}

func readConfig(path string) (*game.ConfigPayload, error) {
	settings, err := load(path)
	if err != nil {
		return nil, err
	}
	type row struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	rows := make([]row, 0, len(settings))
	for k, v := range settings {
		rows = append(rows, row{k, v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
	writable := true
	if f, err := os.Stat(configFile(path)); err != nil || f.Mode().Perm()&0o200 == 0 {
		writable = false
	}
	return &game.ConfigPayload{Settings: rows, Path: configFile(path), Writable: writable}, nil
}

func writeChanges(path string, changes map[string]string) error {
	settings, err := load(path)
	if err != nil {
		return err
	}
	for k, v := range changes {
		if _, ok := settings[k]; !ok {
			return fmt.Errorf("gametest config: unknown key %q", k)
		}
		settings[k] = v
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFile(path), data, 0o644)
}
