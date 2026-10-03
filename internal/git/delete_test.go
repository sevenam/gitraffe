package git

import (
	"errors"
	"os/exec"
	"reflect"
	"testing"
)

func TestDeleteTargets(t *testing.T) {
	tests := []struct {
		name, refs string
		want       []DeleteTarget
	}{
		{"a local branch", "refs/heads/feature", []DeleteTarget{{Local: "feature"}}},
		{"local and its remote", "HEAD -> refs/heads/main, refs/remotes/origin/main, refs/remotes/origin/HEAD",
			[]DeleteTarget{{Local: "main", Remote: "origin/main"}}},
		{"only a remote branch", "refs/remotes/origin/feature/x",
			[]DeleteTarget{{Remote: "origin/feature/x"}}},
		// The second remote's copy is its own target: one push deletes one.
		{"one branch on two remotes", "refs/heads/a, refs/remotes/fork/a, refs/remotes/origin/a",
			[]DeleteTarget{{Local: "a", Remote: "fork/a"}, {Remote: "origin/a"}}},
		{"two branches", "refs/heads/a, refs/heads/b, refs/remotes/origin/b",
			[]DeleteTarget{{Local: "a"}, {Local: "b", Remote: "origin/b"}}},
		{"a tag and the stash", "tag: refs/tags/v1, refs/stash", nil},
		{"nothing", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeleteTargets(tt.refs); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DeleteTargets(%q) = %+v, want %+v", tt.refs, got, tt.want)
			}
		})
	}
}

func hasRef(dir, ref string) bool {
	_, err := Run(dir, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}

// originOf is the bare repository pullRepo pushes to.
func originOf(t *testing.T, dir string) string {
	t.Helper()
	out, err := Run(dir, "remote", "get-url", "origin")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDeleteALocalBranchWhoseCommitsAreOnAnother(t *testing.T) {
	dir, git, commit, _ := pullRepo(t)
	git("branch", "feature")
	commit("second")

	r, err := DeleteBranch(dir, DeleteTarget{Local: "feature"}, DeleteLocal)
	if err != nil || !r.Local || r.Remote {
		t.Fatalf("result = %+v, err = %v (%s), want the local branch deleted", r, err, r.Detail)
	}
	if hasRef(dir, "refs/heads/feature") {
		t.Error("feature is still there")
	}
}

// The commits on it would be on no branch at all afterwards.
func TestDeleteRefusesTheOnlyCopy(t *testing.T) {
	dir, git, commit, _ := pullRepo(t)
	git("switch", "-q", "-c", "feature")
	commit("on feature")
	git("switch", "-q", "main")

	if _, err := DeleteBranch(dir, DeleteTarget{Local: "feature"}, DeleteLocal); !errors.Is(err, ErrOnlyCopy) {
		t.Fatalf("err = %v, want ErrOnlyCopy", err)
	}
	if !hasRef(dir, "refs/heads/feature") {
		t.Fatal("feature was deleted")
	}

	// Pushed, the remote branch holds them, so the local one can go; the two
	// together cannot.
	git("push", "-q", "origin", "feature")
	target := DeleteTarget{Local: "feature", Remote: "origin/feature"}
	if _, err := DeleteBranch(dir, target, DeleteBoth); !errors.Is(err, ErrOnlyCopy) {
		t.Fatalf("both: err = %v, want ErrOnlyCopy", err)
	}
	if !hasRef(dir, "refs/heads/feature") || !hasRef(originOf(t, dir), "refs/heads/feature") {
		t.Fatal("a refused delete removed something")
	}
	if r, err := DeleteBranch(dir, target, DeleteLocal); err != nil || !r.Local {
		t.Fatalf("local: result = %+v, err = %v (%s)", r, err, r.Detail)
	}
	if hasRef(dir, "refs/heads/feature") || !hasRef(dir, "refs/remotes/origin/feature") {
		t.Error("want only the local branch gone")
	}
}

func TestDeleteLocalAndRemote(t *testing.T) {
	dir, git, commit, _ := pullRepo(t)
	git("branch", "feature/x")
	git("push", "-q", "origin", "feature/x")
	commit("second")

	target := DeleteTarget{Local: "feature/x", Remote: "origin/feature/x"}
	r, err := DeleteBranch(dir, target, DeleteBoth)
	if err != nil || !r.Local || !r.Remote {
		t.Fatalf("result = %+v, err = %v (%s), want both deleted", r, err, r.Detail)
	}
	if hasRef(dir, "refs/heads/feature/x") || hasRef(dir, "refs/remotes/origin/feature/x") {
		t.Error("a ref is left in the repository")
	}
	if hasRef(originOf(t, dir), "refs/heads/feature/x") {
		t.Error("the branch is still on the remote")
	}
}

func TestDeleteRemoteOnlyLeavesTheLocalBranch(t *testing.T) {
	dir, git, _, _ := pullRepo(t)
	git("switch", "-q", "-c", "feature")
	git("push", "-q", "-u", "origin", "feature")

	// The branch checked out can lose its remote copy: the local one holds
	// the commits.
	r, err := DeleteBranch(dir, DeleteTarget{Local: "feature", Remote: "origin/feature"}, DeleteRemote)
	if err != nil || r.Local || !r.Remote {
		t.Fatalf("result = %+v, err = %v (%s), want only the remote deleted", r, err, r.Detail)
	}
	if !hasRef(dir, "refs/heads/feature") || hasRef(originOf(t, dir), "refs/heads/feature") {
		t.Error("want the local branch kept and the remote one gone")
	}
}

func TestDeleteRefusesTheCurrentBranch(t *testing.T) {
	dir, git, _, _ := pullRepo(t)
	git("branch", "feature")

	if _, err := DeleteBranch(dir, DeleteTarget{Local: "main"}, DeleteLocal); !errors.Is(err, ErrCurrentBranch) {
		t.Fatalf("err = %v, want ErrCurrentBranch", err)
	}
	if !hasRef(dir, "refs/heads/main") {
		t.Error("main was deleted")
	}
}

// Someone pushed to the branch since the last fetch: what would be deleted is
// not what the graph shows.
func TestDeleteRefusesARemoteBranchThatMoved(t *testing.T) {
	dir, git, commit, _ := pullRepo(t)
	git("branch", "feature")
	git("push", "-q", "origin", "feature")
	commit("second")
	git("push", "-q", "origin", "main")

	origin := originOf(t, dir)
	if out, err := exec.Command("git", "-C", origin, "update-ref", "refs/heads/feature", "refs/heads/main").CombinedOutput(); err != nil {
		t.Fatalf("update-ref: %v\n%s", err, out)
	}

	target := DeleteTarget{Local: "feature", Remote: "origin/feature"}
	r, err := DeleteBranch(dir, target, DeleteBoth)
	if !errors.Is(err, ErrRemoteMoved) || r.Local || r.Remote {
		t.Fatalf("result = %+v, err = %v (%s), want ErrRemoteMoved and nothing deleted", r, err, r.Detail)
	}
	if !hasRef(dir, "refs/heads/feature") || !hasRef(origin, "refs/heads/feature") {
		t.Error("a refused delete removed something")
	}
}
