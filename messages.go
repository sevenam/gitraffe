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

type diffLoadedMsg struct {
	commitIdx int
	diffStat  string
	diffBody  string
}

// Command generators

func checkVersionCmd() tea.Cmd {
	return func() tea.Msg {
		latestVersion := fetchLatestVersion()
		return versionCheckMsg{latestVersion: latestVersion}
	}
}

func loadDiffCmd(repoPath string, fullHash string, idx int, statWidth int) tea.Cmd {
	return func() tea.Msg {
		var stat, body string

		cmd := exec.Command("git", "show", "--format=", fmt.Sprintf("--stat=%d", statWidth), "--no-color", fullHash)
		cmd.Dir = repoPath
		if out, err := cmd.Output(); err == nil {
			stat = strings.TrimSpace(strings.ReplaceAll(string(out), "\r", ""))
		}

		cmd = exec.Command("git", "show", "--format=", "--no-color", "-p", fullHash)
		cmd.Dir = repoPath
		if out, err := cmd.Output(); err == nil {
			diff := strings.ReplaceAll(string(out), "\r", "")
			diffLines := strings.Split(diff, "\n")
			if len(diffLines) > 300 {
				diffLines = diffLines[:300]
				diffLines = append(diffLines, "... (truncated)")
			}
			body = strings.TrimSpace(strings.Join(diffLines, "\n"))
		}

		return diffLoadedMsg{commitIdx: idx, diffStat: stat, diffBody: body}
	}
}
