//go:build windows

// Raw Win32 bindings used by PhoneLock (standard library only, no cgo).
package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procSetWindowsHookExW     = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx        = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx   = user32.NewProc("UnhookWindowsHookEx")
	procGetMessageW           = user32.NewProc("GetMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessageW      = user32.NewProc("DispatchMessageW")
	procPostMessageW          = user32.NewProc("PostMessageW")
	procPostQuitMessage       = user32.NewProc("PostQuitMessage")
	procRegisterClassExW      = user32.NewProc("RegisterClassExW")
	procCreateWindowExW       = user32.NewProc("CreateWindowExW")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procCreatePopupMenu       = user32.NewProc("CreatePopupMenu")
	procAppendMenuW           = user32.NewProc("AppendMenuW")
	procTrackPopupMenu        = user32.NewProc("TrackPopupMenu")
	procDestroyMenu           = user32.NewProc("DestroyMenu")
	procSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	procGetCursorPos          = user32.NewProc("GetCursorPos")
	procSendInput             = user32.NewProc("SendInput")
	procCreateIconFromResEx   = user32.NewProc("CreateIconFromResourceEx")
	procMessageBoxW           = user32.NewProc("MessageBoxW")
	procRegisterWindowMessage = user32.NewProc("RegisterWindowMessageW")
	procSetTimer              = user32.NewProc("SetTimer")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	procCreateMutexW     = kernel32.NewProc("CreateMutexW")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procShellExecuteW    = shell32.NewProc("ShellExecuteW")
	procIsUserAnAdmin    = shell32.NewProc("IsUserAnAdmin")
)

const (
	whKeyboardLL = 13
	whMouseLL    = 14

	wmNull         = 0x0000
	wmDestroy      = 0x0002
	wmTimer        = 0x0113
	wmCommand      = 0x0111
	wmKeyDown      = 0x0100
	wmSysKeyDown   = 0x0104
	wmLButtonDown  = 0x0201
	wmLButtonUp    = 0x0202
	wmRButtonDown  = 0x0204
	wmRButtonUp    = 0x0205
	wmMButtonDown  = 0x0207
	wmMButtonUp    = 0x0208
	wmApp          = 0x8000
	wmAppTray      = wmApp + 1
	wmAppState     = wmApp + 2
	llkhfExtended  = 0x01
	keyeventfExt   = 0x0001
	keyeventfKeyUp = 0x0002
	inputMouse     = 0
	inputKeyboard  = 1

	mouseeventfLeftUp   = 0x0004
	mouseeventfRightUp  = 0x0010
	mouseeventfMiddleUp = 0x0040

	vkLShift = 0xA0
	vkRShift = 0xA1
	vkLMenu  = 0xA4 // left Alt
	vkRMenu  = 0xA5 // right Alt
	vkLWin   = 0x5B
	vkRWin   = 0x5C
	vkMask   = 0xE8 // unassigned key, used so Win/Alt release doesn't open Start or menus

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2
	nifMsg    = 0x1
	nifIcon   = 0x2
	nifTip    = 0x4

	mfString    = 0x0
	mfGrayed    = 0x1
	mfSeparator = 0x800

	tpmRightButton = 0x0002
	tpmBottomAlign = 0x0020
	tpmReturnCmd   = 0x0100

	mbOK          = 0x00
	mbIconError   = 0x10
	mbIconInfo    = 0x40
	mbSetForegrnd = 0x10000
)

// Marks input events PhoneLock injects itself, so its own hooks let them through.
const ourMagic uintptr = 0x504C4F43

type point struct{ X, Y int32 }

type kbdllHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type msllHookStruct struct {
	Pt          point
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	private uint32
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

// INPUT is 40 bytes on 64-bit Windows; these two layouts fill the union.
type keyInput struct {
	Type  uint32
	_     uint32
	Vk    uint16
	Scan  uint16
	Flags uint32
	Time  uint32
	_     uint32
	Extra uintptr
	_     uint64
}

type mouseInput struct {
	Type      uint32
	_         uint32
	Dx        int32
	Dy        int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	_         uint32
	Extra     uintptr
}

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func copyU16(dst []uint16, s string) {
	src, _ := syscall.UTF16FromString(s)
	if len(src) > len(dst) {
		src = src[:len(dst)-1]
		src = append(src, 0)
	}
	copy(dst, src)
}

func messageBox(text string, icon uintptr) {
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16(text))), uintptr(unsafe.Pointer(u16("PhoneLock"))), mbOK|icon|mbSetForegrnd)
}

func postMessage(hwnd, m, w, l uintptr) { procPostMessageW.Call(hwnd, m, w, l) }
