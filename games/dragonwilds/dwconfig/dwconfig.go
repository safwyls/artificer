// Package dwconfig reads and edits a Dragonwilds server's DedicatedServer.ini.
//
// Unlike Palworld's single OptionSettings=(...) line, this is a conventional
// line-based UE ini: optional [Section] headers, one Key=Value per line,
// comments starting with ';' or '#'. There is no published schema, and the
// game grows keys across patches, so the file as written is the only schema:
// parsing keeps every line verbatim and edits only rewrite the value half of
// lines that changed.
//
// The write policy is palconfig's, copied deliberately (see
// docs/adding-a-game.md): never add or remove keys, validate each
// new value against the type inferred from the existing one so a bad edit
// can't brick the server's boot, keep a one-level .wildskeeper.bak, swap the file
// in atomically. Config is the one mount this package touches; saves stay on
// their own read-only mount.
package dwconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ErrNotConfigured is returned for servers with no config path set.
var ErrNotConfigured = errors.New("no config path configured for this server")

// Setting is one DedicatedServer.ini option. Keys are flat: the file's real
// key set is small and unique, and a key that appears twice (a UE array key,
// or the same name in two sections) is served read-only rather than guessed
// at — see Write.
type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// Type is one of "bool", "int", "float", "string" — inferred from how
	// the value is written in the file, and what the editor renders a
	// control for.
	Type string `json:"type"`
	// Section is the [header] the key sits under, "" before the first one.
	// Informational: display it, but address settings by Key.
	Section string `json:"section,omitempty"`
}

var (
	intRe   = regexp.MustCompile(`^-?\d+$`)
	floatRe = regexp.MustCompile(`^-?\d+\.\d+$`)
)

// settingsFile resolves configPath to the DedicatedServer.ini itself.
// configPath may point straight at the file, at the platform folder that
// holds it (LinuxServer/WindowsServer), or one level up at the Config folder.
func settingsFile(configPath string) (string, error) {
	info, err := os.Stat(configPath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return configPath, nil
	}
	for _, rel := range []string{
		"DedicatedServer.ini",
		"LinuxServer/DedicatedServer.ini",
		"WindowsServer/DedicatedServer.ini",
	} {
		p := filepath.Join(configPath, rel)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("DedicatedServer.ini not found under %s", configPath)
}

// Result is what Read returns: the settings plus where they were read from and
// whether the file is writable (a read-only mount is a common misconfiguration
// worth surfacing before the user tries to save).
type Result struct {
	Settings []Setting `json:"settings"`
	Path     string    `json:"path"`
	Writable bool      `json:"writable"`
}

// Read parses DedicatedServer.ini under configPath.
func Read(configPath string) (*Result, error) {
	if configPath == "" {
		return nil, ErrNotConfigured
	}
	file, err := settingsFile(configPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	p := parse(string(data))
	settings := make([]Setting, 0, len(p.entries))
	for _, e := range p.entries {
		if p.dup[e.key] {
			continue
		}
		typ, val := classify(e.raw)
		settings = append(settings, Setting{Key: e.key, Value: val, Type: typ, Section: e.section})
	}
	return &Result{Settings: settings, Path: file, Writable: writable(file)}, nil
}

// Write applies changes (key -> new display value) to DedicatedServer.ini.
// Unknown keys, duplicated keys, and values that don't fit the existing type
// are rejected before anything is written.
func Write(configPath string, changes map[string]string) error {
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
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	p := parse(string(data))
	index := make(map[string]int, len(p.entries))
	for i, e := range p.entries {
		index[e.key] = i
	}
	for k, v := range changes {
		i, ok := index[k]
		if !ok {
			return fmt.Errorf("unknown setting %q", k)
		}
		if p.dup[k] {
			return fmt.Errorf("setting %q appears more than once in the file; edit it by hand", k)
		}
		typ, _ := classify(p.entries[i].raw)
		raw, err := format(typ, v)
		if err != nil {
			return fmt.Errorf("setting %s: %w", k, err)
		}
		p.entries[i].raw = raw
	}
	return atomicWrite(file, []byte(p.render()))
}

// RotateAdminPassword replaces the AdminPassword value. It is its own
// operation rather than a Write special case because it is the one real
// remote-admin lever the game offers — rotating it revokes every active
// admin session — and the caller wants to generate the value, not type it.
func RotateAdminPassword(configPath, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("refusing to set an empty admin password")
	}
	return Write(configPath, map[string]string{"AdminPassword": newPassword})
}

// entry is one Key=Value line; line is the index into parsed.lines it
// rewrites on render.
type entry struct {
	key     string
	raw     string
	section string
	line    int
}

type parsed struct {
	lines   []string
	entries []entry
	// dup marks keys that appear more than once (UE array keys, or one name
	// under two sections). They are shown but not editable: rewriting one
	// occurrence of an array key changes meaning, and palconfig's "the file
	// is the schema" stance has no way to know which one was meant.
	dup map[string]bool
}

func parse(s string) *parsed {
	p := &parsed{dup: map[string]bool{}}
	seen := map[string]bool{}
	section := ""
	// Preserve the file byte-for-byte outside edited values: split keeps
	// content lines; the original newline style is restored by render.
	p.lines = strings.Split(s, "\n")
	for i, line := range p.lines {
		t := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			section = t[1 : len(t)-1]
			continue
		}
		eq := strings.IndexByte(t, '=')
		if eq <= 0 {
			continue // not a Key=Value line; left untouched
		}
		key := strings.TrimSpace(t[:eq])
		raw := strings.TrimSpace(t[eq+1:])
		if seen[key] {
			p.dup[key] = true
		}
		seen[key] = true
		p.entries = append(p.entries, entry{key: key, raw: raw, section: section, line: i})
	}
	return p
}

// render rebuilds the file, rewriting only the value half of entry lines and
// leaving every other byte as read.
func (p *parsed) render() string {
	out := make([]string, len(p.lines))
	copy(out, p.lines)
	for _, e := range p.entries {
		line := p.lines[e.line]
		cr := ""
		if strings.HasSuffix(line, "\r") {
			line, cr = strings.TrimSuffix(line, "\r"), "\r"
		}
		eq := strings.IndexByte(line, '=')
		out[e.line] = line[:eq+1] + e.raw + cr
	}
	return strings.Join(out, "\n")
}

// classify infers a value's type and decodes it for display. UE inis write
// strings bare (spaces and all), so string is the fallback, not a quoted
// form; a value the game happens to quote keeps its quotes as part of the
// string rather than this package inventing an unquoting rule for a format
// nothing verified.
func classify(raw string) (typ, value string) {
	switch strings.ToLower(raw) {
	case "true":
		return "bool", "True"
	case "false":
		return "bool", "False"
	}
	if intRe.MatchString(raw) {
		return "int", raw
	}
	if floatRe.MatchString(raw) {
		return "float", raw
	}
	return "string", raw
}

// format re-encodes a display value into its file representation, rejecting
// anything that doesn't fit the type.
func format(typ, value string) (string, error) {
	switch typ {
	case "bool":
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "1", "on":
			return "True", nil
		case "false", "0", "off":
			return "False", nil
		}
		return "", fmt.Errorf("not a boolean: %q", value)
	case "int":
		v := strings.TrimSpace(value)
		if _, err := strconv.ParseInt(v, 10, 64); err == nil {
			return v, nil
		}
		// The written form is the only schema, and an int-shaped value may
		// really be a float the game wrote without a fraction. Re-widen a
		// float-shaped edit the way palconfig does.
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return strconv.FormatFloat(f, 'f', 6, 64), nil
		}
		return "", fmt.Errorf("not a number: %q", value)
	case "float":
		f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return "", fmt.Errorf("not a number: %q", value)
		}
		return strconv.FormatFloat(f, 'f', 6, 64), nil
	case "string":
		v := strings.TrimSpace(value)
		if strings.ContainsAny(v, "\r\n") {
			return "", fmt.Errorf("invalid value: %q", value)
		}
		return v, nil
	}
	return "", fmt.Errorf("unknown type %q", typ)
}

// writable reports whether the file can be opened for writing, so the UI can
// warn about a read-only mount before an edit fails.
func writable(file string) bool {
	f, err := os.OpenFile(file, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// atomicWrite replaces file's contents in one rename, after stashing the
// previous contents in a sibling .wildskeeper.bak for one-level undo.
func atomicWrite(file string, data []byte) error {
	dir := filepath.Dir(file)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(file); err == nil {
		mode = info.Mode().Perm()
		if orig, err := os.ReadFile(file); err == nil {
			_ = os.WriteFile(file+".wildskeeper.bak", orig, mode)
		}
	}

	tmp, err := os.CreateTemp(dir, ".dwconf-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, file)
}
