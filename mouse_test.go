package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func wheel(x, y int, button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: button, Action: tea.MouseActionPress}
}

// mouseModel is a loaded repository wide enough for two panels, with more
// commits than one wheel notch moves and one selected in the middle, so the
// wheel can move either way without clamping.
func mouseModel(t *testing.T) model {
	t.Helper()
	dir, git, commit := gitFixture(t)
	git("init", "-q", "-b", "main")
	for i := range 10 {
		commit(string(rune('a' + i)))
	}
	m := loadedModel(t, dir)
	m.windowWidth, m.windowHeight = 120, 30
	m.selected = 5
	m.detailsScroll = 10
	return m
}

func columns(m model) (inGraph, inDetails int) {
	l := m.currentLayout()
	return l.leftWidth / 2, l.leftWidth + l.rightWidth/2
}

func TestWheelScrollsThePanelUnderThePointer(t *testing.T) {
	m := mouseModel(t)
	graphX, detailsX := columns(m)

	res, _ := m.Update(wheel(graphX, 10, tea.MouseButtonWheelDown))
	got := res.(model)
	if got.selected != m.selected+mouseScrollLines || got.detailsScroll != 0 {
		t.Errorf("over the graph: selected %d→%d, scroll %d; want the selection moved and the diff back at the top",
			m.selected, got.selected, got.detailsScroll)
	}
	res, _ = m.Update(wheel(detailsX, 10, tea.MouseButtonWheelUp))
	got = res.(model)
	if got.detailsScroll != m.detailsScroll-mouseScrollLines || got.selected != m.selected {
		t.Errorf("over the details: scroll %d→%d, selected %d; want only the diff scrolled",
			m.detailsScroll, got.detailsScroll, got.selected)
	}
}

// Pointing at a panel says which one you mean, so the wheel must not need
// focus first — nor take it.
func TestWheelNeitherNeedsNorTakesFocus(t *testing.T) {
	m := mouseModel(t)
	m.focusedBox = 1
	_, detailsX := columns(m)

	res, _ := m.Update(wheel(detailsX, 10, tea.MouseButtonWheelDown))
	got := res.(model)
	if got.detailsScroll != m.detailsScroll+mouseScrollLines {
		t.Errorf("scroll = %d, want the unfocused panel scrolled anyway", got.detailsScroll)
	}
	if got.focusedBox != 1 {
		t.Errorf("focus = %d, want it left on the graph", got.focusedBox)
	}
}

func TestWheelStopsAtTheEnds(t *testing.T) {
	m := mouseModel(t)
	graphX, detailsX := columns(m)
	m.selected = 0
	m.detailsScroll = 1

	res, _ := m.Update(wheel(graphX, 10, tea.MouseButtonWheelUp))
	if got := res.(model); got.selected != 0 {
		t.Errorf("selected = %d at the newest commit, want 0", got.selected)
	}
	res, _ = m.Update(wheel(detailsX, 10, tea.MouseButtonWheelUp))
	if got := res.(model); got.detailsScroll != 0 {
		t.Errorf("scroll = %d at the top, want 0", got.detailsScroll)
	}

	m.selected = len(m.commits) - 1
	res, _ = m.Update(wheel(graphX, 10, tea.MouseButtonWheelDown))
	if got := res.(model); got.selected != len(m.commits)-1 {
		t.Errorf("selected = %d at the oldest commit, want %d", got.selected, len(m.commits)-1)
	}
}

func TestWheelIgnoresTheChromeAndOtherEvents(t *testing.T) {
	m := mouseModel(t)
	graphX, _ := columns(m)

	for _, tc := range []struct {
		name string
		msg  tea.MouseMsg
	}{
		{"the repository box", wheel(graphX, 1, tea.MouseButtonWheelDown)},
		{"the status line", wheel(graphX, m.windowHeight-1, tea.MouseButtonWheelDown)},
		{"a click", tea.MouseMsg{X: graphX, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}},
		{"pointer motion", tea.MouseMsg{X: graphX, Y: 10, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion}},
	} {
		res, cmd := m.Update(tc.msg)
		if got := res.(model); got.selected != m.selected || got.detailsScroll != m.detailsScroll || cmd != nil {
			t.Errorf("%s: selected=%d scroll=%d; want nothing to happen", tc.name, got.selected, got.detailsScroll)
		}
	}
}

// A box over the panels owns the screen, as it does for the keyboard.
func TestWheelDoesNothingBehindABox(t *testing.T) {
	m := mouseModel(t)
	graphX, _ := columns(m)
	for name, open := range map[string]model{
		"help": press(m, keyPress("?")),
		"theme picker": func() model {
			m := m
			m.configDir = t.TempDir()
			return press(m, keyPress("t"))
		}(),
		"repository box": func() model {
			m := m
			m.configDir = t.TempDir()
			return press(m, keyPress("o"))
		}(),
	} {
		res, _ := open.Update(wheel(graphX, 10, tea.MouseButtonWheelDown))
		if got := res.(model); got.selected != m.selected {
			t.Errorf("behind the %s box: selected %d→%d", name, m.selected, got.selected)
		}
	}
}

// Maximised there is only one panel, so the wheel belongs to it wherever the
// pointer is.
func TestWheelOnAMaximisedPanel(t *testing.T) {
	m := mouseModel(t)
	m.focusedBox = 2
	m = press(m, enter)

	for _, x := range []int{2, m.windowWidth - 2} {
		res, _ := m.Update(wheel(x, 10, tea.MouseButtonWheelDown))
		got := res.(model)
		if got.detailsScroll != m.detailsScroll+mouseScrollLines || got.selected != m.selected {
			t.Errorf("at column %d: scroll %d→%d, selected %d; want the details panel scrolled",
				x, m.detailsScroll, got.detailsScroll, got.selected)
		}
	}
}
