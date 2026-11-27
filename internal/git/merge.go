package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Merging: another branch's commits brought into the branch checked out,
// either by moving the branch forward to them (a fast-forward) or with a
// merge commit that has both as parents.
//
// As with a cherry-pick, a merge that would stop in conflicts leaves files
// half one thing and half the other, so it is first tried out in the object
// store alone (merge-tree), and goes ahead only when it would go through
// cleanly. It is refused while tracked files have uncommitted changes, which
// git would carry through a merge and which are then easy to lose track of
// among its changes.
//
// The merge itself is pinned against config that could turn it into
// something else: branch.<name>.mergeOptions can say --squash or --no-commit,
// merge.autoStash would stash and re-apply changes behind the user's back,
// and merge.log is already in the message the plan gives. What a merge does
// is said on its command line instead. merge.ff is the one setting that is
// honoured, by the plan, because it is the user's own answer to the question
// the merge box asks. Hooks run and are never skipped.

var (
	// ErrMergeConflicts is returned, with nothing done, when the merge
	// would stop in conflicts.
	ErrMergeConflicts = errors.New("the merge would conflict")
	// ErrMergeUpToDate is returned, with nothing done, when every commit
	// of the ref is already on the branch.
	ErrMergeUpToDate = errors.New("already up to date")
	// ErrMergeNotFastForward is returned, with nothing done, when merge.ff
	// is "only" and the branch cannot simply be moved forward to the ref.
	ErrMergeNotFastForward = errors.New("merge.ff is only, and this is not a fast-forward")
	// ErrMergeUnrelated is returned, with nothing done, when the ref shares
	// no history with the branch. git refuses such a merge unless told
	// otherwise, and joining two histories is a question for a terminal.
	ErrMergeUnrelated = errors.New("the histories are unrelated")
)

// MergeKind is what a merge does to the branch.
type MergeKind int

const (
	// MergeUpToDate is nothing: the ref is already on the branch.
	MergeUpToDate MergeKind = iota
	// MergeFastForward moves the branch forward to the ref.
	MergeFastForward
	// MergeCommit makes a commit with the branch and the ref as parents.
	MergeCommit
)

// FastForwardSetting is the repository's merge.ff.
type FastForwardSetting int

const (
	// FastForwardAllowed is git's default: a fast-forward when possible.
	FastForwardAllowed FastForwardSetting = iota
	// FastForwardNever is merge.ff=false: always a merge commit.
	FastForwardNever
	// FastForwardOnly is merge.ff=only: a fast-forward or nothing.
	FastForwardOnly
)

// PlannedMerge is what a merge of a ref into HEAD would do, read without
// changing anything.
type PlannedMerge struct {
	// Kind is what a merge with git's defaults would do, merge.ff
	// included. With FastForwardOnly and a ref that cannot be
	// fast-forwarded to, it is MergeCommit, and Merge without NoFF refuses.
	Kind MergeKind
	// CanFastForward says HEAD is an ancestor of the ref, so the branch
	// could be moved to it, whatever merge.ff says.
	CanFastForward bool
	// Conflicts says a merge commit would stop in conflicts. It is never
	// true when CanFastForward, since a merge commit then holds the ref's
	// own tree.
	Conflicts bool
	// FastForward is the repository's merge.ff.
	FastForward FastForwardSetting
	// Message is the merge commit's message as git would write it:
	// "Merge branch 'x'", "Merge remote-tracking branch 'origin/x'", with
	// " into y" and merge.log's list of commits where the config says so.
	Message string
	// Ref is the ref as Merge names it to git: a full ref name for a
	// branch or tag, so that one cannot be mistaken for the other, or the
	// commit's hash.
	Ref string
}

// MergeOptions are the choices the merge box offers.
type MergeOptions struct {
	// NoFF makes a merge commit even where a fast-forward is possible. It
	// overrides merge.ff, as --no-ff does on git's command line.
	NoFF bool
	// Message is the merge commit's message; empty is the plan's.
	Message string
}

// MergePlan reads what merging ref into HEAD would do. It refuses as Merge
// does, except over uncommitted changes and over the answer the plan gives
// (up to date, conflicts, merge.ff=only), which are for whoever asks to say.
func MergePlan(dir, ref string) (PlannedMerge, error) {
	top, err := Toplevel(dir)
	if err != nil {
		return PlannedMerge{}, err
	}
	head, err := Run(top, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil || head == "" {
		return PlannedMerge{}, ErrNoHead
	}
	return planMerge(top, head, ref)
}

func planMerge(top, head, ref string) (PlannedMerge, error) {
	// A ref that looks like an option is refused here: rev-parse prints
	// --end-of-options back rather than honouring it.
	if strings.HasPrefix(ref, "-") {
		return PlannedMerge{}, fmt.Errorf("%s is not a commit", ref)
	}
	commit, err := Run(top, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil || commit == "" {
		return PlannedMerge{}, fmt.Errorf("%s is not a commit", ref)
	}
	p := PlannedMerge{FastForward: readMergeFF(top)}
	p.Ref, p.Message = mergeName(top, ref, commit)

	if _, err := Run(top, "merge-base", "--is-ancestor", commit, head); err == nil {
		p.Kind = MergeUpToDate
		return p, nil
	}
	if _, err := Run(top, "merge-base", "--is-ancestor", head, commit); err == nil {
		p.CanFastForward = true
		p.Kind = MergeFastForward
		if p.FastForward == FastForwardNever {
			p.Kind = MergeCommit
		}
		return p, nil
	}
	if base, _ := Run(top, "merge-base", head, commit); base == "" {
		return PlannedMerge{}, ErrMergeUnrelated
	}
	p.Kind = MergeCommit

	// The trial: the same merge, written to the object store alone. Exit 1
	// is conflicts; any other failure counts against the merge as well, so
	// a git too old for --write-tree (2.38) refuses rather than risks it.
	out, err := runWrite(top, "", "merge-tree", "--write-tree", "--no-messages", head, commit)
	var exit interface{ ExitCode() int }
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		p.Conflicts = true
	} else if err != nil {
		return PlannedMerge{}, errors.New(firstNonEmptyLine(out, err.Error()))
	}
	return p, nil
}

// readMergeFF reads merge.ff, which is a boolean or "only".
func readMergeFF(top string) FastForwardSetting {
	raw, err := Run(top, "config", "--get", "merge.ff")
	if err != nil {
		return FastForwardAllowed
	}
	if strings.EqualFold(raw, "only") {
		return FastForwardOnly
	}
	// git's own reading of a boolean: false, no, off, 0 and the rest.
	if b, err := Run(top, "config", "--type=bool", "--get", "merge.ff"); err == nil && b == "false" {
		return FastForwardNever
	}
	return FastForwardAllowed
}

// mergeName says how ref is named to git merge, and the message git would
// give the merge. The message comes from fmt-merge-msg, which is what git
// merge itself uses, fed the line git merge would feed it, so that
// merge.suppressDest and merge.log come out as they would from git.
func mergeName(top, ref, commit string) (name, message string) {
	full, _ := Run(top, "rev-parse", "--symbolic-full-name", ref)
	var what string
	switch {
	case strings.HasPrefix(full, "refs/heads/"):
		name, what = full, "branch '"+strings.TrimPrefix(full, "refs/heads/")+"'"
	case strings.HasPrefix(full, "refs/remotes/"):
		name, what = full, "remote-tracking branch '"+strings.TrimPrefix(full, "refs/remotes/")+"'"
	case strings.HasPrefix(full, "refs/tags/"):
		name, what = full, "tag '"+strings.TrimPrefix(full, "refs/tags/")+"'"
	default:
		name, what = commit, "'"+ref+"'"
	}
	line := commit + "\t\t" + what + " of .\n"
	message, err := runWrite(top, line, "fmt-merge-msg")
	if err != nil || message == "" {
		message = "Merge " + what
	}
	return name, message
}

// Merge merges ref into the branch checked out and returns the new HEAD. The
// result's detail is what git said when it failed.
//
// It refuses, changing nothing, during a merge, a rebase or the like; on a
// detached HEAD, where a merge commit would belong to no branch; while
// tracked files have uncommitted changes; on a branch with no commits; when
// the ref is already on the branch; when merge.ff=only and no fast-forward is
// possible (unless NoFF asks for a merge commit); and when it would conflict.
// See the top of this file.
func Merge(dir, ref string, opts MergeOptions) (newHead, detail string, err error) {
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
	plan, err := planMerge(top, head, ref)
	if err != nil {
		return "", "", err
	}
	fastForward := plan.CanFastForward && !opts.NoFF && plan.FastForward != FastForwardNever
	switch {
	case plan.Kind == MergeUpToDate:
		return "", "", ErrMergeUpToDate
	case plan.Conflicts:
		return "", "", ErrMergeConflicts
	case !fastForward && !opts.NoFF && plan.FastForward == FastForwardOnly:
		return "", "", ErrMergeNotFastForward
	}
	message := opts.Message
	if strings.TrimSpace(message) == "" {
		message = plan.Message
	}

	// Each flag stands against a setting that could change the merge;
	// see the top of this file. --ff when a fast-forward was planned is
	// not --ff-only: the plan already knows it is possible.
	ffFlag := "--no-ff"
	if fastForward {
		ffFlag = "--ff"
	}
	out, err := runWrite(top, "",
		"-c", "merge.autoStash=false",
		"merge", "--no-edit", "--commit", "--no-squash", "--no-autostash", "--no-log",
		"--verify", ffFlag, "-m", message, "--end-of-options", plan.Ref)
	if err != nil {
		// Whatever stopped it after the trial said it would go through (a
		// hook, an untracked file in the way), the repository is put back
		// as it was rather than left mid-way. The working tree was clean,
		// so abort has nothing of the user's to throw away.
		if gitDir, gerr := Run(top, "rev-parse", "--absolute-git-dir"); gerr == nil {
			if _, serr := os.Stat(filepath.Join(filepath.FromSlash(gitDir), "MERGE_HEAD")); serr == nil {
				_, _ = runWrite(top, "", "merge", "--abort")
			}
		}
		return "", out, err
	}
	newHead, _ = Run(top, "rev-parse", "HEAD")
	return newHead, "", nil
}
