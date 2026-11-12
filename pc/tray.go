//go:build windows

// Hidden window, tray icon and right-click menu.
package main

import (
	_ "embed"
	"fmt"
	"syscall"
	"unsafe"
)

//go:embed assets/locked.png
var lockedPNG []byte

//go:embed assets/unlocked.png
var unlockedPNG []byte

const (
	cmdLockNow   = 1
	cmdQuit      = 2
	timerHooks   = 1
	timerReblock = 2
)

var (
	mainHwnd       uintptr
	iconLocked     uintptr
	iconUnlocked   uintptr
	taskbarCreated uintptr
	trayAdded      bool
	lastBlockOK    bool
)

func iconFromPNG(data []byte) uintptr {
	h, _, _ := procCreateIconFromResEx.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 1, 0x00030000, 0, 0, 0)
	return h
}

func statusText() string {
	if locked.Load() {
		return "Locked"
	}
	return "Unlocked"
}

func trayData() notifyIconData {
	nid := notifyIconData{HWnd: mainHwnd, UID: 1, UFlags: nifMsg | nifIcon | nifTip, UCallbackMessage: wmAppTray}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HIcon = iconUnlocked
	if locked.Load() {
		nid.HIcon = iconLocked
	}
	copyU16(nid.SzTip[:], fmt.Sprintf("PhoneLock: %s\nPC address: %s", statusText(), localIP()))
	return nid
}

func addTray() {
	nid := trayData()
	r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	trayAdded = r != 0
}

func updateTray() {
	nid := trayData()
	if !trayAdded {
		addTray()
		return
	}
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
}

func removeTray() {
	nid := trayData()
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

func showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	defer procDestroyMenu.Call(menu)
	add := func(flags, id uintptr, text string) {
		procAppendMenuW.Call(menu, flags, id, uintptr(unsafe.Pointer(u16(text))))
	}
	add(mfString|mfGrayed, 0, "PhoneLock: "+statusText())
	add(mfString|mfGrayed, 0, "PC address: "+localIP())
	add(mfString|mfGrayed, 0, "Lock key: Alt + Shift + Win")
	add(mfString|mfGrayed, 0, "Version "+appVersion)
	add(mfSeparator, 0, "")
	add(mfString, cmdLockNow, "Lock now")
	add(mfString, cmdQuit, "Quit PhoneLock")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(mainHwnd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightButton|tpmBottomAlign, uintptr(pt.X), uintptr(pt.Y), 0, mainHwnd, 0)
	postMessage(mainHwnd, wmNull, 0, 0)

	switch cmd {
	case cmdLockNow:
		setLocked(true)
	case cmdQuit:
		removeTray()
		removeHooks()
		procPostQuitMessage.Call(0)
	}
}

func wndProc(hwnd, m, wParam, lParam uintptr) uintptr {
	switch {
	case m == wmAppTray:
		ev := lParam & 0xFFFF
		if ev == wmRButtonUp || ev == wmLButtonUp {
			showMenu()
		}
		return 0
	case m == wmAppState:
		if locked.Load() {
			releaseHeld()
			swallowedClicks.Store(0)
			swallowedKeys.Store(0)
			// Hooks stop normal input; BlockInput also stops raw input (what games read for camera look).
			r, _, e := procBlockInput.Call(1)
			lastBlockOK = r != 0
			logf("LOCKED  hooks kb=%#x mouse=%#x  BlockInput=%d err=%v", kbHook, mouseHook, r, e)
			procSetTimer.Call(mainHwnd, timerReblock, 1000, 0)
		} else {
			procKillTimer.Call(mainHwnd, timerReblock)
			r, _, e := procBlockInput.Call(0)
			logf("UNLOCKED  BlockInput(off)=%d err=%v  swallowed while locked: %d clicks, %d keys", r, e, swallowedClicks.Load(), swallowedKeys.Load())
			clearHeld()
		}
		updateTray()
		return 0
	case m == wmTimer && wParam == timerReblock:
		// Windows lifts BlockInput after Ctrl+Alt+Del; put it back while still locked.
		if locked.Load() {
			r, _, e := procBlockInput.Call(1)
			// r==0 is normal when input is still blocked; only log changes.
			if ok := r != 0; ok != lastBlockOK {
				logf("re-block BlockInput=%d err=%v", r, e)
			}
		}
		return 0
	case m == wmTimer && wParam == timerHooks:
		// Windows can silently drop a slow low-level hook; re-installing regularly keeps the lock reliable.
		installHooks()
		return 0
	case taskbarCreated != 0 && m == taskbarCreated:
		trayAdded = false // Explorer restarted
		addTray()
		return 0
	case m == wmDestroy:
		removeTray()
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wParam, lParam)
	return r
}

func runUI() {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className := u16("PhoneLockWindow")
	wc := wndClassExW{LpfnWndProc: syscall.NewCallback(wndProc), HInstance: hInst, LpszClassName: className}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	mainHwnd, _, _ = procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(u16("PhoneLock"))),
		0, 0, 0, 0, 0, 0, 0, hInst, 0)
	if mainHwnd == 0 {
		messageBox("PhoneLock couldn't start (window creation failed).", mbIconError)
		return
	}
	taskbarCreated, _, _ = procRegisterWindowMessage.Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))

	iconLocked = iconFromPNG(lockedPNG)
	iconUnlocked = iconFromPNG(unlockedPNG)
	addTray()
	installHooks()
	logf("started v%s  admin=%v  hooks kb=%#x mouse=%#x  ip=%s", appVersion, isAdmin(), kbHook, mouseHook, localIP())
	procSetTimer.Call(mainHwnd, timerHooks, 15000, 0)

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	procBlockInput.Call(0)
	removeHooks()
}
