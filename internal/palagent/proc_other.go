//go:build !unix

package palagent

import (
	"os"
	"os/exec"
)

// Supervisor mode deploys as a Linux container; on other platforms the
// build still compiles, with plain per-process signaling.
func setProcessGroup(*exec.Cmd) {}

func signalGroup(pid int, kill bool) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
