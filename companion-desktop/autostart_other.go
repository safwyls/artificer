//go:build !windows

package main

import "errors"

// Autostart is a Windows feature here, because Windows is where the
// players are — the engine's release asset map has no other desktop
// entry, and Linux is a dev-only build. Elsewhere the setting reports
// that it does not apply rather than pretending, and the settings
// dialog disables the checkbox on the strength of this error.
//
// A feature that cannot work answering with a reason, rather than being
// hidden, is the repo's rule for a game that cannot support something;
// it holds just as well for a platform.
var errAutostartUnsupported = errors.New("starting with the computer is a Windows feature; on this platform, add the companion to your desktop session's startup applications")

func autostartEnabled() (bool, error) { return false, errAutostartUnsupported }

func setAutostart(bool) error { return errAutostartUnsupported }
