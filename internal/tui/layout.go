package tui

import (
	"log"
)

const (
	// Date only, fixed width so the column aligns and reads the same in every
	// locale. Relative dates ("3 days ago") vary in width and go stale, since
	// the view isn't redrawn on a timer. The time is in the details panel.
	dateColumnFormat = "2006-01-02"

	minRightPanelWidth = 30
	// Beyond this an author name buys little and costs graph width on every row.
	maxAuthorColWidth = 24
	// Below this a name is truncated past recognition, so the column is dropped.
	minAuthorColWidth = 6
	// Below this a subject is cut so short it says less than the space it took.
	minMessageColWidth = 12
)

// panelLayout is how the window's width is shared between the commit list and
// the details panel, and within the list between its optional columns.
type panelLayout struct {
	leftWidth, rightWidth         int
	branchCol, dateCol, authorCol int
	// messageCol is the subject line at the end of each row, and only the
	// maximised graph has one: beside the details panel the message is already
	// on screen, and the room is better spent on the graph.
	messageCol int
}

// computePanelLayout sizes the panels. The graph always comes first; the room
// left beside it goes to branch labels, then dates, then authors. Refs say where
// you are, which matters most; a date is short and fixed, so it is cheaper to
// keep than a name. dateWidth is 0 when no date column is wanted.
func computePanelLayout(windowWidth, maxGraphWidth, maxBranchWidth, dateWidth, maxAuthorWidth int) panelLayout {
	// Base graph needs: 2 (selection "> ") + maxGraphWidth + 1 (space) +
	// 7 (hash) + borders(2) + padding(2) = maxGraphWidth + 14
	graphBase := maxGraphWidth + 14
	maxLeftWidth := windowWidth * 4 / 5

	var l panelLayout
	if graphBase > maxLeftWidth {
		// graph alone is wider than our normal cap; give it the full window
		l.leftWidth = windowWidth
	} else {
		// Room beside the graph must respect both the left panel's cap and the
		// details panel's minimum.
		room := min(windowWidth-graphBase-minRightPanelWidth, maxLeftWidth-graphBase)
		l.branchCol, l.dateCol, l.authorCol = allocateColumns(room, maxBranchWidth, dateWidth, maxAuthorWidth)

		l.leftWidth = graphBase
		if l.branchCol > 0 {
			l.leftWidth += l.branchCol + 1
		}
		if l.dateCol > 0 {
			l.leftWidth += l.dateCol + 1
		}
		if l.authorCol > 0 {
			l.leftWidth += l.authorCol + 1
		}
		l.leftWidth = max(l.leftWidth, 25)
	}

	l.rightWidth = windowWidth - l.leftWidth // fill remaining space

	// Ensure right panel has a minimum width, but never let total exceed window
	if l.rightWidth < minRightPanelWidth {
		l.rightWidth = minRightPanelWidth
		l.leftWidth = windowWidth - l.rightWidth
		if l.leftWidth < 15 {
			l.leftWidth = 15
			l.rightWidth = windowWidth - l.leftWidth
		}
	}

	// Final safety: total must not exceed window width
	if total := l.leftWidth + l.rightWidth; total > windowWidth {
		log.Printf("View: width overflow detected: left=%d + right=%d = %d > window=%d, adjusting",
			l.leftWidth, l.rightWidth, total, windowWidth)
		l.rightWidth = windowWidth - l.leftWidth
		if l.rightWidth < 10 {
			l.rightWidth = windowWidth / 3
			l.leftWidth = windowWidth - l.rightWidth
		}
	}

	return l
}

// currentLayout is how the window is divided right now. View draws from it,
// and the mouse reads it to work out which panel the pointer is over — the
// division has to be the one on screen, so both ask the same question.
func (m model) currentLayout() panelLayout {
	// Only graph mode draws the date column; the fallback list has no room for it.
	dateWidth := 0
	if len(m.displayRows) > 0 {
		dateWidth = len(dateColumnFormat)
	}
	// Maximised, the focused panel takes the window and the other is not drawn
	// at all; a width of 0 is what View reads to leave it out.
	switch {
	case m.maximised && m.focusedBox == 2:
		return panelLayout{rightWidth: m.windowWidth}
	case m.maximised:
		return computeMaximisedLayout(m.windowWidth, m.maxGraphWidth, m.maxBranchWidth, dateWidth, m.maxAuthorWidth)
	}
	return computePanelLayout(m.windowWidth, m.maxGraphWidth, m.maxBranchWidth, dateWidth, m.maxAuthorWidth)
}

// allocateColumns shares the room beside the graph between the optional
// columns, in priority order.
func allocateColumns(room, maxBranchWidth, dateWidth, maxAuthorWidth int) (branchCol, dateCol, authorCol int) {
	if maxBranchWidth > 0 && room > 1 {
		branchCol = min(maxBranchWidth, room-1) // -1: the space after the labels
		room -= branchCol + 1
	}
	// A cut-off date is useless, so it fits whole (plus its leading space) or
	// not at all.
	if dateWidth > 0 && room > dateWidth {
		dateCol = dateWidth
		room -= dateWidth + 1
	}
	// Columns drop strictly in reverse priority as the window narrows. Letting
	// a short name take room a date couldn't fit would make widening the
	// window swap the author column for the date column.
	dateSettled := dateWidth == 0 || dateCol > 0
	if maxAuthorWidth > 0 && room > 1 && dateSettled {
		authorCol = min(maxAuthorWidth, maxAuthorColWidth, room-1) // -1: the space before the name
		if authorCol < minAuthorColWidth {
			authorCol = 0
		}
	}
	return branchCol, dateCol, authorCol
}

// computeMaximisedLayout sizes the graph when it has the window to itself.
// There is no details panel to leave room for, so the columns get everything
// beside the graph and none of the usual caps apply.
func computeMaximisedLayout(windowWidth, maxGraphWidth, maxBranchWidth, dateWidth, maxAuthorWidth int) panelLayout {
	l := panelLayout{leftWidth: windowWidth}
	l.branchCol, l.dateCol, l.authorCol = allocateColumns(windowWidth-(maxGraphWidth+14), maxBranchWidth, dateWidth, maxAuthorWidth)
	// Whatever is left over goes to the message, but only if enough is left to
	// read: a few characters and an ellipsis say less than the empty space did.
	if spare := windowWidth - 4 - rowWidth(l, maxGraphWidth) - 1; spare >= minMessageColWidth {
		l.messageCol = spare
	}
	return l
}

// rowWidth is how much of a row the columns before the message take:
// "> " + labels + graph + " " + 7-char hash [+ " " + date] [+ " " + author].
func rowWidth(l panelLayout, maxGraphWidth int) int {
	w := 2 + maxGraphWidth + 1 + 7
	for _, col := range []int{l.branchCol, l.dateCol, l.authorCol} {
		if col > 0 {
			w += col + 1
		}
	}
	return w
}
