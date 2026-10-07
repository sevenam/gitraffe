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

// CreateBranch makes a branch where HEAD is and switches to it, the way
// "git switch -c" does.
//
// It starts at HEAD and nowhere else. No file changes, in the working tree
// or the index, so unlike Switch it has nothing to refuse while there are
// uncommitted changes: they are as they were, on a branch that is the old one
// under another name. A branch started anywhere else would be a switch as
// well, with everything Switch has to be careful of.
//
// On a detached HEAD it is the way back onto a branch. During a merge, a
// rebase or the like it is refused, as git itself would: the operation
// belongs to the branch it was started on.
//
// The branch tracks nothing. branch.autoSetupMerge=inherit in someone's
// config would otherwise hand it the old branch's upstream, and a push from
// the new branch would then move that one.
func CreateBranch(dir, name string) (detail string, err error) {
	if !ValidBranchName(dir, name) {
		return "", ErrBadBranchName
	}
	if op := ReadWorkingState(dir).Operation; op != "" {
		return "", fmt.Errorf("%w: %s", ErrInProgress, op)
	}
	if BranchExists(dir, name) {
		return "", ErrBranchExists
	}

	cmd := exec.Command("git", "switch", "--quiet", "--no-track", "-c", name)
	cmd.Dir = dir
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(errOut.String()), err
	}
	return "", nil
}
