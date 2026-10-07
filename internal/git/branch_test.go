package git

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

func TestCreateBranchStartsAtHeadAndSwitches(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	commit("second")
	head, _ := Run(dir, "rev-parse", "HEAD")

	if detail, err := CreateBranch(dir, "feature/x"); err != nil {
		t.Fatalf("create: %v (%s)", err, detail)
	}
	if got := currentBranch(t, dir); got != "feature/x" {
		t.Errorf("on %q, want feature/x", got)
	}
	if got, _ := Run(dir, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD = %s, want it left at %s", got, head)
	}
	if got, _ := Run(dir, "rev-parse", "main"); got != head {
		t.Errorf("main = %s, want it left at %s", got, head)
	}
}

// Nothing is refused over uncommitted changes, staged or not: no file is
// touched, so they are as they were on the new branch.
func TestCreateBranchKeepsUncommittedChanges(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	commit("second")
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("first", "staged")
	git("add", "first")
	write("second", "not staged")
	write("scratch", "untracked")
	before, _ := Run(dir, "status", "--porcelain")

	if detail, err := CreateBranch(dir, "feature"); err != nil {
		t.Fatalf("create: %v (%s)", err, detail)
	}
	if got := currentBranch(t, dir); got != "feature" {
		t.Errorf("on %q, want feature", got)
	}
	if after, _ := Run(dir, "status", "--porcelain"); after != before {
		t.Errorf("status changed:\n%s\nwas:\n%s", after, before)
	}
}

// The way back onto a branch from a detached HEAD.
func TestCreateBranchFromADetachedHead(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	first, _ := Run(dir, "rev-parse", "HEAD")
	commit("second")
	git("switch", "-q", "--detach", first)

	if detail, err := CreateBranch(dir, "from-first"); err != nil {
		t.Fatalf("create: %v (%s)", err, detail)
	}
	if got := currentBranch(t, dir); got != "from-first" {
		t.Errorf("on %q, want from-first", got)
	}
	if got, _ := Run(dir, "rev-parse", "HEAD"); got != first {
		t.Errorf("HEAD = %s, want %s", got, first)
	}
}

func TestCreateBranchRefusals(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("branch", "taken")

	for _, tc := range []struct {
		name string
		want error
	}{
		{"taken", ErrBranchExists},
		{"main", ErrBranchExists},
		{"", ErrBadBranchName},
		{"two words", ErrBadBranchName},
		{"-c", ErrBadBranchName},
		{"a..b", ErrBadBranchName},
	} {
		if _, err := CreateBranch(dir, tc.name); !errors.Is(err, tc.want) {
			t.Errorf("CreateBranch(%q) = %v, want %v", tc.name, err, tc.want)
		}
		if got := currentBranch(t, dir); got != "main" {
			t.Fatalf("on %q after a refused %q, want still main", got, tc.name)
		}
	}
}

// A merge belongs to the branch it was started on.
func TestCreateBranchRefusedDuringAMerge(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("switch", "-q", "-c", "other")
	commit("on other")
	git("switch", "-q", "main")
	commit("on main")
	git("merge", "-q", "--no-commit", "--no-ff", "other")

	if _, err := CreateBranch(dir, "feature"); !errors.Is(err, ErrInProgress) {
		t.Fatalf("err = %v, want ErrInProgress", err)
	}
	if BranchExists(dir, "feature") {
		t.Error("the branch was made all the same")
	}
}

// Whatever the config says about new branches, this one tracks nothing: an
// inherited upstream is where the next push would go.
func TestCreateBranchTracksNothing(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("init", "-q", "--bare", "origin.git")
	git("remote", "add", "origin", filepath.Join(dir, "origin.git"))
	git("push", "-q", "-u", "origin", "main")
	git("config", "branch.autoSetupMerge", "inherit")

	if detail, err := CreateBranch(dir, "feature"); err != nil {
		t.Fatalf("create: %v (%s)", err, detail)
	}
	if up, err := Run(dir, "rev-parse", "--abbrev-ref", "feature@{upstream}"); err == nil {
		t.Errorf("feature tracks %s, want nothing", up)
	}
}
