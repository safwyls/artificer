//go:build !unix

package palagent

// Agents deploy as Linux containers; on other platforms disk space is
// simply unreported rather than a build failure.
func diskFree(string) uint64 { return 0 }
