package esconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The editor layer: enshrouded_server.json rendered as the same flat
// key/value settings the sibling consoles' ini editors serve, so the
// console's settings page stays game-blind.
//
// Scalars only. Top-level scalar keys appear under section "server";
// gameSettings scalars appear as "gameSettings.<key>" under section
// "gameSettings". userGroups (role passwords and permissions) and the
// other array-valued keys are deliberately absent from the flat view —
// passwords already have the rotate flow and the wizard, and a proper
// role editor is roadmap Phase 2. The write policy is inherited: never
// add or remove keys, validate each value against the existing one's
// type, one .bak, atomic swap.

// ErrNotConfigured is returned for servers with no config path set.
var ErrNotConfigured = errors.New("no config path configured for this server")

// Setting is one editable option, in the sibling consoles' wire shape.
type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// Type is one of "bool", "int", "float", "string" — inferred from the
	// JSON value, and what the editor renders a control for.
	Type string `json:"type"`
	// Section groups the flat list: "server" or "gameSettings".
	Section string `json:"section,omitempty"`
}

// Result is what Read returns: the settings plus where they were read
// from and whether the file is writable (a read-only mount is a common
// misconfiguration worth surfacing before the user tries to save).
type Result struct {
	Settings []Setting `json:"settings"`
	Path     string    `json:"path"`
	Writable bool      `json:"writable"`
}

// settingsFile resolves configPath to enshrouded_server.json itself:
// straight at the file, or at the directory holding it.
func settingsFile(configPath string) (string, error) {
	info, err := os.Stat(configPath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return configPath, nil
	}
	p := filepath.Join(configPath, "enshrouded_server.json")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("enshrouded_server.json not found under %s", configPath)
}

// scalar renders a JSON value as (type, display); ok=false for arrays,
// objects and nulls, which the flat editor does not carry.
func scalar(v any) (typ, val string, ok bool) {
	switch x := v.(type) {
	case bool:
		return "bool", strconv.FormatBool(x), true
	case string:
		return "string", x, true
	case json.Number:
		if !strings.ContainsAny(x.String(), ".eE") {
			return "int", x.String(), true
		}
		return "float", x.String(), true
	default:
		return "", "", false
	}
}

// Read parses enshrouded_server.json under configPath into the flat view.
func Read(configPath string) (*Result, error) {
	if configPath == "" {
		return nil, ErrNotConfigured
	}
	file, err := settingsFile(configPath)
	if err != nil {
		return nil, err
	}
	doc, err := Load(file)
	if err != nil {
		return nil, err
	}

	var settings []Setting
	topKeys := make([]string, 0, len(doc))
	for k := range doc {
		topKeys = append(topKeys, k)
	}
	sort.Strings(topKeys)
	for _, k := range topKeys {
		if k == "gameSettings" {
			continue
		}
		if typ, val, ok := scalar(doc[k]); ok {
			settings = append(settings, Setting{Key: k, Value: val, Type: typ, Section: "server"})
		}
	}
	if gs, ok := doc["gameSettings"].(map[string]any); ok {
		gsKeys := make([]string, 0, len(gs))
		for k := range gs {
			gsKeys = append(gsKeys, k)
		}
		sort.Strings(gsKeys)
		for _, k := range gsKeys {
			if typ, val, ok := scalar(gs[k]); ok {
				settings = append(settings, Setting{Key: "gameSettings." + k, Value: val, Type: typ, Section: "gameSettings"})
			}
		}
	}
	return &Result{Settings: settings, Path: file, Writable: writable(file)}, nil
}

func writable(file string) bool {
	f, err := os.OpenFile(file, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// parseAs validates newVal against the existing JSON value's type and
// returns the typed replacement.
func parseAs(existing any, newVal string) (any, error) {
	switch existing.(type) {
	case bool:
		b, err := strconv.ParseBool(newVal)
		if err != nil {
			return nil, errors.New("must be true or false")
		}
		return b, nil
	case json.Number:
		if _, err := strconv.ParseFloat(newVal, 64); err != nil {
			return nil, errors.New("must be a number")
		}
		return json.Number(newVal), nil
	case string:
		return newVal, nil
	default:
		return nil, errors.New("this setting can't be edited here")
	}
}

// WriteChanges applies changes (flat key -> new display value). Unknown
// keys and values that don't fit the existing type are rejected before
// anything is written.
func WriteChanges(configPath string, changes map[string]string) error {
	if configPath == "" {
		return ErrNotConfigured
	}
	if len(changes) == 0 {
		return nil
	}
	file, err := settingsFile(configPath)
	if err != nil {
		return err
	}
	doc, err := Load(file)
	if err != nil {
		return err
	}
	for key, newVal := range changes {
		target := map[string]any(doc)
		k := key
		if rest, ok := strings.CutPrefix(key, "gameSettings."); ok {
			gs, ok := doc["gameSettings"].(map[string]any)
			if !ok {
				return fmt.Errorf("unknown setting %q", key)
			}
			target, k = gs, rest
		}
		existing, ok := target[k]
		if !ok {
			// Never-add policy: a key the file doesn't hold is a typo or a
			// schema from another game version — refuse rather than risk a
			// config the server rejects (which it "fixes" by regenerating
			// an open default).
			return fmt.Errorf("unknown setting %q", key)
		}
		v, err := parseAs(existing, newVal)
		if err != nil {
			return fmt.Errorf("%s: %s", key, err)
		}
		target[k] = v
	}
	if err := Write(file, doc); err != nil {
		return err
	}
	return nil
}

// RotateAdminPassword writes a fresh password onto the first
// kick/ban-capable role group — the credential that grants admin at the
// join screen. Players already in an admin session keep it until they
// leave; the new password gates the next join (recon doc: roles are
// checked at join time).
func RotateAdminPassword(configPath, newPassword string) error {
	if configPath == "" {
		return ErrNotConfigured
	}
	file, err := settingsFile(configPath)
	if err != nil {
		return err
	}
	doc, err := Load(file)
	if err != nil {
		return err
	}
	Enforce(doc, Enforcement{AdminPassword: &newPassword})
	return Write(file, doc)
}
