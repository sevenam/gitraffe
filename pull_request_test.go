package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestPullRequestNumberFromASubject(t *testing.T) {
	for _, tc := range []struct {
		subject string
		want    int
	}{
		{"Merge pull request #59 from sevenam/add-graph-colors", 59},
		{"Merge pull request #7 from a-fork/owner/deep/branch/name", 7},
		{"Merge pull request #1234 from x/y", 1234},
		// GitHub's squash merge puts it at the end instead.
		{"Teach the parser about nested groups (#59)", 59},
		{"Fix the sign flip (#1)", 1},
		// Nothing to find.
		{"Merge branch 'feature' into main", 0},
		{"Merge remote-tracking branch 'origin/main'", 0},
		{"add loan support", 0},
		{"", 0},
		// Shaped like one, but no number in it.
		{"Merge pull request #from nowhere", 0},
		{"Merge pull request # 12 from x/y", 0},
		{"Refactor (#) the parser", 0},
		{"Revert the change (#abc)", 0},
		// A number that isn't one.
		{"Merge pull request #0 from x/y", 0},
		{"Something (#0)", 0},
		// The marker has to end the subject for the squashed form; anything
		// else is prose that happens to mention a number.
		{"See (#59) for the discussion", 0},
	} {
		if got := pullRequestNumber(tc.subject); got != tc.want {
			t.Errorf("pullRequestNumber(%q) = %d, want %d", tc.subject, got, tc.want)
		}
	}
}

// The same repository is written several ways, and every one of them has to
// reach the same page.
func TestRemoteWebURL(t *testing.T) {
	const want = "https://github.com/sevenam/gitraffe"
	for _, remote := range []string{
		"https://github.com/sevenam/gitraffe.git",
		"https://github.com/sevenam/gitraffe",
		"https://github.com/sevenam/gitraffe/",
		"http://github.com/sevenam/gitraffe.git",
		"git@github.com:sevenam/gitraffe.git",
		"git@github.com:sevenam/gitraffe",
		"ssh://git@github.com/sevenam/gitraffe.git",
		"git://github.com/sevenam/gitraffe.git",
		// Credentials in a remote are common and must not reach a browser.
		"https://sevenam@github.com/sevenam/gitraffe.git",
		"https://sevenam:sekrit@github.com/sevenam/gitraffe.git",
		"  https://github.com/sevenam/gitraffe.git  ",
	} {
		if got := remoteWebURL(remote); got != want {
			t.Errorf("remoteWebURL(%q) = %q, want %q", remote, got, want)
		}
	}
}

// A password in the remote must not be carried into the address bar, where it
// would land in history and in any screenshot of the browser.
func TestRemoteWebURLDropsCredentials(t *testing.T) {
	got := remoteWebURL("https://sevenam:sekrit@github.com/sevenam/gitraffe.git")
	if strings.Contains(got, "sekrit") || strings.Contains(got, "@") {
		t.Errorf("remoteWebURL kept the credentials: %q", got)
	}
}

// A self-hosted GitHub is the same shape on a different host.
func TestRemoteWebURLKeepsTheHostAndPath(t *testing.T) {
	for _, tc := range []struct{ remote, want string }{
		{"git@git.example.com:team/tools/repo.git", "https://git.example.com/team/tools/repo"},
		{"https://git.example.com:8443/team/repo.git", "https://git.example.com/team/repo"},
		{"ssh://git@git.example.com:2222/team/repo.git", "https://git.example.com/team/repo"},
	} {
		if got := remoteWebURL(tc.remote); got != tc.want {
			t.Errorf("remoteWebURL(%q) = %q, want %q", tc.remote, got, tc.want)
		}
	}
}

// A repository with no page on the web has no address to guess at.
func TestRemoteWebURLOnRemotesWithNoPage(t *testing.T) {
	for _, remote := range []string{
		"",
		"   ",
		"/home/me/repos/thing.git",
		"../sibling-checkout",
		"file:///home/me/repos/thing.git",
		`C:\git\sevenam\gitraffe`,
		"https://github.com/",
		"git@github.com:",
	} {
		if got := remoteWebURL(remote); got != "" {
			t.Errorf("remoteWebURL(%q) = %q, want nothing", remote, got)
		}
	}
}

func TestPullRequestURL(t *testing.T) {
	got := pullRequestURL("git@github.com:sevenam/gitraffe.git", 59)
	if want := "https://github.com/sevenam/gitraffe/pull/59"; got != want {
		t.Errorf("pullRequestURL = %q, want %q", got, want)
	}
	if got := pullRequestURL("/home/me/repo", 59); got != "" {
		t.Errorf("a local remote gave %q, want nothing", got)
	}
	if got := pullRequestURL("git@github.com:sevenam/gitraffe.git", 0); got != "" {
		t.Errorf("no number gave %q, want nothing", got)
	}
}

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
	dir, git, commit := gitFixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("checkout", "-qb", "side")
	commit("second")
	git("checkout", "-q", "main")
	git("merge", "-q", "--no-ff", "side", "-m", "Merge pull request #59 from sevenam/side")
	git("remote", "add", "origin", "https://github.com/sevenam/gitraffe.git")
	return dir
}

func TestBrowserRemotePrefersOrigin(t *testing.T) {
	dir, git, commit := gitFixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("remote", "add", "upstream", "https://github.com/someone/else.git")
	git("remote", "add", "origin", "https://github.com/sevenam/gitraffe.git")

	m := loadedModel(t, dir)
	if got := m.browserRemote(); got != "https://github.com/sevenam/gitraffe.git" {
		t.Errorf("browserRemote = %q, want origin's URL though it was added second", got)
	}
}

// With no remote there is nothing to name a host, and saying so beats a key
// that looks broken.
func TestPullRequestWithoutARemoteSaysSo(t *testing.T) {
	dir, git, _ := gitFixture(t)
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
	if !ok || pullRequestNumber(c.Message) != 59 {
		t.Fatalf("the view is on %q, want the merge commit", c.Message)
	}

	// Its notice reaches this screen's status line too.
	m.notice = "Opening pull request #59 in your browser"
	if !strings.Contains(ansi.Strip(m.commitViewStatusLine()), "#59") {
		t.Error("the commit view's status line does not carry the notice")
	}
}
