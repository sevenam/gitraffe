package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/selfupdate"
)

// updateAvailable reports whether GitHub advertises a release newer than this build.
func (m *model) updateAvailable() bool {
	return selfupdate.IsNewer(m.latestVersion, Version)
}

// startUpdate arms the confirmation prompt, or explains why there is nothing to do.
func (m model) startUpdate() model {
	switch {
	case m.latestVersion == "":
		m.updateMessage = "Update check unavailable — could not reach GitHub"
	case !m.updateAvailable():
		m.updateMessage = "Already on the latest version (v" + Version + ")"
	default:
		m.updateState = updateConfirming
	}
	return m
}

func checkVersionCmd() tea.Cmd {
	return func() tea.Msg {
		latestVersion := selfupdate.LatestVersion()
		return versionCheckMsg{latestVersion: latestVersion}
	}
}

// performUpdateCmd downloads and installs the latest release. It re-fetches the
// release rather than reusing the startup check so a long-running session
// installs what is current now, not what was current at launch.
func performUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		release, err := selfupdate.LatestRelease()
		if err != nil {
			return updateFinishedMsg{err: err}
		}
		// The prompt was based on the startup check; if the latest release has
		// since become older (e.g. a tag being re-cut), installing would downgrade.
		if !selfupdate.IsNewer(release.TagName, Version) {
			return updateFinishedMsg{err: fmt.Errorf("latest release %s is not newer than v%s", release.TagName, Version)}
		}
		if err := selfupdate.Install(release); err != nil {
			return updateFinishedMsg{err: err}
		}
		return updateFinishedMsg{version: release.TagName}
	}
}
