package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrBranchExists is returned, with nothing changed, when there is already a
// local branch of the name asked for.
var ErrBranchExists = errors.New("a branch of that name exists")

// BranchExists reports whether dir's repository has a local branch name.
func BranchExists(dir, name string) bool {
	_, err := Run(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// CreateBranch makes a branch at the commit start (a full hash) and switches
// to it, the way "git switch -c" does. An empty start is HEAD.
//
// Started where HEAD is, no file changes, in the working tree or the index,
// so there is nothing to refuse while there are uncommitted changes: they are
// as they were, on a branch that is the old one under another name. On a
// detached HEAD that is the way back onto a branch.
//
// Started anywhere else it is a switch as well, and is refused as Switch is,
// with nothing made, while tracked files have uncommitted changes: git would
// carry them across to a branch they were not written for.
//
// During a merge, a rebase or the like it is refused, as git itself would:
// the operation belongs to the branch it was started on.
//
// The branch tracks nothing. branch.autoSetupMerge=inherit in someone's
// config would otherwise hand it the old branch's upstream, and a push from
// the new branch would then move that one.
func CreateBranch(dir, name, start string) (detail string, err error) {
	if !ValidBranchName(dir, name) {
		return "", ErrBadBranchName
	}
	if op := ReadWorkingState(dir).Operation; op != "" {
		return "", fmt.Errorf("%w: %s", ErrInProgress, op)
	}
	if BranchExists(dir, name) {
		return "", ErrBranchExists
	}

	args := []string{"switch", "--quiet", "--no-track", "-c", name}
	// HEAD's own commit is asked for as HEAD, so that it is read here, where
	// it is acted on, and not from what a caller last saw of it.
	if head, _ := Run(dir, "rev-parse", "--verify", "--quiet", "HEAD"); start != "" && start != head {
		dirty, err := LocalChanges(dir)
		if err != nil {
			return "", err
		}
		if dirty {
			return "", ErrLocalChanges
		}
		args = append(args, start)
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
