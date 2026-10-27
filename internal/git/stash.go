package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Stashing: putting uncommitted changes aside and bringing them back.
//
// Putting them aside loses nothing: git keeps them as a commit on refs/stash
// until they are popped. Bringing them back is a merge, and a merge can stop
// in conflicts that leave files half one thing and half the other, which is
// the kind of thing gitraffe leaves to a terminal. So a pop is first tried
// out where nothing can be harmed, and goes ahead only when it would apply
// cleanly; otherwise nothing is touched and the stash stays where it was.

var (
	// ErrStashMoved is returned, with nothing done, when the stash picked is
	// no longer at the place in the list it was read from.
	ErrStashMoved = errors.New("the stash list has changed")
	// ErrStashConflicts is returned, with nothing done, when popping the
	// stash would leave conflicts to resolve.
	ErrStashConflicts = errors.New("the stash would not apply cleanly")
	// ErrStashUntrackedExists is returned, with nothing done, when a file
	// the stash holds as untracked is already in the working tree.
	ErrStashUntrackedExists = errors.New("an untracked file in the stash already exists")
)

// Stash is one entry of the stash list.
type Stash struct {
	Ref     string // "stash@{0}": where it is in the list now
	Hash    string // the stash commit, which says whether Ref still means it
	Subject string // "WIP on main: 1a2b3c4 subject", or "On main: <message>"
	When    string // "2 hours ago"
}

// ListStashes reads the stash list, newest first.
func ListStashes(dir string) ([]Stash, error) {
	out, err := Run(dir, "-c", "log.showSignature=false", "stash", "list",
		"--format=%gd%x1f%H%x1f%cr%x1f%gs")
	if err != nil || out == "" {
		return nil, err
	}
	var stashes []Stash
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(strings.TrimRight(line, "\r"), "\x1f", 4)
		if len(f) < 4 {
			continue
		}
		stashes = append(stashes, Stash{Ref: f[0], Hash: f[1], When: f[2], Subject: f[3]})
	}
	return stashes, nil
}

// StashPaths puts aside all of the uncommitted changes to the files named,
// staged and unstaged, and the files themselves where git has never seen
// them, as one stash called message.
//
// The paths go to git on its standard input rather than its command line,
// which on Windows holds 32,767 characters and a long selection would not fit.
func StashPaths(dir, message string, paths []string) (string, error) {
	if len(paths) == 0 {
		return "", errors.New("no files to stash")
	}
	return change(dir, func(top string) (string, error) {
		return runWrite(top, strings.Join(paths, "\x00")+"\x00",
			"--literal-pathspecs", "stash", "push", "--include-untracked", "--quiet",
			"--message", message, "--pathspec-from-file=-", "--pathspec-file-nul")
	})
}

// StashAll puts aside every uncommitted change, untracked files included,
// as one stash called message, or by git's own "WIP on <branch>: ..." when
// message is "". Naming no paths is what makes it everything, including
// whatever the screen had not read yet. Ignored files stay where they are.
func StashAll(dir, message string) (string, error) {
	return change(dir, func(top string) (string, error) {
		args := []string{"stash", "push", "--include-untracked", "--quiet"}
		if message != "" {
			args = append(args, "--message", message)
		}
		return runWrite(top, "", args...)
	})
}

// PopStash applies a stash to the working tree and drops it from the list,
// but only when that cannot stop halfway: see the top of this file.
//
// Staged changes in the stash come back unstaged, as "git stash pop" without
// --index brings them; asking for the index too is one more way to fail.
func PopStash(dir string, s Stash) (string, error) {
	return change(dir, func(top string) (string, error) {
		// Pushing or popping elsewhere renumbers the list, and the place is
		// what pop is given: it must still be the stash that was picked.
		if hash, err := Run(top, "rev-parse", "--verify", "--quiet", s.Ref); err != nil || hash != s.Hash {
			return "", ErrStashMoved
		}
		if err := popWouldConflict(top, s.Hash); err != nil {
			return "", err
		}
		return runWrite(top, "", "stash", "pop", "--quiet", s.Ref)
	})
}

// popWouldConflict merges the stash in the object store alone, the way pop
// would merge it into the index: its own parent as the base, the index as it
// is now on one side and the stash's working tree on the other.
//
// A failure of any other kind counts against the pop as well. merge-tree has
// taken --merge-base only since git 2.40, and with an older git a pop is
// refused rather than risked.
func popWouldConflict(top, hash string) error {
	index, err := Run(top, "write-tree")
	if err != nil {
		return err
	}
	out, err := runWrite(top, "", "merge-tree", "--write-tree", "--no-messages",
		"--merge-base="+hash+"^1", index, hash)
	var exit interface{ ExitCode() int }
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return ErrStashConflicts
	}
	if err != nil {
		return errors.New(firstNonEmptyLine(out, err.Error()))
	}

	// The untracked files are restored rather than merged, and one already
	// there stops the pop.
	if _, err := Run(top, "rev-parse", "--verify", "--quiet", hash+"^3"); err != nil {
		return nil
	}
	names, err := rawOutput(top, "ls-tree", "-r", "-z", "--name-only", hash+"^3")
	if err != nil {
		return err
	}
	for _, name := range strings.Split(names, "\x00") {
		if name == "" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(top, filepath.FromSlash(name))); err == nil {
			return ErrStashUntrackedExists
		}
	}
	return nil
}

func firstNonEmptyLine(s, fallback string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return fallback
}

// StashName is the name a stash was given, read from its subject: git writes
// "On main: lunch" for a stash with a message and "WIP on main: 1a2b3c4 the
// last subject" for one without, which has no name and gets "". A branch name
// cannot hold a colon, so the first ": " is where the name begins.
func StashName(subject string) string {
	rest, ok := strings.CutPrefix(subject, "On ")
	if !ok {
		return ""
	}
	_, name, _ := strings.Cut(rest, ": ")
	return name
}
