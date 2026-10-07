package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// The diff box on the uncommitted changes shows a file whole: what is staged
// of it and what is not, hunk by hunk in the order they come in the file,
// each marked for the side of the index it is on. Staging a hunk changes its
// mark and leaves it in front of you, where a box that showed one side only
// made it vanish, and the only way to see it again was another row of the
// file list.
//
// Nothing is merged to draw it. Every line is a line of one of the two diffs
// git wrote — the index against HEAD, the working tree against the index —
// and remembers which and where, because lines are still staged by their
// place in the diff they came from (see git.StageLines). A hunk only part of
// which is staged is therefore two hunks here, one from each diff, the lines
// around them drawn with each.

// stagingLine is one line of the diff box.
type stagingLine struct {
	text   string
	staged bool // from the staged entry's diff
	at     int  // its place in the Body of the entry it came from
	num    git.LineNumber
}

// stagingDiff is what the diff box shows of the selected file: its lines, and
// the entries of the file list they were read from. On a commit there is one
// entry and nothing is staged; on the uncommitted changes a file may have
// one of each.
type stagingDiff struct {
	lines            []stagingLine
	staged, unstaged *fileDiff
}

// entry is the file-list entry for one side, if the file has that side.
func (d stagingDiff) entry(staged bool) (fileDiff, bool) {
	e := d.unstaged
	if staged {
		e = d.staged
	}
	if e == nil {
		return fileDiff{}, false
	}
	return *e, true
}

// numbers is each line's number in its file, for the gutter.
func (d stagingDiff) numbers() []git.LineNumber {
	nums := make([]git.LineNumber, len(d.lines))
	for i, l := range d.lines {
		nums[i] = l.num
	}
	return nums
}

// same reports whether two readings would draw the same lines with the same
// marks.
func (d stagingDiff) same(other stagingDiff) bool {
	if len(d.lines) != len(other.lines) {
		return false
	}
	for i, l := range d.lines {
		if l.text != other.lines[i].text || l.staged != other.lines[i].staged {
			return false
		}
	}
	return true
}

// stagingDiff reads the selected file's diff for the box: the selected entry
// and, on the uncommitted changes, the same file's entry for the other side.
// The renderer, the cursor and the mouse all ask here, so they cannot
// disagree about which line a row holds.
func (m model) stagingDiff() stagingDiff {
	return stagingDiffOf(m.viewedFiles(), m.commitView.file, m.commitView.workingTree)
}

// stagingDiffOf is stagingDiff for entry i of a file list. Only the
// uncommitted changes have a second side to look for.
func stagingDiffOf(files []fileDiff, i int, workingTree bool) stagingDiff {
	if i < 0 || i >= len(files) {
		return stagingDiff{}
	}
	var d stagingDiff
	set := func(f fileDiff) {
		if f.Staged {
			d.staged = &f
		} else {
			d.unstaged = &f
		}
	}
	set(files[i])
	if workingTree {
		for _, other := range files {
			if other.Path == files[i].Path && other.Staged != files[i].Staged {
				set(other)
				break
			}
		}
	}
	d.lines = mergeSides(d.staged, d.unstaged)
	return d
}

// mergeSides lays the hunks of a file's two diffs out in the order they come
// in the file. The two are counted in different files — HEAD and the index,
// the index and the working tree — but both know the index, so that is what
// they are ordered by: a staged hunk by where it starts in the index it made,
// an unstaged one by where it starts in the index it changes.
//
// A side with nothing to draw adds no lines, and one side alone is left in
// the order git wrote it.
func mergeSides(staged, unstaged *fileDiff) []stagingLine {
	type chunk struct {
		start int
		lines []stagingLine
	}
	var chunks []chunk
	sides := 0
	for _, f := range []*fileDiff{staged, unstaged} {
		if f == nil || strings.TrimSpace(f.Body) == "" {
			continue
		}
		sides++
		nums := f.LineNumbers()
		// What comes before a file's first hunk — the line git writes for a
		// binary file, an untracked file's contents — sorts ahead of it.
		start := -1
		var lines []stagingLine
		for i, text := range strings.Split(f.Body, "\n") {
			if strings.HasPrefix(text, "@@") {
				if len(lines) > 0 {
					chunks = append(chunks, chunk{start, lines})
					lines = nil
				}
				// A header that cannot be read keeps its place after the
				// hunk before it.
				if old, new, ok := git.HunkStart(text); ok {
					start = old
					if f.Staged {
						start = new
					}
				}
			}
			l := stagingLine{text: text, staged: f.Staged, at: i}
			if i < len(nums) {
				l.num = nums[i]
			}
			lines = append(lines, l)
		}
		if len(lines) > 0 {
			chunks = append(chunks, chunk{start, lines})
		}
	}
	if sides > 1 {
		// Stable, with the staged side read first: where the two start on
		// the same line, what is already staged comes first, as its entry
		// does in the file list.
		sort.SliceStable(chunks, func(i, j int) bool { return chunks[i].start < chunks[j].start })
	}

	var out []stagingLine
	for _, c := range chunks {
		out = append(out, c.lines...)
	}
	return out
}

// marked reports whether a line carries its side's mark: a hunk's header and
// the lines it changes. The lines around them are the same on both sides.
func (l stagingLine) marked() bool {
	return strings.HasPrefix(l.text, "@@") || strings.HasPrefix(l.text, "+") || strings.HasPrefix(l.text, "-")
}

// stageMarkWidth is the room the marks take in front of a diff on the
// uncommitted changes: the mark and a space.
const stageMarkWidth = 2

// stageMark is the column in front of a line of the uncommitted changes that
// says which side of the index it is on, with the file list's own marks: "●"
// for staged, "○" for not. picked gives it the band of the cursor or the
// selection, which every piece of a line has to carry for itself.
func stageMark(l stagingLine, picked bool) string {
	text := "  "
	style := helpStyle
	if l.marked() {
		text = unstagedMarker + " "
		if l.staged {
			text = stagedMarker + " "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.DiffAdd))
		}
	}
	if picked {
		style = style.Background(lipgloss.Color(theme.Current.SelectedBg))
	}
	return style.Render(text)
}

// diffLineCount is how many lines the diff box has for the cursor to be on:
// at least one, so that a file with nothing to draw still has a place for it.
func (m model) diffLineCount() int {
	return max(1, len(m.stagingDiff().lines))
}

// cursorLine is the line the diff's cursor is on, if the box has lines.
func (m model) cursorLine(d stagingDiff) (stagingLine, bool) {
	if len(d.lines) == 0 {
		return stagingLine{}, false
	}
	return d.lines[max(0, min(m.commitView.diffCursor, len(d.lines)-1))], true
}

// actsOnStaged reports whether "s" in the diff box would take something out
// of the index: the side of the line under the cursor, or of the line the
// selection was started on, since picked lines are all of one side.
func (m model) actsOnStaged(d stagingDiff) bool {
	if len(d.lines) == 0 {
		f, _ := m.selectedFile()
		return f.Staged
	}
	v := m.commitView
	at := v.diffCursor
	if v.selecting {
		at = v.diffAnchor
	}
	return d.lines[max(0, min(at, len(d.lines)-1))].staged
}
