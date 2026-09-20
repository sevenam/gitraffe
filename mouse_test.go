package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func wheel(x, y int, button tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: button, Action: tea.MouseActionPress}
}

func click(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
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
	m := splitView(loadedModel(t, dir))
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
		{"pointer motion", tea.MouseMsg{X: graphX, Y: 10, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion}},
	} {
		res, cmd := m.Update(tc.msg)
		if got := res.(model); got.selected != m.selected || got.detailsScroll != m.detailsScroll || cmd != nil {
			t.Errorf("%s: selected=%d scroll=%d; want nothing to happen", tc.name, got.selected, got.detailsScroll)
		}
	}
}

// A box over the panels owns the screen, as it does for the keyboard.
func TestMouseDoesNothingBehindABox(t *testing.T) {
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
		for event, msg := range map[string]tea.MouseMsg{
			"wheel": wheel(graphX, 10, tea.MouseButtonWheelDown),
			"click": click(graphX, 10),
		} {
			res, _ := open.Update(msg)
			if got := res.(model); got.selected != m.selected {
				t.Errorf("%s behind the %s box: selected %d→%d", event, name, m.selected, got.selected)
			}
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

// commitOnRow is the commit whose hash is printed on screen row y. It is read
// off the drawing rather than worked out, so these tests fail if the click
// geometry ever drifts from the renderer.
func commitOnRow(t *testing.T, m model, y int) int {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if y >= len(lines) {
		t.Fatalf("row %d is past the %d-line screen", y, len(lines))
	}
	for i, c := range m.commits {
		if strings.Contains(lines[y], c.Hash) {
			return i
		}
	}
	return -1
}

func TestClickSelectsTheCommitUnderThePointer(t *testing.T) {
	m := mouseModel(t)
	graphX, _ := columns(m)
	// Not the newest: its hash is in the repository box too, which would make
	// commitOnRow ambiguous.
	const row = 8
	want := commitOnRow(t, m, row)
	if want <= 0 {
		t.Fatalf("row %d holds no commit to click", row)
	}

	res, cmd := m.Update(click(graphX, row))
	got := res.(model)
	if got.selected != want {
		t.Errorf("selected = %d, want %d — the commit drawn on row %d", got.selected, want, row)
	}
	if got.detailsScroll != 0 {
		t.Errorf("detailsScroll = %d, want the diff back at the top of the new commit", got.detailsScroll)
	}
	if cmd == nil {
		t.Error("no command, want the clicked commit's diff loaded")
	}
}

// The graph scrolls with the selection rather than from an offset of its own,
// so a click has to read the same window the renderer drew.
func TestClickFindsTheRightCommitWhenScrolled(t *testing.T) {
	dir, git, commit := gitFixture(t)
	git("init", "-q", "-b", "main")
	for i := range 40 {
		commit(fmt.Sprintf("c%02d", i))
	}
	m := splitView(loadedModel(t, dir))
	m.windowWidth, m.windowHeight = 120, 20
	m.selected = 30
	graphX, _ := columns(m)

	for _, row := range []int{4, 9, 15} {
		want := commitOnRow(t, m, row)
		if want < 0 {
			t.Fatalf("row %d holds no commit", row)
		}
		res, _ := m.Update(click(graphX, row))
		if got := res.(model); got.selected != want {
			t.Errorf("clicking row %d selected %d, want %d — the commit drawn there", row, got.selected, want)
		}
	}
}

// Clicking says which commit, not which panel: the keyboard keeps driving what
// it was driving, as it does for the wheel.
func TestClickNeitherNeedsNorTakesFocus(t *testing.T) {
	m := mouseModel(t)
	m.focusedBox = 2
	graphX, _ := columns(m)
	const row = 8

	res, _ := m.Update(click(graphX, row))
	got := res.(model)
	if got.selected != commitOnRow(t, m, row) {
		t.Errorf("selected = %d, want the clicked commit though the graph is unfocused", got.selected)
	}
	if got.focusedBox != 2 {
		t.Errorf("focus = %d, want it left on the details panel", got.focusedBox)
	}
}

func TestClickWithNoCommitUnderItDoesNothing(t *testing.T) {
	m := mouseModel(t) // ten commits in a window with room for far more
	graphX, detailsX := columns(m)
	withNote := m
	withNote.moreCommits = true
	withNote.addMoreCommitsRow()

	for _, tc := range []struct {
		name string
		m    model
		x, y int
	}{
		{"below the oldest commit", m, graphX, 20},
		{"the repository box", m, graphX, 1},
		{"the status line", m, graphX, m.windowHeight - 1},
		{"the details panel", m, detailsX, 8},
		{"the more-history note", withNote, graphX, 4 + len(m.displayRows)},
	} {
		res, cmd := tc.m.Update(click(tc.x, tc.y))
		got := res.(model)
		if got.selected != tc.m.selected || got.detailsScroll != tc.m.detailsScroll || cmd != nil {
			t.Errorf("clicking %s: selected %d→%d, scroll %d→%d; want nothing to happen",
				tc.name, tc.m.selected, got.selected, tc.m.detailsScroll, got.detailsScroll)
		}
	}
}

// Clicking the commit already selected leaves it alone rather than reloading
// its diff.
func TestClickOnTheSelectedCommitIsQuiet(t *testing.T) {
	m := mouseModel(t)
	graphX, _ := columns(m)
	row := 4 + m.selected

	if got := commitOnRow(t, m, row); got != m.selected {
		t.Fatalf("row %d holds commit %d, want the selected %d", row, got, m.selected)
	}
	res, cmd := m.Update(click(graphX, row))
	if got := res.(model); got.selected != m.selected || cmd != nil {
		t.Errorf("selected %d→%d, cmd=%v; want the click to change nothing", m.selected, got.selected, cmd != nil)
	}
}

// The working tree row is a commit like any other as far as clicking goes.
func TestClickSelectsTheWorkingTreeRow(t *testing.T) {
	dir, _, _ := dirtyRepo(t)
	write(t, dir, "first", "changed")

	m := splitView(loadedModel(t, dir))
	m.windowWidth, m.windowHeight = 120, 30
	m.selected = 1
	graphX, _ := columns(m)

	res, _ := m.Update(click(graphX, 4))
	got := res.(model)
	if got.selected != 0 || !got.commits[got.selected].WorkingTree {
		t.Errorf("selected = %d, want the working tree row back", got.selected)
	}
}

// Gitraffe starts maximised, so the graph filling the window is the view most
// clicks land in.
func TestClickInTheMaximisedGraph(t *testing.T) {
	m := mouseModel(t)
	m.maximised = true
	graphX, _ := columns(m)
	const row = 8

	want := commitOnRow(t, m, row)
	if want <= 0 {
		t.Fatalf("row %d holds no commit to click", row)
	}
	if got := res(m.Update(click(graphX, row))).selected; got != want {
		t.Errorf("selected = %d, want %d — the commit drawn on row %d", got, want, row)
	}
	// The whole width is the graph, so the far right is still a commit.
	if got := res(m.Update(click(m.windowWidth-2, row))).selected; got != want {
		t.Errorf("clicking the right edge selected %d, want %d", got, want)
	}

	// With the details panel maximised there is no graph to click.
	m.focusedBox = 2
	if got := res(m.Update(click(graphX, row))).selected; got != m.selected {
		t.Errorf("selected %d→%d, want the click ignored where no graph is drawn", m.selected, got)
	}
}

// res is the model out of an Update, for the checks that want nothing else
// from it.
func res(next tea.Model, _ tea.Cmd) model {
	return next.(model)
}
