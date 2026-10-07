package git

import (
	"path/filepath"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

func TestNewPullRequestURL(t *testing.T) {
	for _, tc := range []struct{ remote, branch, want string }{
		{"git@github.com:sevenam/gitraffe.git", "feature",
			"https://github.com/sevenam/gitraffe/compare/feature?expand=1"},
		// The slashes of a branch name are part of the address.
		{"https://github.com/sevenam/gitraffe.git", "fix/sign-flip",
			"https://github.com/sevenam/gitraffe/compare/fix/sign-flip?expand=1"},
		// A character that would end the path must not.
		{"https://github.com/sevenam/gitraffe.git", "issue#12/what?",
			"https://github.com/sevenam/gitraffe/compare/issue%2312/what%3F?expand=1"},
		{azureRemote, "fix/sign-flip",
			"https://dev.azure.com/sevenam/My%20Project/_git/gitraffe/pullrequestcreate?sourceRef=fix%2Fsign-flip"},
		{"git@ssh.dev.azure.com:v3/sevenam/tools/gitraffe", "feature",
			"https://dev.azure.com/sevenam/tools/_git/gitraffe/pullrequestcreate?sourceRef=feature"},
		{"/home/me/repo", "feature", ""},
		{"git@github.com:sevenam/gitraffe.git", "", ""},
	} {
		if got := NewPullRequestURL(tc.remote, tc.branch); got != tc.want {
			t.Errorf("NewPullRequestURL(%q, %q) = %q, want %q", tc.remote, tc.branch, got, tc.want)
		}
	}
}

// refsOf is a commit's refs as the log reads them.
func refsOf(t *testing.T, dir, rev string) (hash, refs string) {
	t.Helper()
	hash, _ = Run(dir, "rev-parse", rev)
	refs, err := Run(dir, "log", "-1", "--decorate=full", "--format=%D", rev)
	if err != nil {
		t.Fatal(err)
	}
	return hash, refs
}

// newPRRepo is a clone-like repository: main pushed to a bare origin, and a
// helper to ask about the commit a revision names.
func newPRRepo(t *testing.T) (dir string, git func(...string), commit func(string), ask func(rev, current string) NewPullRequest) {
	t.Helper()
	dir, git, commit = gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	origin := filepath.Join(t.TempDir(), "origin.git")
	git("init", "-q", "--bare", origin)
	git("remote", "add", "origin", origin)
	git("push", "-q", "-u", "origin", "main")
	ask = func(rev, current string) NewPullRequest {
		t.Helper()
		hash, refs := refsOf(t, dir, rev)
		return NewPullRequestFor(dir, hash, refs, current)
	}
	return dir, git, commit, ask
}

func TestNewPullRequestForAPushedBranch(t *testing.T) {
	_, git, commit, ask := newPRRepo(t)
	git("switch", "-q", "-c", "feature")
	commit("on feature")
	git("push", "-q", "origin", "feature")

	if got := ask("feature", "feature"); got != (NewPullRequest{Branch: "feature"}) {
		t.Errorf("got %+v, want the pushed branch", got)
	}

	// Commits made since the push leave origin/feature a commit behind; the
	// branch is still on the remote, and the page is still its page.
	commit("more on feature")
	if got := ask("feature", "feature"); got != (NewPullRequest{Branch: "feature"}) {
		t.Errorf("ahead of the remote: got %+v, want the branch all the same", got)
	}
	// The commit the remote's label is on has no local branch on it now.
	if got := ask("origin/feature", "feature"); got != (NewPullRequest{Branch: "feature"}) {
		t.Errorf("on the remote's commit: got %+v, want the branch", got)
	}
}

func TestNewPullRequestForABranchNotPushed(t *testing.T) {
	_, git, commit, ask := newPRRepo(t)
	git("switch", "-q", "-c", "local-only")
	commit("on local-only")

	if got := ask("local-only", "local-only"); got != (NewPullRequest{Unpushed: "local-only"}) {
		t.Errorf("got %+v, want the branch named as not pushed", got)
	}
}

// What the default branch holds has nothing left to pull, whatever other
// branch is sitting on it; and the default branch is never the source.
func TestNewPullRequestNeverFromWhatTheDefaultBranchHolds(t *testing.T) {
	_, git, commit, ask := newPRRepo(t)
	git("branch", "stale")
	git("push", "-q", "origin", "stale")
	if got := ask("main", "main"); got != (NewPullRequest{}) {
		t.Errorf("on main's own commit: got %+v, want nothing", got)
	}

	// Ahead of origin/main, and so not held by it, but still the default.
	commit("second")
	if got := ask("main", "main"); got != (NewPullRequest{}) {
		t.Errorf("on unpushed main: got %+v, want nothing", got)
	}
}

// The remote's own record of its default branch is believed over the names
// one usually has.
func TestNewPullRequestReadsTheRemotesDefaultBranch(t *testing.T) {
	_, git, commit, ask := newPRRepo(t)
	git("switch", "-q", "-c", "develop")
	commit("on develop")
	git("push", "-q", "origin", "develop")
	if got := ask("develop", "develop"); got != (NewPullRequest{Branch: "develop"}) {
		t.Fatalf("with no record: got %+v, want develop offered", got)
	}

	git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
	if got := ask("develop", "develop"); got != (NewPullRequest{}) {
		t.Errorf("develop as the default: got %+v, want nothing", got)
	}
	// And main is then a branch like any other.
	git("switch", "-q", "main")
	commit("on main")
	git("push", "-q", "origin", "main")
	if got := ask("main", "main"); got != (NewPullRequest{Branch: "main"}) {
		t.Errorf("main beside a default of develop: got %+v, want main offered", got)
	}
}

// Of several branches on a commit the one checked out is meant, and failing
// that the same one every time.
func TestNewPullRequestPrefersTheBranchCheckedOut(t *testing.T) {
	_, git, commit, ask := newPRRepo(t)
	git("switch", "-q", "-c", "b-branch")
	commit("work")
	git("branch", "a-branch")
	git("branch", "c-local")
	git("push", "-q", "origin", "a-branch", "b-branch")

	if got := ask("b-branch", "b-branch"); got.Branch != "b-branch" {
		t.Errorf("got %+v, want the branch checked out", got)
	}
	if got := ask("b-branch", "main"); got.Branch != "a-branch" {
		t.Errorf("got %+v, want the first by name", got)
	}
	// A pushed branch is offered before one that is not, even checked out.
	if got := ask("b-branch", "c-local"); got.Branch != "a-branch" || got.Unpushed != "" {
		t.Errorf("got %+v, want a branch the remote has", got)
	}
}

func TestNewPullRequestWithNoBranchOrNoRemote(t *testing.T) {
	_, git, commit, ask := newPRRepo(t)
	git("switch", "-q", "-c", "feature")
	commit("one")
	commit("two")
	git("push", "-q", "origin", "feature")
	if got := ask("feature~1", "feature"); got != (NewPullRequest{}) {
		t.Errorf("a commit under the branch's end: got %+v, want nothing", got)
	}

	dir, run, make := gittest.Fixture(t)
	run("init", "-q", "-b", "main")
	make("first")
	run("switch", "-q", "-c", "feature")
	make("second")
	hash, refs := refsOf(t, dir, "feature")
	if got := NewPullRequestFor(dir, hash, refs, "feature"); got != (NewPullRequest{}) {
		t.Errorf("with no remote: got %+v, want nothing", got)
	}
}
