package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// remoteRef is what origin holds under ref, or "" when it has no such ref.
func remoteRef(t *testing.T, dir, ref string) string {
	t.Helper()
	out, err := Run(dir, "ls-remote", "origin", ref)
	if err != nil {
		t.Fatal(err)
	}
	hash, _, _ := strings.Cut(out, "\t")
	return hash
}

func TestPushSendsTheBranchForward(t *testing.T) {
	dir, _, commit, _ := pullRepo(t)
	commit("second")
	commit("third")

	r, err := Push(dir)
	if err != nil {
		t.Fatalf("push: %v (%s)", err, r.Detail)
	}
	if r.Outcome != PushSent || r.Commits != 2 || r.Branch != "main" || r.Upstream() != "origin/main" {
		t.Errorf("result = %+v, want 2 commits of main sent to origin/main", r)
	}
	if got := remoteRef(t, dir, "refs/heads/main"); got != head(t, dir) {
		t.Errorf("origin's main is at %s, want HEAD %s", got, head(t, dir))
	}
	if ahead, behind := UpstreamSync(dir); ahead != 0 || behind != 0 {
		t.Errorf("ahead/behind = %d/%d after pushing, want in sync", ahead, behind)
	}
}

func TestPushWithNothingNew(t *testing.T) {
	dir, _, _, _ := pullRepo(t)
	r, err := Push(dir)
	if err != nil || r.Outcome != PushUpToDate {
		t.Errorf("result = %+v, err = %v; want up to date", r, err)
	}
}

// The remote's commits are never overwritten: the push is refused, whether
// or not this repository has fetched them yet, and nothing moves at either end.
func TestPushIsNeverForced(t *testing.T) {
	for _, fetched := range []bool{false, true} {
		dir, git, commit, pushElsewhere := pullRepo(t)
		pushElsewhere("theirs.txt")
		if fetched {
			git("fetch", "-q")
		}
		theirs := remoteRef(t, dir, "refs/heads/main")
		commit("mine")
		mine := head(t, dir)

		r, err := Push(dir)
		if err != nil || r.Outcome != PushBehind {
			t.Errorf("fetched=%v: result = %+v, err = %v; want the push refused as behind", fetched, r, err)
		}
		if got := remoteRef(t, dir, "refs/heads/main"); got != theirs {
			t.Errorf("fetched=%v: origin's main moved to %s", fetched, got)
		}
		if head(t, dir) != mine {
			t.Errorf("fetched=%v: the local branch moved", fetched)
		}
	}
}

// What is pushed is this branch to the branch it tracks, whatever the
// configuration would have made of a bare "git push".
func TestPushIgnoresPushConfig(t *testing.T) {
	dir, git, commit, _ := pullRepo(t)
	git("switch", "-q", "-c", "other")
	commit("on other")
	git("tag", "-a", "v1", "-m", "v1")
	git("switch", "-q", "main")
	git("merge", "-q", "--ff-only", "other")
	git("config", "push.default", "matching")
	git("config", "push.followTags", "true")

	if r, err := Push(dir); err != nil || r.Outcome != PushSent {
		t.Fatalf("result = %+v, err = %v", r, err)
	}
	if got := remoteRef(t, dir, "refs/heads/other"); got != "" {
		t.Error("the push sent a branch nobody asked it to")
	}
	if got := remoteRef(t, dir, "refs/tags/v1"); got != "" {
		t.Error("the push sent a tag nobody asked it to")
	}
}

func TestPushFollowsATrackedBranchOfAnotherName(t *testing.T) {
	dir, git, commit, _ := pullRepo(t)
	git("switch", "-q", "-c", "mine", "--track", "origin/main")
	commit("on mine")

	r, err := Push(dir)
	if err != nil || r.Outcome != PushSent || r.Upstream() != "origin/main" || r.Branch != "mine" {
		t.Fatalf("result = %+v, err = %v; want mine sent to origin/main", r, err)
	}
	if got := remoteRef(t, dir, "refs/heads/mine"); got != "" {
		t.Error("the push made a branch named after the local one")
	}
}

func TestPushPlan(t *testing.T) {
	dir, git, _, _ := pullRepo(t)
	if p := ReadPushPlan(dir); p.Branch != "main" || p.Upstream() != "origin/main" || p.NewBranchRemote != "origin" {
		t.Errorf("plan = %+v, want main tracking origin/main", p)
	}

	git("switch", "-q", "-c", "feature")
	if p := ReadPushPlan(dir); p.Branch != "feature" || p.Upstream() != "" || p.NewBranchRemote != "origin" {
		t.Errorf("plan = %+v, want feature tracking nothing, to go to origin", p)
	}

	// A branch tracking a local branch has nowhere of its own to push to.
	git("switch", "-q", "-c", "local", "--track", "main")
	if p := ReadPushPlan(dir); p.Upstream() != "" {
		t.Errorf("plan = %+v, want a branch tracking a local one to track no remote branch", p)
	}

	// A remote branch deleted since: the configuration still names it.
	git("switch", "-q", "-c", "gone")
	git("push", "-q", "-u", "origin", "gone")
	git("push", "-q", "origin", ":gone")
	if p := ReadPushPlan(dir); p.Upstream() != "" {
		t.Errorf("plan = %+v, want a deleted remote branch to count as none", p)
	}

	git("remote", "add", "fork", filepath.Join(t.TempDir(), "nowhere"))
	git("config", "remote.pushDefault", "fork")
	if p := ReadPushPlan(dir); p.NewBranchRemote != "fork" || len(p.Remotes) != 2 {
		t.Errorf("plan = %+v, want a new branch to go to the configured push remote", p)
	}

	git("switch", "-q", "--detach")
	if p := ReadPushPlan(dir); p.Branch != "" || p.Upstream() != "" {
		t.Errorf("plan = %+v, want no branch on a detached HEAD", p)
	}
	if _, err := Push(dir); !errors.Is(err, ErrDetached) {
		t.Errorf("err = %v, want a detached HEAD refused", err)
	}
}

func TestPushNewBranchCreatesAndTracksIt(t *testing.T) {
	dir, git, commit, _ := pullRepo(t)
	git("switch", "-q", "-c", "feature")
	commit("on feature")

	r, err := PushNewBranch(dir, "origin", "feature/renamed")
	if err != nil || r.Outcome != PushCreated || r.Upstream() != "origin/feature/renamed" || r.Branch != "feature" {
		t.Fatalf("result = %+v, err = %v; want the branch created", r, err)
	}
	if got := remoteRef(t, dir, "refs/heads/feature/renamed"); got != head(t, dir) {
		t.Errorf("origin has %q under the new name, want HEAD", got)
	}
	// From here on it is an ordinary push.
	if p := ReadPushPlan(dir); p.Upstream() != "origin/feature/renamed" {
		t.Errorf("plan = %+v, want the branch to track what it was pushed as", p)
	}
	commit("more")
	if r, err := Push(dir); err != nil || r.Outcome != PushSent || r.Commits != 1 {
		t.Errorf("result = %+v, err = %v; want the next commit sent the usual way", r, err)
	}
}

func TestPushNewBranchRefusesABadName(t *testing.T) {
	dir, git, _, _ := pullRepo(t)
	git("switch", "-q", "-c", "feature")
	for _, name := range []string{"", "two words", "-f", "ends.lock", "a..b"} {
		if _, err := PushNewBranch(dir, "origin", name); !errors.Is(err, ErrBadBranchName) {
			t.Errorf("%q: err = %v, want the name refused", name, err)
		}
	}
	if out, _ := Run(dir, "ls-remote", "--heads", "origin"); strings.Contains(out, "\n") {
		t.Errorf("something was pushed:\n%s", out)
	}
}

func TestPushTag(t *testing.T) {
	dir, git, commit, _ := pullRepo(t)
	commit("unpushed")
	git("tag", "v1")
	git("tag", "-a", "v2", "-m", "annotated")

	for _, tag := range []string{"v1", "v2"} {
		r, err := PushTag(dir, "origin", tag)
		if err != nil || r.Outcome != PushCreated || r.Tag != tag {
			t.Fatalf("%s: result = %+v, err = %v; want the tag created", tag, r, err)
		}
	}
	tags, err := RemoteTags(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"v1", "v2"} {
		if !tags[TagRef{Name: tag, Commit: head(t, dir)}] {
			t.Errorf("origin does not hold %s on HEAD: %v", tag, tags)
		}
	}
	// Only the tag went: the branch it is on was not asked for.
	if got := remoteRef(t, dir, "refs/heads/main"); got == head(t, dir) {
		t.Error("pushing a tag moved the branch on the remote as well")
	}

	if r, err := PushTag(dir, "origin", "v1"); err != nil || r.Outcome != PushUpToDate {
		t.Errorf("again: result = %+v, err = %v; want up to date", r, err)
	}
}

// A tag moved locally is not moved on the remote: that takes a forced push.
func TestPushTagNeverMovesARemoteTag(t *testing.T) {
	dir, git, commit, _ := pullRepo(t)
	git("tag", "v1")
	git("push", "-q", "origin", "v1")
	was := remoteRef(t, dir, "refs/tags/v1")
	commit("later")
	git("tag", "-f", "v1")

	r, err := PushTag(dir, "origin", "v1")
	if err != nil || r.Outcome != PushTagTaken {
		t.Errorf("result = %+v, err = %v; want the tag left alone", r, err)
	}
	if got := remoteRef(t, dir, "refs/tags/v1"); got != was {
		t.Error("the remote's tag was moved")
	}
}

// The repository's pre-push hook has the last word, and what it says is what
// is reported.
func TestPushRunsThePrePushHook(t *testing.T) {
	dir, _, commit, _ := pullRepo(t)
	commit("unpushed")
	hook := filepath.Join(dir, ".git", "hooks", "pre-push")
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho not on a Friday >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	was := remoteRef(t, dir, "refs/heads/main")

	r, err := Push(dir)
	if err == nil {
		t.Fatalf("result = %+v; want the hook's refusal to fail the push", r)
	}
	if !strings.Contains(r.Detail, "not on a Friday") {
		t.Errorf("detail = %q, want what the hook said", r.Detail)
	}
	if got := remoteRef(t, dir, "refs/heads/main"); got != was {
		t.Error("the push went through a hook that refused it")
	}
}

func TestPushToAnUnreachableRemoteSaysWhy(t *testing.T) {
	dir, git, commit, _ := pullRepo(t)
	commit("unpushed")
	git("remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))

	r, err := Push(dir)
	if err == nil || r.Detail == "" {
		t.Errorf("result = %+v, err = %v; want a failure with git's reason", r, err)
	}
}
