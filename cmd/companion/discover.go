package main

// Installed-game discovery: find what this machine plays and where those
// games keep their saves, so linking a save folder to a world is a pick,
// not a paste. Steam's own metadata is the ground truth for what is
// installed (libraryfolders.vdf → appmanifest_*.acf); the save locations
// are heuristics — Unreal's Saved/SaveGames convention plus a small
// catalog of known games — and every candidate is marked as the guess it
// is. The player confirms the folder; nothing syncs a guessed path
// unseen.
//
// Finding Steam itself is layered, because "no games found" on a machine
// that plainly has Steam is the failure mode that makes the whole panel
// look broken: the player's configured folders first (they accept a
// Steam root, a steamapps folder, or steamapps/common — whatever
// spelling they pasted), then the registry on Windows (the canonical
// answer for a custom install drive), then STEAM_ROOT, then the default
// install locations. Whatever happens, the scan reports where it looked,
// so an empty result names the fix instead of shrugging.

import (
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
	// machine, best guess first. Heuristic, always — the player confirms.
	SaveDirs []string `json:"saveDirs,omitempty"`
}

// discovery is one scan's result: the games, and where the scan actually
// looked — the difference between "nothing installed" and "never found
// your Steam".
type discovery struct {
	Games []discoveredGame `json:"games"`
	// Libraries are the steamapps folders that were read.
	Libraries []string `json:"libraries"`
}

var (
	vdfPath      = regexp.MustCompile(`"path"\s+"((?:[^"\\]|\\.)+)"`)
	acfName      = regexp.MustCompile(`"name"\s+"((?:[^"\\]|\\.)+)"`)
	acfAppID     = regexp.MustCompile(`"appid"\s+"(\d+)"`)
	acfInstall   = regexp.MustCompile(`"installdir"\s+"((?:[^"\\]|\\.)+)"`)
	vdfUnescaper = regexp.MustCompile(`\\(.)`)
)

func unescapeVDF(s string) string { return vdfUnescaper.ReplaceAllString(s, "$1") }

// normalizeSteamDir maps whatever spelling of "my Steam folder" a player
// pasted onto the steamapps folder it implies: the Steam root, the
// steamapps folder itself, or steamapps/common all land on the same
// place. "" means the path holds no steamapps folder at all.
func normalizeSteamDir(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if strings.EqualFold(filepath.Base(p), "common") {
		p = filepath.Dir(p) // …/steamapps/common → …/steamapps
	}
	if !strings.EqualFold(filepath.Base(p), "steamapps") {
		p = filepath.Join(p, "steamapps") // a Steam root
	}
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p
	}
	return ""
}

// steamRoots lists the Steam install roots worth probing, most
// authoritative first: the registry (Windows — the canonical answer for
// a custom install drive), STEAM_ROOT, then the default locations.
func steamRoots() []string {
	var roots []string
	if reg := steamRootFromRegistry(); reg != "" {
		roots = append(roots, reg)
	}
	if root := os.Getenv("STEAM_ROOT"); root != "" {
		roots = append(roots, root)
	}
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

// steamLibraries resolves every steamapps folder worth reading: the
// player's configured folders, the detected roots, and whatever those
// roots' libraryfolders.vdf name (a second drive's library arrives here
// automatically once any root is found).
func steamLibraries(extraDirs []string) []string {
	seen := map[string]bool{}
	var libs []string
	add := func(steamapps string) {
		if steamapps == "" || seen[strings.ToLower(steamapps)] {
			return
		}
		seen[strings.ToLower(steamapps)] = true
		libs = append(libs, steamapps)
	}
	for _, dir := range extraDirs {
		add(normalizeSteamDir(dir))
	}
	for _, root := range steamRoots() {
		add(normalizeSteamDir(root))
	}
	// Expand each found library's libraryfolders.vdf once.
	for i := 0; i < len(libs); i++ {
		data, err := os.ReadFile(filepath.Join(libs[i], "libraryfolders.vdf"))
		if err != nil {
			continue
		}
		for _, m := range vdfPath.FindAllStringSubmatch(string(data), -1) {
			add(normalizeSteamDir(filepath.FromSlash(unescapeVDF(m[1]))))
		}
	}
	return libs
}

// discoverGames scans the Steam libraries for installed games and
// attaches existing save-folder candidates to each. extraDirs are the
// player's configured Steam folders, tried first.
func discoverGames(extraDirs []string) discovery {
	out := discovery{Libraries: steamLibraries(extraDirs)}
	for _, lib := range out.Libraries {
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
			out.Games = append(out.Games, g)
		}
	}
	sort.Slice(out.Games, func(i, j int) bool {
		// Games with a found save folder first; alphabetical within.
		if (len(out.Games[i].SaveDirs) > 0) != (len(out.Games[j].SaveDirs) > 0) {
			return len(out.Games[i].SaveDirs) > 0
		}
		return out.Games[i].Name < out.Games[j].Name
	})
	return out
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
