//go:build unix

package agent

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the game in its own process group, so signals
// reach the real server process through any wrapper (a launcher script,
// Wine) and not just the wrapper itself.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup signals the whole group; kill=false sends the game's own
// graceful stop signal (SIGTERM for most, SIGINT where the game saves
// the world on it), kill=true the SIGKILL.
func signalGroup(pid int, graceful syscall.Signal, kill bool) {
	sig := graceful
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-pid, sig)
}
