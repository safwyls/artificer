//go:build unix

package flameagent

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the game in its own process group, so signals
// reach enshrouded_server.exe through Wine's own processes and not just
// the wine wrapper.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup signals the whole group; kill=false sends the graceful
// SIGINT — Enshrouded's clean-shutdown signal, on which it saves the
// world; a SIGTERM to the wrapper is not reliably propagated — and
// kill=true the SIGKILL.
func signalGroup(pid int, kill bool) {
	sig := syscall.SIGINT
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-pid, sig)
}
