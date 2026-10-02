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
//
// It also keeps the screen. The model being replaced stays on show until the
// new one has loaded (see stale), and what the new one would otherwise have to
// fetch or work out again is carried across, so that a reload which found
// nothing new draws exactly what was there before: no loading page, no graph
// scrolling back into place, no diff or tag marks blinking out and in.
func (m model) reloadRepo() (model, tea.Cmd) {
	next, cmd := m.switchRepo(m.repoPath)
	next.repoRoot = m.repoRoot
	next.focusedBox = m.focusedBox
	next.maximised = m.maximised
	next.graphTop = m.graphTop
	next.detailsScroll = m.detailsScroll
	// The remotes are asked again, but that takes seconds; until they answer,
	// the last answer is a better guess than no marks at all.
	next.remoteTags = m.remoteTags
	next.autoFetchStopped = m.autoFetchStopped
	prev := m
	prev.stale = nil
	next.stale = &prev
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
	return next, cmd
}

// refresh is the reload the user asked for with "r". The bottom line is the
// only thing that changes while it runs, and it changes twice so that asking
// again when it already says "Refreshed" still visibly does something.
func (m model) refresh() (model, tea.Cmd) {
	next, cmd := m.reloadRepo()
	next.notice = "Refreshing…"
	next.loadedNotice = "Refreshed"
	return next, cmd
}

// applyReselect puts the selection back on the commit that was selected before
// a reload. A commit can go missing — an amend or a rebase replaces it — and
// the newest commit is then the best place to be.
func (m *model) applyReselect() {
	if m.reselect == "" {
		return
	}
	found := false
	for i, c := range m.commits {
		if c.FullHash == m.reselect {
			m.selected = i
			found = true
			break
		}
	}
	if !found {
		// The scroll offset was the old commit's; it means nothing here.
		m.detailsScroll = 0
	}
	m.reselect = ""
}

// keepLoadedDiffs carries the diffs already read into the reloaded model, so
// the details panel doesn't fall back to "Loading diff..." for a commit it was
// showing a moment ago. A commit never changes, so its diff is still its diff.
//
// The working tree is the exception: its changes are what a reload is most
// often for. Its old diff is kept on screen only while it is the selected row,
// and the returned command reads it again to replace it.
func (m *model) keepLoadedDiffs(prev *model) tea.Cmd {
	if prev == nil {
		return nil
	}
	loaded := make(map[string]int)
	for i, c := range prev.commits {
		if c.DiffLoaded && !c.WorkingTree {
			loaded[c.FullHash] = i
		}
	}
	keep := func(c *commit, from commit) {
		c.DiffLoaded = true
		c.DiffStat, c.DiffBody, c.DiffFiles = from.DiffStat, from.DiffBody, from.DiffFiles
	}
	for i := range m.commits {
		if m.commits[i].WorkingTree {
			continue
		}
		if j, ok := loaded[m.commits[i].FullHash]; ok {
			keep(&m.commits[i], prev.commits[j])
		}
	}

	if m.selected < 0 || m.selected >= len(m.commits) || !m.commits[m.selected].WorkingTree {
		return nil
	}
	if len(prev.commits) == 0 || !prev.commits[0].WorkingTree || !prev.commits[0].DiffLoaded {
		return nil
	}
	keep(&m.commits[m.selected], prev.commits[0])
	return loadWorkingDiffCmd(m.repoPath, m.diffStatWidth())
}
