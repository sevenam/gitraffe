package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// reloadRepo reads the open repository again, picking up commits made since it
// was opened. It goes through switchRepo so that everything derived from the
// repository is rebuilt rather than merged with what is on screen: the graph,
// the ahead/behind counts and the tag marks all change together.
//
// Unlike a switch it keeps your place: the same commit stays selected if it is
// still there, and the same panel keeps focus.
func (m model) reloadRepo() (model, tea.Cmd) {
	next, cmd := m.switchRepo(m.repoPath)
	next.repoRoot = m.repoRoot
	next.focusedBox = m.focusedBox
	// However much history was asked for stays asked for; a reload that threw
	// away the batches loaded with "m" would be a reload that loses your place.
	next.commitLimit = m.commitCount()
	if m.selected >= 0 && m.selected < len(m.commits) {
		next.reselect = m.commits[m.selected].FullHash
	}
	// An open commit view is a place too: "r" and a finished fetch mean "show
	// me this again", not "put me back on the graph". A switch drops it
	// instead, since the commit being read is not in the repository being
	// opened. See followSelectionInCommitView for how it finds its commit
	// again once the new graph has loaded.
	next.commitView = m.commitView
	next.notice = "Reloaded " + displayPath(m.repoPath)
	return next, cmd
}

// applyReselect puts the selection back on the commit that was selected before
// a reload. A commit can go missing — an amend or a rebase replaces it — and
// the newest commit is then the best place to be.
func (m *model) applyReselect() {
	if m.reselect == "" {
		return
	}
	for i, c := range m.commits {
		if c.FullHash == m.reselect {
			m.selected = i
			break
		}
	}
	m.reselect = ""
}
