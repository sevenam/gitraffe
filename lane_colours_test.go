package main

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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

func TestGraphLanesParse(t *testing.T) {
	g := newGraphLanes()
	text, lanes := g.parse("\x1b[32m|\x1b[m \x1b[34m|\x1b[m")
	if text != "| |" {
		t.Errorf("text = %q, want the escapes stripped", text)
	}
	if want := []int{1, 0, 2}; !slices.Equal(lanes, want) {
		t.Errorf("lanes = %v, want %v (the space between two lanes is uncoloured)", lanes, want)
	}
	// The same colour later in the log is the same lane number, so a palette
	// entry stays put instead of shifting as the log is read.
	if _, again := g.parse("\x1b[34m/\x1b[m"); again[0] != 2 {
		t.Errorf("second sighting of colour 34 = %d, want 2", again[0])
	}
}

func TestCommitPaths(t *testing.T) {
	// c0 is a merge of the trunk (c1) and a branch (c3).
	commits := []commit{
		{Hash: "c0", Parents: []string{"c1", "c3"}},
		{Hash: "c1", Parents: []string{"c2"}},
		{Hash: "c2", Parents: []string{}},
		{Hash: "c3", Parents: []string{"c2"}},
	}
	got := commitPaths(commits)
	if got[0] != got[1] || got[1] != got[2] {
		t.Errorf("paths = %v, want the first-parent chain c0-c1-c2 on one path", got)
	}
	if got[3] == got[0] {
		t.Errorf("paths = %v, want the merged-in branch c3 on a path of its own", got)
	}
}

// laneRepo builds a repository whose graph makes a branch change columns: a
// long-running branch is still open when a shorter one is created and merged
// beside it, which pushes the long one out a column and pulls it back.
func laneRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Every commit needs a later timestamp than the last. Left to the clock
	// they all share one second, and git then orders the log so that the
	// branches never overlap and nothing ever shifts columns — which is the
	// only thing this repository exists to produce.
	n := 0
	stamp := func() string {
		n++
		return fmt.Sprintf("2026-01-01T00:%02d:00", n)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		at := stamp()
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_AUTHOR_DATE="+at, "GIT_COMMITTER_DATE="+at,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	commit := func(name string) {
		t.Helper()
		// A file per commit, so no merge in this shape can conflict.
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		git("add", "-A")
		git("commit", "-qm", name)
	}

	git("init", "-q", "-b", "main")
	commit("base")
	git("checkout", "-qb", "longrunner")
	commit("L1")
	git("checkout", "-q", "main")
	commit("m1")
	git("checkout", "-qb", "shorty")
	commit("S1")
	git("checkout", "-q", "main")
	commit("m2")
	git("merge", "-q", "--no-ff", "shorty", "-m", "merge shorty")
	git("checkout", "-q", "longrunner")
	commit("L2")
	commit("L3")
	git("checkout", "-q", "main")
	commit("m3")
	git("merge", "-q", "--no-ff", "longrunner", "-m", "merge longrunner")
	return dir
}

// This is the bug the column-based first attempt had: a lane was coloured by
// the column it sat in, so a branch changed colour the moment an unrelated lane
// to its left merged away and everything shifted over.
func TestBranchKeepsOneLaneAcrossColumnShifts(t *testing.T) {
	m := &model{repoPath: laneRepo(t)}
	if err := m.loadGraphData(); err != nil {
		t.Fatal(err)
	}

	// Where each of the branch's commits was drawn, and in which lane.
	type spot struct{ col, lane int }
	spots := map[string]spot{}
	for _, row := range m.displayRows {
		if row.CommitIdx < 0 {
			continue
		}
		runes := []rune(row.GraphChars)
		for c, r := range runes {
			if r == '●' && c < len(row.Lanes) {
				spots[m.commits[row.CommitIdx].Message] = spot{c, row.Lanes[c]}
				break
			}
		}
	}

	for _, name := range []string{"L1", "L2", "L3"} {
		if _, ok := spots[name]; !ok {
			t.Fatalf("%s is missing from the graph; spots = %v", name, spots)
		}
	}
	if spots["L1"].lane != spots["L3"].lane {
		t.Fatalf("L1 is lane %d and L3 is lane %d, want one lane for the whole branch",
			spots["L1"].lane, spots["L3"].lane)
	}
	if spots["L1"].lane == spots["base"].lane {
		t.Error("the branch shares the trunk's lane, so it gets no colour of its own")
	}
	if spots["S1"].lane == spots["L1"].lane {
		t.Error("two different branches share a lane")
	}

	// The branch's commits all sit in one column — they are drawn on
	// consecutive rows — so it is the lane between them that moves. Checking
	// that it really does move is what stops this test passing while covering
	// nothing: without a shift, colouring by column would pass it too.
	columns := map[int]bool{}
	for _, row := range m.displayRows {
		for c, lane := range row.Lanes {
			if lane != spots["L1"].lane {
				continue
			}
			if r := []rune(row.GraphChars); c < len(r) && r[c] != ' ' {
				columns[c] = true
			}
		}
	}
	if len(columns) < 2 {
		t.Fatalf("the branch's lane stayed in column(s) %v, so this shape no longer "+
			"shifts and covers nothing — pick a shape that does", slices.Sorted(maps.Keys(columns)))
	}
}

func TestTrunkStaysOneLaneThroughMerges(t *testing.T) {
	m := &model{repoPath: laneRepo(t)}
	if err := m.loadGraphData(); err != nil {
		t.Fatal(err)
	}
	// Git recolours the trunk after every merge; the path ids must not.
	lanes := map[int]string{}
	for _, row := range m.displayRows {
		if row.CommitIdx < 0 {
			continue
		}
		msg := m.commits[row.CommitIdx].Message
		if msg != "base" && !strings.HasPrefix(msg, "m") {
			continue // not a trunk commit
		}
		runes := []rune(row.GraphChars)
		for c, r := range runes {
			if r == '●' && c < len(row.Lanes) {
				lanes[row.Lanes[c]] = msg
				break
			}
		}
	}
	if len(lanes) != 1 {
		t.Errorf("trunk commits landed in %d lanes (%v), want one", len(lanes), lanes)
	}
}

func TestLaneStyleCyclesPastTheEndOfThePalette(t *testing.T) {
	palette := buildLanePalette()
	trunk := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500"))

	if got := laneStyle(0, trunk, palette); got.GetForeground() != trunk.GetForeground() {
		t.Error("lane 0 should keep the trunk's colour so the graph's spine never moves")
	}
	first := laneStyle(1, trunk, palette).GetForeground()
	wrapped := laneStyle(1+len(palette), trunk, palette).GetForeground()
	if first != wrapped {
		t.Errorf("lane %d = %v, want it to wrap back to lane 1's %v", 1+len(palette), wrapped, first)
	}
}

// laneModel is a graph four lanes wide whose first row carries no commit, so
// that row renders as the graph alone: nothing else contributes a colour.
func laneModel() *model {
	m := &model{windowWidth: 120, windowHeight: 14, colourLanes: true}
	m.commits = []commit{{Hash: "aaaaaaa", Author: "sevenam"}}
	m.displayRows = []displayRow{
		{GraphChars: "│ │ │ │", CommitIdx: -1, GraphWidth: 7, Lanes: []int{0, 0, 1, 1, 2, 2, 3}},
		{GraphChars: "●", CommitIdx: 0, GraphWidth: 1, Lanes: []int{0}},
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

	got := foregroundColours(renderLaneRow(t, laneModel()))
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
	m.displayRows = []displayRow{{GraphChars: "│", CommitIdx: -1, GraphWidth: 1, Lanes: []int{0}}}
	m.maxGraphWidth = 1

	on := foregroundColours(renderLaneRow(t, m))
	m.colourLanes = false
	off := foregroundColours(renderLaneRow(t, m))

	if !slices.Equal(on, off) {
		t.Errorf("trunk-only graph rendered %v with colouring on and %v with it off", on, off)
	}
}

func TestCKeyTogglesLaneColours(t *testing.T) {
	if !initialModel(".").colourLanes {
		t.Fatal("lane colouring should start on, or nobody finds it")
	}

	off, _ := testModel().Update(keyPress("c"))
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
