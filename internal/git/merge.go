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
// A merge that would stop in conflicts leaves files half one thing and half
// the other, so it is first tried out in the object store alone (merge-tree),
// which says whether it would and in which files. It goes ahead only when it
// would go through cleanly, unless AllowConflicts says the user has been told
// and asked for it anyway; then it is left stopped in its conflicts, for the
// conflict view to resolve or abort. It is refused while tracked files have
// uncommitted changes, which git would carry through a merge and which are
// then easy to lose track of among its changes; that is also what makes an
// abort put everything back exactly as it was.
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
	// would stop in conflicts and AllowConflicts was not given.
	ErrMergeConflicts = errors.New("the merge would conflict")
	// ErrMergeStopped is returned when, with AllowConflicts, the merge has
	// begun and stopped in conflicts, as asked: it is under way, with
	// MERGE_HEAD written, until it is continued or aborted.
	ErrMergeStopped = errors.New("the merge stopped in conflicts")
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
	// ConflictFiles are the paths a merge commit would leave in conflict,
	// in merge-tree's order (by path); empty unless Conflicts.
	ConflictFiles []string
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
	// Message is the merge commit's message; empty is the plan's. A
	// squash makes no commit, so it has none.
	Message string
	// Squash stages every change the ref makes instead of committing
	// them (--squash): HEAD stays where it is, and the ref is not
	// recorded as merged. The changes are then committed like any other.
	// It ignores NoFF and merge.ff, since there is no commit and no move
	// of the branch for either to decide.
	Squash bool
	// AllowConflicts lets a merge commit that would conflict begin, and
	// stop in its conflicts (ErrMergeStopped). A squash is still refused,
	// since it would stop with no MERGE_HEAD, which nothing could then
	// tell from changes of the user's own, or abort.
	AllowConflicts bool
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
	// is conflicts, and the paths in them follow the tree's hash, one per
	// NUL; any other failure counts against the merge as well, so a git too
	// old for --write-tree (2.38) refuses rather than risks it.
	out, err := runWrite(top, "", "merge-tree", "--write-tree", "--no-messages", "--name-only", "-z", head, commit)
	var exit interface{ ExitCode() int }
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		p.Conflicts = true
		fields := strings.Split(out, "\x00")
		for _, f := range fields[min(1, len(fields)):] {
			if f != "" {
				p.ConflictFiles = append(p.ConflictFiles, f)
			}
		}
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
// possible (unless NoFF asks for a merge commit, or Squash for no commit);
// and when it would conflict, unless AllowConflicts asks for that. See the
// top of this file.
//
// With AllowConflicts, a merge that stops in conflicts returns
// ErrMergeStopped with HEAD as it was, and is left under way.
//
// With Squash, newHead is HEAD as it was: nothing is committed, and the
// changes are left staged, with git's message for them in SQUASH_MSG (see
// SquashMessage).
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
	case plan.Conflicts && (opts.Squash || !opts.AllowConflicts):
		return "", "", ErrMergeConflicts
	case opts.Squash:
		return squash(top, head, plan.Ref)
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
		inMerge := false
		if gitDir, gerr := Run(top, "rev-parse", "--absolute-git-dir"); gerr == nil {
			_, serr := os.Stat(filepath.Join(filepath.FromSlash(gitDir), "MERGE_HEAD"))
			inMerge = serr == nil
		}
		// Stopped in conflicts, as the trial said it would and as was asked
		// for: left as git left it. Unmerged entries are what tell that
		// from a merge stopped by anything else.
		if inMerge && plan.Conflicts && opts.AllowConflicts {
			if unmerged, _ := Run(top, "ls-files", "--unmerged"); unmerged != "" {
				return head, out, ErrMergeStopped
			}
		}
		// Whatever else stopped it (a hook, an untracked file in the way),
		// the repository is put back as it was rather than left mid-way.
		// The working tree was clean, so abort has nothing of the user's to
		// throw away.
		if inMerge {
			_, _ = runWrite(top, "", "merge", "--abort")
		}
		return "", out, err
	}
	newHead, _ = Run(top, "rev-parse", "HEAD")
	return newHead, "", nil
}

// squash is Merge with Squash, once Merge has refused what it refuses.
func squash(top, head, ref string) (newHead, detail string, err error) {
	// --ff on the command line overrides merge.ff: git refuses --squash
	// with a merge.ff=false it would read as --no-ff, and with merge.ff=only
	// it refuses whatever is not a fast-forward, though a squash moves
	// nothing. --squash on the command line also outranks a --no-squash
	// in branch.<name>.mergeOptions.
	out, err := runWrite(top, "",
		"-c", "merge.autoStash=false",
		"merge", "--squash", "--ff", "--no-autostash", "--end-of-options", ref)
	if err != nil {
		// A squash leaves no MERGE_HEAD to abort, so whatever it staged
		// before it stopped is put back with reset --merge, which, the
		// working tree having been clean, touches only what the squash
		// wrote. Untracked files are kept.
		if _, derr := Run(top, "diff-index", "--cached", "--quiet", head, "--"); derr != nil {
			_, _ = runWrite(top, "", "reset", "--merge", "-q", head)
		}
		removeSquashMessage(top)
		return "", out, err
	}
	return head, "", nil
}

// SquashMessage is the message for a commit of the changes a squash staged,
// made from SQUASH_MSG, or "" when there is none. git writes there a log of
// every commit squashed, newest first, headers and all, which is a poor
// message as it stands. With one commit this is that commit's message; with
// several it is subject, and a body that lists the commits' subjects, oldest
// first, as their history reads.
func SquashMessage(dir, subject string) string {
	gitDir, err := Run(dir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(filepath.FromSlash(gitDir), "SQUASH_MSG"))
	if err != nil {
		return ""
	}
	messages := squashedMessages(string(raw))
	switch len(messages) {
	case 0:
		return ""
	case 1:
		return messages[0]
	}
	var sb strings.Builder
	sb.WriteString(subject + "\n")
	for i := len(messages) - 1; i >= 0; i-- {
		first, _, _ := strings.Cut(messages[i], "\n")
		sb.WriteString("\n* " + first)
	}
	return sb.String()
}

// squashedMessages reads the commits' messages out of SQUASH_MSG, in its
// order. Each commit there is a "commit <hash>" line, headers that config
// can add to (Merge:, a signature), a blank line, and the message indented
// by four spaces.
func squashedMessages(raw string) []string {
	var messages []string
	var current []string
	inCommit, inMessage := false, false
	flush := func() {
		if inCommit {
			if msg := strings.TrimSpace(strings.Join(current, "\n")); msg != "" {
				messages = append(messages, msg)
			}
		}
		current, inMessage = nil, false
	}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "commit "):
			flush()
			inCommit = true
		case !inCommit:
		case strings.HasPrefix(line, "    "):
			inMessage = true
			current = append(current, strings.TrimPrefix(line, "    "))
		case inMessage && strings.TrimSpace(line) == "":
			current = append(current, "")
		}
	}
	flush()
	return messages
}

// removeSquashMessage throws away SQUASH_MSG, which git leaves behind when a
// squash stops after writing it.
func removeSquashMessage(top string) {
	if gitDir, err := Run(top, "rev-parse", "--absolute-git-dir"); err == nil {
		_ = os.Remove(filepath.Join(filepath.FromSlash(gitDir), "SQUASH_MSG"))
	}
}
