package agent

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
	"time"
)

// The file verbs serve exactly two things, both at fixed locations under
// the install root: the world save directory (as a tar bundle) and
// enshrouded_server.json. No client-supplied paths, ever — same stance
// as the steam verbs.
//
// configRelPath lives in launch.go beside the profile that owns it.

// maxConfigBytes caps a config upload; a real enshrouded_server.json with
// a full Custom gameSettings block is a few KB.
const maxConfigBytes = 1 << 20

// findSaveDir locates the world save directory. A fresh install (or one
// that hasn't booted yet) legitimately has none.
func (a *Agent) findSaveDir() (string, error) {
	if a.cfg.Game.FindSaveDir != nil {
		return a.cfg.Game.FindSaveDir(a.cfg.InstallDir)
	}
	full := filepath.Join(a.cfg.InstallDir, a.cfg.Game.SaveDirName)
	entries, err := os.ReadDir(full)
	if err != nil || len(entries) == 0 {
		return "", errors.New("no world save found under the install dir (has the server run yet?)")
	}
	return full, nil
}

// saveEntry is one file in the bundle.
type saveEntry struct {
	rel  string
	size int64
	mod  int64
}

// listSaveFiles walks the save directory, skipping rolling backup folders
// kept next to the save — the same filter the backup archiver applies
// (internal/backup.writeArchive), so an agent-synced backup archives the
// same set a mounted one would. Enshrouded's save files are extensionless
// hex-named blobs plus the -index and -info JSON sidecars (recon doc,
// "Saves"), so every regular file in the directory belongs to the world.
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
// rewritten save file changes it. Content is not hashed — modtime+size is how
// the rest of flametender detects save changes too.
func bundleETag(entries []saveEntry) string {
	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%s|%d|%d\n", e.rel, e.size, e.mod)
	}
	return `"` + hex.EncodeToString(h.Sum(nil)) + `"`
}

// handleGetSave streams the world save directory as a tar bundle, with an
// ETag so the poller's unchanged checks cost a directory walk and no
// transfer. No compression: the world blobs are already compressed.
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
		writeError(w, http.StatusNotFound, "the world save directory is empty")
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
		// ModTime rides along so the mirror can keep the save's true write
		// time — it is what the console's save cache keys on, and what the
		// world panel reports as "last written". PAX, because USTAR rounds
		// times to whole seconds.
		hdr := &tar.Header{Name: e.rel, Mode: 0o644, Size: e.size, ModTime: time.Unix(0, e.mod), Format: tar.FormatPAX}
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

// maxRestoreBytes bounds a save upload — the same ceiling agentctl
// applies when extracting bundles from this agent.
const maxRestoreBytes = 4 << 30

// handleHeadSave answers the bundle ETag without the bundle, so a
// restore caller can state its precondition without a full download. An
// install with no save yet answers the ETag of the empty set — a real
// value, so "restore into a fresh install" has something to match.
func (a *Agent) handleHeadSave(w http.ResponseWriter, _ *http.Request) {
	var entries []saveEntry
	if saveDir, err := a.findSaveDir(); err == nil {
		if entries, err = listSaveFiles(saveDir); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.Header().Set("ETag", bundleETag(entries))
	w.WriteHeader(http.StatusNoContent)
}

// handlePutSave replaces the world save with an uploaded bundle — the
// one deliberate widening of the fixed-verb file surface
// (docs/sidecar-agent.md), argued in docs/save-sync-architecture.md and
// gated three ways:
//
//  1. The supervised game must not be running. (A companion-mode agent
//     cannot see the game container; its console gates on container
//     state before calling.)
//  2. If-Match must name the current bundle ETag — the repo's first
//     write precondition. A save that changed since the caller last
//     looked is never blindly replaced; 412 carries the current ETag so
//     the caller can look again and decide again.
//  3. The upload is extracted beside the save and verified before an
//     atomic swap, with the replaced save kept as one .bak.
func (a *Agent) handlePutSave(w http.ResponseWriter, r *http.Request) {
	if a.game != nil && a.game.Running() {
		writeError(w, http.StatusConflict, "the game is running — stop it before restoring a save")
		return
	}

	saveDir, findErr := a.findSaveDir()
	var entries []saveEntry
	if findErr == nil {
		var err error
		if entries, err = listSaveFiles(saveDir); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		// No save yet: restore targets the game's declared location. A
		// game whose save dir needs discovery has nowhere unambiguous to
		// restore into until the server has run once.
		if a.cfg.Game.SaveDirName == "" {
			writeError(w, http.StatusConflict, "no save directory exists yet and this game's location needs discovery — start the server once first")
			return
		}
		saveDir = filepath.Join(a.cfg.InstallDir, a.cfg.Game.SaveDirName)
	}
	current := bundleETag(entries)
	ifMatch := r.Header.Get("If-Match")
	if ifMatch == "" {
		writeError(w, http.StatusBadRequest, "If-Match is required: state the save you believe you are replacing")
		return
	}
	if ifMatch != current {
		w.Header().Set("ETag", current)
		writeError(w, http.StatusPreconditionFailed, "the save changed since you last looked — fetch it again and decide again")
		return
	}

	tmp := saveDir + ".agent-restore"
	if err := os.RemoveAll(tmp); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := extractBundle(http.MaxBytesReader(w, r.Body, maxRestoreBytes), tmp); err != nil {
		os.RemoveAll(tmp)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if a.cfg.Game.VerifyRestore != nil {
		if err := a.cfg.Game.VerifyRestore(tmp); err != nil {
			os.RemoveAll(tmp)
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	bak := saveDir + ".bak"
	if _, err := os.Stat(saveDir); err == nil {
		if err := os.RemoveAll(bak); err != nil {
			os.RemoveAll(tmp)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := os.Rename(saveDir, bak); err != nil {
			os.RemoveAll(tmp)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else if err := os.MkdirAll(filepath.Dir(saveDir), 0o755); err != nil {
		os.RemoveAll(tmp)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Rename(tmp, saveDir); err != nil {
		// Put the old save back rather than leaving nothing live.
		os.Rename(bak, saveDir)
		os.RemoveAll(tmp)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	newEntries, err := listSaveFiles(saveDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.cfg.Logger.Info("save restored", "files", len(newEntries), "dir", saveDir)
	w.Header().Set("ETag", bundleETag(newEntries))
	w.WriteHeader(http.StatusNoContent)
}

// extractBundle unpacks an uploaded save bundle into dir, admitting only
// relative regular files that resolve inside it — kept in lockstep with
// agentctl's extractTar, which applies the same rules in the other
// direction.
func extractBundle(r io.Reader, dir string) error {
	const maxFiles = 20_000
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(r)
	files := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			if files == 0 {
				return errors.New("the uploaded bundle holds no files")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("not a save bundle: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if files++; files > maxFiles {
			return errors.New("bundle exceeds the file-count bound")
		}
		name := filepath.FromSlash(hdr.Name)
		if filepath.IsAbs(name) || strings.Contains(hdr.Name, "..") {
			return fmt.Errorf("bundle entry %q escapes the destination", hdr.Name)
		}
		dest := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		if !hdr.ModTime.IsZero() {
			_ = os.Chtimes(dest, hdr.ModTime, hdr.ModTime)
		}
	}
}

func (a *Agent) configPath() string {
	// Follow the launch profile when the agent runs the game (a custom
	// profile can relocate the exe and its json); companion mode launches
	// nothing, so the default location stands.
	if a.game != nil {
		return filepath.Join(a.cfg.InstallDir, a.game.Profile().ConfigRel)
	}
	return filepath.Join(a.cfg.InstallDir, filepath.FromSlash(a.cfg.Game.ConfigRelPath))
}

func (a *Agent) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	data, err := os.ReadFile(a.configPath())
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, a.cfg.Game.ConfigRelPath+" not found under the install dir")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ct := a.cfg.Game.ConfigContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(data)
}

// handlePutConfig replaces enshrouded_server.json atomically (tmp +
// rename), so the game can never boot on a half-written file. It also
// refuses a file that doesn't parse: a malformed json makes the server
// regenerate defaults — an *open, password-less* server — which is a far
// worse failure than a rejected edit.
func (a *Agent) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxConfigBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "config too large or unreadable")
		return
	}
	if a.cfg.Game.ValidateConfig != nil {
		if err := a.cfg.Game.ValidateConfig(data); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	path := a.configPath()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		// Refuse to conjure a config where none exists — that means the
		// install dir is wrong or the server never booted (the supervisor
		// seeds one before first start), and a stray file here would mask
		// it.
		writeError(w, http.StatusNotFound, a.cfg.Game.ConfigRelPath+" not found under the install dir")
		return
	}
	tmp := path + ".agent-tmp"
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
