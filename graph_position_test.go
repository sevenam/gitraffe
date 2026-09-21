package main

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
