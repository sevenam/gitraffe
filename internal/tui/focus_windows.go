//go:build windows

package tui

import (
	"log"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	getConsoleWindow = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
	getWindow        = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindow")
	getAncestor      = windows.NewLazySystemDLL("user32.dll").NewProc("GetAncestor")
)

const (
	gwOwner = 4
	gaRoot  = 2
)

var focusProbe = consoleInFront

// ourWindow is what consoleInFront compares the window in front with, worked
// out once: the windows and processes involved stay put while gitraffe runs.
var (
	ourWindowOnce sync.Once
	ourWindow     windows.HWND // the console's own window, when it is a real one
	terminalPID   uint32       // otherwise the process drawing the terminal
)

// consoleInFront reports whether the window gitraffe is drawn in is the one in
// front.
//
// In the classic console that is the console window, which GetConsoleWindow
// gives. Elsewhere that window is a hidden stand-in: Windows Terminal, VS
// Code, mintty and the like draw the terminal in a window of their own, in a
// process gitraffe was started from. So when the console window cannot be
// seen, the window in front counts as ours when it belongs to the nearest of
// gitraffe's ancestors that has a visible window. Not any ancestor: Windows
// Terminal was itself started by Explorer, and the desktop counting as ours
// would make coming back from it no change at all.
//
// Tabs share a window, so switching back to gitraffe's tab is not seen; the
// timer still is.
func consoleInFront() (in, known bool) {
	ourWindowOnce.Do(findOurWindow)
	front := windows.GetForegroundWindow()
	if front == 0 {
		return false, false // between windows, or the lock screen
	}
	switch {
	case ourWindow != 0:
		return topOwner(front) == ourWindow, true
	case terminalPID != 0:
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(front, &pid); err != nil {
			return false, false
		}
		return pid == terminalPID, true
	}
	return false, false
}

// findOurWindow logs what it finds, since which of these applies depends on
// the terminal, and a focus that is never seen gives no other sign why.
func findOurWindow() {
	console, _, _ := getConsoleWindow.Call()
	if owner := topOwner(windows.HWND(console)); owner != 0 && windows.IsWindowVisible(owner) {
		ourWindow = owner
		log.Printf("Focus: watching console window %#x", owner)
		return
	}
	withWindows := visibleWindowOwners()
	for _, p := range ancestors() {
		if withWindows[p.pid] {
			terminalPID = p.pid
			log.Printf("Focus: watching the windows of %s (pid %d)", p.name, p.pid)
			return
		}
	}
	log.Printf("Focus: no window found to watch (console window %#x)", console)
}

// topOwner climbs from a window to the top-level window that owns it, in
// turn, through parents (GetAncestor(GA_ROOT)) and owners
// (GetWindow(GW_OWNER)). Both are needed: the stand-in may be owned by a child
// window of the terminal's rather than the window itself, and
// GetAncestor(GA_ROOTOWNER) would not do, since it walks GetParent, which sees
// an owner only for WS_POPUP windows.
func topOwner(hwnd windows.HWND) windows.HWND {
	for range 16 { // neither chain can loop, but a bound costs nothing
		if hwnd == 0 {
			return 0
		}
		if root, _, _ := getAncestor.Call(uintptr(hwnd), gaRoot); root != 0 {
			hwnd = windows.HWND(root)
		}
		owner, _, _ := getWindow.Call(uintptr(hwnd), gwOwner)
		if owner == 0 {
			break
		}
		hwnd = windows.HWND(owner)
	}
	return hwnd
}

type process struct {
	pid  uint32
	name string
}

// ancestors lists the processes gitraffe was started from, nearest first.
func ancestors() []process {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	parent := map[uint32]uint32{}
	name := map[uint32]string{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		parent[entry.ProcessID] = entry.ParentProcessID
		name[entry.ProcessID] = windows.UTF16ToString(entry.ExeFile[:])
	}
	var chain []process
	seen := map[uint32]bool{}
	// A parent that has exited can have its pid reused by a process started
	// later, even one of ours; the seen set stops the walk going round.
	for pid := parent[uint32(os.Getpid())]; pid != 0 && !seen[pid]; pid = parent[pid] {
		if _, ok := name[pid]; !ok {
			break
		}
		seen[pid] = true
		chain = append(chain, process{pid, name[pid]})
	}
	return chain
}

// visibleWindowOwners gives the processes that have a visible top-level window.
func visibleWindowOwners() map[uint32]bool {
	owners := map[uint32]bool{}
	callback := windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		if windows.IsWindowVisible(hwnd) {
			var pid uint32
			if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err == nil {
				owners[pid] = true
			}
		}
		return 1 // carry on
	})
	_ = windows.EnumWindows(callback, nil)
	return owners
}
