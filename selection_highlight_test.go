package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// backgroundCells decodes a rendered line into one flag per terminal cell:
// whether that cell is drawn with a background colour set.
func backgroundCells(line string) []bool {
	var cells []bool
	bg := false
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "\x1b[") {
			end := strings.IndexByte(line[i:], 'm')
			params := strings.Split(line[i+2:i+end], ";")
			for p := 0; p < len(params); p++ {
				n, _ := strconv.Atoi(params[p])
				switch {
				case params[p] == "" || n == 0 || n == 49:
					bg = false
				case n == 38 || n == 48:
					// Extended colour: skip its arguments so an RGB component
					// such as 48 is never read as a code of its own.
					if n == 48 {
						bg = true
					}
					if p+1 < len(params) && params[p+1] == "2" {
						p += 4
					} else {
						p += 2
					}
				case (n >= 40 && n <= 47) || (n >= 100 && n <= 107):
					bg = true
				}
			}
			i += end + 1
			continue
		}
		r := []rune(line[i:])[0]
		for range ansi.StringWidth(string(r)) {
			cells = append(cells, bg)
		}
		i += len(string(r))
	}
	return cells
}

func withTrueColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func highlightModel() *model {
	m := &model{windowWidth: 160, windowHeight: 12}
	m.commits = []commit{
		{Hash: "aaaaaaa", Author: "sevenam", Refs: "HEAD -> refs/heads/main, tag: refs/tags/v1.0"},
		{Hash: "bbbbbbb", Author: "John Børge Holen-Tjelta", Refs: "refs/remotes/origin/feature"},
		{Hash: "ccccccc", Author: "山田太郎", MergedBranch: "old-branch"},
	}
	m.displayRows = []displayRow{
		{GraphChars: "●", CommitIdx: 0, GraphWidth: 1},
		{GraphChars: "│ ●", CommitIdx: 1, GraphWidth: 3},
		{GraphChars: "│/", CommitIdx: -1, GraphWidth: 2},
		{GraphChars: "●", CommitIdx: 2, GraphWidth: 1},
	}
	m.maxGraphWidth = 3
	m.updateLabelWidth()
	m.maxAuthorWidth = ansi.StringWidth("John Børge Holen-Tjelta")
	return m
}

func TestSelectedRowBandIsUnbroken(t *testing.T) {
	withTrueColor(t)

	for _, tc := range []struct {
		name  string
		width int
	}{
		{"all columns", 160},
		{"authors dropped", 95},
	} {
		for sel := range 3 {
			m := highlightModel()
			m.windowWidth = tc.width
			m.selected = sel
			l := computePanelLayout(m.windowWidth, m.maxGraphWidth, m.maxBranchWidth, len(dateColumnFormat), m.maxAuthorWidth)
			content := l.leftWidth - 4
			lines := strings.Split(m.renderCommitList(l, content), "\n")

			selLine := map[int]int{0: 0, 1: 1, 2: 3}[sel] // commit index -> display row
			for i, line := range lines[:4] {
				cells := backgroundCells(line)
				if i == selLine {
					if len(cells) != content {
						t.Errorf("%s, selected %d: band is %d cells, want the full content width %d", tc.name, sel, len(cells), content)
					}
					for c, on := range cells {
						if !on {
							t.Errorf("%s, selected %d: gap in the band at cell %d of %q", tc.name, sel, c, stripANSI(line))
							break
						}
					}
				} else {
					for c, on := range cells {
						if on {
							t.Errorf("%s, selected %d: unselected row %d has a background at cell %d", tc.name, sel, i, c)
							break
						}
					}
				}
			}
		}
	}
}

func TestSelectedRowBandInFallbackList(t *testing.T) {
	withTrueColor(t)

	m := &model{windowWidth: 100, windowHeight: 12, selected: 1}
	m.commits = []commit{
		{Hash: "aaaaaaa", GraphLine: "● "},
		{Hash: "bbbbbbb", GraphLine: "◆ "},
	}
	lines := strings.Split(m.renderCommitList(panelLayout{}, 40), "\n")

	if cells := backgroundCells(lines[1]); len(cells) != 40 || strings.Contains(boolsString(cells), "0") {
		t.Errorf("selected fallback row band = %s, want 40 cells all set", boolsString(cells))
	}
	if strings.Contains(boolsString(backgroundCells(lines[0])), "1") {
		t.Errorf("unselected fallback row has a background")
	}
}

func boolsString(b []bool) string {
	var sb strings.Builder
	for _, v := range b {
		if v {
			sb.WriteByte('1')
		} else {
			sb.WriteByte('0')
		}
	}
	return sb.String()
}
