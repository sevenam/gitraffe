package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// WingetID is the name winget knows gitraffe by.
const WingetID = "sevenam.gitraffe"

// WingetUpgrade is the command that updates a copy installed with winget.
const WingetUpgrade = "winget upgrade " + WingetID

// ManagedByWinget reports whether the running binary was installed by winget.
// Such a copy must not replace itself: winget records the version and a hash
// of the file it installed, and refuses to upgrade or uninstall one that has
// changed underneath it.
//
// A variable so a test can stand in for a machine gitraffe was installed on
// differently.
var ManagedByWinget = func() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if isWingetPath(exe) {
		return true
	}
	// Started through the link, the path may be the link's or the file's
	// depending on how Windows resolved it, so both are looked at.
	resolved, err := filepath.EvalSymlinks(exe)
	return err == nil && isWingetPath(resolved)
}

// isWingetPath reports whether path is inside one of winget's own folders:
// Packages, where it keeps what it installed, or Links, where the command on
// the PATH is. Both exist per user (under AppData\Local\Microsoft) and per
// machine (under Program Files), so only the last two folders are matched.
func isWingetPath(path string) bool {
	p := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	return strings.Contains(p, `\winget\packages\`) || strings.Contains(p, `\winget\links\`)
}
