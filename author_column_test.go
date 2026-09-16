package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestComputePanelLayoutNeverOverflows(t *testing.T) {
	for window := 30; window <= 260; window++ {
		for _, graph := range []int{1, 6, 20, 60} {
			for _, branch := range []int{0, 12, 31} {
				for _, author := range []int{0, 7, 24, 40} {
					l := computePanelLayout(window, graph, branch, author)
					name := func() string {
						return fmt.Sprintf("window=%d graph=%d branch=%d author=%d", window, graph, branch, author)
					}

					if l.leftWidth+l.rightWidth > window {
						t.Fatalf("%s: panels %d+%d exceed the window", name(), l.leftWidth, l.rightWidth)
					}

					if l.authorCol != 0 && (l.authorCol < minAuthorColWidth || l.authorCol > maxAuthorColWidth) {
						t.Fatalf("%s: author column %d outside [%d,%d]", name(), l.authorCol, minAuthorColWidth, maxAuthorColWidth)
					}

					// Labels are served first: authors only get room once labels
					// have all they want.
					if l.authorCol > 0 && l.branchCol < branch {
						t.Fatalf("%s: authors got %d while labels were cut to %d of %d", name(), l.authorCol, l.branchCol, branch)
					}

					// When the graph fits beside the details panel, no row may be
					// wider than the list's content area, or lipgloss wraps it.
					if graph+14 <= window*4/5 && graph+14+minRightPanelWidth <= window {
						row := 2 + graph + 1 + 7
						if l.branchCol > 0 {
							row += l.branchCol + 1
						}
						if l.authorCol > 0 {
							row += l.authorCol + 1
						}
						if row > l.leftWidth-4 {
							t.Fatalf("%s: row of %d exceeds content width %d", name(), row, l.leftWidth-4)
						}
					}
				}
			}
		}
	}
}

func TestComputePanelLayoutPriorities(t *testing.T) {
	t.Run("wide window caps long names", func(t *testing.T) {
		l := computePanelLayout(300, 6, 10, 60)
		if l.authorCol != maxAuthorColWidth {
			t.Errorf("author column = %d, want the cap %d", l.authorCol, maxAuthorColWidth)
		}
		if l.branchCol != 10 {
			t.Errorf("branch column = %d, want all 10", l.branchCol)
		}
	})

	t.Run("short names are not padded to the cap", func(t *testing.T) {
		if l := computePanelLayout(300, 6, 10, 7); l.authorCol != 7 {
			t.Errorf("author column = %d, want 7", l.authorCol)
		}
	})

	t.Run("tight window drops authors before labels", func(t *testing.T) {
		// 100 wide: room beside a 6-wide graph is 100-20-30 = 50.
		l := computePanelLayout(100, 6, 45, 24)
		if l.branchCol != 45 {
			t.Errorf("branch column = %d, want all 45", l.branchCol)
		}
		if l.authorCol != 0 {
			t.Errorf("author column = %d, want dropped: only 4 columns remain", l.authorCol)
		}
	})

	t.Run("no authors, no column", func(t *testing.T) {
		if l := computePanelLayout(300, 6, 10, 0); l.authorCol != 0 {
			t.Errorf("author column = %d, want 0", l.authorCol)
		}
	})
}

func TestAuthorColumnRendering(t *testing.T) {
	m := testModel()
	m.windowWidth = 140
	m.windowHeight = 20
	m.commits = []commit{
		{Hash: "aaaaaaa", Author: "sevenam", Refs: "refs/heads/main"},
		{Hash: "bbbbbbb", Author: "A Very Long Author Name That Keeps Going"},
		{Hash: "ccccccc", Author: "山田太郎"}, // 4 runes, 8 columns wide
		{Hash: "ddddddd", Author: "Anders Østhus"},
	}
	m.displayRows = []displayRow{
		{GraphChars: "●", CommitIdx: 0, GraphWidth: 1},
		{GraphChars: "●", CommitIdx: 1, GraphWidth: 1},
		{GraphChars: "│", CommitIdx: -1, GraphWidth: 1},
		{GraphChars: "●", CommitIdx: 2, GraphWidth: 1},
		{GraphChars: "●", CommitIdx: 3, GraphWidth: 1},
	}
	m.maxGraphWidth = 1
	m.updateLabelWidth()
	m.maxAuthorWidth = 0
	for _, c := range m.commits {
		m.maxAuthorWidth = max(m.maxAuthorWidth, ansi.StringWidth(c.Author))
	}

	l := computePanelLayout(m.windowWidth, m.maxGraphWidth, m.maxBranchWidth, m.maxAuthorWidth)
	contentWidth := l.leftWidth - 4
	lines := strings.Split(m.renderCommitList(l.branchCol, l.authorCol, contentWidth), "\n")

	authorStart := contentWidth - l.authorCol
	for i, want := range map[int]string{
		0: "sevenam",
		1: "A Very Long Author Name…",
		3: "山田太郎",
		4: "Anders Østhus",
	} {
		line := stripANSI(lines[i])
		if w := ansi.StringWidth(line); w > contentWidth {
			t.Errorf("line %d is %d wide, over the content width %d: %q", i, w, contentWidth, line)
		}
		// Every name starts at the same display column, whatever its width.
		if got := strings.TrimRight(ansi.Cut(line, authorStart, contentWidth), " "); got != want {
			t.Errorf("line %d author column = %q, want %q (line %q)", i, got, want, line)
		}
	}

	// Graph-only connector rows carry no author.
	if got := strings.TrimRight(ansi.Cut(stripANSI(lines[2]), authorStart, contentWidth), " "); got != "" {
		t.Errorf("connector row shows %q in the author column", got)
	}
}

func TestViewWithAuthorsKeepsExactSize(t *testing.T) {
	for _, width := range []int{40, 60, 80, 100, 120, 160, 220} {
		m := testModel()
		m.windowWidth = width
		m.windowHeight = 16
		m.commits = []commit{
			{Hash: "aaaaaaa", Author: "John Børge Holen-Tjelta", Refs: "HEAD -> refs/heads/main, refs/remotes/origin/main"},
			{Hash: "bbbbbbb", Author: "山田太郎"},
		}
		m.displayRows = []displayRow{
			{GraphChars: "●", CommitIdx: 0, GraphWidth: 1},
			{GraphChars: "●", CommitIdx: 1, GraphWidth: 1},
		}
		m.maxGraphWidth = 1
		m.updateLabelWidth()
		m.maxAuthorWidth = 24

		lines := strings.Split(m.View(), "\n")
		if len(lines) != m.windowHeight {
			t.Errorf("width %d: %d lines, want %d", width, len(lines), m.windowHeight)
		}
		for i, line := range lines {
			if w := ansi.StringWidth(line); w > width {
				t.Errorf("width %d: line %d is %d wide", width, i, w)
			}
		}
	}
}
