package git

import "errors"

// Discarding: throwing away uncommitted changes. It is the one thing gitraffe
// does that loses work, so the screen asks before any of it, and it only ever
// reaches what is not staged. Staged changes are kept, whatever is discarded:
// staging is how a change is put beyond a slip of the finger, and a change to
// be thrown away is unstaged first, which is a key press that loses nothing.
//
// A tracked file goes back to what the index holds, which is HEAD's version
// unless part of it was staged; a file git has never seen is deleted.
// Ignored files are never touched.

var (
	// ErrStagedChanges is returned for a staged entry: see the top of this file.
	ErrStagedChanges = errors.New("staged changes are kept")
	// ErrUntrackedLines is returned when lines of an untracked file are
	// picked: it has no version in git to take them back to, and is
	// discarded whole or not at all.
	ErrUntrackedLines = errors.New("an untracked file is discarded whole")
)

// DiscardFile throws away a file's unstaged changes: a tracked file is put
// back to what the index holds, an untracked one is deleted.
func DiscardFile(dir string, f FileDiff) (string, error) {
	if f.Staged {
		return "", ErrStagedChanges
	}
	return change(dir, func(top string) (string, error) {
		return discardFile(top, f)
	})
}

func discardFile(top string, f FileDiff) (string, error) {
	if f.Untracked {
		// -f: clean.requireForce is a guard for "git clean" typed by hand
		// with no path; this names one file, and the screen has asked.
		return runWrite(top, "", "--literal-pathspecs", "clean", "--force", "--quiet", "--", f.Path)
	}
	// From the index, which is restore's default for --worktree: a staged
	// half of the file stays as it is, in the index and in the file.
	return runWrite(top, "", "--literal-pathspecs", "restore", "--worktree", "--", f.Path)
}

// DiscardLines throws away the changed lines among first..last of an unstaged
// file's diff as it is listed, the way StageLines stages them: it refuses
// with ErrStaleDiff when the diff is not the one they were picked from, and
// every change in the file picked is the file discarded.
//
// The patch is the one that would unstage those lines, applied backwards to
// the working tree instead of the index: the file is the new side of the
// unstaged diff as the index is the new side of the staged one.
func DiscardLines(dir string, f FileDiff, first, last int) (string, error) {
	switch {
	case f.Staged:
		return "", ErrStagedChanges
	case f.Untracked:
		return "", ErrUntrackedLines
	}
	return change(dir, func(top string) (string, error) {
		p, err := pickLines(top, f, first, last, 0)
		switch {
		case err != nil:
			return "", err
		case p.whole:
			return discardFile(top, f)
		}
		return runWrite(top, buildPatch(p.head, p.lines, p.picked, true),
			"apply", "--reverse", "--whitespace=nowarn", "-")
	})
}

// DiscardAll throws away every unstaged change: each tracked file back to
// what the index holds, and every untracked file deleted, with any directory
// that held only those.
func DiscardAll(dir string) (string, error) {
	return change(dir, func(top string) (string, error) {
		// Named one by one, and only those with a change: restore given the
		// whole tree would fail on an empty index, and has no business
		// rewriting files that are as the index has them.
		names, err := rawOutput(top, sectionArgs(false, "--name-only", "-z")...)
		if err != nil {
			return "", err
		}
		if names != "" {
			if out, err := runWrite(top, names, "--literal-pathspecs", "restore", "--worktree",
				"--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
				return out, err
			}
		}
		// From the top, so the whole working tree; not -x, so nothing
		// ignored, and not -ff, so no repository nested inside.
		return runWrite(top, "", "clean", "--force", "-d", "--quiet")
	})
}
