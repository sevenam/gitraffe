package git

import (
	"errors"
	"strings"
	"testing"
)

// pickRepo is a main branch and a feature branch off it, each with a commit
// of its own, with main checked out. The feature branch's commit changes
// line 25 of numbered.txt; main's changes line 2, so the two never clash.
func pickRepo(t *testing.T) (dir string, git func(...string), feature string) {
	t.Helper()
	dir, git = stageRepo(t)
	git("switch", "-q", "-c", "feature")
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 25\n", "line twenty-five\n", 1))
	git("commit", "-qam", "feature change")
	feature, _ = Run(dir, "rev-parse", "HEAD")
	git("switch", "-q", "main")
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "line two\n", 1))
	git("commit", "-qam", "main change")
	return dir, git, feature
}

func TestCherryPick(t *testing.T) {
	dir, _, feature := pickRepo(t)
	before, _ := Run(dir, "rev-parse", "HEAD")

	newHash, detail, err := CherryPick(dir, feature)
	if err != nil {
		t.Fatalf("CherryPick: %v: %s", err, detail)
	}
	if head, _ := Run(dir, "rev-parse", "HEAD"); newHash != head || newHash == feature {
		t.Errorf("returned %s, HEAD is %s; want the new commit", newHash, head)
	}
	if parent, _ := Run(dir, "rev-parse", "HEAD^"); parent != before {
		t.Errorf("the new commit's parent is %s, want the old HEAD %s", parent, before)
	}
	if subject, _ := Run(dir, "log", "-1", "--format=%s"); subject != "feature change" {
		t.Errorf("the new commit is called %q, want the original's message", subject)
	}
	want := strings.Replace(strings.Replace(numbered(30), "line 2\n", "line two\n", 1), "line 25\n", "line twenty-five\n", 1)
	if got := read(t, dir, "numbered.txt"); got != want {
		t.Errorf("numbered.txt is:\n%s\nwant both changes", got)
	}
	if branch, _ := Run(dir, "symbolic-ref", "--short", "HEAD"); branch != "main" {
		t.Errorf("HEAD is on %q, want main still", branch)
	}
}

// Every refusal leaves HEAD, the files and the index as they were, and the
// repository in no cherry-pick.
func TestCherryPickRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, dir string, git func(...string), feature string) string
		want  error
	}{
		{"conflict", func(t *testing.T, dir string, git func(...string), feature string) string {
			write(t, dir, "numbered.txt", strings.Replace(read(t, dir, "numbered.txt"), "line 25\n", "line XXV\n", 1))
			git("commit", "-qam", "clash")
			return feature
		}, ErrCherryPickConflicts},
		{"already there", func(t *testing.T, dir string, git func(...string), feature string) string {
			write(t, dir, "numbered.txt", strings.Replace(read(t, dir, "numbered.txt"), "line 25\n", "line twenty-five\n", 1))
			git("commit", "-qam", "same change by hand")
			return feature
		}, ErrCherryPickEmpty},
		{"on the branch", func(t *testing.T, dir string, git func(...string), feature string) string {
			first, _ := Run(dir, "rev-parse", "HEAD^")
			return first
		}, ErrCherryPickOnBranch},
		{"merge commit", func(t *testing.T, dir string, git func(...string), feature string) string {
			git("switch", "-q", "-c", "merged", feature)
			git("merge", "-q", "--no-edit", "--no-ff", "main")
			merge, _ := Run(dir, "rev-parse", "HEAD")
			git("switch", "-q", "main")
			return merge
		}, ErrCherryPickMerge},
		{"root commit", func(t *testing.T, dir string, git func(...string), feature string) string {
			// main's own first commit is already on the branch; a root
			// commit of another history is what reaches the check.
			git("switch", "-q", "--orphan", "other")
			write(t, dir, "elsewhere.txt", "from another history\n")
			git("add", "elsewhere.txt")
			git("commit", "-qm", "unrelated root")
			root, _ := Run(dir, "rev-parse", "HEAD")
			git("switch", "-q", "main")
			return root
		}, ErrCherryPickRoot},
		{"uncommitted changes", func(t *testing.T, dir string, git func(...string), feature string) string {
			write(t, dir, "numbered.txt", read(t, dir, "numbered.txt")+"more\n")
			return feature
		}, ErrLocalChanges},
		{"detached", func(t *testing.T, dir string, git func(...string), feature string) string {
			git("switch", "-q", "--detach")
			return feature
		}, ErrDetached},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, git, feature := pickRepo(t)
			target := tc.setup(t, dir, git, feature)
			head, _ := Run(dir, "rev-parse", "HEAD")
			file := read(t, dir, "numbered.txt")

			_, detail, err := CherryPick(dir, target)
			if !errors.Is(err, tc.want) {
				t.Fatalf("CherryPick = %v (%s), want %v", err, detail, tc.want)
			}
			if after, _ := Run(dir, "rev-parse", "HEAD"); after != head {
				t.Error("HEAD moved")
			}
			if got := read(t, dir, "numbered.txt"); got != file {
				t.Error("numbered.txt changed")
			}
			if op := ReadWorkingState(dir).Operation; op != "" {
				t.Errorf("left in a %s", op)
			}
		})
	}
}

// Whatever stops git after the trial passed leaves no cherry-pick half done.
func TestCherryPickAbortsWhenGitStops(t *testing.T) {
	dir, git, _ := pickRepo(t)
	head, _ := Run(dir, "rev-parse", "HEAD")
	// A file of the commit's, untracked here, is something git itself will
	// not overwrite; the trial in the object store cannot see it.
	git("switch", "-q", "feature")
	write(t, dir, "extra.txt", "tracked on feature\n")
	git("add", "extra.txt")
	git("commit", "-qm", "adds extra")
	extra, _ := Run(dir, "rev-parse", "HEAD")
	git("switch", "-q", "main")
	write(t, dir, "extra.txt", "mine, untracked\n")

	if _, _, err := CherryPick(dir, extra); err == nil {
		t.Fatal("CherryPick went ahead over an untracked file")
	}
	if after, _ := Run(dir, "rev-parse", "HEAD"); after != head {
		t.Error("HEAD moved")
	}
	if got := read(t, dir, "extra.txt"); got != "mine, untracked\n" {
		t.Error("the untracked file was overwritten")
	}
	if op := ReadWorkingState(dir).Operation; op != "" {
		t.Errorf("left in a %s", op)
	}
}
