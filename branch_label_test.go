package main

import "testing"

func TestMergedBranchName(t *testing.T) {
	for _, tc := range []struct {
		subject, want string
	}{
		{"Merge pull request #33 from sevenam/add-tui-update", "add-tui-update"},
		{"Merge pull request #9 from andersosthus/feature/theme-support", "feature/theme-support"},
		{"Merge branch 'develop'", "develop"},
		{"Merge branch 'feature/foo' into main", "feature/foo"},
		{"Merge remote-tracking branch 'origin/hotfix'", "origin/hotfix"},

		// Nothing recoverable: squash/rebase merges and ordinary commits leave
		// no branch name behind.
		{"add loan support", ""},
		{"Update README (#42)", ""},
		{"Merge pull request #7", ""},
		{"Merge branch", ""},
	} {
		if got := mergedBranchName(tc.subject); got != tc.want {
			t.Errorf("mergedBranchName(%q) = %q, want %q", tc.subject, got, tc.want)
		}
	}
}

func TestLabelMergedBranches(t *testing.T) {
	t.Run("names the merge commit's second parent", func(t *testing.T) {
		m := model{commits: []commit{
			{Hash: "aaaaaaa", Message: "Merge pull request #1 from sevenam/feature-x",
				Parents: []string{"bbbbbbb", "ccccccc"}},
			{Hash: "bbbbbbb", Message: "earlier work on main"},
			{Hash: "ccccccc", Message: "last commit on feature-x"},
		}}
		m.labelMergedBranches()

		// The tip of the merged branch, not the merge commit or the main line.
		if got := m.commits[2].MergedBranch; got != "feature-x" {
			t.Errorf("second parent label = %q, want feature-x", got)
		}
		if got := m.commits[1].MergedBranch; got != "" {
			t.Errorf("first parent was labelled %q, want empty", got)
		}
		if got := m.commits[0].MergedBranch; got != "" {
			t.Errorf("merge commit was labelled %q, want empty", got)
		}
	})

	t.Run("skips branches that still exist", func(t *testing.T) {
		m := model{commits: []commit{
			{Hash: "aaaaaaa", Message: "Merge pull request #1 from sevenam/feature-x",
				Parents: []string{"bbbbbbb", "ccccccc"}},
			{Hash: "bbbbbbb", Message: "earlier work on main"},
			{Hash: "ccccccc", Message: "tip", Refs: "origin/feature-x"},
		}}
		m.labelMergedBranches()

		if got := m.commits[2].MergedBranch; got != "" {
			t.Errorf("labelled %q, want empty: the ref already names it", got)
		}
	})

	t.Run("ignores non-merge commits and missing parents", func(t *testing.T) {
		m := model{commits: []commit{
			{Hash: "aaaaaaa", Message: "Merge branch 'gone'", Parents: []string{"bbbbbbb"}},
			{Hash: "bbbbbbb", Message: "Merge branch 'absent'", Parents: []string{"aaaaaaa", "ddddddd"}},
		}}
		m.labelMergedBranches()

		for i, c := range m.commits {
			if c.MergedBranch != "" {
				t.Errorf("commit %d labelled %q, want empty", i, c.MergedBranch)
			}
		}
	})
}

func TestLabelTextMatchesRenderedSegments(t *testing.T) {
	// labelText sizes the column and labelSegments fills it; if they disagree on
	// order or content the column truncates or misaligns.
	for _, tc := range []struct {
		name string
		c    commit
		want string
	}{
		{"branch only", commit{Refs: "main"}, "main"},
		{"branch and tag", commit{Refs: "main, tag: v1.0"}, "main, v1.0"},
		{"merged only", commit{MergedBranch: "feature-x"}, "feature-x"},
		{"all three", commit{Refs: "main, tag: v1.0", MergedBranch: "feature-x"}, "main, v1.0, feature-x"},
		{"nothing", commit{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.labelText(); got != tc.want {
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
