//go:build windows

package main

// "Start with this computer, minimized to the tray."
//
// An HKCU Run entry, which is the light-touch way: it needs no elevation,
// no installer and no scheduled task, and it disappears with the user
// profile it belongs to. The command it registers passes --minimized, so
// logging in brings the companion up syncing in the tray rather than
// throwing a window over whatever the player was doing.

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	// runValueName is this build's own entry. The browser build, if it
	// ever grows one, gets its own name — two companions starting at
	// login would refuse each other, loudly, every morning.
	runValueName = "ReliquaryCompanion"
)

func autostartCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return `"` + exe + `" --minimized`, nil
}

func autostartEnabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(runValueName)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// An entry pointing at a companion that has since moved is not an
	// autostart, it is a broken shortcut. Report it as off so ticking
	// the box rewrites it.
	want, err := autostartCommand()
	if err != nil {
		return v != "", nil
	}
	return strings.EqualFold(v, want), nil
}

func setAutostart(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	cmd, err := autostartCommand()
	if err != nil {
		return err
	}
	return k.SetStringValue(runValueName, cmd)
}
