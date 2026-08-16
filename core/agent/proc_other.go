//go:build !unix

package agent

import (
	"os"
	"os/exec"
	"syscall"
)

// Supervisor mode deploys as a Linux container; on other platforms the
// build still compiles, with plain per-process signaling.
func setProcessGroup(*exec.Cmd) {}

func signalGroup(pid int, graceful syscall.Signal, kill bool) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
