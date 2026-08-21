package main

// Installed-game discovery: find what this machine plays and where those
// games keep their saves, so linking a save folder to a world is a pick,
// not a paste.
//
// Two sources, in order of authority. Steam's own metadata is the ground
// truth for what is installed (libraryfolders.vdf → appmanifest_*.acf);
// where that is missing or unreadable, the folder names under
// steamapps/common are still real installed games, so they are listed
// too rather than showing the player an empty panel next to a library
// they can see in Explorer. Save locations are always heuristics — a
// small catalog plus Unreal's Saved/SaveGames convention — and every
// candidate is marked as the guess it is. The player confirms the
// folder; nothing syncs a guessed path unseen.
//
// Every step records what it tried and what came of it (probe). "No
// games found" is the failure that makes this panel look broken, and it
// is nearly always a path question — so the scan answers that question
// on the page instead of shrugging.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// discoveredGame is one installed game and where its saves might be.
type discoveredGame struct {
	Name       string `json:"name"`
	AppID      string `json:"appId,omitempty"`
	InstallDir string `json:"installDir,omitempty"`
	// SaveDirs are candidate save folders that actually exist on this
	// machine, strongest first, each carrying why it was offered. A
	// Steam Cloud hit is exact; the rest are guesses the player confirms.
	SaveDirs []saveCandidate `json:"saveDirs,omitempty"`
	// Key and Hidden are filled in when the state is served, not by the
	// scan: they are the page's view of this game, not a property of the
	// install (hidden.go).
	Key    string `json:"key,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

// probe is one place the scan looked and what it found there, so an
// empty result can explain itself.
type probe struct {
	// Source names who suggested this path: "configured", "registry",
	// "STEAM_ROOT", "default", "libraryfolders.vdf".
	Source string `json:"source"`
	Path   string `json:"path"`
	// Resolved is the steamapps folder this path led to, empty when it
	// led nowhere.
	Resolved string `json:"resolved,omitempty"`
	// Note explains a miss, or reports the haul on a hit.
	Note string `json:"note"`
}

// discovery is one scan's result: the games, and the trail of where the
// scan looked.
type discovery struct {
	Games  []discoveredGame `json:"games"`
	Probes []probe          `json:"probes"`
}

var (
	vdfPath      = regexp.MustCompile(`"path"\s+"((?:[^"\\]|\\.)+)"`)
	acfName      = regexp.MustCompile(`"name"\s+"((?:[^"\\]|\\.)+)"`)
	acfAppID     = regexp.MustCompile(`"appid"\s+"(\d+)"`)
	acfInstall   = regexp.MustCompile(`"installdir"\s+"((?:[^"\\]|\\.)+)"`)
	vdfUnescaper = regexp.MustCompile(`\\(.)`)
)

func unescapeVDF(s string) string { return vdfUnescaper.ReplaceAllString(s, "$1") }

// cleanPastedPath undoes what pasting does to a path. Windows
// Explorer's "Copy as path" wraps it in double quotes, and a rejected
// path that was only ever quoted is the most frustrating possible miss:
// it looks right on screen.
func cleanPastedPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"'`)
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}

// isLibrary reports whether a directory is a steamapps folder in
// substance: it holds app manifests, or the common/ folder the games
// themselves live in. Substance rather than name, because a library
// with its manifests cleared still has the games.
func isLibrary(dir string) bool {
	if m, _ := filepath.Glob(filepath.Join(dir, "appmanifest_*.acf")); len(m) > 0 {
		return true
	}
	info, err := os.Stat(filepath.Join(dir, "common"))
	return err == nil && info.IsDir()
}

// resolveSteamDir maps whatever the player pasted onto the steamapps
// folder it implies, and says why when it can't. Accepted: a Steam
// root, a steamapps folder, steamapps/common, a game folder inside
// common, or any library-shaped directory — walking up from deep paths
// and down from shallow ones.
func resolveSteamDir(p string) (string, string) {
	p = cleanPastedPath(p)
	if p == "" {
		return "", "empty path"
	}
	if _, err := os.Stat(p); err != nil {
		return "", "no such folder on this machine"
	}
	// Walk up: the path itself, or any ancestor, may be the library —
	// this catches steamapps/common, a game folder inside common, and
	// anything else pasted from deeper in the tree.
	for dir := p; ; {
		if strings.EqualFold(filepath.Base(dir), "steamapps") || isLibrary(dir) {
			if isLibrary(dir) {
				return dir, ""
			}
			return "", "found " + dir + ", but it holds no app manifests or common/ folder"
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Walk down: a Steam root holds steamapps.
	if sub := filepath.Join(p, "steamapps"); isLibrary(sub) {
		return sub, ""
	}
	return "", "no steamapps folder here (nor is this one) — point at your Steam folder, its steamapps folder, or steamapps\\common"
}

// steamCandidates lists the paths worth probing with who suggested each,
// most authoritative first: the player's own setting, the registry (the
// canonical answer on Windows for a custom install drive), STEAM_ROOT,
// then the default install locations.
func steamCandidates(extraDirs []string) []probe {
	var out []probe
	for _, dir := range extraDirs {
		out = append(out, probe{Source: "configured", Path: dir})
	}
	if reg := steamRootFromRegistry(); reg != "" {
		out = append(out, probe{Source: "registry", Path: reg})
	}
	if root := os.Getenv("STEAM_ROOT"); root != "" {
		out = append(out, probe{Source: "STEAM_ROOT", Path: root})
	}
	if runtime.GOOS == "windows" {
		// Steam installs to Program Files (x86); the 64-bit path is a
		// long shot kept only as a fallback.
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
			if base := os.Getenv(env); base != "" {
				out = append(out, probe{Source: "default", Path: filepath.Join(base, "Steam")})
			}
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		// Developer platforms, not player machines — but a dev testing
		// discovery deserves real answers.
		for _, p := range []string{
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".local", "share", "Steam"),
		} {
			out = append(out, probe{Source: "default", Path: p})
		}
	}
	return out
}

// discoverGames scans for installed games, recording the trail.
// extraDirs are the player's configured Steam folders, tried first.
func discoverGames(extraDirs []string) discovery {
	var out discovery
	seen := map[string]bool{}
	var libs []string

	consider := func(p probe) {
		resolved, why := resolveSteamDir(p.Path)
		if resolved == "" {
			p.Note = why
			out.Probes = append(out.Probes, p)
			return
		}
		if seen[strings.ToLower(resolved)] {
			p.Resolved, p.Note = resolved, "already scanned"
			out.Probes = append(out.Probes, p)
			return
		}
		seen[strings.ToLower(resolved)] = true
		libs = append(libs, resolved)
		p.Resolved = resolved
		out.Probes = append(out.Probes, p)
	}

	for _, p := range steamCandidates(extraDirs) {
		consider(p)
	}
	// Expand each library's libraryfolders.vdf: a second drive's library
	// arrives here automatically once any one root is found.
	for i := 0; i < len(libs); i++ {
		data, err := os.ReadFile(filepath.Join(libs[i], "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		for _, m := range vdfPath.FindAllStringSubmatch(string(data), -1) {
			consider(probe{Source: "libraryfolders.vdf", Path: filepath.FromSlash(unescapeVDF(m[1]))})
		}
	}

	byInstallDir := map[string]bool{}
	for _, lib := range libs {
		found := 0
		manifests, _ := filepath.Glob(filepath.Join(lib, "appmanifest_*.acf"))
		for _, manifest := range manifests {
			data, err := os.ReadFile(manifest)
			if err != nil {
				continue
			}
			g := discoveredGame{}
			if m := acfName.FindSubmatch(data); m != nil {
				g.Name = unescapeVDF(string(m[1]))
			}
			if m := acfAppID.FindSubmatch(data); m != nil {
				g.AppID = string(m[1])
			}
			if m := acfInstall.FindSubmatch(data); m != nil {
				g.InstallDir = unescapeVDF(string(m[1]))
			}
			if g.Name == "" {
				g.Name = g.InstallDir // a manifest without a name still names a game
			}
			if g.Name == "" {
				continue
			}
			g.SaveDirs = saveCandidatesFor(g, libs)
			out.Games = append(out.Games, g)
			byInstallDir[strings.ToLower(g.InstallDir)] = true
			found++
		}
		// Whatever the manifests missed, the common/ folder still holds:
		// list those too rather than showing an empty panel beside a
		// library the player can see in Explorer.
		extra := 0
		if entries, err := os.ReadDir(filepath.Join(lib, "common")); err == nil {
			for _, e := range entries {
				if !e.IsDir() || byInstallDir[strings.ToLower(e.Name())] {
					continue
				}
				g := discoveredGame{Name: e.Name(), InstallDir: e.Name()}
				g.SaveDirs = saveCandidatesFor(g, libs)
				out.Games = append(out.Games, g)
				byInstallDir[strings.ToLower(e.Name())] = true
				extra++
			}
		}
		note := fmt.Sprintf("%d game%s from manifests", found, plural(found))
		if extra > 0 {
			note += fmt.Sprintf(", %d more from common/", extra)
		}
		for i := range out.Probes {
			if out.Probes[i].Resolved == lib && out.Probes[i].Note == "" {
				out.Probes[i].Note = note
			}
		}
	}

	sort.Slice(out.Games, func(i, j int) bool {
		// Games with a found save folder first; alphabetical within.
		if (len(out.Games[i].SaveDirs) > 0) != (len(out.Games[j].SaveDirs) > 0) {
			return len(out.Games[i].SaveDirs) > 0
		}
		return strings.ToLower(out.Games[i].Name) < strings.ToLower(out.Games[j].Name)
	})
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// knownSaveDirs is the catalog: games whose save location is known or
// credibly guessed, keyed by Steam install dir. Grows as games are
// verified; a wrong entry only ever costs a player one glance, because
// candidates are confirmed, never followed blindly.
var knownSaveDirs = map[string][]func() string{
	// RuneScape: Dragonwilds — the SaveGames guess is recon-pending
	// (docs/save-sync-architecture.md phase 0); SaveCharacters beside it
	// is verified but is per-character data, not the world.
	"RSDragonwilds": {func() string { return filepath.Join(localAppData(), "RSDragonwilds", "Saved", "SaveGames") }},
}

func localAppData() string { return os.Getenv("LOCALAPPDATA") }
