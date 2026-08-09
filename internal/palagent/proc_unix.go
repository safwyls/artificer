//go:build unix

package palagent

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the game in its own process group, so signals
// reach the shipped binary (RSDragonwildsServer-Linux-Shipping) and not
// just the RSDragonwildsServer.sh wrapper.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup signals the whole group; kill=false sends the graceful
// SIGTERM, kill=true the SIGKILL.
func signalGroup(pid int, kill bool) {
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-pid, sig)
}
