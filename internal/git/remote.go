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
//
// Each host writes that subject its own way and keeps the page at its own
// address, and the two belong together: reading one host's subject and opening
// another's address would send people to a page that isn't there, or to the
// wrong pull request. So the remote decides both.

// forge is the kind of host a remote points at, as far as pull requests go.
type forge int

const (
	// forgeGitHub is also what every host not recognised as something else is
	// taken for: its conventions are the ones other forges tend to copy.
	forgeGitHub forge = iota
	forgeAzure
)

// forgeOf says which host's conventions a remote calls for.
func forgeOf(remote string) forge {
	if host, path, ok := splitRemote(remote); ok && isAzure(host, path) {
		return forgeAzure
	}
	return forgeGitHub
}

// isAzure recognises Azure DevOps: its hosted names, old and new, and the
// "_git" path segment every one of its repository addresses has. The segment
// is what finds a server run on someone's own hostname, which no list of names
// could.
func isAzure(host, path string) bool {
	host = strings.ToLower(host)
	switch {
	case host == "dev.azure.com", host == "ssh.dev.azure.com", strings.HasSuffix(host, ".visualstudio.com"):
		return true
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "_git" {
			return true
		}
	}
	return false
}

// PullRequestNumber reads the pull request number out of a commit subject, or
// 0 when there is none. The remote says whose convention the subject follows;
// with no remote to go by, either is accepted.
//
// GitHub writes one of two, since which a repository uses is a setting rather
// than a choice made commit by commit:
//
//	Merge pull request #59 from owner/branch   — a merge commit
//	Teach the parser about groups (#59)        — a squashed pull request
//
// Azure DevOps writes the same subject whichever way a pull request is
// completed:
//
//	Merged PR 59: Teach the parser about groups
//
// A rebase leaves nothing behind to find on either, and a hand-written "(#59)"
// may mean an issue rather than a pull request; GitHub redirects one to the
// other, so the worst that happens is landing on the issue it names.
func PullRequestNumber(subject, remote string) int {
	if strings.TrimSpace(remote) == "" {
		if n := gitHubPullRequest(subject); n > 0 {
			return n
		}
		return azurePullRequest(subject)
	}
	if forgeOf(remote) == forgeAzure {
		return azurePullRequest(subject)
	}
	return gitHubPullRequest(subject)
}

func gitHubPullRequest(subject string) int {
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

func azurePullRequest(subject string) int {
	rest, ok := strings.CutPrefix(subject, "Merged PR ")
	if !ok {
		return 0
	}
	// The number runs up to the colon; a title that was cleared leaves the
	// number as the whole of the rest.
	digits, _, _ := strings.Cut(rest, ":")
	if n, err := strconv.Atoi(strings.TrimSpace(digits)); err == nil && n > 0 {
		return n
	}
	return 0
}

// splitRemote takes a git remote apart into the host and the path on it, the
// path still percent-encoded as written. It fails for a remote that names no
// host — a path on disk, or something unparseable.
//
// Remotes come written several ways for the same repository, and the scp-like
// form (git@host:owner/repo) is not a URL at all, so it is taken apart by
// hand. Any credentials in the remote are left behind rather than carried
// into a browser.
func splitRemote(remote string) (host, path string, ok bool) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", "", false
	}

	// git@github.com:owner/repo.git — a colon where a URL would have a slash,
	// and no scheme. Told apart from a "host:port/path" URL by the absence of
	// "//" and, on Windows, from "C:\path" by the length before the colon.
	if !strings.Contains(remote, "://") {
		host, path, found := strings.Cut(remote, ":")
		if !found || len(host) < 2 || path == "" {
			return "", "", false
		}
		if _, h, found := strings.Cut(host, "@"); found {
			host = h
		}
		return host, path, true
	}

	u, err := url.Parse(remote)
	if err != nil || u.Host == "" {
		return "", "", false
	}
	switch u.Scheme {
	case "http", "https", "ssh", "git":
	default:
		// file:// and anything else names no web page.
		return "", "", false
	}
	// Escaped, not decoded: an Azure project called "My Project" is written
	// My%20Project in the remote, and has to stay that way in the address.
	return u.Hostname(), u.EscapedPath(), true
}

// remoteWebURL turns a git remote into the address of its page on the web, or
// "" when it has none.
func remoteWebURL(remote string) string {
	host, path, ok := splitRemote(remote)
	if !ok {
		return ""
	}
	if isAzure(host, path) {
		host, path = azureWebAddress(host, path)
	}
	return webURL(host, path)
}

// azureWebAddress turns Azure DevOps's SSH remotes into where the same
// repository is on the web. Over HTTPS the remote already is that address;
// over SSH it is written differently and served from a different host:
//
//	git@ssh.dev.azure.com:v3/org/project/repo
//	https://dev.azure.com/org/project/_git/repo
//
// The older vs-ssh.visualstudio.com remotes have the same shape, and
// dev.azure.com answers for every organisation whichever name it was made
// under.
func azureWebAddress(host, path string) (string, string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 4 || parts[0] != "v3" {
		return host, path
	}
	return "dev.azure.com", parts[1] + "/" + parts[2] + "/_git/" + parts[3]
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
// names, at the address that remote's host keeps its pull requests under.
func PullRequestURL(remote string, number int) string {
	base := remoteWebURL(remote)
	if base == "" || number <= 0 {
		return ""
	}
	if forgeOf(remote) == forgeAzure {
		return fmt.Sprintf("%s/pullrequest/%d", base, number)
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
