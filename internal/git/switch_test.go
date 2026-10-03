package git

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

func TestSwitchTargets(t *testing.T) {
	tests := []struct {
		name, refs string
		want       []SwitchTarget
	}{
		{"a local branch", "HEAD -> refs/heads/main",
			[]SwitchTarget{{SwitchBranch, "main"}}},
		// The remote branch stands for the local one already listed.
		{"local and its remote", "HEAD -> refs/heads/main, refs/remotes/origin/main, refs/remotes/origin/HEAD",
			[]SwitchTarget{{SwitchBranch, "main"}}},
		{"only a remote branch", "refs/remotes/origin/feature/x",
			[]SwitchTarget{{SwitchRemote, "origin/feature/x"}}},
		{"two branches", "refs/heads/a, refs/heads/b, refs/remotes/origin/c",
			[]SwitchTarget{{SwitchBranch, "a"}, {SwitchBranch, "b"}, {SwitchRemote, "origin/c"}}},
		// Tags and the stash are not branches, so only the commit is left.
		{"a tag and the stash", "tag: refs/tags/v1, refs/stash",
			[]SwitchTarget{{SwitchDetached, "abc"}}},
		{"nothing", "", []SwitchTarget{{SwitchDetached, "abc"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SwitchTargets(tt.refs, "abc"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SwitchTargets(%q) = %+v, want %+v", tt.refs, got, tt.want)
			}
		})
	}
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	out, _ := Run(dir, "symbolic-ref", "-q", "--short", "HEAD")
	return out
}

func TestSwitchToALocalBranch(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("branch", "feature")
	commit("second")

	if detail, err := Switch(dir, SwitchTarget{SwitchBranch, "feature"}); err != nil {
		t.Fatalf("switch: %v (%s)", err, detail)
	}
	if got := currentBranch(t, dir); got != "feature" {
		t.Errorf("on %q, want feature", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "second")); !os.IsNotExist(err) {
		t.Error("the working tree still has main's file")
	}
}

func TestSwitchToARemoteBranchMakesATrackingBranch(t *testing.T) {
	dir, git, _, _ := pullRepo(t)
	git("push", "-q", "origin", "main:feature")
	git("fetch", "-q", "origin")

	if detail, err := Switch(dir, SwitchTarget{SwitchRemote, "origin/feature"}); err != nil {
		t.Fatalf("switch: %v (%s)", err, detail)
	}
	if got := currentBranch(t, dir); got != "feature" {
		t.Errorf("on %q, want feature", got)
	}
	if up, _ := Run(dir, "rev-parse", "--abbrev-ref", "@{upstream}"); up != "origin/feature" {
		t.Errorf("upstream = %q, want origin/feature", up)
	}
}

func TestSwitchDetached(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	first := head(t, dir)
	commit("second")

	if detail, err := Switch(dir, SwitchTarget{SwitchDetached, first}); err != nil {
		t.Fatalf("switch: %v (%s)", err, detail)
	}
	if got := currentBranch(t, dir); got != "" {
		t.Errorf("on %q, want no branch", got)
	}
	if got := head(t, dir); got != first {
		t.Errorf("HEAD = %s, want %s", got, first)
	}
}

// A change git could carry across is refused all the same: carried, it ends
// up committed on a branch it wasn't written for.
func TestSwitchRefusesWithUncommittedChanges(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("branch", "feature")
	if err := os.WriteFile(filepath.Join(dir, "first"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Switch(dir, SwitchTarget{SwitchBranch, "feature"})
	if !errors.Is(err, ErrLocalChanges) {
		t.Fatalf("err = %v, want ErrLocalChanges", err)
	}
	if got := currentBranch(t, dir); got != "main" {
		t.Errorf("on %q, want still main", got)
	}
}

// Untracked files belong to no branch, so they don't stand in the way.
func TestSwitchIgnoresUntrackedFiles(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("branch", "feature")
	if err := os.WriteFile(filepath.Join(dir, "scratch"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if detail, err := Switch(dir, SwitchTarget{SwitchBranch, "feature"}); err != nil {
		t.Fatalf("switch: %v (%s)", err, detail)
	}
	if got := currentBranch(t, dir); got != "feature" {
		t.Errorf("on %q, want feature", got)
	}
}
