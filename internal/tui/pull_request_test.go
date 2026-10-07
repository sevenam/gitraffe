package tui

import (
	"path/filepath"
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

// pullRequest presses o and feeds back what was found, with a browser that
// only notes the address it was asked to open.
func pullRequest(t *testing.T, m model) (model, string) {
	t.Helper()
	opened := ""
	real := openBrowser
	openBrowser = func(address string) error { opened = address; return nil }
	defer func() { openBrowser = real }()

	next, cmd := m.Update(keyPress("o"))
	if cmd == nil {
		t.Fatal("o started nothing")
	}
	next, _ = next.(model).Update(cmd())
	return next.(model), opened
}

// issueRepo is the history that sends a reader of subjects to the wrong
// page: commits that end in an issue's number, merged by pull requests with
// numbers of their own, beside a squashed pull request on main itself.
func issueRepo(t *testing.T) string {
	t.Helper()
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("checkout", "-qb", "side")
	commit("Delete a branch with d (#141)")
	git("checkout", "-q", "main")
	git("merge", "-q", "--no-ff", "side", "-m", "Merge pull request #142 from sevenam/side")
	commit("Teach the parser about groups (#60)")
	git("remote", "add", "origin", "https://github.com/sevenam/gitraffe.git")
	return dir
}

// o on a commit inside a pull request opens that pull request, not the issue
// its subject mentions.
func TestPullRequestOpensTheOneThatMergedTheCommit(t *testing.T) {
	dir := issueRepo(t)
	for subject, want := range map[string]string{
		"Delete a branch with d (#141)":             "https://github.com/sevenam/gitraffe/pull/142",
		"Merge pull request #142 from sevenam/side": "https://github.com/sevenam/gitraffe/pull/142",
		// No merge brought this one in, so the brackets are GitHub's.
		"Teach the parser about groups (#60)": "https://github.com/sevenam/gitraffe/pull/60",
	} {
		m, opened := pullRequest(t, selectMessage(t, loadedModel(t, dir), subject))
		if opened != want {
			t.Errorf("%q opened %q, want %q (notice %q)", subject, opened, want, m.notice)
		}
		if !strings.Contains(m.notice, "Opening pull request #"+want[strings.LastIndex(want, "/")+1:]) {
			t.Errorf("%q: notice = %q, want it to name the pull request opened", subject, m.notice)
		}
	}
}

// An answer that arrives after the repository was switched is about another
// repository's numbers.
func TestLatePullRequestAnswerIsDropped(t *testing.T) {
	m := selectMessage(t, loadedModel(t, issueRepo(t)), "Delete a branch with d (#141)")
	next, cmd := m.Update(keyPress("o"))
	msg := cmd().(pullRequestFoundMsg)
	msg.repoPath = "elsewhere"

	opened := false
	real := openBrowser
	openBrowser = func(string) error { opened = true; return nil }
	defer func() { openBrowser = real }()
	if got := res(next.(model).Update(msg)); opened || got.notice != "" {
		t.Errorf("opened=%v notice=%q, want the answer ignored", opened, got.notice)
	}
}

// With no remote there is nothing to name a host, and saying so beats a key
// that looks broken.
func TestPullRequestWithoutARemoteSaysSo(t *testing.T) {
	dir, git, _ := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "Merge pull request #59 from sevenam/side")

	m, _ := pullRequest(t, loadedModel(t, dir))
	if !strings.Contains(m.notice, "No remote") {
		t.Errorf("notice = %q, want it to say there is no remote", m.notice)
	}
}

// Most commits are not merges of a pull request, so the common case has to
// explain itself.
func TestPullRequestOnAnOrdinaryCommitSaysSo(t *testing.T) {
	m := loadedModel(t, prRepo(t))
	m.selected = len(m.commits) - 1 // the first commit, which merged nothing

	m, _ = pullRequest(t, m)
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
	if !ok || git.PullRequestNumber(c.Message, "") != 59 {
		t.Fatalf("the view is on %q, want the merge commit", c.Message)
	}

	// Its notice reaches this screen's status line too.
	m.notice = "Opening pull request #59 in your browser"
	if !strings.Contains(ansi.Strip(m.commitViewStatusLine()), "#59") {
		t.Error("the commit view's status line does not carry the notice")
	}
}

// unmergedRepo is main with a branch of two commits no pull request has
// merged, and a GitHub remote that, as far as the repository knows, has main.
// pushed says whether it has the branch too.
func unmergedRepo(t *testing.T, pushed bool) string {
	t.Helper()
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("remote", "add", "origin", "https://github.com/sevenam/gitraffe.git")
	git("update-ref", "refs/remotes/origin/main", "main")
	git("switch", "-q", "-c", "fix/sign-flip")
	commit("start the fix")
	commit("finish the fix")
	if pushed {
		git("update-ref", "refs/remotes/origin/fix/sign-flip", "fix/sign-flip")
	}
	return dir
}

// With no pull request to open, o on the end of a branch the remote has
// opens the page that starts one.
func TestPullRequestKeyStartsOneFromAPushedBranch(t *testing.T) {
	dir := unmergedRepo(t, true)
	m, opened := pullRequest(t, selectMessage(t, loadedModel(t, dir), "finish the fix"))
	if want := "https://github.com/sevenam/gitraffe/compare/fix/sign-flip?expand=1"; opened != want {
		t.Errorf("opened %q, want %q (notice %q)", opened, want, m.notice)
	}
	if m.notice != "Opening a new pull request for fix/sign-flip in your browser" {
		t.Errorf("notice = %q", m.notice)
	}

	// The commit view is about the same commit, and its key does the same.
	m, _ = selectMessage(t, loadedModel(t, dir), "finish the fix").openCommitView()
	if _, opened := pullRequest(t, m); !strings.HasSuffix(opened, "/compare/fix/sign-flip?expand=1") {
		t.Errorf("from the commit view: opened %q", opened)
	}
}

// A branch the remote has never seen cannot start one, and the way to change
// that is a key away.
func TestPullRequestKeyOnAnUnpushedBranchSaysToPush(t *testing.T) {
	m, opened := pullRequest(t, selectMessage(t, loadedModel(t, unmergedRepo(t, false)), "finish the fix"))
	if opened != "" {
		t.Errorf("opened %q for a branch that is not on the remote", opened)
	}
	if want := "fix/sign-flip is not on the remote yet — push it (P)"; !strings.Contains(m.notice, want) {
		t.Errorf("notice = %q, want %q", m.notice, want)
	}
}

// Nothing opens from a commit with no branch on it, nor from what the
// default branch already holds, nor from the uncommitted changes.
func TestPullRequestKeyWithNothingToStartFrom(t *testing.T) {
	dir := unmergedRepo(t, true)
	for _, subject := range []string{"start the fix", "first"} {
		m, opened := pullRequest(t, selectMessage(t, loadedModel(t, dir), subject))
		if opened != "" || !strings.Contains(m.notice, "No pull request on this commit, and no branch on it") {
			t.Errorf("%q: opened=%q notice=%q; want nothing to open", subject, opened, m.notice)
		}
	}

	writeFile(t, filepath.Join(dir, "first"), "edited")
	m := loadedModel(t, dir)
	if !m.commits[0].WorkingTree {
		t.Fatal("no uncommitted changes row")
	}
	m.selected = 0
	got, cmd := m.Update(keyPress("o"))
	if cmd != nil || !strings.Contains(got.(model).notice, "uncommitted changes") {
		t.Errorf("notice = %q; want the uncommitted changes to have no pull request", got.(model).notice)
	}
}

// On Azure DevOps the page that starts one is at another address.
func TestPullRequestKeyStartsOneOnAzure(t *testing.T) {
	dir := unmergedRepo(t, true)
	if _, err := git.Run(dir, "remote", "set-url", "origin", "git@ssh.dev.azure.com:v3/sevenam/tools/gitraffe"); err != nil {
		t.Fatal(err)
	}
	_, opened := pullRequest(t, selectMessage(t, loadedModel(t, dir), "finish the fix"))
	if want := "https://dev.azure.com/sevenam/tools/_git/gitraffe/pullrequestcreate?sourceRef=fix%2Fsign-flip"; opened != want {
		t.Errorf("opened %q, want %q", opened, want)
	}
}
