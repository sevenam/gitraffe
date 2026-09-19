package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// commitBatch is how many commits are read at a time. Enough that most
// repositories arrive whole, and few enough that a very old one doesn't spend
// seconds drawing history nobody asked to see.
const commitBatch = 5000

// commitCount is how many commits to read, for a model that predates the
// field or was built by a test.
func (m *model) commitCount() int {
	if m.commitLimit <= 0 {
		return commitBatch
	}
	return m.commitLimit
}

// addMoreCommitsRow marks the bottom of a cut-short history. The graph simply
// stopping looks the same as a repository that ends there, which is the whole
// complaint: the row says the list was cut and which key continues it.
func (m *model) addMoreCommitsRow() {
	if !m.moreCommits {
		return
	}
	// Kept short: the graph panel is only as wide as the graph needs, so a
	// long note would be truncated to nothing useful.
	m.displayRows = append(m.displayRows, displayRow{
		CommitIdx: -1,
		Note:      "… more history — press m",
	})
}

// loadMoreCommits reads the next batch. The whole graph is read again rather
// than the new commits appended: git draws the lanes for the commits it is
// given, so a second batch drawn on its own would not join up with the first.
func (m model) loadMoreCommits() (model, tea.Cmd) {
	if !m.moreCommits {
		return m, nil
	}
	m.commitLimit = m.commitCount() + commitBatch
	next, cmd := m.reloadRepo()
	next.notice = "Reading up to " + thousands(m.commitLimit) + " commits..."
	return next, cmd
}

// thousands groups a number for reading: 10000 is a glance away from 100000.
func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
