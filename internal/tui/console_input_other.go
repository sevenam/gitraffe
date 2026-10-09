//go:build !windows

package tui

import tea "github.com/charmbracelet/bubbletea"

// Elsewhere Bubble Tea's own reader passes focus on; see
// console_input_windows.go.
type consoleInput struct{}

func openConsoleInput() *consoleInput   { return nil }
func (*consoleInput) run(func(tea.Msg)) {}
func (*consoleInput) close()            {}
