//go:build unix

package flameagent

import "syscall"

// diskFree reports free bytes on the filesystem holding path, so flametender
// can warn before a ~20GB validate hits a full disk. Best-effort: 0 on
// error, which the dashboard treats as "unknown" rather than "full".
func diskFree(path string) uint64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0
	}
	return uint64(st.Bavail) * uint64(st.Bsize)
}
