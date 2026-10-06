package tui

import (
	"strings"
	"testing"
)

func TestLabelTextMatchesRenderedSegments(t *testing.T) {
	// labelText sizes the column and labelSegments fills it; if they disagree on
	// order or content the column truncates or misaligns.
	for _, tc := range []struct {
		name string
		c    commit
		want string
	}{
		{"local only", commit{Refs: "refs/heads/main"}, "main"},
		{"local before remote", commit{Refs: "refs/remotes/origin/main, refs/heads/main"}, "main, origin/main"},
		{"branch and tag", commit{Refs: "refs/heads/main, tag: refs/tags/v1.0"}, "main, v1.0"},
		{"merged only", commit{MergedBranch: "feature-x"}, "feature-x"},
		{"all kinds", commit{
			Refs:         "HEAD -> refs/heads/main, refs/remotes/origin/main, tag: refs/tags/v1.0",
			MergedBranch: "feature-x",
		}, "main, origin/main, v1.0, feature-x"},
		{"nothing", commit{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := labelText(tc.c); got != tc.want {
				t.Errorf("labelText() = %q, want %q", got, tc.want)
			}

			var joined string
			for i, seg := range labelSegments(tc.c) {
				if i > 0 {
					joined += ", "
				}
				joined += seg.text
			}
			if joined != tc.want {
				t.Errorf("labelSegments() joined = %q, want %q", joined, tc.want)
			}
		})
	}
}

// drawnLabel is a row's label as the graph writes it: each segment after its
// lead, or after a comma where it has none.
func drawnLabel(segs []labelSegment) string {
	var sb strings.Builder
	for i, seg := range segs {
		switch {
		case seg.lead != "":
			sb.WriteString(seg.lead)
		case i > 0:
			sb.WriteString(", ")
		}
		sb.WriteString(seg.text)
	}
	return sb.String()
}

// Remote branches start in one column on every row, and the gap that puts
// them there is the whole of what separates them from the local branches: no
// comma is left hanging at the end of it. Names that do run together, two
// branches of a kind or a tag after a branch, keep theirs.
func TestRemoteColumnGapHasNoComma(t *testing.T) {
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
		"one, two         origin/one, fork/one, v1.0",
		"                 origin/gone",
		"local-only, v0.9",
	}
	for i, c := range commits {
		segs := alignRemote(labelSegments(c), m.localLabelWidth)
		if got := drawnLabel(segs); got != want[i] {
			t.Errorf("row %d:\ngot  %q\nwant %q", i, got, want[i])
		}
		// The column is sized by the same segments it is filled with.
		if got := segmentsWidth(segs); got != len(want[i]) {
			t.Errorf("row %d: counted %d wide, drawn %d", i, got, len(want[i]))
		}
	}
	if m.maxBranchWidth != len(want[2]) {
		t.Errorf("the column is %d wide, the widest label %d", m.maxBranchWidth, len(want[1]))
	}
}
