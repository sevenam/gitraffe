package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Cherry-picking: copying one commit's changes onto the branch checked out,
// as a new commit there.
//
// A cherry-pick is a merge, and a merge can stop in conflicts that leave files
// half one thing and half the other, which is the kind of thing gitraffe
// leaves to a terminal. So, as with popping a stash, it is first tried out in
// the object store alone, where nothing can be harmed, and goes ahead only
// when it would apply cleanly. And it is refused while tracked files have
// uncommitted changes: git would carry them through, and the commit it makes
// is then easy to mistake for one that holds them.

var (
	// ErrCherryPickConflicts is returned, with nothing done, when the commit
	// would not apply cleanly to HEAD.
	ErrCherryPickConflicts = errors.New("the commit would not apply cleanly")
	// ErrCherryPickEmpty is returned, with nothing done, when HEAD already
	// holds every change the commit makes.
	ErrCherryPickEmpty = errors.New("the branch already has these changes")
	// ErrCherryPickMerge is returned, with nothing done, for a merge commit:
	// which of its parents its changes are counted from is a question for
	// a terminal.
	ErrCherryPickMerge = errors.New("a merge commit")
	// ErrCherryPickOnBranch is returned, with nothing done, when the commit
	// is HEAD or one of its ancestors: it is already on the branch.
	ErrCherryPickOnBranch = errors.New("the commit is already on the branch")
	// ErrCherryPickRoot is returned, with nothing done, for a commit with no
	// parent: it has no base to be tried out against (merge-tree before git
	// 2.45 takes only a commit as the base, and there is none), and copying
	// the first commit of a history onto another is rarely what was meant.
	ErrCherryPickRoot = errors.New("a commit with no parent")
	// ErrNoHead is returned, with nothing done, by a branch with no commit
	// yet, which has nothing to apply a change to.
	ErrNoHead = errors.New("the branch has no commits yet")
)

// CherryPick copies the commit hash (a full hash) onto the branch checked
// out as a new commit, with the original's message and author, and returns
// the new commit's hash. The result's detail is what git said when it failed.
//
// It refuses, changing nothing, during a merge, a rebase or the like; on a
// detached HEAD, where the new commit would belong to no branch; while
// tracked files have uncommitted changes; for a merge commit or a commit with
// no parent; for a commit
// already on the branch; and when the change would conflict or would change
// nothing. See the top of this file.
func CherryPick(dir, hash string) (newHash, detail string, err error) {
	state := ReadWorkingState(dir)
	switch {
	case state.Operation != "":
		return "", "", fmt.Errorf("%w: %s", ErrInProgress, state.Operation)
	case state.Detached:
		return "", "", ErrDetached
	}
	top, err := Toplevel(dir)
	if err != nil {
		return "", "", err
	}
	head, err := Run(top, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil || head == "" {
		return "", "", ErrNoHead
	}
	dirty, err := LocalChanges(top)
	if err != nil {
		return "", "", err
	}
	if dirty {
		return "", "", ErrLocalChanges
	}
	if err := cherryPickWouldFail(top, hash, head); err != nil {
		return "", "", err
	}

	// --no-edit is git's default already; it is said so that no config or
	// terminal could make a cherry-pick stop for an editor nobody can see.
	out, err := runWrite(top, "", "cherry-pick", "--no-edit", hash)
	if err != nil {
		// Whatever stopped it after the trial said it would go through (a
		// hook, say), the repository is put back as it was, rather than left
		// mid-way for a terminal to finish. The working tree was clean, so
		// abort has nothing of the user's to throw away.
		if gitDir, gerr := Run(top, "rev-parse", "--absolute-git-dir"); gerr == nil {
			if _, serr := os.Stat(filepath.Join(filepath.FromSlash(gitDir), "CHERRY_PICK_HEAD")); serr == nil {
				_, _ = runWrite(top, "", "cherry-pick", "--abort")
			}
		}
		return "", out, err
	}
	newHash, _ = Run(top, "rev-parse", "HEAD")
	return newHash, "", nil
}

// cherryPickWouldFail is the trial: the commit merged onto head with its own
// parent as the base, which is what a cherry-pick does, in the object store
// alone.
//
// A failure of any other kind counts against the cherry-pick as well.
// merge-tree has taken --merge-base only since git 2.40, and with an older
// git a cherry-pick is refused rather than risked.
func cherryPickWouldFail(top, hash, head string) error {
	// Asked first: a commit already on the branch may also be a merge or the
	// first commit, and "already on it" is the answer that says why picking
	// it makes no sense.
	if _, err := Run(top, "merge-base", "--is-ancestor", hash, head); err == nil {
		return ErrCherryPickOnBranch
	}
	parents, err := Run(top, "rev-list", "--parents", "-n", "1", hash)
	if err != nil {
		return err
	}
	fields := strings.Fields(parents)
	switch len(fields) {
	case 1:
		return ErrCherryPickRoot
	case 2:
	default:
		return ErrCherryPickMerge
	}
	base := fields[1]

	out, err := runWrite(top, "", "merge-tree", "--write-tree", "--no-messages",
		"--merge-base="+base, head, hash)
	var exit interface{ ExitCode() int }
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return ErrCherryPickConflicts
	}
	if err != nil {
		return errors.New(firstNonEmptyLine(out, err.Error()))
	}
	headTree, err := Run(top, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	if firstNonEmptyLine(out, "") == headTree {
		return ErrCherryPickEmpty
	}
	return nil
}
