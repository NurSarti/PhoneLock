//go:build windows

// Low-level keyboard and mouse hooks: detect Alt+Shift+Win and swallow all input while locked.
package main

import (
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	locked atomic.Bool

	kbHook, mouseHook uintptr
	kbCallback        = syscall.NewCallback(keyboardProc)
	mouseCallback     = syscall.NewCallback(mouseProc)

	// Only touched on the UI/hook thread.
	heldKeys     [256]bool
	heldExtended [256]bool
	heldLeft     bool
	heldRight    bool
	heldMiddle   bool
)

// Compile-time size checks for the Win32 structs (64-bit).
var (
	_ [40 - unsafe.Sizeof(keyInput{})]byte
	_ [unsafe.Sizeof(keyInput{}) - 40]byte
	_ [40 - unsafe.Sizeof(mouseInput{})]byte
	_ [unsafe.Sizeof(mouseInput{}) - 40]byte
	_ [976 - unsafe.Sizeof(notifyIconData{})]byte
	_ [unsafe.Sizeof(notifyIconData{}) - 976]byte
)

func setLocked(v bool) {
	if locked.Swap(v) != v {
		postMessage(mainHwnd, wmAppState, 0, 0)
	}
}

func installHooks() {
	hmod, _, _ := procGetModuleHandleW.Call(0)
	newKb, _, _ := procSetWindowsHookExW.Call(whKeyboardLL, kbCallback, hmod, 0)
	newMouse, _, _ := procSetWindowsHookExW.Call(whMouseLL, mouseCallback, hmod, 0)
	// Install the new hooks before removing the old ones so there's never a gap.
	if kbHook != 0 {
		procUnhookWindowsHookEx.Call(kbHook)
	}
	if mouseHook != 0 {
		procUnhookWindowsHookEx.Call(mouseHook)
	}
	kbHook, mouseHook = newKb, newMouse
}

func removeHooks() {
	if kbHook != 0 {
		procUnhookWindowsHookEx.Call(kbHook)
		kbHook = 0
	}
	if mouseHook != 0 {
		procUnhookWindowsHookEx.Call(mouseHook)
		mouseHook = 0
	}
}

func isModifier(vk uint32) bool {
	switch vk {
	case vkLMenu, vkRMenu, vkLShift, vkRShift, vkLWin, vkRWin:
		return true
	}
	return false
}

func keyboardProc(nCode, wParam, lParam uintptr) uintptr {
	if int32(nCode) >= 0 {
		k := (*kbdllHookStruct)(unsafe.Pointer(lParam))
		if k.DwExtraInfo == ourMagic {
			goto pass
		}
		if locked.Load() {
			return 1
		}
		vk := k.VkCode & 0xFF
		down := wParam == wmKeyDown || wParam == wmSysKeyDown
		heldKeys[vk] = down
		heldExtended[vk] = k.Flags&llkhfExtended != 0

		if down && isModifier(vk) {
			alt := heldKeys[vkLMenu] || heldKeys[vkRMenu]
			shift := heldKeys[vkLShift] || heldKeys[vkRShift]
			win := heldKeys[vkLWin] || heldKeys[vkRWin]
			if alt && shift && win {
				heldKeys[vk] = false // this key press is swallowed, Windows never sees it
				setLocked(true)
				return 1
			}
		}
	}
pass:
	r, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
	return r
}

func mouseProc(nCode, wParam, lParam uintptr) uintptr {
	if int32(nCode) >= 0 {
		m := (*msllHookStruct)(unsafe.Pointer(lParam))
		if m.DwExtraInfo == ourMagic {
			goto pass
		}
		if locked.Load() {
			return 1
		}
		switch wParam {
		case wmLButtonDown:
			heldLeft = true
		case wmLButtonUp:
			heldLeft = false
		case wmRButtonDown:
			heldRight = true
		case wmRButtonUp:
			heldRight = false
		case wmMButtonDown:
			heldMiddle = true
		case wmMButtonUp:
			heldMiddle = false
		}
	}
pass:
	r, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
	return r
}

// releaseHeld tells Windows every key/button that was down at lock time is now up,
// so nothing gets stuck (the real "up" events are swallowed while locked).
func releaseHeld() {
	var keys []keyInput
	mk := func(vk uint16, flags uint32) keyInput {
		return keyInput{Type: inputKeyboard, Vk: vk, Flags: flags, Extra: ourMagic}
	}
	keys = append(keys, mk(vkMask, 0), mk(vkMask, keyeventfKeyUp))
	for vk := 1; vk < 256; vk++ {
		if heldKeys[vk] {
			f := uint32(keyeventfKeyUp)
			if heldExtended[vk] {
				f |= keyeventfExt
			}
			keys = append(keys, mk(uint16(vk), f))
			heldKeys[vk] = false
		}
	}
	procSendInput.Call(uintptr(len(keys)), uintptr(unsafe.Pointer(&keys[0])), unsafe.Sizeof(keys[0]))

	var mice []mouseInput
	for _, b := range []struct {
		held *bool
		flag uint32
	}{{&heldLeft, mouseeventfLeftUp}, {&heldRight, mouseeventfRightUp}, {&heldMiddle, mouseeventfMiddleUp}} {
		if *b.held {
			mice = append(mice, mouseInput{Type: inputMouse, Flags: b.flag, Extra: ourMagic})
			*b.held = false
		}
	}
	if len(mice) > 0 {
		procSendInput.Call(uintptr(len(mice)), uintptr(unsafe.Pointer(&mice[0])), unsafe.Sizeof(mice[0]))
	}
}

func clearHeld() {
	heldKeys = [256]bool{}
	heldExtended = [256]bool{}
	heldLeft, heldRight, heldMiddle = false, false, false
}
