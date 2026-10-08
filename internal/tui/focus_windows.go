//go:build windows

package tui

import "golang.org/x/sys/windows"

var (
	getConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	getAncestor      = windows.NewLazySystemDLL("user32.dll").NewProc("GetAncestor")
)

const gaRootOwner = 3

var focusProbe = consoleInFront

// consoleInFront reports whether the window gitraffe is drawn in is the one in
// front. In the classic console that is the console window itself. Under
// Windows Terminal the console window is a hidden stand-in owned by the
// terminal's window, so it is the stand-in's root owner that is compared. A
// terminal that leaves the stand-in unowned gives a window that is never
// visible, which is reported as unknown rather than as never in front.
//
// Tabs share a window, so switching back to gitraffe's tab is not seen; the
// timer still is.
func consoleInFront() (in, known bool) {
	console, _, _ := getConsoleWindow.Call()
	if console == 0 {
		return false, false
	}
	ours := rootOwner(windows.HWND(console))
	if !windows.IsWindowVisible(ours) {
		return false, false
	}
	return rootOwner(windows.GetForegroundWindow()) == ours, true
}

func rootOwner(hwnd windows.HWND) windows.HWND {
	if hwnd == 0 {
		return 0
	}
	if root, _, _ := getAncestor.Call(uintptr(hwnd), gaRootOwner); root != 0 {
		return windows.HWND(root)
	}
	return hwnd
}
