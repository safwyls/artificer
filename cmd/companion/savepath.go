package main

// Where a world lives, split into the part that differs per machine and
// the part that does not.
//
// A save folder has two halves. The root is machine-specific and
// discoverable: %LOCALAPPDATA%/Witchspire/Saved/SaveGames. The leaf is
// the world's own identity inside it, and Unreal games routinely make it
// an opaque id — K2hAc0p_LH74aymwOemkgg — generated once and never
// renameable. Everyone playing that world shares the leaf; nobody can
// retype it.
//
// Save bundles carry paths relative to the linked folder, so the leaf
// never travels inside an archive. That is what makes this tractable:
// the world records its leaf as metadata, a joining player picks only
// their own root, and their companion recreates the leaf beneath it.
// Nothing about the transfer changes.
//
// The split is a guess with two good sources and a safe fallback, and
// the player sees it before anything is created.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	errSaveRootRequired = errors.New("choose the folder this game keeps its saves in")
	errBadSavePath      = errors.New("the world's folder name is not a usable path")
	errSaveRootMissing  = errors.New("that save folder does not exist on this machine — check the path, or let the game create it by running once")
)

// savePathSplit is one save folder, in two halves that join back to it.
type savePathSplit struct {
	// Root is the folder a joining player would choose on their own
	// machine.
	Root string `json:"root"`
	// Leaf is what lies beneath it, slash-separated and relative. Empty
	// means the root is the world's folder and there is nothing to
	// reproduce — the ordinary case for most games.
	Leaf string `json:"leaf"`
	// Why names where the split came from, because a guess the player
	// cannot see is a guess they cannot correct.
	Why string `json:"why,omitempty"`
}

// splitSaveDir works out which part of a chosen folder is the root.
//
// Strongest first: a catalogue or discovery candidate that is a prefix
// of the folder is the root by definition — the manifest describes where
// the game keeps saves, so anything below that is the world. Failing
// that, a parent named like a save container (SaveGames, Saved, Saves)
// means the folder inside it is one save among several. Failing both,
// the whole folder is the root and there is no leaf, which is correct
// for every game that keeps one save folder per game.
func splitSaveDir(dir string, knownRoots []string) savePathSplit {
	dir = filepath.Clean(dir)
	for _, root := range knownRoots {
		root = filepath.Clean(root)
		if root == "" || root == "." || strings.EqualFold(root, dir) {
			continue
		}
		rel, err := filepath.Rel(root, dir)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		return savePathSplit{Root: root, Leaf: filepath.ToSlash(rel), Why: "the catalogue puts this game's saves in the folder above"}
	}
	// A folder that is itself a save container is the root, whatever it
	// sits inside. Without this, Saved/SaveGames splits into a "world"
	// called SaveGames — every player would then get a SaveGames folder
	// created inside their own SaveGames folder.
	if looksLikeSaveFolder(filepath.Base(dir)) {
		return savePathSplit{Root: dir}
	}
	parent := filepath.Dir(dir)
	if parent != dir && looksLikeSaveFolder(filepath.Base(parent)) {
		return savePathSplit{
			Root: parent,
			Leaf: filepath.ToSlash(filepath.Base(dir)),
			Why:  "this folder sits inside " + filepath.Base(parent) + ", so it is one save among several",
		}
	}
	return savePathSplit{Root: dir}
}

// joinSavePath places a world's leaf under a player's own root. The leaf
// is checked here as well as at the service, because this is the side
// that turns it into a real path.
func joinSavePath(root, leaf string) (string, error) {
	root = cleanPastedPath(root)
	if root == "" {
		return "", errSaveRootRequired
	}
	leaf = strings.ReplaceAll(leaf, "\\", "/")
	if leaf == "" {
		return filepath.Clean(root), nil
	}
	// Absolute is refused rather than quietly reinterpreted as relative:
	// the service enforces the same rule when it stores the path, and one
	// rule stated twice beats two rules that mostly agree.
	if strings.HasPrefix(leaf, "/") || (len(leaf) > 1 && leaf[1] == ':') {
		return "", errBadSavePath
	}
	parts := strings.Split(leaf, "/")
	for _, part := range parts {
		switch part {
		case "", ".", "..":
			return "", errBadSavePath
		}
	}
	return filepath.Join(append([]string{filepath.Clean(root)}, parts...)...), nil
}

// prepareWorldDir joins the world's folder under the player's root and
// creates it if it is not there yet.
//
// Creating is the whole point: the second player to join a world cannot
// type an opaque id they have never seen, and the game will not make the
// folder until it has saved into it. The root must already exist, so a
// typo produces a refusal rather than a tree of empty folders in a place
// nobody meant.
func prepareWorldDir(root, leaf string) (string, error) {
	dir, err := joinSavePath(root, leaf)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(filepath.Clean(cleanPastedPath(root)))
	if err != nil || !info.IsDir() {
		return "", errSaveRootMissing
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
