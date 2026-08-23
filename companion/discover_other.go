//go:build !windows

package companion

// steamRootFromRegistry has no non-Windows answer; the env and default
// locations carry discovery on developer platforms.
func steamRootFromRegistry() string { return "" }
