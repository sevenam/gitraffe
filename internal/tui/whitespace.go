package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Whitespace in a diff is half of what some diffs are about — a tab that
// became spaces, a space left at the end of a line — and none of it can be
// seen. So the two places that draw a diff, the details panel and the commit
// view, can spell it out: a mark for every space and every tab, in the quiet
// colour of the hints so the text still reads as text. "w" turns it on and
// off, and gitraffe remembers which.
//
// The marks are only drawn. Lines are still staged and copied as git wrote
// them, and a mark takes exactly the columns of what it stands for, so the
// widths the boxes count are unchanged.

const (
	spaceMark = "·"
	// A tab is drawn four columns wide (see expandTabs): the arrow, and the
	// rest of the way in blanks, so the mark reads as one thing.
	tabMark = "→   "
)

// diffContent splits a line of a diff into the column git writes in front of
// it and the line of the file after it. ok is false for a line that is not
// part of a file: a header, a note, the mark where a long diff was cut.
//
// The "+++" and "---" lines of a patch are headers, told from an added and a
// removed line the way styleDiffLine tells them.
func diffContent(line string) (prefix, content string, ok bool) {
	switch {
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return "", "", false
	case strings.HasPrefix(line, "+"), strings.HasPrefix(line, "-"), strings.HasPrefix(line, " "):
		return line[:1], line[1:], true
	}
	return "", "", false
}

// markWhitespace spells out the spaces and tabs of text, with nothing to set
// the marks apart: for a line drawn in one style from end to end.
func markWhitespace(text string) string {
	return strings.NewReplacer(" ", spaceMark, "\t", tabMark).Replace(text)
}

// renderWhitespace draws text in style with its spaces and tabs spelled out
// in the hints' colour. Each run is styled for itself: every styled piece
// ends in a reset, so one style around the whole would stop at the first
// mark.
func renderWhitespace(text string, style lipgloss.Style) string {
	var sb strings.Builder
	isSpace := func(r rune) bool { return r == ' ' || r == '\t' }
	runes := []rune(text)
	for start := 0; start < len(runes); {
		end := start
		for end < len(runes) && isSpace(runes[end]) == isSpace(runes[start]) {
			end++
		}
		run := string(runes[start:end])
		if isSpace(runes[start]) {
			sb.WriteString(helpStyle.Render(markWhitespace(run)))
		} else {
			sb.WriteString(style.Render(run))
		}
		start = end
	}
	return sb.String()
}

// toggleWhitespace is "w": the marks on or off, with a word on the status
// line, since on a diff with no tabs and nothing trailing the screen may not
// visibly change.
func (m model) toggleWhitespace() model {
	m.showWhitespace = !m.showWhitespace
	m.notice = "Whitespace hidden — w shows it"
	if m.showWhitespace {
		m.notice = "Whitespace shown: " + spaceMark + " is a space, " + strings.TrimSpace(tabMark) + " a tab — w hides it"
	}
	return m
}
