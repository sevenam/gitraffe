// Package tui is gitraffe's terminal interface: the model, its Update and
// View, and every screen and overlay drawn from them.
package tui

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
)

const appName = "Gitraffe"

// Version and LogPath describe this build and where it logs. main sets them
// before Run: the version constant has to stay in main.go, where the release
// workflow reads it, and the log file is opened before anything else starts.
var (
	Version = "0.0.0"
	LogPath string
)

// Run opens repoPath in the interface and blocks until the user quits. It
// returns the release tag installed during the session, or "" when there was
// none, so main can say so once Bubble Tea has restored the normal screen.
//
// The theme must already be loaded: a bad -theme has to be reported before the
// alt screen takes over.
func Run(repoPath, configDir string) (updatedTo string, err error) {
	initStyles()

	// Set terminal title (works on Windows 10+, macOS, Linux)
	setTerminalTitle(appName)
	defer resetTerminalTitle()

	m := initialModel(repoPath)
	m.configDir = configDir
	m = applyPreferences(m)

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	if final, ok := finalModel.(model); ok {
		if err := savePreferences(final); err != nil {
			log.Printf("Preferences: could not save: %v", err)
		}
		return final.updatedTo, nil
	}
	return "", nil
}
