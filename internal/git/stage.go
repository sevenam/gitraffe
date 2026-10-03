package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Staging and committing: the part of gitraffe that writes what you chose
// into the repository. Nothing here discards anything. Staging copies changes
// into the index and unstaging takes them back out, and the working tree is
// never touched by either; a commit holds what was staged and nothing else.
//
// Everything runs from the top of the working tree, since the paths a diff
// names are relative to it wherever gitraffe was started. Each exported
// function finds the top once and hands it down, a git call being what a key
// press here is made of.

// WorkingState is what the repository is in the middle of, as far as staging
// and committing go.
type WorkingState struct {
	// Operation is "merge", "rebase", "cherry-pick" or "revert" while one is
	// under way, "conflict" when files are left unmerged by anything else
	// (a stash that did not apply cleanly), and "" otherwise. Staging then
	// means "this conflict is resolved", and a commit concludes the
	// operation: both are that operation's own steps, and a terminal's work.
	Operation string
	// Detached is HEAD on no branch. Changes can be staged, but a commit made
	// there belongs to nothing and is easy to lose.
	Detached bool
}

var (
	// ErrStaleDiff is returned, with nothing staged, when the file no longer
	// reads as it did when the lines were picked.
	ErrStaleDiff = errors.New("the file has changed")
	// ErrNoLines is returned when the lines picked hold no change.
	ErrNoLines = errors.New("no changed lines")
	// ErrNothingStaged is returned by CommitStaged when the index holds nothing HEAD
	// lacks.
	ErrNothingStaged = errors.New("nothing staged")
	// ErrDetached is returned by CommitStaged while HEAD is on no branch.
	ErrDetached = errors.New("HEAD is not on a branch")
	// ErrInProgress is returned while a merge, rebase or the like is under
	// way; see WorkingState.
	ErrInProgress = errors.New("an operation is in progress")
)

// ReadWorkingState reads the WorkingState of the repository dir is in.
func ReadWorkingState(dir string) WorkingState {
	var s WorkingState
	if _, err := Run(dir, "symbolic-ref", "-q", "HEAD"); err != nil {
		// Only when there is a HEAD to be detached: failing for any other
		// reason must not read as "on no branch".
		if _, err := Run(dir, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
			s.Detached = true
		}
	}

	// The worktree's own git directory, which is where these are kept.
	if gitDir, err := Run(dir, "rev-parse", "--absolute-git-dir"); err == nil {
		exists := func(name string) bool {
			_, err := os.Stat(filepath.Join(filepath.FromSlash(gitDir), name))
			return err == nil
		}
		switch {
		case exists("rebase-merge"), exists("rebase-apply"):
			s.Operation = "rebase"
		case exists("MERGE_HEAD"):
			s.Operation = "merge"
		case exists("CHERRY_PICK_HEAD"):
			s.Operation = "cherry-pick"
		case exists("REVERT_HEAD"):
			s.Operation = "revert"
		}
	}
	if s.Operation == "" {
		if unmerged, err := Run(dir, "ls-files", "--unmerged"); err == nil && unmerged != "" {
			s.Operation = "conflict"
		}
	}
	return s
}

// runWrite runs a git command that changes the repository, in top, with stdin
// for the commands that read one. The first result is everything git and its
// hooks printed, which is the reason when it fails.
func runWrite(top, stdin string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = top
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(strings.ReplaceAll(out.String(), "\r", "")), err
}

// sectionArgs is the "git diff" that lists one half of the uncommitted
// changes: the index against HEAD when staged, the working tree against the
// index when not.
//
// The lines picked on screen are sent back to git as a patch built from this
// same output, so it is pinned against whatever a config could change about
// it: prefixes, an external diff driver, a textconv filter. Renames are left
// as a deletion and an addition, which are two things that can be staged one
// at a time; a rename is one thing with two paths, which cannot.
func sectionArgs(staged bool, extra ...string) []string {
	// --literal-pathspecs: a path is a name, not a pattern, however many
	// stars or colons it holds.
	args := []string{"-c", "core.quotePath=false", "--literal-pathspecs", "diff"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames",
		"--src-prefix=a/", "--dst-prefix=b/")
	return append(args, extra...)
}

// rawOutput is a command's stdout as git wrote it, carriage returns and
// final newline included: a patch is only a patch with both.
func rawOutput(top string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = top
	out, err := cmd.Output()
	return string(out), err
}

// sectionFiles lists one half of the uncommitted changes, a file at a time.
func sectionFiles(top string, staged bool) []FileDiff {
	// The two readings are of the same thing and take as long as each other,
	// so they run side by side.
	namesCh := make(chan string, 1)
	go func() {
		names, err := rawOutput(top, sectionArgs(staged, "--name-only", "-z")...)
		if err != nil {
			names = ""
		}
		namesCh <- names
	}()
	raw, err := rawOutput(top, sectionArgs(staged)...)
	names := <-namesCh
	if err != nil {
		return nil
	}
	// Only the final newline goes: trimming all trailing space would take a
	// blank context line with it, and the lines here are counted.
	files := parseFileDiffs(strings.TrimRight(strings.ReplaceAll(raw, "\r", ""), "\n"))

	// The header a file is named in quotes a path with anything unusual in
	// it. This lists the same files in the same order with nothing quoted,
	// so the path is one that can be handed back to git.
	if list := strings.Split(strings.TrimRight(names, "\x00"), "\x00"); len(list) == len(files) {
		for i := range files {
			files[i].Path = list[i]
		}
	}
	for i := range files {
		files[i].Staged = staged
	}
	return files
}

// refuseInProgress is the check every change to the index makes first.
func refuseInProgress(top string) error {
	if op := ReadWorkingState(top).Operation; op != "" {
		return fmt.Errorf("%w: %s", ErrInProgress, op)
	}
	return nil
}

// change is the frame every change to the index is made in: from the top of
// the working tree, and not while an operation is under way.
func change(dir string, do func(top string) (string, error)) (string, error) {
	top, err := Toplevel(dir)
	if err != nil {
		return "", err
	}
	if err := refuseInProgress(top); err != nil {
		return "", err
	}
	return do(top)
}

// StageFile puts all of a file's changes in the index: a modification, a
// deletion, or a file git has not seen before. The result is what git said
// when it failed.
func StageFile(dir, path string) (string, error) {
	return change(dir, func(top string) (string, error) {
		return runWrite(top, "", "--literal-pathspecs", "add", "--", path)
	})
}

// UnstageFile takes all of a file's changes back out of the index, leaving
// the file itself as it is.
//
// "git reset", not "git restore --staged": restore needs a HEAD to restore
// from and fails in a repository with no commit yet, where reset simply
// forgets the file.
func UnstageFile(dir, path string) (string, error) {
	return change(dir, func(top string) (string, error) {
		return runWrite(top, "", "--literal-pathspecs", "reset", "--quiet", "--", path)
	})
}

// StageAll stages every change in the working tree, untracked files included.
func StageAll(dir string) (string, error) {
	return change(dir, func(top string) (string, error) {
		return runWrite(top, "", "add", "--all")
	})
}

// UnstageAll empties the index of everything HEAD lacks.
func UnstageAll(dir string) (string, error) {
	return change(dir, func(top string) (string, error) {
		return runWrite(top, "", "reset", "--quiet")
	})
}

// truncatedMark is the line parseFileDiffs and untrackedFile end a cut diff
// with.
const truncatedMark = "... (truncated)"

// StageLines stages the changed lines among first..last of a file's diff as
// it is listed (indexes into f.Body's lines), or unstages them when the entry
// is a staged one. A hunk is the lines from its header to the next; one line
// is a range of one.
//
// It reads the file's diff again and refuses with ErrStaleDiff if that is not
// what the lines were picked from: a line number in a diff that has since
// changed points at something else. The lines are then sent to "git apply
// --cached" as a patch of their own (see buildPatch).
func StageLines(dir string, f FileDiff, first, last int) (string, error) {
	return change(dir, func(top string) (string, error) {
		return stageLines(top, f, first, last)
	})
}

func stageLines(top string, f FileDiff, first, last int) (string, error) {
	whole := func() (string, error) {
		if f.Staged {
			return runWrite(top, "", "--literal-pathspecs", "reset", "--quiet", "--", f.Path)
		}
		return runWrite(top, "", "--literal-pathspecs", "add", "--", f.Path)
	}

	shown := strings.Split(f.Body, "\n")
	cut := len(shown) > 0 && shown[len(shown)-1] == truncatedMark
	if cut {
		shown = shown[:len(shown)-1]
	}
	if first > last {
		first, last = last, first
	}

	// An untracked file is listed as its contents, with no hunk header: the
	// diff git gives once it knows the file starts one line later.
	offset := 0
	if f.Untracked {
		if f.Binary || f.Body == "" {
			return whole()
		}
		// Intent to add: the file is entered in the index with no content,
		// which is what lets part of it be staged like part of any other.
		if detail, err := runWrite(top, "", "--literal-pathspecs", "add", "--intent-to-add", "--", f.Path); err != nil {
			return detail, err
		}
		offset = 1
	}
	// Taken back if nothing came of it, so a refusal leaves no trace.
	forget := func() {
		if f.Untracked {
			runWrite(top, "", "--literal-pathspecs", "reset", "--quiet", "--", f.Path)
		}
	}

	raw, err := rawOutput(top, sectionArgs(f.Staged, "--", f.Path)...)
	if err != nil {
		forget()
		return "", err
	}
	head, lines := splitFilePatch(raw)
	if len(lines) == 0 {
		if strings.TrimSpace(raw) == "" {
			forget()
			return "", ErrStaleDiff
		}
		// A binary file or a change of mode: nothing to pick from, so the
		// file goes as one.
		return whole()
	}
	if !f.Untracked && !sameDiff(shown, lines, cut) {
		return "", ErrStaleDiff
	}

	picked := map[int]bool{}
	changed := 0
	for i, line := range lines {
		if !isChange(line) {
			continue
		}
		changed++
		if shownAt := i - offset; shownAt >= first && shownAt <= last && shownAt < len(shown) {
			picked[i] = true
		}
	}
	switch {
	case len(picked) == 0:
		forget()
		return "", ErrNoLines
	case len(picked) == changed:
		// Every change in the file: git does that itself, and gets right the
		// things a patch of lines cannot say, such as a file being new or
		// deleted, or its mode.
		return whole()
	}

	args := []string{"apply", "--cached", "--whitespace=nowarn"}
	if f.Staged {
		args = append(args, "--reverse")
	}
	detail, err := runWrite(top, buildPatch(head, lines, picked, f.Staged), append(args, "-")...)
	if err != nil {
		forget()
	}
	return detail, err
}

// isChange reports whether a diff line adds or removes something.
func isChange(line string) bool {
	return strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-")
}

// sameDiff reports whether the lines listed on screen are the diff git gives
// now. A listing that was cut short has to match as far as it goes.
func sameDiff(shown, lines []string, cut bool) bool {
	if len(shown) > len(lines) || (!cut && len(shown) != len(lines)) {
		return false
	}
	for i, line := range shown {
		if line != strings.ReplaceAll(lines[i], "\r", "") {
			return false
		}
	}
	return true
}

// patchHead is the three lines of a file's diff a patch needs to say which
// file it is for.
type patchHead struct {
	diff, from, to string
}

// splitFilePatch takes one file's raw diff apart into its head and its hunk
// lines, headers included, each as git wrote it.
func splitFilePatch(raw string) (patchHead, []string) {
	var head patchHead
	all := strings.Split(strings.TrimSuffix(raw, "\n"), "\n")
	for i, line := range all {
		switch {
		case strings.HasPrefix(line, "@@"):
			return head, all[i:]
		case strings.HasPrefix(line, "diff --git "):
			head.diff = line
		case strings.HasPrefix(line, "--- "):
			head.from = line
		case strings.HasPrefix(line, "+++ "):
			head.to = line
		}
	}
	return head, nil
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

// buildPatch writes the patch that stages the picked lines of a file's diff
// and no others, or with reverse the one that unstages them.
//
// A patch has to describe the file it is applied to exactly, so the lines not
// picked cannot simply be left out: each has to become what it is on the side
// the patch starts from. Staging applies it forwards to the index, which is
// the old side of the diff, where a removal not picked is still a line that
// is there (so it becomes context) and an addition not picked is not there
// yet (so it goes). Unstaging applies it backwards to the index, which is
// then the new side, and the two swap: an addition not picked stays as
// context, a removal not picked goes.
//
// The hunk headers are counted again to match, and the head is always that of
// a change to an existing file: "new file" and "deleted file" are true of the
// whole diff but not of some lines of it. (All of the lines never get here;
// see StageLines.)
func buildPatch(head patchHead, lines []string, picked map[int]bool, reverse bool) string {
	var sb strings.Builder
	sb.WriteString(head.diff + "\n")
	from, to := head.from, head.to
	if strings.HasPrefix(from, "--- /dev/null") {
		from = "--- " + swapPrefix(strings.TrimPrefix(to, "+++ "), "b/", "a/")
	}
	if strings.HasPrefix(to, "+++ /dev/null") {
		to = "+++ " + swapPrefix(strings.TrimPrefix(from, "--- "), "a/", "b/")
	}
	sb.WriteString(from + "\n" + to + "\n")

	// shift is how far the hunks written so far have moved the lines after
	// them: the side the patch does not start from is numbered by it.
	shift := 0
	for start := 0; start < len(lines); {
		end := start + 1
		for end < len(lines) && !strings.HasPrefix(lines[end], "@@") {
			end++
		}
		m := hunkHeader.FindStringSubmatch(lines[start])
		if m == nil {
			start = end
			continue
		}

		var body []string
		oldCount, newCount, any, kept := 0, 0, false, false
		for i := start + 1; i < end; i++ {
			line := lines[i]
			switch {
			case strings.HasPrefix(line, "\\"):
				// "\ No newline at end of file" is about the line before it,
				// and goes where that went.
				if kept {
					body = append(body, line)
				}
				continue
			case strings.HasPrefix(line, "+"):
				switch {
				case picked[i]:
					any = true
					newCount++
				case reverse:
					line = " " + line[1:]
					oldCount++
					newCount++
				default:
					kept = false
					continue
				}
			case strings.HasPrefix(line, "-"):
				switch {
				case picked[i]:
					any = true
					oldCount++
				case reverse:
					kept = false
					continue
				default:
					line = " " + line[1:]
					oldCount++
					newCount++
				}
			default:
				oldCount++
				newCount++
			}
			kept = true
			body = append(body, line)
		}
		start = end
		if !any {
			continue
		}

		// A side with no lines is numbered by the line before it, which is
		// git's convention for "after line n".
		oldStart, _ := strconv.Atoi(m[1])
		newStart, _ := strconv.Atoi(m[3])
		if reverse {
			oldStart = position(newStart, hunkCount(m[4])) - shift
			if oldCount == 0 {
				oldStart--
			}
		} else {
			newStart = position(oldStart, hunkCount(m[2])) + shift
			if newCount == 0 {
				newStart--
			}
		}
		shift += newCount - oldCount
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@%s\n", oldStart, oldCount, newStart, newCount, m[5])
		sb.WriteString(strings.Join(body, "\n") + "\n")
	}
	return sb.String()
}

// hunkCount reads a hunk header's line count, which git leaves out when it is 1.
func hunkCount(s string) int {
	if s == "" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}

// position is the line a hunk side starts at, given how its header numbers
// it: a side with no lines names the line before.
func position(start, count int) int {
	if count == 0 {
		return start + 1
	}
	return start
}

// swapPrefix turns the "b/path" of a diff's head into "a/path" or back,
// inside the quotes git puts round an unusual path as well as without them.
func swapPrefix(path, from, to string) string {
	if strings.HasPrefix(path, `"`+from) {
		return `"` + to + path[len(from)+1:]
	}
	if strings.HasPrefix(path, from) {
		return to + path[len(from):]
	}
	return path
}

// CommitResult is the commit CommitStaged made.
type CommitResult struct {
	Hash    string // full hash
	Subject string
	// Output is what git and the repository's hooks printed: the reason,
	// when the commit was refused.
	Output string
}

// CommitStaged commits what is staged, with message as given.
//
// The message goes in on stdin, so no editor opens and nothing in it is read
// as an option, and it is cleaned of stray blank lines only: a line starting
// with "#" is a line of the message, since no template put comments there.
// The repository's hooks run as they would for any commit, and a hook that
// refuses is reported rather than skipped: --no-verify is the user's call to
// make in a terminal, not ours.
//
// It refuses while there is nothing staged (an empty commit is not what "c"
// means), while HEAD is on no branch, and while a merge or rebase is under
// way, which a commit would conclude.
func CommitStaged(dir, message string) (CommitResult, error) {
	var r CommitResult
	state := ReadWorkingState(dir)
	switch {
	case state.Operation != "":
		return r, fmt.Errorf("%w: %s", ErrInProgress, state.Operation)
	case state.Detached:
		return r, ErrDetached
	}
	// --quiet exits 1 when there is a difference, and 0 when there is none.
	if _, err := Run(dir, "diff", "--cached", "--quiet"); err == nil {
		return r, ErrNothingStaged
	}

	top, err := Toplevel(dir)
	if err != nil {
		return r, err
	}
	out, err := runWrite(top, message, "commit", "--quiet", "--cleanup=whitespace", "--file=-")
	r.Output = out
	if err != nil {
		return r, err
	}
	r.Hash, _ = Run(dir, "rev-parse", "HEAD")
	r.Subject, _ = Run(dir, "log", "-1", "--format=%s")
	return r, nil
}
