package dwsave

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Source adapts this parser to savecache.Source[World]: Locate picks the
// file whose mtime keys the cache, Parse turns it into a World. The cache
// generics keep savecache itself free of anything Dragonwilds.
type Source struct{}

// Locate resolves a configured save path. A file is taken verbatim; a
// directory means the newest *.sav in it — which is also the file the
// server itself loads on start, so the console and the game agree on which
// world is "the" world. The game's own rolling copy ends in .sav.backup and
// is therefore never matched.
func (Source) Locate(savePath string) (string, error) {
	info, err := os.Stat(savePath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return savePath, nil
	}
	entries, err := os.ReadDir(savePath)
	if err != nil {
		return "", err
	}
	var newest string
	var newestAt time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".sav") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if newest == "" || fi.ModTime().After(newestAt) {
			newest, newestAt = e.Name(), fi.ModTime()
		}
	}
	if newest == "" {
		return "", fmt.Errorf("no .sav file in %s", savePath)
	}
	return filepath.Join(savePath, newest), nil
}

// Parse reads and decodes one save file. The whole file is read up front:
// a world save is a few hundred KB per hour of play, not the multi-GB kind
// that would justify streaming, and savecache serializes parses anyway.
func (Source) Parse(ctx context.Context, file string, modTime time.Time) (*World, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	w, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	w.File = filepath.Base(file)
	w.ModTime = modTime.UTC()
	return w, nil
}
