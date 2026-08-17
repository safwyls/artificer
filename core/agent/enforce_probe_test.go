package agent

// The two guards that turn a data directory the container cannot write
// into a fixable message instead of a silent dead end: health must not
// call an unwritable install dir "ok", and an install that reports
// success without producing the game must not pass for installed.
// SteamCMD exits 0 in exactly that case.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallDirUsableRequiresWritability(t *testing.T) {
	dir := t.TempDir()
	if !installDirUsable(dir) {
		t.Fatal("a fresh temp dir should be usable")
	}
	if installDirUsable(filepath.Join(dir, "does-not-exist")) {
		t.Error("a missing dir is not usable")
	}

	// Root ignores the mode bits, so this half only means something as an
	// unprivileged user — which is precisely the case that broke.
	if os.Geteuid() == 0 {
		t.Skip("running as root: mode bits do not deny writes")
	}
	readonly := filepath.Join(dir, "readonly")
	if err := os.Mkdir(readonly, 0o555); err != nil {
		t.Fatal(err)
	}
	if installDirUsable(readonly) {
		t.Error("a directory this process cannot write must not report usable")
	}
}
