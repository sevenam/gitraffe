package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// pullRepo is a clone-like repository on main, tracking a bare origin next
// door, plus a way to push a commit to that origin from somewhere else.
func pullRepo(t *testing.T) (dir string, git func(...string), commit func(string), pushElsewhere func(file string)) {
	t.Helper()
	dir, git, commit = gittest.Fixture(t)
	origin := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", origin).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	git("init", "-q", "-b", "main")
	commit("first")
	git("remote", "add", "origin", origin)
	git("push", "-q", "-u", "origin", "main")

	pushElsewhere = func(file string) {
		t.Helper()
		other := filepath.Join(t.TempDir(), "other")
		run := func(wd string, args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = wd
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
				"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
			}
		}
		run(filepath.Dir(other), "clone", "-q", origin, other)
		if err := os.WriteFile(filepath.Join(other, file), []byte("theirs"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(other, "add", "-A")
		run(other, "commit", "-qm", "theirs: "+file)
		run(other, "push", "-q", "origin", "main")
	}
	return dir, git, commit, pushElsewhere
}

func head(t *testing.T, dir string) string {
	t.Helper()
	out, err := Run(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPullFastForwards(t *testing.T) {
	dir, _, _, pushElsewhere := pullRepo(t)
	pushElsewhere("theirs.txt")
	pushElsewhere("more.txt")

	r, err := Pull(dir)
	if err != nil {
		t.Fatalf("pull: %v (%s)", err, r.Detail)
	}
	if r.Outcome != PullFastForwarded || r.Behind != 2 || r.Branch != "main" || r.Upstream != "origin/main" || !r.Fetched {
		t.Errorf("result = %+v, want main fast-forwarded by 2 to origin/main", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "theirs.txt")); err != nil {
		t.Error("the working tree was not brought forward with the branch")
	}
	if ahead, behind := UpstreamSync(dir); ahead != 0 || behind != 0 {
		t.Errorf("ahead/behind = %d/%d after pulling, want in sync", ahead, behind)
	}
}

func TestPullWithNothingNew(t *testing.T) {
	dir, _, commit, _ := pullRepo(t)
	if r, err := Pull(dir); err != nil || r.Outcome != PullUpToDate || !r.Fetched {
		t.Errorf("result = %+v (%v), want up to date", r, err)
	}

	// Ahead of the remote is still nothing to pull.
	commit("mine")
	was := head(t, dir)
	if r, err := Pull(dir); err != nil || r.Outcome != PullUpToDate || r.Ahead != 1 {
		t.Errorf("result = %+v (%v), want up to date and one ahead", r, err)
	}
	if head(t, dir) != was {
		t.Error("the branch moved with nothing to pull")
	}
}

// Catching up a diverged branch takes a merge or a rebase, either of which can
// stop in a conflict. That is refused, and refused before anything is touched.
func TestPullLeavesADivergedBranchAlone(t *testing.T) {
	dir, _, commit, pushElsewhere := pullRepo(t)
	commit("mine")
	pushElsewhere("theirs.txt")
	was := head(t, dir)

	r, err := Pull(dir)
	if err != nil || r.Outcome != PullDiverged || r.Ahead != 1 || r.Behind != 1 {
		t.Errorf("result = %+v (%v), want diverged, one each way", r, err)
	}
	if head(t, dir) != was {
		t.Error("a diverged branch was moved")
	}
	if out, _ := Run(dir, "status", "--porcelain"); out != "" {
		t.Errorf("the working tree was left changed: %q", out)
	}
}

// A setting that makes "git pull" rebase must not make this one rebase.
func TestPullIgnoresPullRebaseConfig(t *testing.T) {
	dir, git, commit, pushElsewhere := pullRepo(t)
	git("config", "pull.rebase", "true")
	commit("mine")
	pushElsewhere("theirs.txt")
	was := head(t, dir)

	if r, err := Pull(dir); err != nil || r.Outcome != PullDiverged {
		t.Errorf("result = %+v (%v), want diverged", r, err)
	}
	if head(t, dir) != was {
		t.Error("the local commit was rebased")
	}
}

// Git refuses a fast-forward that would overwrite an uncommitted edit, and
// leaves both the edit and the branch as they were.
func TestPullRefusesToOverwriteLocalEdits(t *testing.T) {
	dir, _, _, pushElsewhere := pullRepo(t)
	pushElsewhere("first") // changes the file committed as "first"
	mine := filepath.Join(dir, "first")
	if err := os.WriteFile(mine, []byte("my edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	was := head(t, dir)

	r, err := Pull(dir)
	if err == nil || !r.Fetched || r.Detail == "" {
		t.Fatalf("result = %+v (%v), want the fast-forward refused with git's reason", r, err)
	}
	if head(t, dir) != was {
		t.Error("the branch moved over a local edit")
	}
	if body, _ := os.ReadFile(mine); string(body) != "my edit" {
		t.Errorf("the local edit was lost: %q", body)
	}
}

// An edit to a file the incoming commits don't touch is not in the way.
func TestPullKeepsUnrelatedLocalEdits(t *testing.T) {
	dir, _, _, pushElsewhere := pullRepo(t)
	pushElsewhere("theirs.txt")
	mine := filepath.Join(dir, "first")
	if err := os.WriteFile(mine, []byte("my edit"), 0o644); err != nil {
		t.Fatal(err)
	}

	if r, err := Pull(dir); err != nil || r.Outcome != PullFastForwarded {
		t.Fatalf("result = %+v (%v), want a fast-forward", r, err)
	}
	if body, _ := os.ReadFile(mine); string(body) != "my edit" {
		t.Errorf("the local edit was lost: %q", body)
	}
}

func TestPullWithNowhereToPullFrom(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")

	if r, err := Pull(dir); err != nil || r.Outcome != PullNoUpstream || r.Branch != "main" || r.Fetched {
		t.Errorf("result = %+v (%v), want no upstream and no fetch", r, err)
	}

	git("checkout", "-q", "--detach")
	if r, err := Pull(dir); err != nil || r.Outcome != PullDetached || r.Fetched {
		t.Errorf("result = %+v (%v), want detached and no fetch", r, err)
	}
}
