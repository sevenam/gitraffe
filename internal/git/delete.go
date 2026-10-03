package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// DeleteTarget is a branch on a commit that can be deleted: the local
// branch, the remote one of the same name on the same commit, or both.
type DeleteTarget struct {
	Local  string // "feature"; "" when the commit carries no local branch of the name
	Remote string // "origin/feature"; "" when it carries no remote one
}

// DeleteWhat is which side of a DeleteTarget to delete.
type DeleteWhat int

const (
	DeleteLocal DeleteWhat = iota
	DeleteRemote
	DeleteBoth
)

// DeleteResult is what a delete got done. Both can be false with no error
// only when nothing was asked for.
type DeleteResult struct {
	Local, Remote bool
	// Detail is what git wrote to stderr when a step failed.
	Detail string
}

var (
	// ErrOnlyCopy is returned, with nothing deleted, when the branch holds
	// commits that no other branch or tag would keep.
	ErrOnlyCopy = errors.New("commits on no other branch")
	// ErrCurrentBranch is returned, with nothing deleted, for the branch HEAD
	// is on.
	ErrCurrentBranch = errors.New("branch is checked out")
	// ErrRemoteMoved is returned, with nothing deleted, when the branch on
	// the remote is no longer where the last fetch saw it.
	ErrRemoteMoved = errors.New("remote branch has moved")
)

// DeleteTargets lists the branches on the commit carrying refs (its %D
// string, read with --decorate=full). A local branch and a remote branch of
// the same name are one target, so both can be deleted together; they pair
// only when they sit on the same commit, because a remote branch somewhere
// else in the graph is not what is being pointed at.
//
// Like SwitchTargets, only refs/heads and refs/remotes count.
func DeleteTargets(refs string) []DeleteTarget {
	var local, remote []string
	for _, ref := range strings.Split(refs, ", ") {
		ref = strings.TrimPrefix(strings.TrimSpace(ref), "HEAD -> ")
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			local = append(local, strings.TrimPrefix(ref, "refs/heads/"))
		case strings.HasPrefix(ref, "refs/remotes/") && !strings.HasSuffix(ref, "/HEAD"):
			remote = append(remote, strings.TrimPrefix(ref, "refs/remotes/"))
		}
	}
	sort.Strings(local)
	sort.Strings(remote)

	var targets []DeleteTarget
	paired := map[string]bool{}
	for _, name := range local {
		t := DeleteTarget{Local: name}
		for _, r := range remote {
			// The first remote wins; the same branch on a second remote is
			// listed on its own below. See SwitchTargets on remote names
			// that hold a slash.
			if _, branch, _ := strings.Cut(r, "/"); branch == name && !paired[r] {
				t.Remote = r
				paired[r] = true
				break
			}
		}
		targets = append(targets, t)
	}
	for _, r := range remote {
		if !paired[r] {
			targets = append(targets, DeleteTarget{Remote: r})
		}
	}
	return targets
}

// DeleteBranch deletes the local branch, the remote one, or both, and
// refuses, deleting nothing, unless every commit on them stays on some other
// branch or tag.
//
// That check is ours rather than "git branch -d"'s. Git asks whether the
// branch is merged into its upstream or into HEAD, which says no to a branch
// merged into main while something else is checked out, and yes to a pushed
// branch whose remote copy is about to be deleted in the same breath. What
// matters is whether the commits are still held once the delete is done, so
// that is what is asked, and the local delete is then a "-D".
//
// The remote branch goes first: it is the step that can fail for reasons out
// of our hands, and failing there leaves everything as it was. It is deleted
// only if it is still where the last fetch saw it, so commits pushed since by
// someone else are never thrown away unseen.
func DeleteBranch(dir string, t DeleteTarget, what DeleteWhat) (DeleteResult, error) {
	var r DeleteResult
	local := what != DeleteRemote && t.Local != ""
	remote := what != DeleteLocal && t.Remote != ""

	var going []string
	if local {
		if current, _ := Run(dir, "symbolic-ref", "-q", "--short", "HEAD"); current == t.Local {
			return r, ErrCurrentBranch
		}
		going = append(going, "refs/heads/"+t.Local)
	}
	if remote {
		going = append(going, "refs/remotes/"+t.Remote)
	}
	for _, ref := range going {
		held, err := heldElsewhere(dir, ref, going)
		if err != nil {
			return r, err
		}
		if !held {
			return r, ErrOnlyCopy
		}
	}

	if remote {
		detail, err := deleteRemoteBranch(dir, t.Remote)
		if err != nil {
			r.Detail = detail
			return r, err
		}
		r.Remote = true
	}
	if local {
		cmd := exec.Command("git", "branch", "--quiet", "-D", t.Local)
		cmd.Dir = dir
		var errOut bytes.Buffer
		cmd.Stderr = &errOut
		if err := cmd.Run(); err != nil {
			r.Detail = strings.TrimSpace(errOut.String())
			return r, err
		}
		r.Local = true
	}
	return r, nil
}

// heldElsewhere reports whether the commit ref points at is reachable from a
// branch or tag that is not among going, the refs about to be deleted.
func heldElsewhere(dir, ref string, going []string) (bool, error) {
	tip, err := Run(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return false, fmt.Errorf("no such branch: %s", ref)
	}
	out, err := Run(dir, "for-each-ref", "--contains", tip,
		"--format=%(refname)%00%(symref)", "refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(out, "\n") {
		name, symref, _ := strings.Cut(line, "\x00")
		// origin/HEAD is a pointer to a branch, not a branch: it holds
		// nothing by itself, and goes stale when its branch is deleted.
		if name == "" || symref != "" || contains(going, name) {
			continue
		}
		return true, nil
	}
	return false, nil
}

// deleteRemoteBranch deletes "origin/feature" on its remote, provided it is
// still on the commit the remote-tracking ref says. A successful push drops
// the remote-tracking ref as well.
func deleteRemoteBranch(dir, name string) (detail string, err error) {
	remoteName, branch := splitRemoteBranch(dir, name)
	if remoteName == "" {
		return "", fmt.Errorf("no remote for %s", name)
	}
	tip, err := Run(dir, "rev-parse", "--verify", "--quiet", "refs/remotes/"+name)
	if err != nil {
		return "", fmt.Errorf("no such branch: %s", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "push", "--quiet",
		"--force-with-lease=refs/heads/"+branch+":"+tip,
		remoteName, ":refs/heads/"+branch)
	cmd.Dir = dir
	cmd.Env = noPromptEnv(dir)
	var errOut bytes.Buffer
	cmd.Stderr = &errOut

	err = cmd.Run()
	detail = strings.TrimSpace(errOut.String())
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		err = fmt.Errorf("no answer after %s", fetchTimeout)
	case err != nil && strings.Contains(detail, "stale info"):
		err = ErrRemoteMoved
	}
	return detail, err
}

// splitRemoteBranch splits "origin/feature/x" into the remote and the branch
// on it. The remotes are asked for because either part may hold a slash; the
// longest remote name that fits is the one git would have written the ref
// under.
func splitRemoteBranch(dir, name string) (remoteName, branch string) {
	for _, r := range Remotes(dir) {
		if strings.HasPrefix(name, r+"/") && len(r) > len(remoteName) {
			remoteName, branch = r, strings.TrimPrefix(name, r+"/")
		}
	}
	return remoteName, branch
}
