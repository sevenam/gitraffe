package tui

import "testing"

// A comma joins two labels of one kind; where the kind changes, the colour
// changes with it and a gap is all that stands between them.
func TestLabelText(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    commit
		want string
	}{
		{"local only", commit{Refs: "refs/heads/main"}, "main"},
		{"local before remote", commit{Refs: "refs/remotes/origin/main, refs/heads/main"}, "main  origin/main"},
		{"branch and tag", commit{Refs: "refs/heads/main, tag: refs/tags/v1.0"}, "main  v1.0"},
		{"two of a kind", commit{
			Refs: "refs/heads/a, refs/heads/b, refs/remotes/origin/a, refs/remotes/fork/a, tag: refs/tags/v1, tag: refs/tags/v2",
		}, "a, b  origin/a, fork/a  v1, v2"},
		{"a tag only the remote has", commit{
			Refs: "tag: refs/tags/v1", RemoteOnlyTags: []string{"v2"},
		}, "v1, v2" + tagRemoteOnlyMark},
		{"merged only", commit{MergedBranch: "feature-x"}, "feature-x"},
		{"all kinds", commit{
			Refs:         "HEAD -> refs/heads/main, refs/remotes/origin/main, tag: refs/tags/v1.0",
			MergedBranch: "feature-x",
		}, "main  origin/main  v1.0  feature-x"},
		{"nothing", commit{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := labelText(tc.c); got != tc.want {
				t.Errorf("labelText() = %q, want %q", got, tc.want)
			}
			// The column is sized by counting what it will be given.
			if got := segmentsWidth(labelSegments(tc.c)); got != len([]rune(tc.want)) {
				t.Errorf("segmentsWidth() = %d, want %d", got, len([]rune(tc.want)))
			}
		})
	}
}

// Remote branches start in one column on every row, and the gap that puts
// them there is the whole of what separates them from the local branches: no
// comma is left hanging at the end of it.
func TestRemoteColumnIsAligned(t *testing.T) {
	commits := []commit{
		{Refs: "refs/heads/main, refs/remotes/origin/main"},
		{Refs: "refs/heads/a-longer-branch, refs/remotes/origin/a-longer-branch"},
		{Refs: "refs/heads/one, refs/heads/two, refs/remotes/origin/one, refs/remotes/fork/one, tag: refs/tags/v1.0"},
		{Refs: "refs/remotes/origin/gone"},
		{Refs: "refs/heads/local-only, tag: refs/tags/v0.9"},
	}
	m := model{commits: commits}
	m.updateLabelWidth()

	want := []string{
		"main             origin/main",
		"a-longer-branch  origin/a-longer-branch",
		"one, two         origin/one, fork/one  v1.0",
		"                 origin/gone",
		"local-only  v0.9",
	}
	for i, c := range commits {
		segs := alignRemote(labelSegments(c), m.localLabelWidth)
		if got := segmentsText(segs); got != want[i] {
			t.Errorf("row %d:\ngot  %q\nwant %q", i, got, want[i])
		}
		if got := segmentsWidth(segs); got != len(want[i]) {
			t.Errorf("row %d: counted %d wide, drawn %d", i, got, len(want[i]))
		}
	}
	if m.maxBranchWidth != len(want[2]) {
		t.Errorf("the column is %d wide, the widest label %d", m.maxBranchWidth, len(want[2]))
	}
}
