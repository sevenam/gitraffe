package git

import (
	"strconv"
	"strings"
)

// LineNumber is where one line of a diff sits in its file: Old before the
// change and New after it. Zero means the line is not there on that side, so
// an added line has only New, a removed one only Old, and a line that is not
// part of the file at all (a hunk header, a file header) has neither.
type LineNumber struct {
	Old, New int
}

// LineNumbers gives each line of a diff its place in the file, one entry per
// line of body. body may be one file's hunks or a whole patch.
//
// A hunk ends when its header's counts run out rather than at the next line
// that looks like a header: after the last hunk of a file a whole patch goes
// on to "--- a/next", which starts as a removed line does, and inside a hunk a
// removed line that began "-- " reads "--- " too. Only the counts tell the two
// apart.
//
// A merge's combined diff ("@@@ -1,2 -1,2 +1,3 @@@") has a column per parent
// and no single old side to number; its lines are left without numbers.
func LineNumbers(body string) []LineNumber {
	lines := strings.Split(body, "\n")
	nums := make([]LineNumber, len(lines))

	var old, new, oldLeft, newLeft int
	for i, line := range lines {
		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			old, oldLeft = hunkRange(m[1], m[2])
			new, newLeft = hunkRange(m[3], m[4])
			continue
		}
		if oldLeft == 0 && newLeft == 0 {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+") && newLeft > 0:
			nums[i].New = new
			new, newLeft = new+1, newLeft-1
		case strings.HasPrefix(line, "-") && oldLeft > 0:
			nums[i].Old = old
			old, oldLeft = old+1, oldLeft-1
		// An empty line is an empty context line under diff.suppressBlankEmpty,
		// which drops the space git otherwise starts it with.
		case (strings.HasPrefix(line, " ") || line == "") && oldLeft > 0 && newLeft > 0:
			nums[i] = LineNumber{Old: old, New: new}
			old, oldLeft = old+1, oldLeft-1
			new, newLeft = new+1, newLeft-1
		case strings.HasPrefix(line, `\`):
			// "\ No newline at end of file": about the line above, not a line.
		default:
			// Not what the header promised, such as the mark left where a long
			// diff was cut. Stop rather than number what follows wrongly.
			oldLeft, newLeft = 0, 0
		}
	}
	return nums
}

// hunkRange reads one side of a hunk header: the line it starts at and how
// many lines it covers. git leaves the count out when it is one.
func hunkRange(start, count string) (first, lines int) {
	first, _ = strconv.Atoi(start)
	lines = 1
	if count != "" {
		lines, _ = strconv.Atoi(count)
	}
	return first, lines
}

// LineNumbers is LineNumbers for this file's body. An untracked file has no
// hunk header to count from: its body is the file itself, every line written
// as an addition, so the lines are numbered as they are in the file.
func (f FileDiff) LineNumbers() []LineNumber {
	if !f.Untracked {
		return LineNumbers(f.Body)
	}
	lines := strings.Split(f.Body, "\n")
	nums := make([]LineNumber, len(lines))
	for i, line := range lines {
		// Anything else is a note in place of the contents, or the mark
		// where they were cut.
		if strings.HasPrefix(line, "+") {
			nums[i].New = i + 1
		}
	}
	return nums
}
