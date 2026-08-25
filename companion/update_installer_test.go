package companion

// The Reliquary Companion updates by *installing*, not by replacing a
// file. Its release asset is an installer, and the thing that has to be
// replaced is the whole application — companiond is one file inside it,
// and on Windows a running executable cannot be overwritten at all.
//
// So applying stops one step earlier than the browser build's flow: this
// package downloads and verifies, and hands back a path. Running it means
// quitting the app, which only the shell around this daemon can do.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installerRelease is a stand-in GitHub serving one installer under the
// Reliquary Companion's own asset and manifest names.
func installerRelease(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(payload)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asset := updateAssetName()
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+UpdateVersionAsset):
			w.Write([]byte("feedfacecafe\n"))
		case strings.HasSuffix(r.URL.Path, "/"+UpdateShaAsset):
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), asset)
		case strings.HasSuffix(r.URL.Path, asset):
			w.Write(payload)
		default:
			json.NewEncoder(w).Encode(map[string]any{
				"tag_name": UpdateTag,
				"assets": []map[string]any{
					{"name": UpdateVersionAsset, "browser_download_url": srvURL(r) + "/" + UpdateVersionAsset},
					{"name": UpdateShaAsset, "browser_download_url": srvURL(r) + "/" + UpdateShaAsset},
					{"name": asset, "browser_download_url": srvURL(r) + "/" + asset, "size": len(payload)},
				},
			})
		}
	}))
}

// asReliquaryCompanion switches the package globals the way
// cmd/companiond does, and puts them back.
func asReliquaryCompanion(t *testing.T) {
	t.Helper()
	tag, ver, sha, assets, installs := UpdateTag, UpdateVersionAsset, UpdateShaAsset, UpdateAssets, UpdateInstalls
	UpdateTag = "reliquary-companion-latest"
	UpdateVersionAsset = "reliquary-companion-version.txt"
	UpdateShaAsset = "reliquary-companion-sha256.txt"
	UpdateInstalls = true
	UpdateAssets = map[string]string{
		"windows": "Reliquary-Companion-Setup.exe",
		"linux":   "Reliquary-Companion.AppImage",
		"darwin":  "Reliquary-Companion.dmg",
	}
	t.Cleanup(func() {
		UpdateTag, UpdateVersionAsset, UpdateShaAsset, UpdateAssets, UpdateInstalls = tag, ver, sha, assets, installs
	})
}

// The whole point: nothing of this application is touched. The installer
// is downloaded, checked against the release's own checksum, and left
// somewhere the shell can run it.
func TestApplyStagesAnInstallerInsteadOfReplacingAnything(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS installs by hand from the dmg; see canSelfUpdateLocked")
	}
	asReliquaryCompanion(t)
	payload := fakeBinary("this is the installer")
	srv := installerRelease(t, payload)
	defer srv.Close()

	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
	path, err := a.stageUpdateFrom(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if filepath.Base(path) != updateAssetName() {
		t.Errorf("staged as %q, want the release's own asset name %q", filepath.Base(path), updateAssetName())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the staged installer: %v", err)
	}
	if string(got) != string(payload) {
		t.Error("the staged installer is not what the release served")
	}
	// Not next to the running application: a half-finished download
	// beside a live app is a file something else picks up.
	if strings.HasPrefix(path, filepath.Dir(mustExecutable(t))) {
		t.Errorf("staged beside the running binary (%s)", path)
	}
}

// A download that does not match the release's checksum installs
// nothing, and leaves nothing behind for anyone to run by accident.
func TestAMismatchedInstallerIsNotLeftOnDisk(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS installs by hand from the dmg")
	}
	asReliquaryCompanion(t)
	// The checksum manifest is computed over one payload and a different
	// one is served, which is what a corrupted or swapped asset looks
	// like from here.
	honest := fakeBinary("the real installer")
	sum := sha256.Sum256(honest)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asset := updateAssetName()
		switch {
		case strings.HasSuffix(r.URL.Path, "/"+UpdateVersionAsset):
			w.Write([]byte("feedfacecafe\n"))
		case strings.HasSuffix(r.URL.Path, "/"+UpdateShaAsset):
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), asset)
		case strings.HasSuffix(r.URL.Path, asset):
			w.Write(fakeBinary("something else entirely"))
		default:
			json.NewEncoder(w).Encode(map[string]any{
				"tag_name": UpdateTag,
				"assets": []map[string]any{
					{"name": UpdateVersionAsset, "browser_download_url": srvURL(r) + "/" + UpdateVersionAsset},
					{"name": UpdateShaAsset, "browser_download_url": srvURL(r) + "/" + UpdateShaAsset},
					{"name": asset, "browser_download_url": srvURL(r) + "/" + asset},
				},
			})
		}
	}))
	defer srv.Close()

	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
	path, err := a.stageUpdateFrom(context.Background(), srv.URL)
	if err == nil {
		t.Fatalf("a mismatched download was accepted, and staged at %s", path)
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error = %q, want it to name the checksum", err)
	}
	if path != "" {
		t.Errorf("a rejected download still reported a path: %s", path)
	}
}

// The two flows are told apart by whether a path came back, so the
// browser build must never report one.
func TestTheReplaceInPlaceBuildStagesNothing(t *testing.T) {
	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
	if got := a.StagedInstaller(); got != "" {
		t.Errorf("StagedInstaller = %q before any update, want empty", got)
	}
}

// macOS ships a dmg, which is mounted and dragged rather than run — and
// these builds are unsigned besides. Saying so beats a button that
// downloads ninety megabytes and then cannot do anything with it.
func TestMacOSSaysItInstallsByHand(t *testing.T) {
	asReliquaryCompanion(t)
	a := NewApp(Config{}, filepath.Join(t.TempDir(), "config.json"))
	a.mu.Lock()
	ok, why := a.canSelfUpdateLocked()
	a.mu.Unlock()

	if runtime.GOOS == "darwin" {
		if ok {
			t.Error("offered to run a dmg")
		}
		if !strings.Contains(why, ".dmg") {
			t.Errorf("why = %q, want it to name the dmg", why)
		}
		return
	}
	// Everywhere else the installer flow is available, and notably does
	// not depend on the application's own directory being writable —
	// which is what an installed app in Program Files never is.
	if !ok {
		t.Errorf("installer mode refused on %s: %s", runtime.GOOS, why)
	}
}

func mustExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return exe
}
