package host

// The provisioning guard that matters most in practice: a data directory
// the game container cannot write is not a successful provision. It used
// to pass, and the resulting server failed much later and much less
// legibly — SteamCMD exits 0 having installed nothing.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirUsableBy(t *testing.T) {
	root := t.TempDir()

	owned := filepath.Join(root, "owned")
	if err := os.Mkdir(owned, 0o755); err != nil {
		t.Fatal(err)
	}
	me := os.Geteuid()
	if !dataDirUsableBy(owned, me, os.Getegid()) {
		t.Error("a directory owned by the target user must be usable")
	}

	if dataDirUsableBy(filepath.Join(root, "missing"), me, os.Getegid()) {
		t.Error("a missing directory is never usable")
	}

	// The real case: owned by someone else, no world write — which is
	// exactly a root-created bind source under a 568:568 container.
	if dataDirUsableBy(owned, me+1, os.Getegid()+1) {
		t.Error("a directory owned by another user with mode 755 must not be usable")
	}

	// World-writable rescues it whoever owns it.
	shared := filepath.Join(root, "shared")
	if err := os.Mkdir(shared, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o777); err != nil {
		t.Fatal(err)
	}
	if !dataDirUsableBy(shared, me+1, os.Getegid()+1) {
		t.Error("a world-writable directory is usable by anyone")
	}
}
