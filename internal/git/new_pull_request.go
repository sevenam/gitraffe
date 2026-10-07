package git

import (
	"net/url"
	"sort"
	"strings"
)

// A commit that no pull request brought in may be the end of a branch that
// has yet to get one, and the page that starts one is as far from the
// repository as the page of one that exists: the host is in the remote and
// the branch is on the commit.

// NewPullRequest is what is known about starting a pull request from a
// commit. At most one of its fields is set; neither is, for a commit with no
// branch to start one from.
type NewPullRequest struct {
	// Branch is the branch to open the page for, by its name on the remote.
	Branch string
	// Unpushed is a local branch on the commit that the remote does not have,
	// when that is all there is: a pull request cannot start from it yet.
	Unpushed string
}

// NewPullRequestFor finds the branch a pull request from a commit would
// start at: one of the branches on it (refs is its %D string, read with
// --decorate=full) that the remote the pages are built from also has. The
// branch checked out is preferred, since it is the one being worked on.
//
// The remote's default branch is never the answer, a pull request being what
// goes into it, and nor is any branch of a commit the default branch already
// holds: there is nothing left there to ask anyone to pull.
//
// "Has" is by the remote-tracking branches, as last fetched. A local branch
// counts when the remote has one of its name, wherever that one is: commits
// made since the last push leave the remote's label further down the graph,
// and the page is still the branch's.
func NewPullRequestFor(dir, hash, refs, current string) NewPullRequest {
	remote := browserRemoteName(dir)
	if remote == "" {
		return NewPullRequest{}
	}
	isDefault := defaultBranches(dir, remote)
	for name := range isDefault {
		if _, err := Run(dir, "merge-base", "--is-ancestor", hash, "refs/remotes/"+remote+"/"+name); err == nil {
			return NewPullRequest{}
		}
	}

	var pushed, unpushed []string
	for _, ref := range strings.Split(refs, ", ") {
		ref = strings.TrimPrefix(strings.TrimSpace(ref), "HEAD -> ")
		if name, ok := strings.CutPrefix(ref, "refs/remotes/"+remote+"/"); ok {
			if name != "HEAD" && !isDefault[name] {
				pushed = append(pushed, name)
			}
			continue
		}
		name, ok := strings.CutPrefix(ref, "refs/heads/")
		if !ok || isDefault[name] {
			continue
		}
		if _, err := Run(dir, "show-ref", "--verify", "--quiet", "refs/remotes/"+remote+"/"+name); err == nil {
			pushed = append(pushed, name)
		} else {
			unpushed = append(unpushed, name)
		}
	}

	// Sorted, because git lists a commit's refs in no order worth keeping,
	// and the same key on the same commit should open the same page.
	pick := func(names []string) string {
		if len(names) == 0 {
			return ""
		}
		sort.Strings(names)
		if contains(names, current) {
			return current
		}
		return names[0]
	}
	if branch := pick(pushed); branch != "" {
		return NewPullRequest{Branch: branch}
	}
	return NewPullRequest{Unpushed: pick(unpushed)}
}

// defaultBranches is the remote's default branch, by the name it has there.
// A clone records it as the remote's HEAD; a repository that added its remote
// by hand has no such record, and then the two names a default branch nearly
// always has stand in for it.
func defaultBranches(dir, remote string) map[string]bool {
	head, err := Run(dir, "symbolic-ref", "--quiet", "refs/remotes/"+remote+"/HEAD")
	if name, ok := strings.CutPrefix(head, "refs/remotes/"+remote+"/"); err == nil && ok && name != "" {
		return map[string]bool{name: true}
	}
	return map[string]bool{"main": true, "master": true}
}

// NewPullRequestURL is the page that starts a pull request from a branch in
// the repository a remote names, into whatever branch the host offers first,
// which is the repository's default.
//
// On GitHub it is the comparison page, opened ready to fill in; when the
// branch already has an open pull request, which nothing in the repository
// could have said, that page shows it.
func NewPullRequestURL(remote, branch string) string {
	base := remoteWebURL(remote)
	if base == "" || branch == "" {
		return ""
	}
	if forgeOf(remote) == forgeAzure {
		return base + "/pullrequestcreate?sourceRef=" + url.QueryEscape(branch)
	}
	// Escaped a segment at a time: the slashes in "feature/x" are part of the
	// address, and a "#" or "?" in a name must not end it.
	parts := strings.Split(branch, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return base + "/compare/" + strings.Join(parts, "/") + "?expand=1"
}
