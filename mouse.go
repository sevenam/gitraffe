package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

// mouseScrollLines is how far one wheel notch moves, matching what terminals
// and editors do. One line a notch makes the wheel feel broken; a whole page
// loses your place.
const mouseScrollLines = 3

const (
	// The repository box is three rows (one line plus its border), so the
	// panels start at row 3 and the first commit is drawn one row lower again,
	// under the panel border.
	panelsTopRow  = 3
	firstGraphRow = panelsTopRow + 1
)

// handleMouse scrolls the panel the pointer is over. Deliberately not the
// focused one: pointing at something is how a mouse says which thing it means,
// and scrolling a panel you can see but aren't focused on is the whole point of
// reaching for the mouse. Focus is left alone, so the keyboard still drives
// whatever it was driving.
func (m model) handleMouse(msg tea.MouseMsg) (model, tea.Cmd) {
	// A box over the panels owns the screen; scrolling what is behind it would
	// move things out of sight.
	if m.showHelp || m.picker.open || m.switcher.open || m.updateState != updateIdle {
		return m, nil
	}
	if !m.ready || m.err != nil || len(m.commits) == 0 {
		return m, nil
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		return m.selectClicked(msg.X, msg.Y)
	}

	var delta int
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		delta = -mouseScrollLines
	case tea.MouseButtonWheelDown:
		delta = mouseScrollLines
	default:
		// Every other mouse event, including the motion ones that arrive while
		// a button is held.
		return m, nil
	}

	switch m.panelAt(msg.X, msg.Y) {
	case 1:
		// The graph has no scroll offset of its own: the selected commit is
		// what the view follows, so the wheel moves the selection, as j and k do.
		m.selected = max(0, min(m.selected+delta, len(m.commits)-1))
		m.detailsScroll = 0
		return m, m.maybeLoadDiff()
	case 2:
		m.detailsScroll = max(0, m.detailsScroll+delta)
	}
	return m, nil
}

// selectClicked moves the selection to the commit under the pointer. Like the
// wheel it leaves focus alone: clicking a commit says which commit you mean,
// not which panel the keyboard should drive.
//
// A click anywhere else does nothing rather than reaching for the nearest
// commit: the blank rows under a short history, the line between two commits
// and the note saying the history was cut short are all places where no commit
// was pointed at, and moving the selection there would move it somewhere the
// click did not name.
func (m model) selectClicked(x, y int) (model, tea.Cmd) {
	if m.panelAt(x, y) != 1 {
		return m, nil
	}
	idx := m.commitAt(y)
	if idx < 0 || idx == m.selected {
		return m, nil
	}
	m.selected = idx
	m.detailsScroll = 0
	return m, m.maybeLoadDiff()
}

// commitAt is the commit drawn on screen row y, or -1 where the graph panel
// shows no commit. It reads the rows the graph is actually drawn from, so a
// history that scrolled, or one with graph-only lines between commits, lands
// on the commit the row shows rather than on the nth commit.
func (m model) commitAt(y int) int {
	start, end := m.graphWindow()
	row := start + y - firstGraphRow
	if row < start || row >= end {
		return -1
	}
	if len(m.displayRows) == 0 {
		return row
	}
	// -1 on a graph-only line and on the "more history" note.
	if idx := m.displayRows[row].CommitIdx; idx >= 0 && idx < len(m.commits) {
		return idx
	}
	return -1
}

// panelAt reports which panel covers a point, or 0 for the repository box, the
// status line and anything outside the window. Coordinates are 0-based from
// the top left.
func (m model) panelAt(x, y int) int {
	// The repository box is three rows (one line plus its border) and the
	// status line is the last, leaving the panels between them.
	if y < panelsTopRow || y >= m.windowHeight-1 || x < 0 || x >= m.windowWidth {
		return 0
	}
	layout := m.currentLayout()
	if layout.leftWidth > 0 && x < layout.leftWidth {
		return 1
	}
	if layout.rightWidth > 0 {
		return 2
	}
	return 0
}
