package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
)

func (m *model) maybeLoadDiff() tea.Cmd {
	if m.selected >= 0 && m.selected < len(m.commits) && !m.commits[m.selected].DiffLoaded {
		statWidth := m.detailsContentWidth
		if statWidth <= 0 {
			statWidth = 80
		}
		if m.commits[m.selected].WorkingTree {
			return loadWorkingDiffCmd(m.repoPath, statWidth)
		}
		return loadDiffCmd(m.repoPath, m.commits[m.selected].FullHash, m.selected, statWidth)
	}
	return nil
}

func loadDiffCmd(repoPath string, fullHash string, idx int, statWidth int) tea.Cmd {
	return func() tea.Msg {
		d := git.ShowCommit(repoPath, fullHash, statWidth)
		return diffLoadedMsg{repoPath: repoPath, commitIdx: idx, diffStat: d.Stat, diffBody: d.Body, diffFiles: d.Files}
	}
}
