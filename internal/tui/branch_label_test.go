package tui

import "testing"

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
