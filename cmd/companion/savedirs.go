package main

// Finding a game's save folder — the heuristic half of discovery, and
// the half that decides whether the panel is useful or just a list of
// names.
//
// Three sources, strongest first:
//
//  1. Steam Cloud. <steam>/userdata/<account>/<appid>/remote is keyed by
//     the app id we already read from the manifest, so when it exists it
//     is not a guess at all — it is Steam telling us where the save is.
//  2. The catalog: games whose location someone has verified by hand.
//  3. A name search across the handful of places Windows games actually
//     keep saves, one and two levels deep (two, because a publisher
//     folder between the root and the game is the common shape), with a
//     Saved/SaveGames-style subfolder preferred when the matched folder
//     has one.
//
// Every candidate carries why it was offered, because a guess the player
// can't judge is worse than no guess: "Steam Cloud" earns a different
// amount of trust than "name match under Documents".

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// saveCandidate is one possible save folder and the reason it is here.
type saveCandidate struct {
	Path string `json:"path"`
	Why  string `json:"why"`
}

// saveSubdirs are the folder names games put their saves in, checked
// inside a matched game folder.
var saveSubdirs = []string{
	"Saved/SaveGames", "Saved", "SaveGames", "Savegames", "Saves", "Save",
	"Saved Games", "savedata", "SaveData", "storage", "profiles", "PlayerData",
}

// normalizeTitle folds a game name or folder name to letters and digits
// so "RuneScape: Dragonwilds", "RSDragonwilds" and "runescape dragonwilds"
// can be compared.
func normalizeTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// titlesMatch is deliberately conservative: equal, or one contains the
// other and the shorter is long enough that the containment means
// something. "Saved" must never match "Save the World".
func titlesMatch(a, b string) bool {
	a, b = normalizeTitle(a), normalizeTitle(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	short, long := a, b
	if len(short) > len(long) {
		short, long = long, short
	}
	return len(short) >= 6 && strings.Contains(long, short)
}

// saveSearchRoots are the places worth scanning, with a label for the
// reason line. OneDrive's redirected Documents is included because
// Windows moves Documents there by default on a lot of machines, and a
// save search that misses it looks broken to exactly the people who
// didn't choose the redirection.
func saveSearchRoots() []struct{ Path, Label string } {
	var out []struct{ Path, Label string }
	add := func(p, label string) {
		if p == "" {
			return
		}
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			out = append(out, struct{ Path, Label string }{p, label})
		}
	}
	home, _ := os.UserHomeDir()
	local := os.Getenv("LOCALAPPDATA")
	roaming := os.Getenv("APPDATA")

	add(filepath.Join(home, "Saved Games"), "Saved Games")
	add(local, "%LOCALAPPDATA%")
	if local != "" {
		// AppData\LocalLow, where Unity games in particular land.
		add(filepath.Join(filepath.Dir(local), "LocalLow"), "%LOCALAPPDATA%Low")
	}
	add(roaming, "%APPDATA%")
	for _, docs := range []string{
		filepath.Join(home, "Documents"),
		filepath.Join(home, "OneDrive", "Documents"),
	} {
		add(filepath.Join(docs, "My Games"), "Documents\\My Games")
		add(docs, "Documents")
	}
	if home != "" {
		// Linux/dev platforms: Proton prefixes and XDG data.
		add(filepath.Join(home, ".local", "share"), "~/.local/share")
	}
	return out
}

// bestSaveDir returns the save-ish subfolder inside a matched game
// folder, or the folder itself when it holds no such thing.
func bestSaveDir(dir string) string {
	for _, sub := range saveSubdirs {
		p := filepath.Join(dir, filepath.FromSlash(sub))
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return dir
}

// steamCloudSaves finds <steam>/userdata/<account>/<appid>/remote for
// every Steam account on this machine. Exact, not a guess: the app id
// came from the game's own manifest.
func steamCloudSaves(appID string, libs []string) []saveCandidate {
	if appID == "" {
		return nil
	}
	var out []saveCandidate
	seen := map[string]bool{}
	for _, lib := range libs {
		// A library is <steamRoot>/steamapps; userdata is its sibling.
		userdata := filepath.Join(filepath.Dir(lib), "userdata")
		accounts, err := os.ReadDir(userdata)
		if err != nil {
			continue
		}
		for _, acct := range accounts {
			if !acct.IsDir() {
				continue
			}
			p := filepath.Join(userdata, acct.Name(), appID, "remote")
			if info, err := os.Stat(p); err != nil || !info.IsDir() {
				continue
			}
			if entries, err := os.ReadDir(p); err != nil || len(entries) == 0 {
				continue // an empty remote/ is Steam Cloud reserved but unused
			}
			if seen[strings.ToLower(p)] {
				continue
			}
			seen[strings.ToLower(p)] = true
			out = append(out, saveCandidate{Path: p, Why: "Steam Cloud save for app " + appID})
		}
	}
	return out
}

// searchSaveRoots looks for folders named after the game, one and two
// levels down (two catches the publisher folder shape), and returns the
// save-ish folder inside each match.
func searchSaveRoots(g discoveredGame) []saveCandidate {
	names := []string{g.Name, g.InstallDir}
	matches := func(dirName string) bool {
		for _, n := range names {
			if n != "" && titlesMatch(n, dirName) {
				return true
			}
		}
		return false
	}

	var out []saveCandidate
	for _, root := range saveSearchRoots() {
		entries, err := os.ReadDir(root.Path)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(root.Path, e.Name())
			if matches(e.Name()) {
				out = append(out, saveCandidate{
					Path: bestSaveDir(dir),
					Why:  fmt.Sprintf("folder named for the game under %s", root.Label),
				})
				continue
			}
			// One level deeper: <root>/<Publisher>/<Game>.
			children, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, c := range children {
				if !c.IsDir() || !matches(c.Name()) {
					continue
				}
				out = append(out, saveCandidate{
					Path: bestSaveDir(filepath.Join(dir, c.Name())),
					Why:  fmt.Sprintf("folder named for the game under %s\\%s", root.Label, e.Name()),
				})
			}
		}
	}
	return out
}

// saveCandidatesFor assembles every candidate for one game, strongest
// first, keeping only folders that exist and dropping duplicates.
func saveCandidatesFor(g discoveredGame, libs []string) []saveCandidate {
	var all []saveCandidate
	all = append(all, steamCloudSaves(g.AppID, libs)...)
	for _, fn := range knownSaveDirs[g.InstallDir] {
		if p := fn(); p != "" {
			all = append(all, saveCandidate{Path: p, Why: "known location for this game"})
		}
	}
	all = append(all, searchSaveRoots(g)...)
	// The install folder itself, for games that keep saves beside the
	// binary — last, because it is the least likely and the most likely
	// to be wiped by a game update.
	for _, lib := range libs {
		if g.InstallDir == "" {
			break
		}
		install := filepath.Join(lib, "common", g.InstallDir)
		if info, err := os.Stat(install); err != nil || !info.IsDir() {
			continue
		}
		if best := bestSaveDir(install); best != install {
			all = append(all, saveCandidate{Path: best, Why: "save folder inside the game's install"})
		}
	}

	seen := map[string]bool{}
	var out []saveCandidate
	for _, c := range all {
		key := strings.ToLower(c.Path)
		if c.Path == "" || seen[key] {
			continue
		}
		if info, err := os.Stat(c.Path); err != nil || !info.IsDir() {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}
