package main

// Installed-game discovery: find what this machine plays and where those
// games keep their saves, so linking a save folder to a world is a pick,
// not a paste. Steam's own metadata is the ground truth for what is
// installed (libraryfolders.vdf → appmanifest_*.acf); the save locations
// are heuristics — Unreal's Saved/SaveGames convention plus a small
// catalog of known games — and every candidate is marked as the guess it
// is. The player confirms the folder; nothing syncs a guessed path
// unseen.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
)

// discoveredGame is one installed game and where its saves might be.
type discoveredGame struct {
	Name       string `json:"name"`
	AppID      string `json:"appId,omitempty"`
	InstallDir string `json:"installDir,omitempty"`
	// SaveDirs are candidate save folders that actually exist on this
	// machine, best guess first. Heuristic, always — the player confirms.
	SaveDirs []string `json:"saveDirs,omitempty"`
}

var (
	vdfPath      = regexp.MustCompile(`"path"\s+"((?:[^"\\]|\\.)+)"`)
	acfName      = regexp.MustCompile(`"name"\s+"((?:[^"\\]|\\.)+)"`)
	acfAppID     = regexp.MustCompile(`"appid"\s+"(\d+)"`)
	acfInstall   = regexp.MustCompile(`"installdir"\s+"((?:[^"\\]|\\.)+)"`)
	vdfUnescaper = regexp.MustCompile(`\\(.)`)
)

func unescapeVDF(s string) string { return vdfUnescaper.ReplaceAllString(s, "$1") }

// steamRoots lists the Steam install roots worth probing. STEAM_ROOT
// overrides for odd installs.
func steamRoots() []string {
	if root := os.Getenv("STEAM_ROOT"); root != "" {
		return []string{root}
	}
	var roots []string
	if runtime.GOOS == "windows" {
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
			if base := os.Getenv(env); base != "" {
				roots = append(roots, filepath.Join(base, "Steam"))
			}
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		// Developer platforms, not player machines — but a dev testing
		// discovery deserves real answers.
		roots = append(roots,
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".local", "share", "Steam"))
	}
	return roots
}

// steamLibraries resolves every Steam library folder: the root itself
// plus whatever libraryfolders.vdf names.
func steamLibraries() []string {
	seen := map[string]bool{}
	var libs []string
	add := func(dir string) {
		steamapps := filepath.Join(dir, "steamapps")
		if seen[steamapps] {
			return
		}
		if info, err := os.Stat(steamapps); err == nil && info.IsDir() {
			seen[steamapps] = true
			libs = append(libs, steamapps)
		}
	}
	for _, root := range steamRoots() {
		add(root)
		data, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		for _, m := range vdfPath.FindAllStringSubmatch(string(data), -1) {
			add(filepath.FromSlash(unescapeVDF(m[1])))
		}
	}
	return libs
}

// discoverGames scans the Steam libraries for installed games and
// attaches existing save-folder candidates to each.
func discoverGames() []discoveredGame {
	var games []discoveredGame
	for _, lib := range steamLibraries() {
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
				continue
			}
			g.SaveDirs = saveCandidates(g)
			games = append(games, g)
		}
	}
	sort.Slice(games, func(i, j int) bool {
		// Games with a found save folder first; alphabetical within.
		if (len(games[i].SaveDirs) > 0) != (len(games[j].SaveDirs) > 0) {
			return len(games[i].SaveDirs) > 0
		}
		return games[i].Name < games[j].Name
	})
	return games
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

func localAppData() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	return ""
}

// saveCandidates probes where this game's saves might live and returns
// the folders that exist, catalog entries first, then the generic
// Unreal/My Games conventions.
func saveCandidates(g discoveredGame) []string {
	var candidates []string
	for _, fn := range knownSaveDirs[g.InstallDir] {
		candidates = append(candidates, fn())
	}
	if local := localAppData(); local != "" && g.InstallDir != "" {
		candidates = append(candidates, filepath.Join(local, g.InstallDir, "Saved", "SaveGames"))
	}
	if home, err := os.UserHomeDir(); err == nil && g.Name != "" {
		candidates = append(candidates, filepath.Join(home, "Documents", "My Games", g.Name))
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range candidates {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			out = append(out, c)
		}
	}
	return out
}
