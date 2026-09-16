package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSyncLabel(t *testing.T) {
	for _, tc := range []struct {
		ahead, behind int
		want          string
	}{
		{0, 0, ""},
		{3, 0, "↑3"},
		{0, 1, "↓1"},
		{3, 1, "↑3 ↓1"},
	} {
		if got := syncLabel(tc.ahead, tc.behind); got != tc.want {
			t.Errorf("syncLabel(%d, %d) = %q, want %q", tc.ahead, tc.behind, got, tc.want)
		}
	}
}

// gitEnv isolates test repos from the user's git configuration, where settings
// like commit signing or a different default branch would break the fixtures.
func gitEnv(t *testing.T) []string {
	empty := filepath.Join(t.TempDir(), "empty.gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+empty,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
}

func run(t *testing.T, env []string, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func syncOf(dir string) (ahead, behind int) {
	m := &model{repoPath: dir}
	m.loadUpstreamSync()
	return m.ahead, m.behind
}

func TestLoadUpstreamSync(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	env := gitEnv(t)
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	other := filepath.Join(root, "other")

	run(t, env, root, "init", "-q", "--bare", "-b", "main", remote)
	run(t, env, root, "clone", "-q", remote, work)
	run(t, env, work, "commit", "-q", "--allow-empty", "-m", "first")
	run(t, env, work, "push", "-q", "-u", "origin", "main")

	check := func(name string, dir string, wantAhead, wantBehind int) {
		t.Helper()
		if a, b := syncOf(dir); a != wantAhead || b != wantBehind {
			t.Errorf("%s: ahead/behind = %d/%d, want %d/%d", name, a, b, wantAhead, wantBehind)
		}
	}

	check("in sync", work, 0, 0)

	run(t, env, work, "commit", "-q", "--allow-empty", "-m", "local 1")
	run(t, env, work, "commit", "-q", "--allow-empty", "-m", "local 2")
	check("ahead", work, 2, 0)

	// Someone else pushes. Until work fetches, its origin/main hasn't moved.
	run(t, env, root, "clone", "-q", remote, other)
	run(t, env, other, "commit", "-q", "--allow-empty", "-m", "theirs")
	run(t, env, other, "push", "-q", "origin", "main")
	check("remote moved, not yet fetched", work, 2, 0)

	run(t, env, work, "fetch", "-q")
	check("diverged after fetch", work, 2, 1)

	run(t, env, work, "checkout", "-q", "--detach")
	check("detached HEAD", work, 0, 0)

	run(t, env, work, "checkout", "-q", "-b", "local-only")
	check("branch with no upstream", work, 0, 0)

	// A tracking branch whose remote branch has since been deleted.
	run(t, env, work, "push", "-q", "-u", "origin", "local-only")
	run(t, env, other, "push", "-q", "origin", "--delete", "local-only")
	run(t, env, work, "fetch", "-q", "--prune")
	check("upstream deleted", work, 0, 0)

	noRemote := filepath.Join(root, "no-remote")
	run(t, env, root, "init", "-q", "-b", "main", noRemote)
	run(t, env, noRemote, "commit", "-q", "--allow-empty", "-m", "first")
	check("no remote", noRemote, 0, 0)

	check("not a repository", root, 0, 0)
}
