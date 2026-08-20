//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// detachOwnConsole closes the console window a double-clicked build drags
// along. A plain `go build` produces a console-subsystem exe, and a player
// who builds or receives one shouldn't get a black window for forgetting
// -ldflags="-H windowsgui" (the polished build, which never opens one and
// makes this a no-op).
//
// The test for "is this console ours to close": GetConsoleProcessList
// reporting exactly one attached process means Windows created the console
// for this process alone (a double-click launch). Run from a terminal, the
// shell is attached too, and the console is the user's — leave it alone so
// developers keep their output. Logs mirror to companion.log either way.
func detachOwnConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleProcessList := kernel32.NewProc("GetConsoleProcessList")
	if getConsoleProcessList.Find() != nil {
		return
	}
	pids := make([]uint32, 4)
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n == 1 {
		kernel32.NewProc("FreeConsole").Call()
	}
}
