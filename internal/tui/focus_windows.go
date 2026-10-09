//go:build windows

package tui

import (
	"log"
	"sync"

	"golang.org/x/sys/windows"
)

var (
	getConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	getWindow        = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindow")
)

const gwOwner = 4

var focusProbe = consoleInFront

// logProbeOnce writes what the first probe found to the log, since whether
// focus is seen depends on how the terminal hosts the console, which is
// otherwise invisible from outside.
var logProbeOnce sync.Once

// consoleInFront reports whether the window gitraffe is drawn in is the one in
// front. In the classic console that is the console window itself. Under
// Windows Terminal the console window is a hidden stand-in owned by the
// terminal's window, so it is the stand-in's top owner that is compared. A
// terminal that leaves the stand-in unowned gives a window that is never
// visible, which is reported as unknown rather than as never in front.
//
// Tabs share a window, so switching back to gitraffe's tab is not seen; the
// timer still is.
func consoleInFront() (in, known bool) {
	console, _, _ := getConsoleWindow.Call()
	ours := topOwner(windows.HWND(console))
	visible := ours != 0 && windows.IsWindowVisible(ours)
	logProbeOnce.Do(func() {
		log.Printf("Focus: console window %#x, its top owner %#x, visible %v", console, ours, visible)
	})
	if !visible {
		return false, false
	}
	return topOwner(windows.GetForegroundWindow()) == ours, true
}

// topOwner follows a window's owners to the last one. GetWindow(GW_OWNER) is
// asked rather than GetAncestor(GA_ROOTOWNER): the latter walks GetParent,
// which reports an owner only for WS_POPUP windows, and the stand-in is not
// one, so it would come back as itself.
func topOwner(hwnd windows.HWND) windows.HWND {
	for range 16 { // owners cannot loop, but a bound costs nothing
		owner, _, _ := getWindow.Call(uintptr(hwnd), gwOwner)
		if owner == 0 {
			return hwnd
		}
		hwnd = windows.HWND(owner)
	}
	return hwnd
}
