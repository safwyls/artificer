package savesync

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/safwyls/artificer/core/game"
)

// The wire and storage format is the agent's save bundle: a tar of
// relative regular files with PAX mtimes (core/agent/files.go). Versions
// are stored as the tar itself, byte-for-byte what was uploaded and
// byte-for-byte what a checkout downloads — no recompression, no second
// format to be wrong in.

// maxBundleFiles mirrors agentctl's extraction bound; a save directory
// with more files than this is not a save directory.
const maxBundleFiles = 20_000

// UploadError is a refused upload — too big, not a tar, or not a save.
// The API layer renders it as the client's fault (4xx), not the
// console's.
type UploadError struct{ Msg string }

func (e *UploadError) Error() string { return e.Msg }

// stageUpload streams an upload to path, hashing as it writes and
// refusing to write past maxBytes. The file is left in place on success
// and removed on failure.
func stageUpload(body io.Reader, path string, maxBytes int64) (bytes int64, sum string, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(body, maxBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return 0, "", err
	}
	if n > maxBytes {
		os.Remove(path)
		return 0, "", &UploadError{Msg: fmt.Sprintf("upload exceeds this world's size limit (%d bytes)", maxBytes)}
	}
	if n == 0 {
		os.Remove(path)
		return 0, "", &UploadError{Msg: "empty upload"}
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// extractBundle unpacks a staged tar into dir, admitting only relative
// regular files that resolve inside it — the same paranoia agentctl
// applies to bundles from the agent (core/agentctl/files.go extractTar);
// when one changes the other should be checked.
func extractBundle(r io.Reader, dir string, maxBytes int64) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(r)
	var total int64
	files := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			if files == 0 {
				return &UploadError{Msg: "the bundle holds no files"}
			}
			return nil
		}
		if err != nil {
			return &UploadError{Msg: "not a save bundle: " + err.Error()}
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if files++; files > maxBundleFiles {
			return &UploadError{Msg: "bundle exceeds the file-count bound"}
		}
		if total += hdr.Size; total > maxBytes {
			return &UploadError{Msg: "bundle exceeds this world's size limit"}
		}
		name := filepath.FromSlash(hdr.Name)
		if filepath.IsAbs(name) || strings.Contains(hdr.Name, "..") {
			return &UploadError{Msg: fmt.Sprintf("bundle entry %q escapes the destination", hdr.Name)}
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
	}
}

// verifyExtracted applies the game's save layout to an extracted bundle:
// find the world file, check it is a world. The defaults below are kept
// in lockstep with core/backup's (newestWorldFile / verifyWorldFile,
// isSidecar) — the same "match what the game writes, not what looks like
// a save" rules, applied at check-in instead of snapshot time, because
// committing a torn or empty save as the canonical version is strictly
// worse than refusing the upload.
func verifyExtracted(dir string, layout game.SaveLayout) error {
	sidecar := layout.IsSidecar
	if sidecar == nil {
		sidecar = func(name string) bool {
			return strings.HasSuffix(name, "-index") || strings.HasSuffix(name, "-info")
		}
	}
	var world string
	var err error
	if layout.WorldFile != nil {
		world, err = layout.WorldFile(dir)
	} else {
		world, err = newestWorldFile(dir, sidecar)
	}
	if err != nil {
		return &UploadError{Msg: err.Error()}
	}
	verify := layout.VerifyWorld
	if verify == nil {
		verify = func(path string) error {
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if info.Size() < 16 {
				return fmt.Errorf("world save is only %d bytes — mid-write or corrupt?", info.Size())
			}
			return nil
		}
	}
	if err := verify(world); err != nil {
		return &UploadError{Msg: err.Error()}
	}
	return nil
}

// newestWorldFile is core/backup's default world finder, applied to the
// extracted bundle: most recently written non-sidecar regular file, no
// extension assumed. Note tar extraction does not restore mtimes here, so
// "newest" degrades to "any non-sidecar file" — which is fine, because
// the question this answers is "is there a world in the bundle at all".
func newestWorldFile(saveDir string, sidecar func(string) bool) (string, error) {
	entries, err := os.ReadDir(saveDir)
	if err != nil {
		return "", fmt.Errorf("reading the bundle: %w", err)
	}
	best, bestMod := "", int64(-1)
	for _, e := range entries {
		if e.IsDir() || sidecar(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if mod := info.ModTime().UnixNano(); mod > bestMod {
			best, bestMod = filepath.Join(saveDir, e.Name()), mod
		}
	}
	if best == "" {
		return "", errors.New("the bundle holds no world files")
	}
	return best, nil
}
