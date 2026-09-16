package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

var dateWidth = len(dateColumnFormat)

func TestComputePanelLayoutNeverOverflows(t *testing.T) {
	for window := 30; window <= 260; window++ {
		for _, graph := range []int{1, 6, 20, 60} {
			for _, branch := range []int{0, 12, 31} {
				for _, date := range []int{0, dateWidth} {
					for _, author := range []int{0, 7, 24, 40} {
						l := computePanelLayout(window, graph, branch, date, author)
						name := func() string {
							return fmt.Sprintf("window=%d graph=%d branch=%d date=%d author=%d", window, graph, branch, date, author)
						}

						if l.leftWidth+l.rightWidth > window {
							t.Fatalf("%s: panels %d+%d exceed the window", name(), l.leftWidth, l.rightWidth)
						}

						if l.authorCol != 0 && (l.authorCol < minAuthorColWidth || l.authorCol > maxAuthorColWidth) {
							t.Fatalf("%s: author column %d outside [%d,%d]", name(), l.authorCol, minAuthorColWidth, maxAuthorColWidth)
						}

						// A date is shown whole or not at all.
						if l.dateCol != 0 && l.dateCol != date {
							t.Fatalf("%s: date column %d, want 0 or %d", name(), l.dateCol, date)
						}

						// Priority is labels, then dates, then authors: nothing
						// later gets room while something earlier was cut.
						if (l.dateCol > 0 || l.authorCol > 0) && l.branchCol < branch {
							t.Fatalf("%s: labels cut to %d of %d while dates=%d authors=%d", name(), l.branchCol, branch, l.dateCol, l.authorCol)
						}
						if l.authorCol > 0 && l.dateCol < date {
							t.Fatalf("%s: authors got %d while the date was dropped", name(), l.authorCol)
						}

						// When the graph fits beside the details panel, no row may be
						// wider than the list's content area, or lipgloss wraps it.
						if graph+14 <= window*4/5 && graph+14+minRightPanelWidth <= window {
							row := 2 + graph + 1 + 7
							for _, col := range []int{l.branchCol, l.dateCol, l.authorCol} {
								if col > 0 {
									row += col + 1
								}
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
}

func TestComputePanelLayoutPriorities(t *testing.T) {
	t.Run("wide window shows everything, capping long names", func(t *testing.T) {
		l := computePanelLayout(300, 6, 10, dateWidth, 60)
		if l.branchCol != 10 || l.dateCol != dateWidth || l.authorCol != maxAuthorColWidth {
			t.Errorf("columns = %d/%d/%d, want 10/%d/%d", l.branchCol, l.dateCol, l.authorCol, dateWidth, maxAuthorColWidth)
		}
	})

	t.Run("short names are not padded to the cap", func(t *testing.T) {
		if l := computePanelLayout(300, 6, 10, dateWidth, 7); l.authorCol != 7 {
			t.Errorf("author column = %d, want 7", l.authorCol)
		}
	})

	// 100 wide: room beside a 6-wide graph is 100-20-30 = 50.
	t.Run("authors go first", func(t *testing.T) {
		// 33 labels (+1 space) and a date (+1 space) leave 5: too few for the
		// minimum name plus its space.
		l := computePanelLayout(100, 6, 33, dateWidth, 24)
		if l.branchCol != 33 || l.dateCol != dateWidth || l.authorCol != 0 {
			t.Errorf("columns = %d/%d/%d, want 33/%d/0", l.branchCol, l.dateCol, l.authorCol, dateWidth)
		}
	})

	t.Run("dates go before labels are cut", func(t *testing.T) {
		// 45 labels leaves 4: no room for a whole date.
		l := computePanelLayout(100, 6, 45, dateWidth, 24)
		if l.branchCol != 45 || l.dateCol != 0 || l.authorCol != 0 {
			t.Errorf("columns = %d/%d/%d, want 45/0/0", l.branchCol, l.dateCol, l.authorCol)
		}
	})

	t.Run("a name never takes room a date couldn't fit", func(t *testing.T) {
		// 52 wide: room beside a 1-wide graph is 7 — enough for a 6-wide name,
		// not for a date.
		l := computePanelLayout(52, 1, 0, dateWidth, 7)
		if l.dateCol != 0 || l.authorCol != 0 {
			t.Errorf("columns = date %d author %d, want 0/0", l.dateCol, l.authorCol)
		}
	})

	t.Run("columns not wanted take no room", func(t *testing.T) {
		l := computePanelLayout(300, 6, 10, 0, 0)
		if l.dateCol != 0 || l.authorCol != 0 {
			t.Errorf("columns = date %d author %d, want 0/0", l.dateCol, l.authorCol)
		}
	})
}

func TestListColumnsRendering(t *testing.T) {
	m := testModel()
	m.windowWidth = 140
	m.windowHeight = 20
	day := func(d int) time.Time { return time.Date(2026, 9, d, 23, 30, 0, 0, time.Local) }
	m.commits = []commit{
		{Hash: "aaaaaaa", Date: day(16), Author: "sevenam", Refs: "refs/heads/main"},
		{Hash: "bbbbbbb", Date: day(15), Author: "A Very Long Author Name That Keeps Going"},
		{Hash: "ccccccc", Date: day(2), Author: "山田太郎"}, // 4 runes, 8 columns wide
		{Hash: "ddddddd", Date: day(1), Author: "Anders Østhus"},
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

	l := computePanelLayout(m.windowWidth, m.maxGraphWidth, m.maxBranchWidth, dateWidth, m.maxAuthorWidth)
	contentWidth := l.leftWidth - 4
	lines := strings.Split(m.renderCommitList(l.branchCol, l.dateCol, l.authorCol, contentWidth), "\n")

	// Date directly after the hash; author against the right edge.
	hashEnd := 2 + l.branchCol + 1 + m.maxGraphWidth + 1 + 7
	dateStart := hashEnd + 1
	authorStart := contentWidth - l.authorCol

	for i, want := range map[int]struct{ hash, date, author string }{
		0: {"aaaaaaa", "2026-09-16", "sevenam"},
		1: {"bbbbbbb", "2026-09-15", "A Very Long Author Name…"},
		3: {"ccccccc", "2026-09-02", "山田太郎"},
		4: {"ddddddd", "2026-09-01", "Anders Østhus"},
	} {
		line := stripANSI(lines[i])
		if w := ansi.StringWidth(line); w > contentWidth {
			t.Errorf("line %d is %d wide, over the content width %d: %q", i, w, contentWidth, line)
		}
		if got := ansi.Cut(line, hashEnd-7, hashEnd); got != want.hash {
			t.Errorf("line %d hash = %q, want %q (line %q)", i, got, want.hash, line)
		}
		if got := ansi.Cut(line, dateStart, dateStart+dateWidth); got != want.date {
			t.Errorf("line %d date column = %q, want %q (line %q)", i, got, want.date, line)
		}
		// Every name starts at the same display column, whatever its width.
		if got := strings.TrimRight(ansi.Cut(line, authorStart, contentWidth), " "); got != want.author {
			t.Errorf("line %d author column = %q, want %q (line %q)", i, got, want.author, line)
		}
	}

	// Graph-only connector rows carry neither date nor author.
	if got := strings.TrimRight(ansi.Cut(stripANSI(lines[2]), hashEnd, contentWidth), " "); got != "" {
		t.Errorf("connector row shows %q after the graph", got)
	}
}

func TestViewWithColumnsKeepsExactSize(t *testing.T) {
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
