package agentctl

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Extraction guards. The agent is trusted-ish (we configured it), but a
// tar stream crossing the network gets the same paranoia any archive
// gets: relative regular files only, bounded size and count.
const (
	maxBundleBytes = 4 << 30 // no Palworld save dir approaches 4GB
	maxBundleFiles = 20_000
)

// raw performs one request with the auth header and error mapping shared
// with do(), but hands the successful response back for streaming.
func (c *Client) raw(ctx context.Context, method, path string, header http.Header, body io.Reader, timeout time.Duration) (*http.Response, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("agent unreachable: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer cancel()
		defer resp.Body.Close()
		msg := ""
		var parsed struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&parsed) == nil {
			msg = parsed.Error
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, nil, fmt.Errorf("%w: the agent rejected the token — re-check it on both sides", ErrRejected)
		case resp.StatusCode == http.StatusNotFound && msg == "":
			// A JSON-less 404 is the router, not a handler: the agent
			// predates this verb.
			return nil, nil, fmt.Errorf("%w: the agent does not support this operation — update the palagent image", ErrRejected)
		case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusBadRequest:
			return nil, nil, fmt.Errorf("%w: %s", ErrRejected, msg)
		}
		return nil, nil, fmt.Errorf("agent returned %d: %s", resp.StatusCode, msg)
	}
	return resp, cancel, nil
}

// SyncSave mirrors the agent's world save bundle into destDir. etag is the
// value from the previous sync ("" for none); when the bundle is unchanged
// the agent answers 304 and nothing is transferred. Returns the new etag
// and whether destDir was rewritten.
func (c *Client) SyncSave(ctx context.Context, destDir, etag string) (string, bool, error) {
	header := http.Header{}
	if etag != "" {
		header.Set("If-None-Match", etag)
	}
	// Generous timeout: a big world over a slow link is exactly the case
	// this exists for.
	resp, cancel, err := c.raw(ctx, http.MethodGet, "/v1/files/save", header, nil, 10*time.Minute)
	if err != nil {
		return "", false, err
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return etag, false, nil
	}
	newETag := resp.Header.Get("ETag")

	// Extract next to the destination, then swap, so a torn download can
	// never leave destDir half-new. Readers hold the path, not the inode:
	// the swap is invisible to them.
	tmp := destDir + ".sync-tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return "", false, err
	}
	if err := extractTar(resp.Body, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", false, fmt.Errorf("extracting save bundle: %w", err)
	}
	if err := os.RemoveAll(destDir); err != nil {
		os.RemoveAll(tmp)
		return "", false, err
	}
	if err := os.Rename(tmp, destDir); err != nil {
		os.RemoveAll(tmp)
		return "", false, err
	}
	return newETag, true, nil
}

// extractTar unpacks a bundle into dir, admitting only relative regular
// files that resolve inside dir.
func extractTar(r io.Reader, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(r)
	var total int64
	files := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue // the agent only sends regular files; skip decoration like dir entries
		}
		if files++; files > maxBundleFiles {
			return errors.New("bundle exceeds the file-count bound")
		}
		if total += hdr.Size; total > maxBundleBytes {
			return errors.New("bundle exceeds the size bound")
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
	}
}

// GetConfig fetches the raw PalWorldSettings.ini.
func (c *Client) GetConfig(ctx context.Context) ([]byte, error) {
	resp, cancel, err := c.raw(ctx, http.MethodGet, "/v1/files/config", nil, nil, 30*time.Second)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// PutConfig replaces PalWorldSettings.ini; the agent writes atomically.
func (c *Client) PutConfig(ctx context.Context, data []byte) error {
	resp, cancel, err := c.raw(ctx, http.MethodPut, "/v1/files/config", nil, bytes.NewReader(data), 30*time.Second)
	if err != nil {
		return err
	}
	defer cancel()
	resp.Body.Close()
	return nil
}
