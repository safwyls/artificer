package main

// Browsing this machine's folders, so the save folder can be picked
// rather than typed.
//
// Discovery finds most save folders and asks the player to confirm the
// rest. "The rest" used to mean pasting a Windows path into a text box —
// which is where the last mile went wrong most often: the wrong slash,
// a quoted "Copy as path", a folder one level off. Browsing removes the
// transcription entirely.
//
// This lists folder names only, never file contents, and the app is
// bound to 127.0.0.1. It is the player's own machine and the app already
// reads and writes the save folders they point it at; enumerating
// directory names is strictly less than it does with them.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// browseEntry is one folder inside the listed one.
type browseEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Saveish marks a folder whose name suggests saves live in it, so
	// the obvious next click is visibly the obvious next click.
	Saveish bool `json:"saveish,omitempty"`
}

// browseRoot is a jumping-off point: the places saves actually live,
// plus whatever drives this machine has.
type browseRoot struct {
	Label string `json:"label"`
	Path  string `json:"path"`
}

type browseResult struct {
	Path    string        `json:"path"`
	Parent  string        `json:"parent,omitempty"`
	Entries []browseEntry `json:"entries"`
	Roots   []browseRoot  `json:"roots"`
	Error   string        `json:"error,omitempty"`
}

// browseRoots are the shortcuts offered beside the listing: the same
// places the save search already walks (so the browser opens where the
// answer usually is), then the drives.
func browseRoots() []browseRoot {
	seen := map[string]bool{}
	out := []browseRoot{}
	add := func(label, path string) {
		if path == "" || seen[path] {
			return
		}
		if _, err := os.Stat(path); err != nil {
			return
		}
		seen[path] = true
		out = append(out, browseRoot{Label: label, Path: path})
	}
	if home, err := os.UserHomeDir(); err == nil {
		add("Home", home)
	}
	for _, r := range saveSearchRoots() {
		add(r.Label, r.Path)
	}
	if runtime.GOOS == "windows" {
		for c := 'C'; c <= 'Z'; c++ {
			add(string(c)+":", string(c)+`:\`)
		}
		return out
	}
	add("/", "/")
	return out
}

// browse lists the folders inside one directory. An unreadable folder is
// an answer, not an error: the browser stays open on the parent and says
// what happened, because "Permission denied" is information the player
// can act on.
func browse(path string) browseResult {
	res := browseResult{Roots: browseRoots(), Entries: []browseEntry{}}
	path = cleanPastedPath(path)
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = home
		} else {
			path = string(filepath.Separator)
		}
	}
	// A file (or a path with a filename appended) browses its folder —
	// pasting the full path to a save file is a natural mistake and an
	// obvious intent.
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		path = filepath.Dir(path)
	}
	res.Path = filepath.Clean(path)
	if parent := filepath.Dir(res.Path); parent != res.Path {
		res.Parent = parent
	}

	entries, err := os.ReadDir(res.Path)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		res.Entries = append(res.Entries, browseEntry{
			Name:    e.Name(),
			Path:    filepath.Join(res.Path, e.Name()),
			Saveish: looksLikeSaveFolder(e.Name()),
		})
	}
	sort.Slice(res.Entries, func(i, j int) bool {
		return strings.ToLower(res.Entries[i].Name) < strings.ToLower(res.Entries[j].Name)
	})
	return res
}

func (a *app) handleBrowse(w http.ResponseWriter, r *http.Request) {
	out := browse(r.URL.Query().Get("path"))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "browse": out})
}

// handleHide puts a shelf entry away, or brings it back.
func (a *app) handleHide(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Key    string `json:"key"`
		Hidden bool   `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Key) == "" {
		writeJSON(w, map[string]any{"ok": false, "error": "invalid body"})
		return
	}
	a.mu.Lock()
	a.cfg.setHidden(strings.TrimSpace(in.Key), in.Hidden)
	a.mu.Unlock()
	if err := a.saveCfg(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
