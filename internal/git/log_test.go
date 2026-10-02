package git

import (
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// mergeRepo is a branch merged back into main with a merge commit, so the
// history holds one commit with two parents among ones with one or none.
func mergeRepo(t *testing.T) string {
	t.Helper()
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("base")
	git("checkout", "-qb", "side")
	commit("on side")
	git("checkout", "-q", "main")
	commit("on main")
	git("merge", "-q", "--no-ff", "side", "-m", "merge side")
	return dir
}

// The line of a merged branch ends on the row under the commit that absorbed
// it, drawn in the branch's own colour. The marker is what says that commit is
// where it went.
func TestMergeCommitsGetTheirOwnMarker(t *testing.T) {
	g, err := LoadGraph(mergeRepo(t), 100)
	if err != nil {
		t.Fatal(err)
	}

	merges := 0
	for _, row := range g.Rows {
		var markers []rune
		for _, r := range row.GraphChars {
			if IsCommitMarker(r) {
				markers = append(markers, r)
			}
		}
		if row.CommitIdx < 0 {
			if len(markers) != 0 {
				t.Errorf("the connecting row %q has a commit marker on it", row.GraphChars)
			}
			continue
		}

		c := g.Commits[row.CommitIdx]
		if len(markers) != 1 {
			t.Fatalf("%q is drawn %q, want exactly one marker", c.Message, row.GraphChars)
		}
		want := CommitMarker
		if len(c.Parents) > 1 {
			want = MergeMarker
			merges++
		}
		if markers[0] != want {
			t.Errorf("%q (%d parents) is marked %c, want %c", c.Message, len(c.Parents), markers[0], want)
		}
		// One lane per character still: the marker is swapped, never widened.
		if got := len([]rune(row.GraphChars)); got != len(row.Lanes) {
			t.Errorf("%q: %d characters but %d lanes", c.Message, got, len(row.Lanes))
		}
	}
	if merges != 1 {
		t.Errorf("found %d merge commits, want the one this history has", merges)
	}
}
