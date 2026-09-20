package main

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-git/go-git/v5"
)

// Message types for the Bubble Tea event system

type repoMsg struct {
	repo *git.Repository
}

type errMsg struct {
	err error
}

func (e errMsg) Error() string {
	return e.err.Error()
}

type versionCheckMsg struct {
	latestVersion string
}

type remoteTagsMsg struct {
	tags     map[tagRef]bool // nil when unknown: no remotes, or one didn't answer
	repoPath string          // the repository it answers for; see the handler
}

type updateFinishedMsg struct {
	version string
	err     error
}

type diffLoadedMsg struct {
	repoPath  string // the repository it was loaded for; see the handler
	commitIdx int
	diffStat  string
	diffBody  string
	// diffFiles is the same diff split per file. It is parsed before diffBody
	// is cut to length, so the commit view can list files the panel's text no
	// longer reaches.
	diffFiles []fileDiff
}

// Command generators

func checkVersionCmd() tea.Cmd {
	return func() tea.Msg {
		latestVersion := fetchLatestVersion()
		return versionCheckMsg{latestVersion: latestVersion}
	}
}

// performUpdateCmd downloads and installs the latest release. It re-fetches the
// release rather than reusing the startup check so a long-running session
// installs what is current now, not what was current at launch.
func performUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		release, err := fetchLatestRelease()
		if err != nil {
			return updateFinishedMsg{err: err}
		}
		// The prompt was based on the startup check; if the latest release has
		// since become older (e.g. a tag being re-cut), installing would downgrade.
		if !isNewerVersion(release.TagName, version) {
			return updateFinishedMsg{err: fmt.Errorf("latest release %s is not newer than v%s", release.TagName, version)}
		}
		if err := downloadAndUpdate(release); err != nil {
			return updateFinishedMsg{err: err}
		}
		return updateFinishedMsg{version: release.TagName}
	}
}

func loadDiffCmd(repoPath string, fullHash string, idx int, statWidth int) tea.Cmd {
	return func() tea.Msg {
		var stat, body string
		var files []fileDiff

		cmd := exec.Command("git", "show", "--format=", fmt.Sprintf("--stat=%d", statWidth), "--no-color", fullHash)
		cmd.Dir = repoPath
		if out, err := cmd.Output(); err == nil {
			stat = strings.TrimSpace(strings.ReplaceAll(string(out), "\r", ""))
		}

		cmd = exec.Command("git", "show", "--format=", "--no-color", "-p", fullHash)
		cmd.Dir = repoPath
		if out, err := cmd.Output(); err == nil {
			diff := strings.ReplaceAll(string(out), "\r", "")
			// Split before the cut below: the details panel shows as much as
			// it can hold, but the commit view has a list to fill, and a file
			// it never named could not be opened.
			files = parseFileDiffs(diff)
			diffLines := strings.Split(diff, "\n")
			if len(diffLines) > 300 {
				diffLines = diffLines[:300]
				diffLines = append(diffLines, "... (truncated)")
			}
			body = strings.TrimSpace(strings.Join(diffLines, "\n"))
		}

		return diffLoadedMsg{repoPath: repoPath, commitIdx: idx, diffStat: stat, diffBody: body, diffFiles: files}
	}
}
