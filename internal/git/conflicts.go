package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Conflicts: a merge or cherry-pick that stopped part-way, whoever started
// it. Gitraffe's own merges and cherry-picks are refused when they would stop
// (see merge.go), so one found here was most likely started in a terminal;
// this file reads where it stands and ends it either way: put back as it was
// (abort), or committed once every file is resolved (continue).
//
// Only a merge and a cherry-pick are ended here. A rebase or a revert stops
// the same way, but a rebase continues with commits of its own to replay,
// and both are a terminal's work.

var (
	// ErrNoMerge is returned, with nothing done, when no merge or
	// cherry-pick is under way to abort or continue.
	ErrNoMerge = errors.New("no merge or cherry-pick is in progress")
	// ErrUnmerged is returned by ContinueMerge, with nothing done, while
	// any file is still unmerged.
	ErrUnmerged = errors.New("files are still unmerged")
	// ErrConflictMarkers is returned, with nothing done, while a file
	// still holds the <<<<<<<, ======= or >>>>>>> lines git wrote into it.
	ErrConflictMarkers = errors.New("conflict markers are left")
	// ErrNotUnmerged is returned by MarkResolved, with nothing done, for a
	// file with no conflict to resolve.
	ErrNotUnmerged = errors.New("the file has no conflict")
)

// ConflictKind is how a file came to be unmerged: what each side did to it.
type ConflictKind int

const (
	// BothModified is both sides changing the file (UU).
	BothModified ConflictKind = iota
	// BothAdded is both sides adding a file at the same path (AA).
	BothAdded
	// DeletedByUs is the branch checked out deleting a file the other side
	// changed (DU).
	DeletedByUs
	// DeletedByThem is the other side deleting a file the branch checked
	// out changed (UD).
	DeletedByThem
	// AddedByUs is a file only the branch checked out has, unmerged because
	// of a rename or a directory on the other side (AU).
	AddedByUs
	// AddedByThem is a file only the other side has, unmerged likewise (UA).
	AddedByThem
	// BothDeleted is both sides deleting the file, each having renamed it
	// to somewhere else (DD).
	BothDeleted
)

// conflictKinds is git status's two letters for each kind, ours then theirs.
var conflictKinds = map[string]ConflictKind{
	"UU": BothModified,
	"AA": BothAdded,
	"DU": DeletedByUs,
	"UD": DeletedByThem,
	"AU": AddedByUs,
	"UA": AddedByThem,
	"DD": BothDeleted,
}

func (k ConflictKind) String() string {
	switch k {
	case BothModified:
		return "both modified"
	case BothAdded:
		return "both added"
	case DeletedByUs:
		return "deleted by us"
	case DeletedByThem:
		return "deleted by them"
	case AddedByUs:
		return "added by us"
	case AddedByThem:
		return "added by them"
	case BothDeleted:
		return "both deleted"
	}
	return "unmerged"
}

// ConflictFile is one unmerged file.
type ConflictFile struct {
	// Path is from the top of the working tree, with forward slashes.
	Path string
	Kind ConflictKind
}

// ConflictState is where a merge or cherry-pick that stopped stands.
type ConflictState struct {
	// Operation is ReadWorkingState's: "merge" or "cherry-pick" for what
	// can be aborted or continued here, and "" when nothing is under way.
	Operation string
	// Files are the unmerged files, in path order. A merge whose files are
	// all resolved but not yet committed has none.
	Files []ConflictFile
	// Ours is the branch checked out, or HEAD's short hash on no branch.
	Ours string
	// Theirs is what is being merged in, so the two sides can be called
	// "main" and "feature/login" rather than "ours" and "theirs": for a
	// merge, the name git's message gives it; for a cherry-pick, the branch
	// the commit is the tip of; otherwise its short hash. It is "" for
	// anything but a merge or a cherry-pick.
	Theirs string
}

// Conflicts reads where the merge or cherry-pick under way stands. With
// nothing under way it gives the unmerged files left by anything else (a
// stash that did not apply cleanly), and no names.
func Conflicts(dir string) (ConflictState, error) {
	var s ConflictState
	top, err := Toplevel(dir)
	if err != nil {
		return s, err
	}
	s.Operation = ReadWorkingState(top).Operation
	if s.Files, err = unmergedFiles(top); err != nil {
		return s, err
	}
	s.Ours = oursName(top)
	switch s.Operation {
	case "merge":
		s.Theirs = mergeHeadName(top)
	case "cherry-pick":
		s.Theirs = tipName(top, "CHERRY_PICK_HEAD")
	}
	return s, nil
}

// unmergedFiles reads the "u" lines of git status --porcelain=v2.
//
// -z gives each path as it is, unquoted; --no-renames keeps every entry one
// field long, where a rename's would carry its old path as a second.
func unmergedFiles(top string) ([]ConflictFile, error) {
	out, err := rawOutput(top, "status", "--porcelain=v2", "-z", "--no-renames", "--untracked-files=no")
	if err != nil {
		return nil, err
	}
	var files []ConflictFile
	for _, entry := range strings.Split(out, "\x00") {
		if !strings.HasPrefix(entry, "u ") {
			continue
		}
		// u <XY> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>
		fields := strings.SplitN(entry, " ", 11)
		if len(fields) < 11 {
			continue
		}
		kind, ok := conflictKinds[fields[1]]
		if !ok {
			kind = BothModified
		}
		files = append(files, ConflictFile{Path: fields[10], Kind: kind})
	}
	return files, nil
}

// oursName is the branch checked out, or HEAD's short hash.
func oursName(top string) string {
	if branch, err := Run(top, "symbolic-ref", "--short", "-q", "HEAD"); err == nil && branch != "" {
		return branch
	}
	short, _ := Run(top, "rev-parse", "--short", "HEAD")
	return short
}

// mergeHeadName is the name of what is being merged. git's message names it
// as it was asked for ("Merge branch 'feature/login'", "Merge
// remote-tracking branch 'origin/x'", "Merge tag 'v1'"), which a branch it
// happens to be the tip of might not; a message of the user's own (merge -m)
// names nothing, and then it is named from the commit.
func mergeHeadName(top string) string {
	if gitDir, err := Run(top, "rev-parse", "--absolute-git-dir"); err == nil {
		if raw, err := os.ReadFile(filepath.Join(filepath.FromSlash(gitDir), "MERGE_MSG")); err == nil {
			if name := nameInMergeMessage(string(raw)); name != "" {
				return name
			}
		}
	}
	return tipName(top, "MERGE_HEAD")
}

// nameInMergeMessage is the first quoted name on the first line of a message
// git wrote for a merge, or "".
func nameInMergeMessage(msg string) string {
	first, _, _ := strings.Cut(strings.ReplaceAll(msg, "\r\n", "\n"), "\n")
	if !strings.HasPrefix(first, "Merge ") {
		return ""
	}
	_, rest, ok := strings.Cut(first, "'")
	if !ok {
		return ""
	}
	name, _, ok := strings.Cut(rest, "'")
	if !ok {
		return ""
	}
	return name
}

// tipName names the commit ref points at by a branch it is the tip of, or
// by its short hash. name-rev alone would also name a commit further down
// ("feature~3"), which reads as a branch the commit is not the end of.
func tipName(top, ref string) string {
	if name, err := Run(top, "name-rev", "--name-only", "--no-undefined",
		"--refs=refs/heads/*", "--refs=refs/remotes/*", ref); err == nil &&
		name != "" && !strings.ContainsAny(name, "~^") {
		return strings.TrimPrefix(name, "remotes/")
	}
	short, _ := Run(top, "rev-parse", "--short", ref)
	return short
}

// ConflictMarkers lists the files, of paths or of the whole working tree when
// none are given, that hold conflict markers: in the working tree, or staged
// with them still in. It asks git diff --check, which knows each file's
// marker size and passes over binary files, where git writes none; its
// whitespace complaints are not counted.
func ConflictMarkers(dir string, paths ...string) ([]string, error) {
	top, err := Toplevel(dir)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var found []string
	for _, cached := range []bool{false, true} {
		args := []string{"-c", "core.quotePath=false", "--literal-pathspecs", "diff", "--check", "--no-color", "--no-ext-diff"}
		if cached {
			args = append(args, "--cached")
		}
		args = append(args, "--")
		args = append(args, paths...)
		out, err := runChecked(top, args...)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(out, "\n") {
			at, ok := strings.CutSuffix(line, ": leftover conflict marker")
			if !ok {
				continue
			}
			// <path>:<line>, and a path may hold colons of its own.
			if i := strings.LastIndex(at, ":"); i > 0 {
				at = at[:i]
			}
			if !seen[at] {
				seen[at] = true
				found = append(found, at)
			}
		}
	}
	return found, nil
}

// runChecked is git diff --check: exit 2 says it found something, which is
// not a failure to run. Its messages are read, so they are asked for
// untranslated.
func runChecked(top string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = top
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=C")
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 2 {
		err = nil
	}
	return string(out), err
}

// markersError names the files ConflictMarkers found.
func markersError(paths []string) error {
	return fmt.Errorf("%w: %s", ErrConflictMarkers, strings.Join(paths, ", "))
}

// mergeOperation is the merge or cherry-pick under way, or ErrNoMerge.
func mergeOperation(dir string) (WorkingState, error) {
	state := ReadWorkingState(dir)
	if state.Operation != "merge" && state.Operation != "cherry-pick" {
		return state, ErrNoMerge
	}
	return state, nil
}

// AbortMerge puts the repository back as it was before the merge or
// cherry-pick under way started: git merge --abort, or git cherry-pick
// --abort, which for a sequence of picks abandons them all. The result's
// first value is what git said when it failed.
//
// git keeps the changes that were uncommitted before the operation started
// and that it did not touch; gitraffe only starts either with a clean working
// tree, so for its own there are none.
func AbortMerge(dir string) (string, error) {
	state, err := mergeOperation(dir)
	if err != nil {
		return "", err
	}
	top, err := Toplevel(dir)
	if err != nil {
		return "", err
	}
	return runWrite(top, "", state.Operation, "--abort")
}

// MergeMessage is the message ContinueMerge commits with when given none:
// the one git wrote for the merge or cherry-pick, less its comment lines
// (the list of files that conflicted, which git means for the editor and
// takes out when the editor closes). It is "" when there is none.
func MergeMessage(dir string) string {
	gitDir, err := Run(dir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(filepath.FromSlash(gitDir), "MERGE_MSG"))
	if err != nil {
		return ""
	}
	top, err := Toplevel(dir)
	if err != nil {
		return ""
	}
	// stripspace honours core.commentChar, as the commit does.
	out, err := runWrite(top, string(raw), "stripspace", "--strip-comments")
	if err != nil {
		return ""
	}
	return out
}

// ContinueMerge commits the merge or cherry-pick under way once every file is
// resolved, and returns the commit made.
//
// With message "" it is git's own message, as MergeMessage reads it;
// otherwise message as given, cleaned of stray blank lines only, as
// CommitStaged does. A cherry-pick keeps its commit's author. The
// repository's hooks run as for any commit, and a hook that refuses is
// reported rather than skipped.
//
// It refuses, changing nothing, when no merge or cherry-pick is under way; on
// a detached HEAD, where the commit would belong to no branch; while any file
// is unmerged; and while any file still holds conflict markers.
//
// The commit concludes one cherry-pick. Of a sequence started in a terminal,
// the picks still to come are left for git cherry-pick --continue there.
func ContinueMerge(dir, message string) (CommitResult, error) {
	var r CommitResult
	state, err := mergeOperation(dir)
	if err != nil {
		return r, err
	}
	if state.Detached {
		return r, ErrDetached
	}
	top, err := Toplevel(dir)
	if err != nil {
		return r, err
	}
	unmerged, err := unmergedFiles(top)
	if err != nil {
		return r, err
	}
	if len(unmerged) > 0 {
		paths := make([]string, len(unmerged))
		for i, f := range unmerged {
			paths[i] = f.Path
		}
		return r, fmt.Errorf("%w: %s", ErrUnmerged, strings.Join(paths, ", "))
	}
	markers, err := ConflictMarkers(top)
	if err != nil {
		return r, err
	}
	if len(markers) > 0 {
		return r, markersError(markers)
	}

	// --no-edit alone would keep git's comment lines in the message: it
	// cleans up only whitespace when no editor is shown. --cleanup=strip
	// takes them out, as closing the editor would.
	args := []string{"commit", "--quiet", "--no-edit", "--cleanup=strip"}
	stdin := ""
	if strings.TrimSpace(message) != "" {
		args = []string{"commit", "--quiet", "--cleanup=whitespace", "--file=-"}
		stdin = message
	}
	out, err := runWrite(top, stdin, args...)
	r.Output = out
	if err != nil {
		return r, err
	}
	r.Hash, _ = Run(top, "rev-parse", "HEAD")
	r.Subject, _ = Run(top, "log", "-1", "--format=%s")
	return r, nil
}

// MarkResolved records one unmerged file of the merge or cherry-pick under
// way as resolved, as it now stands in the working tree: git add, or, when
// the file is no longer there (the side that deleted it kept), git rm, which
// then touches only the index. The path is from the top of the working tree,
// as Conflicts gives it. The result's first value is what git said when it
// failed.
//
// It refuses, changing nothing, when no merge or cherry-pick is under way,
// for a file that is not unmerged, and while the file holds conflict markers.
func MarkResolved(dir, path string) (string, error) {
	if _, err := mergeOperation(dir); err != nil {
		return "", err
	}
	top, err := Toplevel(dir)
	if err != nil {
		return "", err
	}
	unmerged, err := unmergedFiles(top)
	if err != nil {
		return "", err
	}
	found := false
	for _, f := range unmerged {
		if f.Path == path {
			found = true
			break
		}
	}
	if !found {
		return "", ErrNotUnmerged
	}
	// Lstat: a symbolic link is there even when what it points at is not.
	if _, err := os.Lstat(filepath.Join(top, filepath.FromSlash(path))); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		return runWrite(top, "", "--literal-pathspecs", "rm", "--cached", "--quiet", "--", path)
	}
	markers, err := ConflictMarkers(top, path)
	if err != nil {
		return "", err
	}
	if len(markers) > 0 {
		return "", markersError(markers)
	}
	return runWrite(top, "", "--literal-pathspecs", "add", "--", path)
}
