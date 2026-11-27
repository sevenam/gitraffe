package tui

import (
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
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

// moreHistoryNote is the row at the bottom of a cut-short history. The graph
// simply stopping looks the same as a repository that ends there; the row
// says the list was cut, and reaching it reads on.
//
// Kept short: the graph panel is only as wide as the graph needs, so a long
// note would be truncated to nothing useful.
const (
	moreHistoryNote    = "… more history"
	readingHistoryNote = "… reading more history"
)

// addMoreCommitsRow marks the bottom of a cut-short history.
func (m *model) addMoreCommitsRow() {
	if !m.moreCommits {
		return
	}
	m.displayRows = append(m.displayRows, displayRow{
		CommitIdx: -1,
		Note:      moreHistoryNote,
	})
}

// loadMoreCommits reads the next batch. The whole graph is read again rather
// than the new commits appended: the lanes are laid out for the commits
// given, so a second batch laid out on its own would not join up with the
// first.
//
// It is asked for once per batch: the reload makes the model not ready until
// the batch has arrived, and nothing asks for more of a model that isn't.
func (m model) loadMoreCommits() (model, tea.Cmd) {
	if !m.ready || !m.moreCommits {
		return m, nil
	}
	// The screen stays up while the batch is read (see reloadRepo), so the
	// note on it is what says something is happening.
	if n := len(m.displayRows); n > 0 && m.displayRows[n-1].Note == moreHistoryNote {
		rows := append([]displayRow(nil), m.displayRows...)
		rows[n-1].Note = readingHistoryNote
		m.displayRows = rows
	}
	m.commitLimit = m.commitCount() + commitBatch
	return m.reloadRepo()
}

// readOnAtTheEnd reads the next batch when a key or the mouse has taken the
// selection to the bottom of a cut-short history. That replaced a key of its
// own ("m", now merge): the end of what is loaded is never where anyone
// meant to stop, so asking them to say so was a step for nothing.
//
// Only input counts. A reload that finds its commit again on the last row,
// or a jump to a branch that reads exactly as deep as that branch, has not
// asked to see what is below.
func (m model) readOnAtTheEnd(before model, msg tea.Msg) (model, tea.Cmd) {
	if !m.ready || !m.moreCommits || m.err != nil || m.repoPath != before.repoPath ||
		len(m.commits) == 0 || m.selected != len(m.commits)-1 {
		return m, nil
	}
	if m.selected != before.selected || m.pushesPastTheEnd(msg) {
		return m.loadMoreCommits()
	}
	return m, nil
}

// pushesPastTheEnd is a key or wheel turn that would move down the graph,
// which on the last row has nowhere to go but the history not read yet.
// Without it, a selection left on the last row by a jump would have no way
// on but to step off it and back.
func (m model) pushesPastTheEnd(msg tea.Msg) bool {
	if m.commitView.open || m.focusedBox != 1 || !m.canReloadUnasked() {
		return false
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down", "ctrl+d", "pgdown", "G", "ctrl+end", "end":
			return true
		}
	case tea.MouseMsg:
		return msg.Button == tea.MouseButtonWheelDown && m.panelAt(msg.X, msg.Y) == 1
	}
	return false
}

// thousands groups a number for reading: 10000 is a glance away from 100000.
func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// historyCount is the length of the whole history, counted in the background
// while only part of it is loaded, so the bottom line can say how far through
// it the selection is. key says which history it counts: the repository as
// fingerprinted and the filter, since a count of either from before is a
// count of something else.
type historyCount struct {
	key   string
	total int // 0 when not counted, or the count failed
}

// historyCountMsg is a count finished in the background.
type historyCountMsg struct {
	repoPath string
	key      string
	total    int
	err      error
}

// historyKey names the history on screen for historyCount; "" when the
// repository's state is unknown, which is no state to keep a count for.
func (m *model) historyKey() string {
	if m.fingerprint == "" {
		return ""
	}
	return m.fingerprint + "\x00" + m.filter.Path
}

// countHistory asks for the history to be counted once a cut-short history
// has loaded. A count is kept across reloads and asked for again only when
// the repository or the filter has changed: reading every commit of a long
// history is the slow part, and a reload that changed nothing has nothing
// new to count.
func (m *model) countHistory() tea.Cmd {
	key := m.historyKey()
	if !m.moreCommits || key == "" || key == m.historyCount.key || key == m.historyCountAsked {
		return nil
	}
	m.historyCountAsked = key
	repoPath, filter := m.repoPath, m.filter
	return func() tea.Msg {
		total, err := git.CountCommits(repoPath, filter)
		return historyCountMsg{repoPath: repoPath, key: key, total: total, err: err}
	}
}

// finishHistoryCount keeps a count that is still about the history on screen.
// A late one, from another repository or from before a change, is dropped;
// so is a failed one, which leaves the bottom line saying "5,000+" as it did
// before anything was counted.
func (m model) finishHistoryCount(msg historyCountMsg) model {
	if msg.key == m.historyCountAsked {
		m.historyCountAsked = ""
	}
	if msg.repoPath != m.repoPath || msg.key != m.historyKey() {
		return m
	}
	if msg.err != nil {
		log.Printf("Counting the history failed: %v", msg.err)
		return m
	}
	m.historyCount = historyCount{key: msg.key, total: msg.total}
	return m
}

// countedTotal is the whole history's length when it has been counted for the
// history on screen, or 0. A count smaller than what is already loaded is a
// count of a history that has since grown, and no total at all.
func (m *model) countedTotal(loaded int) int {
	if m.historyCount.key == "" || m.historyCount.key != m.historyKey() || m.historyCount.total < loaded {
		return 0
	}
	return m.historyCount.total
}
