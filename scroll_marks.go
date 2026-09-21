package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// scrollMarks says which ends of a box have content beyond them. A box that is
// cut short otherwise looks exactly like one that has ended, which is how the
// rest of a long diff went unread.
type scrollMarks struct{ above, below bool }

// marksFor is the marks for rows lines drawn from offset out of total.
func marksFor(total, offset, rows int) scrollMarks {
	return scrollMarks{above: offset > 0, below: rows > 0 && offset+rows < total}
}

// textLines counts lines up to the last one with anything on it. Content is
// often written a line at a time with a newline after each, and the empty
// line that leaves at the end is not more to read.
func textLines(lines []string) int {
	n := len(lines)
	for n > 0 && strings.TrimSpace(ansi.Strip(lines[n-1])) == "" {
		n--
	}
	return n
}

const (
	markAbove = " ⇡ "
	markBelow = " ⇣ "
)

// markScroll writes the marks into a rendered box's borders: "⇡" in the top
// one when there is more above, "⇣" in the bottom one when there is more
// below. The borders are the one place that is never content, so a mark there
// can't be mistaken for a line of the diff, and it costs the box no rows.
func markScroll(box string, s scrollMarks, border lipgloss.TerminalColor) string {
	if !s.above && !s.below {
		return box
	}
	lines := strings.Split(box, "\n")
	if len(lines) < 2 {
		return box
	}
	style := lipgloss.NewStyle().Bold(true).Foreground(border)
	if s.above {
		lines[0] = markBorder(lines[0], markAbove, style)
	}
	if s.below {
		lines[len(lines)-1] = markBorder(lines[len(lines)-1], markBelow, style)
	}
	return strings.Join(lines, "\n")
}

// markBorder puts mark over the middle of a border line, or as near the middle
// as it can go without covering anything that isn't plain border — the box's
// label shares the top line, and a narrow box may have no room at all.
func markBorder(line, mark string, style lipgloss.Style) string {
	plain := []rune(ansi.Strip(line))
	w := ansi.StringWidth(mark)
	isBorder := func(at int) bool {
		for i := at; i < at+w; i++ {
			if plain[i] != '─' {
				return false
			}
		}
		return true
	}
	// Spiral out from the middle, trying the right side first: the label
	// grows from the left.
	mid := (len(plain) - w) / 2
	for d := 0; d <= len(plain); d++ {
		for _, at := range []int{mid + d, mid - d} {
			// Never over the corners, so the box keeps its shape.
			if at >= 1 && at+w <= len(plain)-1 && isBorder(at) {
				return ansi.Truncate(line, at, "") + style.Render(mark) + ansi.TruncateLeft(line, at+w, "")
			}
		}
	}
	return line
}
