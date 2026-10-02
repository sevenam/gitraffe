package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/gittest"
)

// Only a web address is ever handed to the desktop: a remote is repository
// data, and gitraffe should not open whatever it happens to say.
func TestOpenURLRefusesWhatIsNotAWebAddress(t *testing.T) {
	for _, address := range []string{
		"file:///etc/passwd",
		"ftp://example.com/x",
		"javascript:alert(1)",
		"/home/me/repo",
		"",
	} {
		if err := openURL(address); err == nil {
			t.Errorf("openURL(%q) was allowed, want it refused", address)
		}
	}
}

// prRepo has a merge commit with a pull request in its subject, and a remote
// to build the address from.
func prRepo(t *testing.T) string {
	t.Helper()
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("checkout", "-qb", "side")
	commit("second")
	git("checkout", "-q", "main")
	git("merge", "-q", "--no-ff", "side", "-m", "Merge pull request #59 from sevenam/side")
	git("remote", "add", "origin", "https://github.com/sevenam/gitraffe.git")
	return dir
}

// With no remote there is nothing to name a host, and saying so beats a key
// that looks broken.
func TestPullRequestWithoutARemoteSaysSo(t *testing.T) {
	dir, git, _ := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "Merge pull request #59 from sevenam/side")

	m := loadedModel(t, dir)
	m, _ = m.openPullRequest()
	if !strings.Contains(m.notice, "No remote") {
		t.Errorf("notice = %q, want it to say there is no remote", m.notice)
	}
}

// Most commits are not merges of a pull request, so the common case has to
// explain itself.
func TestPullRequestOnAnOrdinaryCommitSaysSo(t *testing.T) {
	m := loadedModel(t, prRepo(t))
	m.selected = len(m.commits) - 1 // the first commit, which merged nothing

	m, _ = m.openPullRequest()
	if !strings.Contains(m.notice, "No pull request") {
		t.Errorf("notice = %q, want it to say the commit names no pull request", m.notice)
	}
	if !strings.Contains(ansi.Strip(m.renderStatusLine()), "No pull request") {
		t.Error("the status line does not carry the notice")
	}
}

// The key works on the commit the commit view is open on, not on whatever the
// graph last had selected.
func TestPullRequestFollowsTheCommitView(t *testing.T) {
	m := loadedModel(t, prRepo(t))
	m.windowWidth, m.windowHeight = 110, 26
	m.selected = 0 // the merge commit
	m, _ = m.openCommitView()
	if !m.commitView.open {
		t.Fatal("the commit view did not open")
	}

	c, ok := m.commitOnScreen()
	if !ok || git.PullRequestNumber(c.Message) != 59 {
		t.Fatalf("the view is on %q, want the merge commit", c.Message)
	}

	// Its notice reaches this screen's status line too.
	m.notice = "Opening pull request #59 in your browser"
	if !strings.Contains(ansi.Strip(m.commitViewStatusLine()), "#59") {
		t.Error("the commit view's status line does not carry the notice")
	}
}
