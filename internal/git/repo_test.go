package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// Both ways of reading HEAD give its full hash, which the graph matches
// commits against, and agree on it after HEAD has moved off the branch tip.
func TestReadInfoHeadHash(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("a")
	commit("b")
	git("switch", "-q", "--detach", "HEAD~1")

	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(out))

	repo, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, info := range map[string]Info{
		"go-git": ReadInfo(dir, repo),
		"cli":    ReadInfoCLI(dir),
	} {
		if info.HeadHash != want {
			t.Errorf("%s: HeadHash = %q, want %q", name, info.HeadHash, want)
		}
		if info.Commit != want[:7] {
			t.Errorf("%s: Commit = %q, want %q", name, info.Commit, want[:7])
		}
	}
}

// A repository with no commits has no HEAD to mark.
func TestReadInfoNoCommits(t *testing.T) {
	dir := gittest.NewRepo(t)
	if info := ReadInfoCLI(dir); info.HeadHash != "" {
		t.Errorf("HeadHash = %q, want empty", info.HeadHash)
	}
}

// topOf answers as Toplevel does, both at the top, where it does not ask git,
// and from a folder further down, where it does.
func TestTopOf(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("a")
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	want, err := Toplevel(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, from := range []string{dir, sub} {
		got, err := topOf(from)
		if err != nil {
			t.Fatalf("topOf(%q): %v", from, err)
		}
		// The temporary folder can be reached through a link, which git
		// resolves and topOf does not; the same folder is what matters.
		gotInfo, err1 := os.Stat(got)
		wantInfo, err2 := os.Stat(want)
		if err1 != nil || err2 != nil || !os.SameFile(gotInfo, wantInfo) {
			t.Errorf("topOf(%q) = %q, want %q", from, got, want)
		}
	}
	if _, err := topOf(t.TempDir()); err == nil {
		t.Error("topOf outside a repository did not fail")
	}
}
