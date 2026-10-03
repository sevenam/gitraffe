// Package gittest builds throwaway git repositories for tests.
package gittest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Fixture returns helpers for building a throwaway repository: one to run a
// git command and one to make a commit.
func Fixture(t *testing.T) (dir string, git func(...string), commit func(string)) {
	t.Helper()
	dir = t.TempDir()
	// Every commit needs a later timestamp than the last. Left to the clock
	// they all share one second, and git then orders the log so the branches
	// never overlap and the graph stays a single column.
	n := 0
	git = func(args ...string) {
		t.Helper()
		n++
		at := fmt.Sprintf("2026-01-01T00:%02d:00", n)
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
			"GIT_AUTHOR_DATE="+at, "GIT_COMMITTER_DATE="+at,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	commit = func(name string) {
		t.Helper()
		// A file per commit, so no merge in these shapes can conflict.
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		git("add", "-A")
		git("commit", "-qm", name)
	}
	return dir, git, commit
}

// Identify gives the repository an author of its own. The fixture's git
// already commits as someone, through its environment; this is for the
// commits and merges a test has gitraffe itself make, which run git with no
// such environment and would otherwise depend on whoever is running the test
// having a name configured. A build machine has none.
func Identify(git func(...string)) {
	git("config", "user.name", "t")
	git("config", "user.email", "t@t")
}

// NewRepo makes an empty git repository and returns its root as git reports
// it, which is the form the switcher stores and compares.
func NewRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	// Git prints forward slashes even on Windows.
	return filepath.Clean(filepath.FromSlash(strings.TrimSpace(string(out))))
}
