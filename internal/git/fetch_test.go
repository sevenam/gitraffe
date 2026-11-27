package git

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// A --single-branch clone is configured to fetch one branch; Fetch still
// brings in the others, and prunes one deleted on the server (issue #258).
func TestFetchBringsEveryBranchOfANarrowedRemote(t *testing.T) {
	src, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("branch", "old")

	clone := filepath.Join(t.TempDir(), "clone")
	if out, err := exec.Command("git", "clone", "-q", "--no-local", "--single-branch", "-b", "main", src, clone).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	// Fetched once with the refspec, so it is there to be pruned later.
	if out, err := exec.Command("git", "-C", clone, "fetch", "-q", "origin", "+refs/heads/old:refs/remotes/origin/old").CombinedOutput(); err != nil {
		t.Fatalf("git fetch old: %v\n%s", err, out)
	}

	git("branch", "feature")
	git("branch", "-D", "old")

	if detail, err := Fetch(clone); err != nil {
		t.Fatalf("Fetch: %v (%s)", err, detail)
	}
	refs, _ := Run(clone, "for-each-ref", "--format=%(refname)", "refs/remotes/origin")
	if !strings.Contains(refs, "refs/remotes/origin/feature") {
		t.Errorf("remote refs = %q, want origin/feature fetched despite the one-branch config", refs)
	}
	if strings.Contains(refs, "refs/remotes/origin/old") {
		t.Errorf("remote refs = %q, want origin/old pruned", refs)
	}
}

// One remote failing does not stop the others, and the failure is reported.
func TestFetchCarriesOnPastABrokenRemote(t *testing.T) {
	src, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")

	dir, git2, commit2 := gittest.Fixture(t)
	git2("init", "-q", "-b", "main")
	commit2("mine")
	git2("remote", "add", "a-gone", filepath.Join(t.TempDir(), "nowhere"))
	git2("remote", "add", "b-good", src)

	_, err := Fetch(dir)
	if err == nil {
		t.Error("a remote that isn't there was not reported")
	}
	if refs, _ := Run(dir, "for-each-ref", "--format=%(refname)", "refs/remotes/b-good"); !strings.Contains(refs, "refs/remotes/b-good/main") {
		t.Errorf("remote refs = %q, want b-good fetched after a-gone failed", refs)
	}
}

// skipFetchAll is honoured, as fetch --all honoured it.
func TestFetchSkipsSkipFetchAll(t *testing.T) {
	src, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")

	dir, git2, commit2 := gittest.Fixture(t)
	git2("init", "-q", "-b", "main")
	commit2("mine")
	git2("remote", "add", "skipped", src)
	git2("config", "remote.skipped.skipFetchAll", "true")

	if _, err := Fetch(dir); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if refs, _ := Run(dir, "for-each-ref", "refs/remotes/skipped"); refs != "" {
		t.Errorf("remote refs = %q, want the skipFetchAll remote left alone", refs)
	}
}
