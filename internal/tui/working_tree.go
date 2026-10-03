package tui

import (
	"fmt"
	"log"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
)

// workingTreeMarker is the graph glyph for uncommitted changes: hollow, since
// the change isn't committed yet, against the filled dot of a real commit.
const workingTreeMarker = "○"

// addWorkingTreeRow puts a row for uncommitted changes above the newest
// commit, so "what have I got in progress" is answered by the same screen that
// answers "where am I". Nothing is added when the working tree is clean.
//
// It runs after the graph has been read, and shifts the rows' commit indexes
// along, rather than being woven into the layout: git log has nothing to say
// about uncommitted work, and the layout is of commits and their parents,
// which a made-up row has none of.
//
// Not while the graph is filtered: the row counts every uncommitted change,
// and among a file's history it would read as changes to that file.
func (m *model) addWorkingTreeRow() {
	if !m.filter.IsZero() {
		return
	}
	summary, ok := m.workingTreeSummary()
	if !ok {
		return
	}

	m.commits = append([]commit{{
		Message:     summary,
		WorkingTree: true,
		Date:        time.Now(),
	}}, m.commits...)
	for i := range m.displayRows {
		if m.displayRows[i].CommitIdx >= 0 {
			m.displayRows[i].CommitIdx++
		}
	}
	m.displayRows = append([]displayRow{m.workingTreeRow()}, m.displayRows...)
}

// workingTreeRow draws the marker in the column the topmost commit sits in, so
// the row reads as sitting on top of the graph rather than floating beside it.
func (m *model) workingTreeRow() displayRow {
	row := displayRow{GraphChars: workingTreeMarker, CommitIdx: 0, GraphWidth: 1, Lanes: []int{0}}
	if len(m.displayRows) == 0 {
		return row
	}

	first := m.displayRows[0]
	runes := []rune(first.GraphChars)
	marker := -1
	for i, r := range runes {
		if git.IsCommitMarker(r) {
			marker = i
			break
		}
	}
	if marker < 0 {
		return row
	}
	// Nothing is listed above the topmost commit, so no lane runs past this
	// row: whatever else is on the commit's row is a connection to commits
	// below it.
	row.GraphChars = strings.Repeat(" ", marker) + workingTreeMarker
	row.GraphWidth = marker + 1
	row.Lanes = make([]int, marker+1)
	row.Lanes[marker] = first.Lanes[marker]
	return row
}

// workingTreeSummary counts what is uncommitted, as "3 changed, 1 untracked".
// Staged and unstaged changes are one number: both are work in progress, and
// the split is a detail for the details panel, not the graph.
func (m *model) workingTreeSummary() (string, bool) {
	changed, untracked, err := git.Status(m.repoPath)
	if err != nil {
		log.Printf("Working tree: status failed: %v", err)
		return "", false
	}
	if changed == 0 && untracked == 0 {
		return "", false
	}

	var parts []string
	if changed > 0 {
		parts = append(parts, fmt.Sprintf("%d changed", changed))
	}
	if untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", untracked))
	}
	return strings.Join(parts, ", "), true
}

// loadWorkingDiffCmd reads the uncommitted changes for the details panel. The
// working-tree row is always the first, so the answer is for commit 0.
func loadWorkingDiffCmd(repoPath string, statWidth int) tea.Cmd {
	return func() tea.Msg {
		d := git.WorkingTree(repoPath, statWidth)
		return diffLoadedMsg{repoPath: repoPath, commitIdx: 0, diffStat: d.Stat, diffBody: d.Body, diffFiles: d.Files}
	}
}
