package git

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

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
		commits := []Commit{
			{Hash: "aaaaaaa", Message: "Merge pull request #1 from sevenam/feature-x",
				Parents: []string{"bbbbbbb", "ccccccc"}},
			{Hash: "bbbbbbb", Message: "earlier work on main"},
			{Hash: "ccccccc", Message: "last commit on feature-x"},
		}
		labelMergedBranches(commits)

		// The tip of the merged branch, not the merge commit or the main line.
		if got := commits[2].MergedBranch; got != "feature-x" {
			t.Errorf("second parent label = %q, want feature-x", got)
		}
		if got := commits[1].MergedBranch; got != "" {
			t.Errorf("first parent was labelled %q, want empty", got)
		}
		if got := commits[0].MergedBranch; got != "" {
			t.Errorf("merge commit was labelled %q, want empty", got)
		}
	})

	t.Run("skips branches that still exist", func(t *testing.T) {
		for _, refs := range []string{
			"refs/remotes/origin/feature-x",
			"refs/heads/feature-x",
		} {
			commits := []Commit{
				{Hash: "aaaaaaa", Message: "Merge pull request #1 from sevenam/feature-x",
					Parents: []string{"bbbbbbb", "ccccccc"}},
				{Hash: "bbbbbbb", Message: "earlier work on main"},
				{Hash: "ccccccc", Message: "tip", Refs: refs},
			}
			labelMergedBranches(commits)

			if got := commits[2].MergedBranch; got != "" {
				t.Errorf("with refs %q: labelled %q, want empty — the ref already names it", refs, got)
			}
		}
	})

	t.Run("ignores non-merge commits and missing parents", func(t *testing.T) {
		commits := []Commit{
			{Hash: "aaaaaaa", Message: "Merge branch 'gone'", Parents: []string{"bbbbbbb"}},
			{Hash: "bbbbbbb", Message: "Merge branch 'absent'", Parents: []string{"aaaaaaa", "ddddddd"}},
		}
		labelMergedBranches(commits)

		for i, c := range commits {
			if c.MergedBranch != "" {
				t.Errorf("commit %d labelled %q, want empty", i, c.MergedBranch)
			}
		}
	})
}

func TestParseRefs(t *testing.T) {
	for _, tc := range []struct {
		name, refs          string
		local, remote, tags []string
	}{
		{
			name: "checked-out branch with its remote",
			refs: "HEAD -> refs/heads/main, refs/remotes/origin/main, refs/remotes/origin/HEAD",
			// origin/HEAD is a symref duplicating origin/main, so it is dropped.
			local: []string{"main"}, remote: []string{"origin/main"},
		},
		{
			name:  "slashes do not make a local branch look remote",
			refs:  "refs/heads/feature/theme-support",
			local: []string{"feature/theme-support"},
		},
		{
			name:   "remote-only branch",
			refs:   "refs/remotes/origin/show-more-branch-info",
			remote: []string{"origin/show-more-branch-info"},
		},
		{
			name:   "multiple remotes on one commit",
			refs:   "refs/heads/main, refs/remotes/origin/main, refs/remotes/upstream/main",
			local:  []string{"main"},
			remote: []string{"origin/main", "upstream/main"},
		},
		{"tag", "tag: refs/tags/v1.0", nil, nil, []string{"v1.0"}},
		{"detached head", "HEAD", nil, nil, nil},
		{"empty", "", nil, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local, remote, tags := ParseRefs(tc.refs)
			eq := func(what string, got, want []string) {
				if len(got) != len(want) {
					t.Errorf("%s = %v, want %v", what, got, want)
					return
				}
				for i := range got {
					if got[i] != want[i] {
						t.Errorf("%s = %v, want %v", what, got, want)
						return
					}
				}
			}
			eq("local", local, tc.local)
			eq("remote", remote, tc.remote)
			eq("tags", tags, tc.tags)
		})
	}
}

// The depth has to be counted in the order the graph is drawn in: git log
// --graph implies --topo-order, and the date order git otherwise uses puts
// commits somewhere else.
func TestCommitDepthMatchesTheGraphsOrder(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("checkout", "-qb", "side")
	commit("on the side")
	git("checkout", "-q", "main")
	commit("on main")
	git("merge", "-q", "--no-ff", "side", "-m", "merge the side")

	out, err := exec.Command("git", "-C", dir, "log", "--all", "--topo-order", "--format=%H").Output()
	if err != nil {
		t.Fatal(err)
	}
	order := strings.Fields(strings.ReplaceAll(string(out), "\r", ""))

	for want, hash := range order {
		if got := CommitDepth(dir, hash); got != want {
			t.Errorf("CommitDepth(%s) = %d, want %d", hash[:7], got, want)
		}
	}
	if got := CommitDepth(dir, strings.Repeat("0", 40)); got != -1 {
		t.Errorf("CommitDepth of a commit that isn't there = %d, want -1", got)
	}
}
