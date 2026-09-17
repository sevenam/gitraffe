package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// foregroundColours lists the distinct foreground colours a rendered line sets,
// in the order they first appear. One entry per lane is what lane colouring
// looks like from the outside.
func foregroundColours(line string) []string {
	var seen []string
	for i := 0; i < len(line); {
		if !strings.HasPrefix(line[i:], "\x1b[") {
			i++
			continue
		}
		end := strings.IndexByte(line[i:], 'm')
		if end < 0 {
			break
		}
		// Walk the parameters rather than matching a prefix: lipgloss puts
		// bold in front of the colour, so "38;..." is not where a foreground
		// reliably starts.
		params := strings.Split(line[i+2:i+end], ";")
		for p := 0; p < len(params); p++ {
			if params[p] != "38" {
				continue
			}
			n := 3 // 38;5;n
			if p+1 < len(params) && params[p+1] == "2" {
				n = 5 // 38;2;r;g;b
			}
			if p+n > len(params) {
				break
			}
			if fg := strings.Join(params[p:p+n], ";"); !slices.Contains(seen, fg) {
				seen = append(seen, fg)
			}
			p += n - 1
		}
		i += end + 1
	}
	return seen
}

func TestLaneAt(t *testing.T) {
	// Git draws lane n at column 2n; the odd columns hold the diagonals.
	for _, tc := range []struct {
		name string
		col  int
		ch   rune
		want int
	}{
		{"trunk node", 0, '●', 0},
		{"trunk line", 0, '│', 0},
		{"second lane", 2, '│', 1},
		{"third lane", 4, '●', 2},
		// A diagonal takes the lane on its right, the one it creates or closes,
		// so a branch is one colour from the row it splits off.
		{"branch out", 1, '\\', 1},
		{"branch in", 1, '/', 1},
		{"deeper branch out", 3, '\\', 2},
		{"deeper branch in", 3, '/', 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := laneAt(tc.col, tc.ch); got != tc.want {
				t.Errorf("laneAt(%d, %q) = %d, want %d", tc.col, tc.ch, got, tc.want)
			}
		})
	}
}

func TestLaneStyleCyclesPastTheEndOfThePalette(t *testing.T) {
	palette := buildLanePalette()
	trunk := buildLanePalette()[0] // any style distinct from the palette's use here

	if got := laneStyle(0, trunk, palette); got.GetForeground() != trunk.GetForeground() {
		t.Error("lane 0 should keep the trunk's colour so the graph's spine never moves")
	}
	first := laneStyle(1, trunk, palette).GetForeground()
	wrapped := laneStyle(1+len(palette), trunk, palette).GetForeground()
	if first != wrapped {
		t.Errorf("lane %d = %v, want it to wrap back to lane 1's %v", 1+len(palette), wrapped, first)
	}
}

// laneModel is a graph four lanes wide whose rows carry no commit, so a
// rendered row is only the graph: nothing else contributes a colour.
func laneModel() *model {
	m := &model{windowWidth: 120, windowHeight: 14, colourLanes: true}
	m.commits = []commit{{Hash: "aaaaaaa", Author: "sevenam"}}
	m.displayRows = []displayRow{
		{GraphChars: "│ │ │ │", CommitIdx: -1, GraphWidth: 7},
		{GraphChars: "●", CommitIdx: 0, GraphWidth: 1},
	}
	m.maxGraphWidth = 7
	m.updateLabelWidth()
	m.maxAuthorWidth = ansi.StringWidth("sevenam")
	return m
}

func renderLaneRow(t *testing.T, m *model) string {
	t.Helper()
	l := computePanelLayout(m.windowWidth, m.maxGraphWidth, m.maxBranchWidth, len(dateColumnFormat), m.maxAuthorWidth)
	out := m.renderCommitList(l.branchCol, l.dateCol, l.authorCol, l.leftWidth-4)
	return strings.Split(out, "\n")[0]
}

func TestEachLaneGetsItsOwnColour(t *testing.T) {
	withTrueColor(t)

	m := laneModel()
	got := foregroundColours(renderLaneRow(t, m))
	if len(got) != 4 {
		t.Errorf("a four-lane row used %d colours (%v), want one per lane", len(got), got)
	}
}

func TestLaneColoursOffLeavesOneColour(t *testing.T) {
	withTrueColor(t)

	m := laneModel()
	m.colourLanes = false
	got := foregroundColours(renderLaneRow(t, m))
	if len(got) != 1 {
		t.Errorf("with colouring off a four-lane row used %d colours (%v), want the graph colour alone", len(got), got)
	}
}

// The trunk is the one lane whose colour the toggle must not change, so a graph
// with no branches looks the same either way.
func TestTrunkKeepsTheThemeGraphColour(t *testing.T) {
	withTrueColor(t)

	m := laneModel()
	m.displayRows = []displayRow{{GraphChars: "│", CommitIdx: -1, GraphWidth: 1}}
	m.maxGraphWidth = 1

	on := foregroundColours(renderLaneRow(t, m))
	m.colourLanes = false
	off := foregroundColours(renderLaneRow(t, m))

	if !slices.Equal(on, off) {
		t.Errorf("trunk-only graph rendered %v with colouring on and %v with it off", on, off)
	}
}

func TestCKeyTogglesLaneColours(t *testing.T) {
	m := testModel()
	if !initialModel(".").colourLanes {
		t.Fatal("lane colouring should start on, or nobody finds it")
	}

	off, _ := m.Update(keyPress("c"))
	if off.(model).colourLanes {
		t.Error("c did not turn lane colouring off")
	}
	on, _ := off.(model).Update(keyPress("c"))
	if !on.(model).colourLanes {
		t.Error("c did not turn lane colouring back on")
	}
}

// The graph is on screen whichever box has focus, so the key has to reach it
// from both rather than only from the commit list.
func TestCKeyWorksFromEitherBox(t *testing.T) {
	for _, box := range []int{1, 2} {
		m := testModel()
		m.focusedBox = box
		m.colourLanes = true
		got, _ := m.Update(keyPress("c"))
		if got.(model).colourLanes {
			t.Errorf("c did nothing with box %d focused", box)
		}
	}
}
