package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func positionModel(n int) model {
	m := testModel()
	m.commits = make([]commit, n)
	for i := range m.commits {
		m.commits[i].DiffLoaded = true
	}
	return m
}

func TestGraphPosition(t *testing.T) {
	for _, tc := range []struct {
		name     string
		commits  int
		selected int
		working  bool
		more     bool
		want     string
	}{
		{"newest commit", 200, 0, false, false, "1/200 · 0%"},
		{"a quarter in", 200, 49, false, false, "50/200 · 25%"},
		{"oldest commit", 200, 199, false, false, "200/200 · 100%"},
		{"grouped thousands", 5000, 1233, false, false, "1,234/5,000 · 24%"},
		// The uncommitted row is not a commit, so it must not shift the count.
		{"uncommitted row", 201, 0, true, false, "0/200 · 0%"},
		{"newest under uncommitted row", 201, 1, true, false, "1/200 · 0%"},
		{"oldest under uncommitted row", 201, 200, true, false, "200/200 · 100%"},
		// Cut short: no percentage, since the bottom of what's loaded isn't the end.
		{"history cut short", 5000, 4999, false, true, "5,000/5,000+"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := positionModel(tc.commits)
			m.commits[0].WorkingTree = tc.working
			m.selected = tc.selected
			m.moreCommits = tc.more
			if got := m.graphPosition(); got != tc.want {
				t.Errorf("position = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGraphPositionHiddenWithoutHistory(t *testing.T) {
	m := testModel()
	if got := m.graphPosition(); got != "" {
		t.Errorf("position with no commits = %q, want nothing", got)
	}
	m = positionModel(1)
	m.commits[0].WorkingTree = true
	if got := m.graphPosition(); got != "" {
		t.Errorf("position with only uncommitted changes = %q, want nothing", got)
	}
}

func TestStatusLinePinsPositionRight(t *testing.T) {
	for _, width := range []int{200, 120, 60, 30} {
		m := positionModel(5000)
		m.selected = 1233
		m.windowWidth = width
		line := ansi.Strip(m.renderStatusLine())
		if w := ansi.StringWidth(line); w != width {
			t.Errorf("%d columns: line is %d wide, want the full width", width, w)
		}
		if !strings.HasSuffix(line, "1,234/5,000 · 24%") {
			t.Errorf("%d columns: line = %q, want the position at the right edge", width, line)
		}
		if !strings.HasPrefix(line, "?: help") {
			t.Errorf("%d columns: line = %q, want it still to lead with help", width, line)
		}
	}
}

// fileListModel is a commit view open on a commit of n files.
func fileListModel(n int) model {
	m := testModel()
	m.commits = []commit{{DiffLoaded: true, DiffFiles: make([]fileDiff, n)}}
	m.commitView = commitView{open: true, focus: commitBoxFiles}
	return m
}

func TestFilePosition(t *testing.T) {
	for _, tc := range []struct {
		name        string
		files, file int
		want        string
	}{
		{"first file", 12, 0, "1/12 · 8%"},
		{"a quarter in", 12, 2, "3/12 · 25%"},
		{"last file", 12, 11, "12/12 · 100%"},
		{"only file", 1, 0, "1/1 · 100%"},
		// A selection left over from a longer list is drawn on the last file.
		{"selection past the end", 3, 9, "3/3 · 100%"},
		{"no files", 0, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := fileListModel(tc.files)
			m.commitView.file = tc.file
			if got := m.filePosition(); got != tc.want {
				t.Errorf("position = %q, want %q", got, tc.want)
			}
		})
	}
}

// Before the diff arrives there is no list to be anywhere in.
func TestFilePositionWaitsForTheDiff(t *testing.T) {
	m := fileListModel(3)
	m.commits[0].DiffLoaded = false
	if got := m.filePosition(); got != "" {
		t.Errorf("position before the diff loaded = %q, want nothing", got)
	}
}

// The position holds the bottom right corner on every form of the line that
// shows key hints, however narrow the window, and gives it up to a notice.
func TestCommitViewStatusLineEndsInTheFilePosition(t *testing.T) {
	for _, width := range []int{200, 60, 30} {
		for _, working := range []bool{false, true} {
			m := fileListModel(12)
			m.commits[0].WorkingTree = working
			m.commitView.workingTree = working
			m.commitView.file = 2
			m.windowWidth = width
			line := ansi.Strip(m.commitViewStatusLine())
			if !strings.HasSuffix(line, "3/12 · 25%") || ansi.StringWidth(line) != width {
				t.Errorf("width %d, working tree %v: line = %q", width, working, line)
			}
		}
	}

	m := fileListModel(12)
	m.notice = "Copied"
	if line := ansi.Strip(m.commitViewStatusLine()); strings.Contains(line, "/12") {
		t.Errorf("a notice shares the line with the position: %q", line)
	}
}
