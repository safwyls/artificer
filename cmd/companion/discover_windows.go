//go:build windows

package main

import (
	"golang.org/x/sys/windows/registry"
)

// steamRootFromRegistry asks Windows where Steam actually is — the
// canonical answer for the custom-install-drive case that makes the
// Program Files probes come up empty. Per-user first (SteamPath is
// written by the client the player runs), machine-wide as fallback.
func steamRootFromRegistry() string {
	if root := readRegistryString(registry.CURRENT_USER, `Software\Valve\Steam`, "SteamPath"); root != "" {
		return root
	}
	return readRegistryString(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath")
}

func readRegistryString(root registry.Key, path, name string) string {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	return v
}
