package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pickRepo (cherry_pick_test.go) has feature and main each a commit of its
// own that never clash, so a merge of feature into main is a merge commit;
// stageRepo has given the commits gitraffe makes an author.

// ffRepo is a feature branch one commit ahead of main, with main checked
// out: a merge of feature can be a fast-forward.
func ffRepo(t *testing.T) (dir string, git func(...string), feature string) {
	t.Helper()
	dir, git = stageRepo(t)
	git("switch", "-q", "-c", "feature")
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 25\n", "line twenty-five\n", 1))
	git("commit", "-qam", "feature change")
	feature, _ = Run(dir, "rev-parse", "HEAD")
	git("switch", "-q", "main")
	return dir, git, feature
}

func parents(t *testing.T, dir string) []string {
	t.Helper()
	out, err := Run(dir, "rev-list", "--parents", "-n", "1", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(out)[1:]
}

func TestMergeFastForward(t *testing.T) {
	dir, _, feature := ffRepo(t)
	plan, err := MergePlan(dir, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != MergeFastForward || !plan.CanFastForward || plan.Conflicts {
		t.Errorf("plan = %+v, want a fast-forward", plan)
	}
	if plan.Message != "Merge branch 'feature'" {
		t.Errorf("message %q", plan.Message)
	}
	newHead, detail, err := Merge(dir, "feature", MergeOptions{})
	if err != nil {
		t.Fatalf("Merge: %v: %s", err, detail)
	}
	if newHead != feature {
		t.Errorf("HEAD is %s, want feature's commit %s", newHead, feature)
	}
	if branch, _ := Run(dir, "symbolic-ref", "--short", "HEAD"); branch != "main" {
		t.Errorf("HEAD is on %q, want main", branch)
	}
}

func TestMergeCommit(t *testing.T) {
	dir, _, feature := pickRepo(t)
	before, _ := Run(dir, "rev-parse", "HEAD")
	plan, err := MergePlan(dir, "refs/heads/feature")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != MergeCommit || plan.CanFastForward || plan.Conflicts {
		t.Errorf("plan = %+v, want a clean merge commit", plan)
	}
	if plan.Message != "Merge branch 'feature'" {
		t.Errorf("message %q", plan.Message)
	}
	newHead, detail, err := Merge(dir, "feature", MergeOptions{})
	if err != nil {
		t.Fatalf("Merge: %v: %s", err, detail)
	}
	if head, _ := Run(dir, "rev-parse", "HEAD"); head != newHead {
		t.Errorf("returned %s, HEAD is %s", newHead, head)
	}
	if got := parents(t, dir); len(got) != 2 || got[0] != before || got[1] != feature {
		t.Errorf("parents %v, want [%s %s]", got, before, feature)
	}
	if subject, _ := Run(dir, "log", "-1", "--format=%s"); subject != "Merge branch 'feature'" {
		t.Errorf("subject %q", subject)
	}
	want := strings.Replace(strings.Replace(numbered(30), "line 2\n", "line two\n", 1), "line 25\n", "line twenty-five\n", 1)
	if got := read(t, dir, "numbered.txt"); got != want {
		t.Errorf("numbered.txt is:\n%s\nwant both changes", got)
	}
}

// Outside main and master git says which branch was merged into, and a
// message of the caller's own replaces git's.
func TestMergeMessage(t *testing.T) {
	dir, git, _ := pickRepo(t)
	git("switch", "-q", "-c", "work")
	plan, err := MergePlan(dir, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Message != "Merge branch 'feature' into work" {
		t.Errorf("message %q", plan.Message)
	}
	if _, detail, err := Merge(dir, "feature", MergeOptions{Message: "bring in the feature"}); err != nil {
		t.Fatalf("Merge: %v: %s", err, detail)
	}
	if msg, _ := Run(dir, "log", "-1", "--format=%B"); msg != "bring in the feature" {
		t.Errorf("message %q", msg)
	}
}

func TestMergeNoFF(t *testing.T) {
	dir, _, feature := ffRepo(t)
	before, _ := Run(dir, "rev-parse", "HEAD")
	if _, detail, err := Merge(dir, "feature", MergeOptions{NoFF: true}); err != nil {
		t.Fatalf("Merge: %v: %s", err, detail)
	}
	if got := parents(t, dir); len(got) != 2 || got[0] != before || got[1] != feature {
		t.Errorf("parents %v, want a merge commit of [%s %s]", got, before, feature)
	}
}

func TestMergeRemoteBranch(t *testing.T) {
	dir, git, feature := pickRepo(t)
	git("update-ref", "refs/remotes/origin/feature", feature)
	git("branch", "-q", "-D", "feature")
	plan, err := MergePlan(dir, "refs/remotes/origin/feature")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Message != "Merge remote-tracking branch 'origin/feature'" || plan.Ref != "refs/remotes/origin/feature" {
		t.Errorf("plan = %+v", plan)
	}
	if _, detail, err := Merge(dir, "origin/feature", MergeOptions{}); err != nil {
		t.Fatalf("Merge: %v: %s", err, detail)
	}
	if got := parents(t, dir); len(got) != 2 || got[1] != feature {
		t.Errorf("parents %v", got)
	}
	if subject, _ := Run(dir, "log", "-1", "--format=%s"); subject != plan.Message {
		t.Errorf("subject %q", subject)
	}
}

// A branch and a tag of the same name: the ref the plan names is the branch,
// whatever git's guess at a short name would be.
func TestMergeNamesTheRefInFull(t *testing.T) {
	dir, git, feature := ffRepo(t)
	git("tag", "feature", "main")
	plan, err := MergePlan(dir, "refs/heads/feature")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Ref != "refs/heads/feature" || plan.Kind != MergeFastForward {
		t.Fatalf("plan = %+v", plan)
	}
	if newHead, detail, err := Merge(dir, "refs/heads/feature", MergeOptions{}); err != nil || newHead != feature {
		t.Fatalf("Merge = %s, %v: %s; want feature's commit", newHead, err, detail)
	}
}

func TestMergeFFSetting(t *testing.T) {
	t.Run("false makes a merge commit", func(t *testing.T) {
		dir, git, _ := ffRepo(t)
		git("config", "merge.ff", "false")
		plan, err := MergePlan(dir, "feature")
		if err != nil {
			t.Fatal(err)
		}
		if plan.Kind != MergeCommit || !plan.CanFastForward || plan.FastForward != FastForwardNever {
			t.Errorf("plan = %+v", plan)
		}
		if _, detail, err := Merge(dir, "feature", MergeOptions{}); err != nil {
			t.Fatalf("Merge: %v: %s", err, detail)
		}
		if got := parents(t, dir); len(got) != 2 {
			t.Errorf("parents %v, want a merge commit", got)
		}
	})
	t.Run("only fast-forwards", func(t *testing.T) {
		dir, git, feature := ffRepo(t)
		git("config", "merge.ff", "only")
		if newHead, detail, err := Merge(dir, "feature", MergeOptions{}); err != nil || newHead != feature {
			t.Fatalf("Merge = %s, %v: %s; want a fast-forward", newHead, err, detail)
		}
	})
	t.Run("only refuses a merge commit", func(t *testing.T) {
		dir, git, _ := pickRepo(t)
		git("config", "merge.ff", "only")
		head, _ := Run(dir, "rev-parse", "HEAD")
		plan, err := MergePlan(dir, "feature")
		if err != nil {
			t.Fatal(err)
		}
		if plan.FastForward != FastForwardOnly {
			t.Errorf("plan = %+v", plan)
		}
		if _, _, err := Merge(dir, "feature", MergeOptions{}); !errors.Is(err, ErrMergeNotFastForward) {
			t.Fatalf("Merge = %v, want %v", err, ErrMergeNotFastForward)
		}
		if after, _ := Run(dir, "rev-parse", "HEAD"); after != head {
			t.Error("HEAD moved")
		}
		// --no-ff is asked for in so many words, and wins, as on the
		// command line.
		if _, detail, err := Merge(dir, "feature", MergeOptions{NoFF: true}); err != nil {
			t.Fatalf("Merge with NoFF: %v: %s", err, detail)
		}
	})
}

// Every refusal leaves HEAD and the files as they were, and the repository
// in no merge.
func TestMergeRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, dir string, git func(...string), feature string) string
		want  error
	}{
		{"conflict", func(t *testing.T, dir string, git func(...string), feature string) string {
			write(t, dir, "numbered.txt", strings.Replace(read(t, dir, "numbered.txt"), "line 25\n", "line XXV\n", 1))
			git("commit", "-qam", "clash")
			return "feature"
		}, ErrMergeConflicts},
		{"up to date", func(t *testing.T, dir string, git func(...string), feature string) string {
			return "main~1"
		}, ErrMergeUpToDate},
		{"up to date after a merge", func(t *testing.T, dir string, git func(...string), feature string) string {
			git("merge", "-q", "--no-edit", "feature")
			return "feature"
		}, ErrMergeUpToDate},
		{"uncommitted changes", func(t *testing.T, dir string, git func(...string), feature string) string {
			write(t, dir, "numbered.txt", read(t, dir, "numbered.txt")+"more\n")
			return "feature"
		}, ErrLocalChanges},
		{"detached", func(t *testing.T, dir string, git func(...string), feature string) string {
			git("switch", "-q", "--detach")
			return "feature"
		}, ErrDetached},
		{"in progress", func(t *testing.T, dir string, git func(...string), feature string) string {
			// A cherry-pick stopped in a conflict.
			git("switch", "-q", "-c", "clash", "main~1")
			write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 25\n", "line XXV\n", 1))
			git("commit", "-qam", "clash")
			clash, _ := Run(dir, "rev-parse", "HEAD")
			git("switch", "-q", "feature")
			// It fails, which the fixture's git would not allow.
			cmd := exec.Command("git", "cherry-pick", clash)
			cmd.Dir = dir
			_ = cmd.Run()
			return "main"
		}, ErrInProgress},
		{"unrelated", func(t *testing.T, dir string, git func(...string), feature string) string {
			git("switch", "-q", "--orphan", "other")
			write(t, dir, "elsewhere.txt", "from another history\n")
			git("add", "elsewhere.txt")
			git("commit", "-qm", "unrelated root")
			git("switch", "-q", "main")
			return "other"
		}, ErrMergeUnrelated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, git, feature := pickRepo(t)
			target := tc.setup(t, dir, git, feature)
			head, _ := Run(dir, "rev-parse", "HEAD")
			file := read(t, dir, "numbered.txt")
			op := ReadWorkingState(dir).Operation

			_, detail, err := Merge(dir, target, MergeOptions{})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Merge = %v (%s), want %v", err, detail, tc.want)
			}
			if after, _ := Run(dir, "rev-parse", "HEAD"); after != head {
				t.Error("HEAD moved")
			}
			if got := read(t, dir, "numbered.txt"); got != file {
				t.Error("numbered.txt changed")
			}
			if after := ReadWorkingState(dir).Operation; after != op {
				t.Errorf("left in %q, was in %q", after, op)
			}
		})
	}
}

func TestMergeRefusesOnNoCommits(t *testing.T) {
	dir, git := stageRepo(t)
	git("switch", "-q", "--orphan", "empty")
	if _, _, err := Merge(dir, "main", MergeOptions{}); !errors.Is(err, ErrNoHead) {
		t.Errorf("Merge = %v, want %v", err, ErrNoHead)
	}
	if _, err := MergePlan(dir, "main"); !errors.Is(err, ErrNoHead) {
		t.Errorf("MergePlan = %v, want %v", err, ErrNoHead)
	}
}

func TestMergePlanSeesConflicts(t *testing.T) {
	dir, git, _ := pickRepo(t)
	write(t, dir, "numbered.txt", strings.Replace(read(t, dir, "numbered.txt"), "line 25\n", "line XXV\n", 1))
	git("commit", "-qam", "clash")
	plan, err := MergePlan(dir, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != MergeCommit || !plan.Conflicts {
		t.Errorf("plan = %+v, want a conflicting merge commit", plan)
	}
}

// A hook that stops the merge commit after the trial passed leaves no
// merge half done, and the hook is not skipped.
func TestMergeAbortsWhenAHookRefuses(t *testing.T) {
	dir, _, _ := pickRepo(t)
	head, _ := Run(dir, "rev-parse", "HEAD")
	hooks, _ := Run(dir, "rev-parse", "--git-path", "hooks")
	if !filepath.IsAbs(hooks) {
		hooks = filepath.Join(dir, hooks)
	}
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	hook := "#!/bin/sh\necho no merges today >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(hooks, "commit-msg"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}

	_, detail, err := Merge(dir, "feature", MergeOptions{})
	if err == nil {
		t.Fatal("Merge went ahead past a refusing commit-msg hook")
	}
	if !strings.Contains(detail, "no merges today") {
		t.Errorf("detail %q, want the hook's words", detail)
	}
	if after, _ := Run(dir, "rev-parse", "HEAD"); after != head {
		t.Error("HEAD moved")
	}
	if op := ReadWorkingState(dir).Operation; op != "" {
		t.Errorf("left in a %s", op)
	}
	if dirty, _ := LocalChanges(dir); dirty {
		t.Error("the working tree was left changed")
	}
}
