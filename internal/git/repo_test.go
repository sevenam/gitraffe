package git

import (
	"os/exec"
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
