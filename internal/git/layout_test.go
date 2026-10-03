package git

import (
	"slices"
	"strings"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// history builds commits from lines of "hash parent parent...", in the order
// git would list them: every commit before its parents.
func history(lines ...string) []Commit {
	var commits []Commit
	for _, line := range lines {
		fields := strings.Fields(line)
		commits = append(commits, Commit{Hash: fields[0], Parents: fields[1:]})
	}
	return commits
}

func drawn(rows []DisplayRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.GraphChars)
	}
	return out
}

func TestLayoutGraph(t *testing.T) {
	for _, tc := range []struct {
		name    string
		commits []Commit
		want    []string
	}{
		{
			"a straight line",
			history("c b", "b a", "a"),
			[]string{"●", "●", "●"},
		},
		{
			// The merge and the branch-off are each on the row of the commit
			// they belong to, not on rows of their own in between.
			"a branch merged back",
			history("merge base tip", "tip first", "first base", "base"),
			[]string{
				"◆─╮",
				"│ ●",
				"│ ●",
				"●─╯",
			},
		},
		{
			// The reason gitraffe draws the graph itself. Git folds the merged
			// branch's line into the trunk as soon as its commits are done,
			// rows above "base", so only the other branch appears to start
			// there. Both lanes stay open until base's own row.
			"two branches from one commit both reach it",
			history("merge base g2", "g2 g1", "g1 base", "other base", "base"),
			[]string{
				"◆─╮",
				"│ ●",
				"│ ●",
				"│ │ ●",
				"●─┴─╯",
			},
		},
		{
			// One pull request after another: each branch starts at the merge
			// before it. The lane that ends and the lane that opens share the
			// column, so the branches stack instead of stepping sideways.
			"successive branches stack in one column",
			history("m2 m1 x", "x m1", "m1 base y", "y base", "base"),
			[]string{
				"◆─╮",
				"│ ●",
				"◆─┤",
				"│ ●",
				"●─╯",
			},
		},
		{
			// A lane in the way is drawn unbroken, and the stroke resumes on
			// its far side: a cross there would read as the two being joined.
			"a connection crosses a lane that is not part of it",
			history("m0 m y", "m base g", "y base", "g base", "base"),
			[]string{
				"◆─╮",
				"◆─│─╮",
				"│ ● │",
				"│ │ ●",
				"●─┴─╯",
			},
		},
		{
			// Main merged into a feature branch: main's line is already
			// running down the left, and the merge's stroke meets it.
			"a merge whose other parent already has a lane",
			history("top main2", "merge f1 main2", "f1 main1", "main2 main1", "main1"),
			[]string{
				"●",
				"├─◆",
				"│ ●",
				"● │",
				"●─╯",
			},
		},
		{
			"a merge of three",
			history("octopus a b c", "b base", "c base", "a base", "base"),
			[]string{
				"◆─┬─╮",
				"│ ● │",
				"│ │ ●",
				"● │ │",
				"●─┴─╯",
			},
		},
		{
			// Nothing leads to a second root, and nothing follows from it.
			"unrelated histories",
			history("b2 b1", "a2 a1", "b1", "a1"),
			[]string{
				"●",
				"│ ●",
				"● │",
				"  ●",
			},
		},
		{
			// A parent outside what was read: the line carries on to the
			// bottom, which is the truth — the history does too.
			"a history cut short",
			history("merge c tip", "tip older", "c older-still"),
			[]string{
				"◆─╮",
				"│ ●",
				"● │",
			},
		},
		{
			// A column freed in the middle is the nearest room for the next
			// branch, rather than widening the graph.
			"a freed column is used again",
			history("merge a side", "side a", "a root", "tip root", "root"),
			[]string{
				"◆─╮",
				"│ ●",
				"●─╯",
				"│ ●",
				"●─╯",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, width := layoutGraph(tc.commits)
			got := drawn(rows)
			if !slices.Equal(got, tc.want) {
				t.Errorf("drawn\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}

			widest := 0
			for i, row := range rows {
				runes := []rune(row.GraphChars)
				widest = max(widest, len(runes))
				if row.CommitIdx != i {
					t.Errorf("row %d is for commit %d, want one row per commit in order", i, row.CommitIdx)
				}
				if len(row.Lanes) != len(runes) || row.GraphWidth != len(runes) {
					t.Errorf("row %d %q: %d lanes and width %d for %d characters", i, row.GraphChars, len(row.Lanes), row.GraphWidth, len(runes))
				}
				if strings.HasSuffix(row.GraphChars, " ") {
					t.Errorf("row %d %q ends in a space", i, row.GraphChars)
				}
				markers := 0
				for _, r := range runes {
					if IsCommitMarker(r) {
						markers++
					}
				}
				if markers != 1 {
					t.Errorf("row %d %q has %d commit markers, want its own and no other", i, row.GraphChars, markers)
				}
			}
			if width != widest {
				t.Errorf("width = %d, want the widest row's %d", width, widest)
			}
		})
	}
}

// laneAt is the lane of the character in a column of a row.
func laneAt(t *testing.T, rows []DisplayRow, row, char int) int {
	t.Helper()
	if row >= len(rows) || char >= len(rows[row].Lanes) {
		t.Fatalf("no character %d on row %d", char, row)
	}
	return rows[row].Lanes[char]
}

// A branch is one colour from the stroke that merges it to the stroke that
// branches it off, and the trunk is one colour through every merge.
func TestLayoutColoursABranchForItsWholeLength(t *testing.T) {
	rows, _ := layoutGraph(history("merge base tip", "tip first", "first base", "base"))
	// ◆─╮
	// │ ●
	// │ ●
	// ●─╯
	trunk := laneAt(t, rows, 0, 0)
	branch := laneAt(t, rows, 1, 2)
	if trunk == branch {
		t.Fatalf("the trunk and the branch share lane %d", trunk)
	}
	for _, at := range [][2]int{{0, 1}, {0, 2}, {1, 2}, {2, 2}, {3, 1}, {3, 2}} {
		if got := laneAt(t, rows, at[0], at[1]); got != branch {
			t.Errorf("row %d character %d (%c) is lane %d, want the branch's %d",
				at[0], at[1], []rune(rows[at[0]].GraphChars)[at[1]], got, branch)
		}
	}
	for row := range rows {
		if got := laneAt(t, rows, row, 0); got != trunk {
			t.Errorf("row %d: the trunk is lane %d, want %d throughout", row, got, trunk)
		}
	}
}

// Where two branches end on one row, each stroke is the colour of the branch
// it belongs to: the part both share is drawn as the nearer one.
func TestLayoutColoursEachEndingBranchItsOwn(t *testing.T) {
	rows, _ := layoutGraph(history("merge base g2", "g2 g1", "g1 base", "other base", "base"))
	// ◆─╮
	// │ ●
	// │ ●
	// │ │ ●
	// ●─┴─╯
	green, other := laneAt(t, rows, 1, 2), laneAt(t, rows, 3, 4)
	if green == other {
		t.Fatalf("both branches are lane %d", green)
	}
	last := 4
	for char, want := range map[int]int{1: green, 2: green, 3: other, 4: other} {
		if got := laneAt(t, rows, last, char); got != want {
			t.Errorf("%q character %d is lane %d, want %d", rows[last].GraphChars, char, got, want)
		}
	}
}

// Merging main into a feature branch: the stroke to main's line, and the
// character where it meets it, are main's colour and not the feature's.
func TestLayoutDrawsAJoinInTheJoinedLanesColour(t *testing.T) {
	rows, _ := layoutGraph(history("top main2", "merge f1 main2", "f1 main1", "main2 main1", "main1"))
	// ●
	// ├─◆
	// │ ●
	main, feature := laneAt(t, rows, 0, 0), laneAt(t, rows, 2, 2)
	if main == feature {
		t.Fatalf("main and the feature branch share lane %d", main)
	}
	if got := laneAt(t, rows, 1, 0); got != main {
		t.Errorf("the join is lane %d, want main's %d", got, main)
	}
	if got := laneAt(t, rows, 1, 1); got != main {
		t.Errorf("the stroke to main is lane %d, want main's %d", got, main)
	}
	if got := laneAt(t, rows, 1, 2); got != feature {
		t.Errorf("the merge commit is lane %d, want the feature's %d", got, feature)
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

// The same shape as the report that led here, read from a real repository: a
// branch merged into main and another left open, both started at one commit.
func TestLoadGraphDrawsBothBranchesFromTheirCommit(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("base")
	git("checkout", "-qb", "other")
	commit("other 1")
	git("checkout", "-q", "main")
	git("checkout", "-qb", "merged")
	commit("merged 1")
	commit("merged 2")
	git("checkout", "-q", "main")
	git("merge", "-q", "--no-ff", "merged", "-m", "merge it")

	g, err := LoadGraph(dir, 100, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Rows) != len(g.Commits) {
		t.Fatalf("%d rows for %d commits, want one each", len(g.Rows), len(g.Commits))
	}
	last := g.Rows[len(g.Rows)-1]
	if got := g.Commits[last.CommitIdx].Message; got != "base" {
		t.Fatalf("the last commit listed is %q, want base", got)
	}
	// Three lanes arrive at base: main's own, and one from each branch.
	ends := strings.Count(last.GraphChars, "┴") + strings.Count(last.GraphChars, "╯")
	if !strings.HasPrefix(last.GraphChars, "●") || ends != 2 {
		t.Errorf("base is drawn %q, want both branches' lines ending on its row\n%s",
			last.GraphChars, strings.Join(drawn(g.Rows), "\n"))
	}
	if g.Commits[0].Message != "merge it" || !strings.HasPrefix(g.Rows[0].GraphChars, "◆") {
		t.Errorf("the merge is drawn %q", g.Rows[0].GraphChars)
	}
}
