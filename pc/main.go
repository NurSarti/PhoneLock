//go:build windows

// PhoneLock for Windows: Alt+Shift+Win locks the keyboard and mouse; the phone app unlocks.
// Always starts UNLOCKED. Does not start with Windows.
package main

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

func init() {
	// Hooks and the window must live on one OS thread with a message loop.
	runtime.LockOSThread()
}

func main() {
	if secretKey == "" {
		messageBox("This copy of PhoneLock was built without its pairing key.", mbIconError)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		messageBox("PhoneLock couldn't find its own location.", mbIconError)
		return
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}

	initLog(exe)
	if !isAdmin() {
		logf("opened without admin, handing off (v%s)", appVersion)
		launchElevated(exe)
		return
	}
	if err := ensureSetup(exe); err != nil {
		messageBox(err.Error()+"\n\nPhoneLock will still run, but may ask for admin next time.", mbIconInfo)
	}

	// Only one copy at a time.
	_, _, lastErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16("Local\\PhoneLock_SingleInstance"))))
	if errno, ok := lastErr.(syscall.Errno); ok && errno == 183 { // ERROR_ALREADY_EXISTS
		messageBox("PhoneLock is already running. Look for the padlock in the system tray (bottom-right, maybe under the ^ arrow).", mbIconInfo)
		return
	}

	if err := startServer(); err != nil {
		messageBox("PhoneLock couldn't open its network port 47811. Is another copy running?\n\n"+err.Error(), mbIconError)
		return
	}
	runUI()
}
