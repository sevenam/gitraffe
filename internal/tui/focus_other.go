//go:build !windows

package tui

// Elsewhere the terminal reports focus itself; see focus_watch.go.
var focusProbe func() (in, known bool)
