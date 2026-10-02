package git

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// A merge commit's subject is the only place the pull request that made it
// survives in the repository: git stores no link to one, and a PR page is
// where the review, the discussion and the rest of the story are. The number
// is in the subject and the host is in the remote, so between them there is
// enough to reach the page without asking anyone.

// PullRequestNumber reads the pull request number out of a commit subject, or
// 0 when there is none.
//
// Both of GitHub's own conventions are read, since which one a repository uses
// is a setting rather than a choice made commit by commit:
//
//	Merge pull request #59 from owner/branch   — a merge commit
//	Teach the parser about groups (#59)        — a squashed pull request
//
// A rebase merge leaves nothing behind to find, and a hand-written "(#59)" may
// mean an issue rather than a pull request; GitHub redirects one to the other,
// so the worst that happens is landing on the issue it names.
func PullRequestNumber(subject string) int {
	if rest, ok := strings.CutPrefix(subject, "Merge pull request #"); ok {
		digits, _, _ := strings.Cut(rest, " ")
		if n, err := strconv.Atoi(digits); err == nil && n > 0 {
			return n
		}
		return 0
	}

	// The squashed form, which GitHub puts at the end of the subject.
	if !strings.HasSuffix(subject, ")") {
		return 0
	}
	open := strings.LastIndex(subject, "(#")
	if open < 0 {
		return 0
	}
	if n, err := strconv.Atoi(subject[open+2 : len(subject)-1]); err == nil && n > 0 {
		return n
	}
	return 0
}

// remoteWebURL turns a git remote into the address of its page on the web, or
// "" when it has none — a path on disk, or something unparseable.
//
// Remotes come written several ways for the same repository, and the scp-like
// form (git@host:owner/repo) is not a URL at all, so it is taken apart by
// hand. Any credentials in the remote are dropped rather than carried into a
// browser.
func remoteWebURL(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}

	// git@github.com:owner/repo.git — a colon where a URL would have a slash,
	// and no scheme. Told apart from a "host:port/path" URL by the absence of
	// "//" and, on Windows, from "C:\path" by the length before the colon.
	if !strings.Contains(remote, "://") {
		host, path, ok := strings.Cut(remote, ":")
		if !ok || len(host) < 2 || path == "" {
			return ""
		}
		if _, h, found := strings.Cut(host, "@"); found {
			host = h
		}
		return webURL(host, path)
	}

	u, err := url.Parse(remote)
	if err != nil || u.Host == "" {
		return ""
	}
	switch u.Scheme {
	case "http", "https", "ssh", "git":
	default:
		// file:// and anything else names no web page.
		return ""
	}
	return webURL(u.Hostname(), u.Path)
}

// webURL assembles the https address of a repository's page.
func webURL(host, path string) string {
	host = strings.TrimSpace(host)
	path = strings.Trim(strings.TrimSpace(path), "/")
	path = strings.TrimSuffix(path, ".git")
	if host == "" || path == "" {
		return ""
	}
	// Always https, whatever the remote used: ssh and git are not schemes a
	// browser can open, and http would be a downgrade on a host offering both.
	return "https://" + host + "/" + path
}

// PullRequestURL is the page for a pull request in the repository a remote
// names.
//
// The path is GitHub's, because the subjects PullRequestNumber reads are
// GitHub's own: another host writes its merge commits differently and would
// need its own reading of the subject as well as its own path. So the two
// belong together, and adding one without the other would send people to a
// page that isn't there.
func PullRequestURL(remote string, number int) string {
	base := remoteWebURL(remote)
	if base == "" || number <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/pull/%d", base, number)
}

// BrowserRemote is the remote to build URLs from: "origin" when there is one,
// else whichever git lists first. Origin is where a clone came from, which is
// the repository whose numbering a merge subject counts in.
func BrowserRemote(dir string) string {
	remotes := Remotes(dir)
	if len(remotes) == 0 {
		return ""
	}
	name := remotes[0]
	for _, r := range remotes {
		if r == "origin" {
			name = r
			break
		}
	}
	url, err := Run(dir, "remote", "get-url", name)
	if err != nil {
		return ""
	}
	return url
}
