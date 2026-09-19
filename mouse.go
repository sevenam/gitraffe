package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

// mouseScrollLines is how far one wheel notch moves, matching what terminals
// and editors do. One line a notch makes the wheel feel broken; a whole page
// loses your place.
const mouseScrollLines = 3

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

// panelAt reports which panel covers a point, or 0 for the repository box, the
// status line and anything outside the window. Coordinates are 0-based from
// the top left.
func (m model) panelAt(x, y int) int {
	// The repository box is three rows (one line plus its border) and the
	// status line is the last, leaving the panels between them.
	if y < 3 || y >= m.windowHeight-1 || x < 0 || x >= m.windowWidth {
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
