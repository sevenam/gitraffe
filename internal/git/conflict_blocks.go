package git

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Conflict blocks: the stretches of a file in conflict between the
// <<<<<<< and >>>>>>> lines git wrote, each resolved on its own by keeping
// one side's lines, or both. Most conflicts are a few blocks in a file whose
// rest merged fine, and taking a side of the whole file would throw that
// away.
//
// The blocks are read the way git diff --check reads markers, which is what
// MarkResolved and ContinueMerge refuse on: a marker is exactly the file's
// marker size of one character ("<", "|", "=" or ">"), then whitespace or
// the end of the line. A longer run is text — it is what git writes for the
// conflicts inside a merge base that was itself merged (diff3 and zdiff3),
// and what a file of "=" underlines holds.

// DefaultMarkerSize is how long git makes a conflict marker unless the
// conflict-marker-size attribute says otherwise.
const DefaultMarkerSize = 7

// ConflictBlock is one conflict in a file, by line, counted from 0. Start is
// the <<<<<<< line, Base the ||||||| line (-1 without one: diff3 and zdiff3
// write it, the default style does not), Middle the ======= line and End the
// >>>>>>> line. The branch checked out's lines are between Start and Base (or
// Middle), the base's between Base and Middle, and the incoming lines between
// Middle and End.
type ConflictBlock struct {
	Start, Base, Middle, End int
}

// Ours, BaseLines and Theirs are the lines of each part of the block, of the
// file's lines it was read from.
func (b ConflictBlock) Ours(lines []string) []string {
	end := b.Middle
	if b.Base >= 0 {
		end = b.Base
	}
	return lines[b.Start+1 : end]
}

func (b ConflictBlock) BaseLines(lines []string) []string {
	if b.Base < 0 {
		return nil
	}
	return lines[b.Base+1 : b.Middle]
}

func (b ConflictBlock) Theirs(lines []string) []string {
	return lines[b.Middle+1 : b.End]
}

// Contains reports whether line i is in the block, its markers included.
func (b ConflictBlock) Contains(i int) bool {
	return i >= b.Start && i <= b.End
}

// isMarker reports whether line is a conflict marker of c, size long, as
// git diff --check tells one: whitespace or nothing after it. A "\r" left on
// the line by a CRLF file is whitespace.
func isMarker(line string, c byte, size int) bool {
	if len(line) < size {
		return false
	}
	for i := 0; i < size; i++ {
		if line[i] != c {
			return false
		}
	}
	if len(line) == size {
		return true
	}
	switch line[size] {
	case ' ', '\t', '\r', '\n', '\v', '\f':
		return true
	}
	return false
}

// ConflictBlocks reads the conflict blocks of a file's lines, in order, with
// size the file's marker size (DefaultMarkerSize, or MarkerSizes').
//
// A block is a <<<<<<<, at most one ||||||| before the =======, the =======
// and a >>>>>>>. Lines that look like markers but make no block — a =======
// with no <<<<<<< before it, a <<<<<<< never closed — are text, as is a
// second <<<<<<< inside a block, which starts the block again from there:
// git never writes one, so the first was text of the file's own. Inside the
// incoming side, only a >>>>>>> means anything.
func ConflictBlocks(lines []string, size int) []ConflictBlock {
	if size <= 0 {
		size = DefaultMarkerSize
	}
	var blocks []ConflictBlock
	none := ConflictBlock{Start: -1, Base: -1, Middle: -1, End: -1}
	b := none
	for i, line := range lines {
		switch {
		case b.Middle < 0 && isMarker(line, '<', size):
			// Outside a block, or again before its =======.
			b = ConflictBlock{Start: i, Base: -1, Middle: -1, End: -1}
		case b.Start < 0:
		case b.Middle < 0 && b.Base < 0 && isMarker(line, '|', size):
			b.Base = i
		case b.Middle < 0 && isMarker(line, '=', size):
			b.Middle = i
		case b.Middle >= 0 && isMarker(line, '>', size):
			b.End = i
			blocks = append(blocks, b)
			b = none
		}
	}
	return blocks
}

// Pick is what a conflict block is resolved to.
type Pick int

const (
	// PickOurs keeps the branch checked out's lines.
	PickOurs Pick = iota
	// PickTheirs takes the incoming lines.
	PickTheirs
	// PickBoth keeps both, the branch checked out's first.
	PickBoth
	// PickBothTheirsFirst keeps both, the incoming lines first.
	PickBothTheirsFirst
)

// ResolveBlockText is lines with block resolved as pick: the markers gone,
// the base gone, and the side or sides picked in their place.
func ResolveBlockText(lines []string, b ConflictBlock, pick Pick) []string {
	var keep []string
	switch pick {
	case PickOurs:
		keep = b.Ours(lines)
	case PickTheirs:
		keep = b.Theirs(lines)
	case PickBoth:
		keep = append(append([]string{}, b.Ours(lines)...), b.Theirs(lines)...)
	case PickBothTheirsFirst:
		keep = append(append([]string{}, b.Theirs(lines)...), b.Ours(lines)...)
	}
	out := make([]string, 0, len(lines)-(b.End-b.Start+1)+len(keep))
	out = append(out, lines[:b.Start]...)
	out = append(out, keep...)
	out = append(out, lines[b.End+1:]...)
	return out
}

// MarkerSizes is the conflict marker size of each of paths, from the
// conflict-marker-size attribute git writes markers by; DefaultMarkerSize
// where it is not set, or cannot be read.
func MarkerSizes(dir string, paths ...string) map[string]int {
	sizes := map[string]int{}
	for _, p := range paths {
		sizes[p] = DefaultMarkerSize
	}
	if len(paths) == 0 {
		return sizes
	}
	args := append([]string{"--literal-pathspecs", "check-attr", "-z", "conflict-marker-size", "--"}, paths...)
	out, err := rawOutput(dir, args...)
	if err != nil {
		return sizes
	}
	// <path> NUL <attribute> NUL <value> NUL, for each path.
	fields := strings.Split(out, "\x00")
	for i := 0; i+2 < len(fields); i += 3 {
		if n, err := strconv.Atoi(fields[i+2]); err == nil && n > 0 {
			sizes[fields[i]] = n
		}
	}
	return sizes
}

// ErrBlockChanged is returned by ResolveBlock, with nothing done, when the
// block is not on disk as it was shown: the file changed since it was read.
var ErrBlockChanged = errors.New("the conflict is not as it was read")

// BlockResult is what ResolveBlock did.
type BlockResult struct {
	// Before and After are the file's text before and after, as on disk,
	// for an undo to put back and to know as its own.
	Before, After string
	// Left is how many blocks the file still has.
	Left int
	// Resolved is set when that was the last block, and the file was marked
	// resolved; MarkErr says why not when it was the last and the file was
	// not (lines git takes for markers that make no block), and Output what
	// git said then.
	Resolved bool
	MarkErr  error
	Output   string
}

// ResolveBlock resolves block index of an unmerged file of the merge or
// cherry-pick under way as pick, and rewrites the file with that block's
// lines in place of the block, the rest left byte for byte as it was, line
// endings included. When it was the file's last block, the file is marked
// resolved (MarkResolved).
//
// shown is the block's lines as they were read for the screen, markers
// included, with any "\r" taken out; it refuses with ErrBlockChanged, changing
// nothing, when the block on disk is not those lines. It refuses likewise
// when no merge or cherry-pick is under way, and for a file that is not
// unmerged.
func ResolveBlock(dir, path string, index int, shown []string, pick Pick) (BlockResult, error) {
	var r BlockResult
	if _, err := mergeOperation(dir); err != nil {
		return r, err
	}
	top, err := Toplevel(dir)
	if err != nil {
		return r, err
	}
	if !isUnmerged(top, path) {
		return r, ErrNotUnmerged
	}
	full := filepath.Join(top, filepath.FromSlash(path))
	info, err := os.Lstat(full)
	if err != nil {
		return r, err
	}
	if !info.Mode().IsRegular() {
		return r, ErrBlockChanged
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return r, err
	}
	r.Before = string(raw)
	lines := strings.Split(r.Before, "\n")
	blocks := ConflictBlocks(lines, MarkerSizes(top, path)[path])
	if index < 0 || index >= len(blocks) {
		return r, ErrBlockChanged
	}
	b := blocks[index]
	on := lines[b.Start : b.End+1]
	if len(on) != len(shown) {
		return r, ErrBlockChanged
	}
	for i, l := range on {
		if strings.ReplaceAll(l, "\r", "") != shown[i] {
			return r, ErrBlockChanged
		}
	}

	r.After = strings.Join(ResolveBlockText(lines, b, pick), "\n")
	if err := os.WriteFile(full, []byte(r.After), info.Mode().Perm()); err != nil {
		return r, err
	}
	r.Left = len(blocks) - 1
	if r.Left == 0 {
		r.Output, r.MarkErr = MarkResolved(top, path)
		r.Resolved = r.MarkErr == nil
	}
	return r, nil
}

// isUnmerged reports whether path is unmerged in the index.
func isUnmerged(top, path string) bool {
	blobs, err := indexStages(top, path)
	return err == nil && (blobs[1] != "" || blobs[2] != "" || blobs[3] != "")
}

// RestoreConflictText puts an unmerged file of the merge or cherry-pick under
// way back to before, its text before a block was resolved: the undo of
// ResolveBlock. after is the text that resolving wrote. Where that was the
// file's last block and it was marked resolved, the file is made unmerged
// again first, from the index's record of the conflict, as Unresolve does.
//
// It refuses with ErrResolvedByHand, changing nothing, when the file is not
// after on disk (it was edited, or another block resolved, since), or when
// it was marked resolved and the index no longer holds what is on disk;
// with ErrNotResolved for a file that is neither unmerged nor a resolved
// conflict of this merge; and when no merge or cherry-pick is under way.
func RestoreConflictText(dir, path, before, after string) (string, error) {
	state, err := mergeOperation(dir)
	if err != nil {
		return "", err
	}
	top, err := Toplevel(dir)
	if err != nil {
		return "", err
	}
	full := filepath.Join(top, filepath.FromSlash(path))
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		return "", ErrResolvedByHand
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	if string(raw) != after {
		return "", ErrResolvedByHand
	}
	if !isUnmerged(top, path) {
		theirs := "MERGE_HEAD"
		if state.Operation == "cherry-pick" {
			theirs = "CHERRY_PICK_HEAD"
		}
		resolved, err := resolvedFiles(top, theirs, nil)
		if err != nil {
			return "", err
		}
		found := false
		for _, f := range resolved {
			found = found || f.Path == path
		}
		if !found {
			return "", ErrNotResolved
		}
		if _, err := Run(top, "--literal-pathspecs", "diff", "--quiet", "--no-ext-diff", "--", path); err != nil {
			return "", ErrResolvedByHand
		}
		if out, err := runWrite(top, "", "--literal-pathspecs", "update-index", "--unresolve", "--", path); err != nil {
			return out, err
		}
	}
	return "", os.WriteFile(full, []byte(before), info.Mode().Perm())
}

// RecreateConflicts writes every conflict of an unmerged file back into it,
// markers and all, as git left it when the merge stopped (git checkout -m),
// for when nothing remembers the text it had before. Whatever was resolved in
// the file, by block or by hand, is written over, so it is only ever done
// after asking.
//
// It refuses with ErrNotUnmerged, changing nothing, for a file that is not
// unmerged or that one side has no version of (there is no conflict to write
// into it), and when no merge or cherry-pick is under way.
func RecreateConflicts(dir, path string) (string, error) {
	if _, err := mergeOperation(dir); err != nil {
		return "", err
	}
	top, err := Toplevel(dir)
	if err != nil {
		return "", err
	}
	blobs, err := indexStages(top, path)
	if err != nil {
		return "", err
	}
	if blobs[2] == "" || blobs[3] == "" {
		return "", ErrNotUnmerged
	}
	return runWrite(top, "", "--literal-pathspecs", "checkout", "-m", "--", path)
}
