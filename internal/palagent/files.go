package palagent

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The file verbs serve exactly two things, both at fixed locations under
// the install root: the world save directory (as a tar bundle) and
// PalWorldSettings.ini. No client-supplied paths, ever — same stance as
// the steam verbs.

// configRelPath is where the dedicated server keeps its settings.
const configRelPath = "Pal/Saved/Config/LinuxServer/PalWorldSettings.ini"

// maxConfigBytes caps a config upload; a real PalWorldSettings.ini is a
// few KB.
const maxConfigBytes = 1 << 20

// findSaveDir locates the world save directory: the folder holding
// Level.sav under Pal/Saved/SaveGames/0/<world id>/. A fresh install (or
// one that hasn't booted yet) legitimately has none. With multiple worlds
// the most recently written Level.sav wins — that's the world the server
// is actually running.
func (a *Agent) findSaveDir() (string, error) {
	matches, err := filepath.Glob(filepath.Join(a.cfg.InstallDir, "Pal", "Saved", "SaveGames", "0", "*", "Level.sav"))
	if err != nil || len(matches) == 0 {
		return "", errors.New("no world save found under the install dir (has the server run yet?)")
	}
	best, bestMod := "", int64(-1)
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if mod := info.ModTime().UnixNano(); mod > bestMod {
			best, bestMod = filepath.Dir(m), mod
		}
	}
	if best == "" {
		return "", errors.New("no readable world save under the install dir")
	}
	return best, nil
}

// saveEntry is one file in the bundle.
type saveEntry struct {
	rel  string
	size int64
	mod  int64
}

// listSaveFiles walks the save directory for .sav files, skipping the
// rolling backup folders some server images keep next to the save — the
// same filter the backup archiver applies (internal/backup.writeArchive),
// so an agent-synced backup archives the same set a mounted one would.
func listSaveFiles(saveDir string) ([]saveEntry, error) {
	var out []saveEntry
	err := filepath.WalkDir(saveDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.EqualFold(d.Name(), "backup") || strings.EqualFold(d.Name(), "backups") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".sav") {
			return nil
		}
		rel, err := filepath.Rel(saveDir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, saveEntry{rel: filepath.ToSlash(rel), size: info.Size(), mod: info.ModTime().UnixNano()})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out, err
}

// bundleETag fingerprints the file set: any added, removed, resized or
// rewritten .sav changes it. Content is not hashed — modtime+size is how
// the rest of palcon detects save changes too.
func bundleETag(entries []saveEntry) string {
	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%s|%d|%d\n", e.rel, e.size, e.mod)
	}
	return `"` + hex.EncodeToString(h.Sum(nil)) + `"`
}

// handleGetSave streams the world save directory as a tar bundle, with an
// ETag so the poller's unchanged checks cost a directory walk and no
// transfer. No compression: .sav files are already compressed containers.
func (a *Agent) handleGetSave(w http.ResponseWriter, r *http.Request) {
	saveDir, err := a.findSaveDir()
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	entries, err := listSaveFiles(saveDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(entries) == 0 {
		writeError(w, http.StatusNotFound, "world save directory holds no .sav files")
		return
	}
	etag := bundleETag(entries)
	if r.Header.Get("If-None-Match") == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("ETag", etag)
	tw := tar.NewWriter(w)
	for _, e := range entries {
		f, err := os.Open(filepath.Join(saveDir, filepath.FromSlash(e.rel)))
		if err != nil {
			// Mid-stream now; the client sees a truncated tar and retries.
			a.cfg.Logger.Warn("save bundle: file vanished mid-stream", "file", e.rel, "error", err)
			return
		}
		// Header sizes come from the listing; a file rewritten mid-stream
		// is copied at exactly the promised length so the archive stays
		// well-formed (the ETag the client stored still tells on it).
		hdr := &tar.Header{Name: e.rel, Mode: 0o644, Size: e.size}
		if err := tw.WriteHeader(hdr); err != nil {
			f.Close()
			return
		}
		if _, err := io.CopyN(tw, f, e.size); err != nil {
			f.Close()
			return
		}
		f.Close()
	}
	_ = tw.Close()
}

func (a *Agent) configPath() string {
	return filepath.Join(a.cfg.InstallDir, filepath.FromSlash(configRelPath))
}

func (a *Agent) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	data, err := os.ReadFile(a.configPath())
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "PalWorldSettings.ini not found under the install dir")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

// handlePutConfig replaces PalWorldSettings.ini atomically (tmp + rename),
// so the game can never boot on a half-written file.
func (a *Agent) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxConfigBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "config too large or unreadable")
		return
	}
	path := a.configPath()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		// Refuse to conjure a config where none exists — that means the
		// install dir is wrong or the server never booted, and a stray
		// file here would mask it.
		writeError(w, http.StatusNotFound, "PalWorldSettings.ini not found under the install dir")
		return
	}
	tmp := path + ".palagent-tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.cfg.Logger.Info("config written", "bytes", len(data))
	w.WriteHeader(http.StatusNoContent)
}
