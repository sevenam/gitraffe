package git

import (
	"bytes"
	"errors"
	"os/exec"
	"sort"
	"strings"
)

// SwitchKind is what a switch moves HEAD to.
type SwitchKind int

const (
	// SwitchBranch: a local branch, as it is.
	SwitchBranch SwitchKind = iota
	// SwitchRemote: a remote-tracking branch with no local branch beside it.
	// Switching makes the local branch, tracking the remote one, the way
	// "git switch feature" does when only origin/feature exists.
	SwitchRemote
	// SwitchDetached: the commit itself, on no branch.
	SwitchDetached
)

// SwitchTarget is one place a switch can go.
type SwitchTarget struct {
	Kind SwitchKind
	// Name is the branch as shown: "main", or "origin/feature" for a remote
	// one. For a detached switch it is the full hash.
	Name string
}

// ErrLocalChanges is returned, with nothing changed, when the working tree or
// the index holds changes to tracked files.
var ErrLocalChanges = errors.New("uncommitted changes")

// SwitchTargets lists where a switch to the commit carrying refs (its %D
// string, read with --decorate=full) can go: its local branches, then its
// remote branches that have no local branch of the same name on it, since
// that local branch is the one anyone would mean. A commit with neither can
// only be checked out detached.
//
// Only refs/heads and refs/remotes count. ParseRefs shows anything else under
// refs/ as a local branch so that nothing is hidden, but "refs/stash" is not
// something to switch to.
func SwitchTargets(refs, fullHash string) []SwitchTarget {
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

	// Sorted, because git lists a commit's refs in no order worth keeping,
	// and the numbers in the question should mean the same thing each time.
	sort.Strings(local)
	sort.Strings(remote)

	var targets []SwitchTarget
	for _, name := range local {
		targets = append(targets, SwitchTarget{Kind: SwitchBranch, Name: name})
	}
	for _, name := range remote {
		// "origin/feature" stands for "feature". A remote name rarely holds a
		// slash; when one does, the name compared here is wrong and the
		// remote branch is merely offered beside the local one.
		_, branch, _ := strings.Cut(name, "/")
		if contains(local, branch) {
			continue
		}
		targets = append(targets, SwitchTarget{Kind: SwitchRemote, Name: name})
	}
	if len(targets) == 0 {
		targets = append(targets, SwitchTarget{Kind: SwitchDetached, Name: fullHash})
	}
	return targets
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Switch moves HEAD to the target and the working tree with it, and refuses,
// changing nothing, while tracked files have uncommitted changes.
//
// Git itself would carry those changes across when they don't clash, which is
// how work ends up committed on the wrong branch without anyone noticing; and
// when they do clash it stops with a message meant for a terminal. Refusing
// both keeps a switch to what cannot lose or misplace work. Untracked files
// are left alone: they belong to no branch, and git refuses by itself if the
// target would overwrite one.
//
// It is "git switch" rather than "git checkout": switch only ever changes
// branches, where checkout given the wrong argument restores files over local
// edits.
func Switch(dir string, t SwitchTarget) (detail string, err error) {
	dirty, err := Run(dir, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return "", err
	}
	if dirty != "" {
		return "", ErrLocalChanges
	}

	args := []string{"switch", "--quiet"}
	switch t.Kind {
	case SwitchRemote:
		args = append(args, "--track", t.Name)
	case SwitchDetached:
		args = append(args, "--detach", t.Name)
	default:
		args = append(args, t.Name)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(errOut.String()), err
	}
	return "", nil
}
