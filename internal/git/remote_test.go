package git

import (
	"strings"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
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
		if got := PullRequestNumber(tc.subject); got != tc.want {
			t.Errorf("PullRequestNumber(%q) = %d, want %d", tc.subject, got, tc.want)
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
	got := PullRequestURL("git@github.com:sevenam/gitraffe.git", 59)
	if want := "https://github.com/sevenam/gitraffe/pull/59"; got != want {
		t.Errorf("pullRequestURL = %q, want %q", got, want)
	}
	if got := PullRequestURL("/home/me/repo", 59); got != "" {
		t.Errorf("a local remote gave %q, want nothing", got)
	}
	if got := PullRequestURL("git@github.com:sevenam/gitraffe.git", 0); got != "" {
		t.Errorf("no number gave %q, want nothing", got)
	}
}

func TestBrowserRemotePrefersOrigin(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("remote", "add", "upstream", "https://github.com/someone/else.git")
	git("remote", "add", "origin", "https://github.com/sevenam/gitraffe.git")

	if got := BrowserRemote(dir); got != "https://github.com/sevenam/gitraffe.git" {
		t.Errorf("BrowserRemote = %q, want origin's URL though it was added second", got)
	}
}
