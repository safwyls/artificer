package main

// Turning the manifest's path templates into folders on this machine.
//
// The Ludusavi manifest states save locations in placeholders —
// <winLocalAppData>/Pal/Saved/SaveGames/<storeUserId> — because the same
// game's saves sit somewhere different on every machine and every store.
// The service that holds the catalogue cannot resolve those; this can,
// and only this can, which is why expansion lives on the player's side
// and the service stays OS-blind.
//
// Expansion is deliberately conservative: a template becomes a glob, the
// glob is matched against the real filesystem, and only folders that
// actually exist are offered. A placeholder this build does not know
// makes the whole template unusable rather than half-resolved — half a
// path is a wrong path, and a wrong path is worse than one fewer
// suggestion. The player still confirms whatever comes back.

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// location mirrors core/savedb.Location on the wire.
type location struct {
	Template string `json:"template"`
	OS       string `json:"os,omitempty"`
	Store    string `json:"store,omitempty"`
}

// hostOS names this machine in the manifest's vocabulary.
func hostOS() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "mac"
	default:
		return "linux"
	}
}

// appliesHere filters out entries for another operating system or
// another store. A blank constraint means "any", which is the manifest's
// convention and the common case.
//
// Store is judged loosely: the games here came from a Steam library, so
// a steam-specific entry applies and a microsoft/epic/gog-specific one
// does not — that is exactly the Palworld trap, where the Microsoft
// Store path exists in the manifest and would send a Steam player's save
// sync at an empty folder.
func (l location) appliesHere() bool {
	if l.OS != "" && l.OS != hostOS() {
		return false
	}
	return l.Store == "" || l.Store == "steam"
}

// expandRoots are the placeholders whose value is a folder on this
// machine. Built per call because the environment can change under a
// long-running app (a mounted drive, a redirected Documents).
func expandRoots(installDir string, libraries []string) map[string]string {
	roots := map[string]string{}
	home, _ := os.UserHomeDir()
	if home != "" {
		roots["<home>"] = home
		roots["<xdgData>"] = filepath.Join(home, ".local", "share")
		roots["<xdgConfig>"] = filepath.Join(home, ".config")
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		roots["<xdgData>"] = v
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		roots["<xdgConfig>"] = v
	}
	if v := os.Getenv("APPDATA"); v != "" {
		roots["<winAppData>"] = v
	} else if home != "" {
		roots["<winAppData>"] = filepath.Join(home, "AppData", "Roaming")
	}
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		roots["<winLocalAppData>"] = v
	} else if home != "" {
		roots["<winLocalAppData>"] = filepath.Join(home, "AppData", "Local")
	}
	if home != "" {
		roots["<winDocuments>"] = filepath.Join(home, "Documents")
	}
	if v := os.Getenv("PUBLIC"); v != "" {
		roots["<winPublic>"] = v
	}
	if v := os.Getenv("ProgramData"); v != "" {
		roots["<winProgramData>"] = v
	}
	if v := os.Getenv("SystemRoot"); v != "" {
		roots["<winDir>"] = v
	}
	if v := os.Getenv("USERNAME"); v != "" {
		roots["<osUserName>"] = v
	} else if v := os.Getenv("USER"); v != "" {
		roots["<osUserName>"] = v
	}
	// <base> is the game's own install folder, <root> the Steam library
	// holding it. Both are per-library, so they expand to a wildcard
	// across every library rather than picking one.
	if installDir != "" && len(libraries) > 0 {
		roots["<base>"] = filepath.Join(libraries[0], "common", installDir)
	}
	if len(libraries) > 0 {
		// <root> is the store root — the folder above steamapps.
		roots["<root>"] = filepath.Dir(libraries[0])
	}
	return roots
}

// unknownPlaceholder finds a <...> this build cannot resolve.
func unknownPlaceholder(s string) string {
	for {
		open := strings.Index(s, "<")
		if open < 0 {
			return ""
		}
		close := strings.Index(s[open:], ">")
		if close < 0 {
			return ""
		}
		return s[open : open+close+1]
	}
}

// expandTemplate turns one template into concrete folders that exist.
//
// <storeUserId> becomes a wildcard rather than a lookup: a machine can
// hold several Steam accounts, the manifest does not say which, and the
// player is the one who knows. Every match is offered.
func expandTemplate(tmpl, installDir string, libraries []string) []string {
	roots := expandRoots(installDir, libraries)
	path := strings.ReplaceAll(tmpl, "\\", "/")
	for placeholder, value := range roots {
		path = strings.ReplaceAll(path, placeholder, filepath.ToSlash(value))
	}
	// Account and profile ids are wildcards: any of them may be the
	// player's, and the folder listing settles it.
	for _, wild := range []string{"<storeUserId>", "<winProfile>"} {
		path = strings.ReplaceAll(path, wild, "*")
	}
	if left := unknownPlaceholder(path); left != "" {
		// Refuse rather than guess: a template we only half understand
		// resolves to a folder that is not the save folder.
		return nil
	}
	pattern := filepath.FromSlash(path)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && info.IsDir() {
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// manifestCandidates expands every location the catalogue offered for one
// game, strongest first. Duplicates of what discovery already found are
// the caller's problem to merge.
func manifestCandidates(g discoveredGame, locs []location, libraries []string) []saveCandidate {
	var out []saveCandidate
	seen := map[string]bool{}
	for _, l := range locs {
		if !l.appliesHere() {
			continue
		}
		for _, dir := range expandTemplate(l.Template, g.InstallDir, libraries) {
			if seen[dir] {
				continue
			}
			seen[dir] = true
			out = append(out, saveCandidate{Path: dir, Why: "known save location for this game (Ludusavi manifest)"})
		}
	}
	return out
}
