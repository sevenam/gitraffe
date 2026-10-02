package git

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

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
	commits := []Commit{
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

// laneOf reports the lane of the character drawn for a commit, and the lanes of
// every diagonal in the graph keyed by the row they are on.
func laneOf(m Graph, message string) (int, bool) {
	for _, row := range m.Rows {
		if row.CommitIdx < 0 || m.Commits[row.CommitIdx].Message != message {
			continue
		}
		for c, r := range []rune(row.GraphChars) {
			if IsCommitMarker(r) && c < len(row.Lanes) {
				return row.Lanes[c], true
			}
		}
	}
	return 0, false
}

// A branch that merges into another branch rather than into the trunk closes
// with a diagonal that touches the lane it is joining. Git gives both the same
// colour once they converge, so the diagonal used to be drawn in the colour of
// the branch being merged into instead of the one doing the merging.
func TestClosingDiagonalKeepsItsOwnBranch(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("base")
	git("checkout", "-qb", "feature")
	commit("F1")
	git("checkout", "-qb", "sub")
	commit("S1")
	git("checkout", "-q", "feature")
	git("merge", "-q", "--no-ff", "sub", "-m", "merge sub")
	git("checkout", "-q", "main")
	commit("m1")
	git("merge", "-q", "--no-ff", "feature", "-m", "merge feature")

	m, err := LoadGraph(dir, 5000)
	if err != nil {
		t.Fatal(err)
	}

	subLane, ok := laneOf(m, "S1")
	if !ok {
		t.Fatal("S1 is missing from the graph")
	}
	featureLane, ok := laneOf(m, "F1")
	if !ok {
		t.Fatal("F1 is missing from the graph")
	}
	if subLane == featureLane {
		t.Fatalf("sub and feature share lane %d, so this shape cannot show the bug", subLane)
	}

	// Every diagonal that leaves the sub-branch must still be the sub-branch.
	// Find the row holding S1, then the diagonal that closes its lane below.
	var diagonals []int
	for _, row := range m.Rows {
		for c, r := range []rune(row.GraphChars) {
			if r == '/' && c < len(row.Lanes) {
				diagonals = append(diagonals, row.Lanes[c])
			}
		}
	}
	if !slices.Contains(diagonals, subLane) {
		t.Errorf("no diagonal carries the sub-branch's lane %d; diagonals are in lanes %v — "+
			"its closing stroke was drawn as the branch it merged into",
			subLane, diagonals)
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
	m, err := LoadGraph(laneRepo(t), 5000)
	if err != nil {
		t.Fatal(err)
	}

	// Where each of the branch's commits was drawn, and in which lane.
	type spot struct{ col, lane int }
	spots := map[string]spot{}
	for _, row := range m.Rows {
		if row.CommitIdx < 0 {
			continue
		}
		runes := []rune(row.GraphChars)
		for c, r := range runes {
			if IsCommitMarker(r) && c < len(row.Lanes) {
				spots[m.Commits[row.CommitIdx].Message] = spot{c, row.Lanes[c]}
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
	for _, row := range m.Rows {
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
	m, err := LoadGraph(laneRepo(t), 5000)
	if err != nil {
		t.Fatal(err)
	}
	// Git recolours the trunk after every merge; the path ids must not.
	lanes := map[int]string{}
	for _, row := range m.Rows {
		if row.CommitIdx < 0 {
			continue
		}
		msg := m.Commits[row.CommitIdx].Message
		if msg != "base" && !strings.HasPrefix(msg, "m") {
			continue // not a trunk commit
		}
		runes := []rune(row.GraphChars)
		for c, r := range runes {
			if IsCommitMarker(r) && c < len(row.Lanes) {
				lanes[row.Lanes[c]] = msg
				break
			}
		}
	}
	if len(lanes) != 1 {
		t.Errorf("trunk commits landed in %d lanes (%v), want one", len(lanes), lanes)
	}
}

// Merging main into a feature branch draws main's line leaving the merge to
// the right and closing back over the feature's own line into main, in a kink
// git renders as "| |\", "| |/", "|/|". That line is main's, as git's own
// colours say; drawn in the feature's colour it looked like the feature branch
// starting a second time.
func TestMainMergedIntoFeatureKeepsMainsColour(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("base")
	git("checkout", "-qb", "feature")
	commit("F1")
	git("checkout", "-q", "main")
	git("checkout", "-qb", "side")
	commit("S1")
	git("checkout", "-q", "main")
	git("merge", "-q", "--no-ff", "side", "-m", "merge side")
	git("checkout", "-q", "feature")
	git("merge", "-q", "--no-ff", "main", "-m", "merge main into feature")
	commit("F2")
	git("checkout", "-q", "main")
	git("merge", "-q", "--no-ff", "feature", "-m", "merge feature")

	m, err := LoadGraph(dir, 5000)
	if err != nil {
		t.Fatal(err)
	}
	mainLane, _ := laneOf(m, "merge side")
	featureLane, _ := laneOf(m, "F2")
	if mainLane == featureLane {
		t.Fatalf("main and feature share lane %d, so this shape cannot show the bug", mainLane)
	}

	// The rows between the merge into the feature and the main commit it
	// merged hold only two lines: the feature's own bar, and main's kink.
	var between []DisplayRow
	inside := false
	for _, row := range m.Rows {
		if row.CommitIdx >= 0 {
			msg := m.Commits[row.CommitIdx].Message
			if msg == "merge main into feature" {
				inside = true
				continue
			}
			if msg == "merge side" {
				break
			}
		}
		if inside {
			between = append(between, row)
		}
	}
	if len(between) == 0 {
		t.Fatal("no rows between the two merges; git drew a different shape")
	}
	kinks := 0
	for _, row := range between {
		for c, r := range []rune(row.GraphChars) {
			if r == '\\' || r == '/' {
				kinks++
				if row.Lanes[c] != mainLane {
					t.Errorf("%q: %c at column %d is lane %d, want main's lane %d",
						row.GraphChars, r, c, row.Lanes[c], mainLane)
				}
			}
		}
	}
	if kinks < 3 {
		t.Fatalf("found %d strokes of main's kink, want 3; git drew a different shape", kinks)
	}
}
