package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func helpOpen() model {
	m := testModel()
	m.commits = make([]commit, 25)
	for i := range m.commits {
		m.commits[i].DiffLoaded = true
	}
	res, _ := m.Update(keyPress("?"))
	return res.(model)
}

func TestQuestionMarkOpensHelp(t *testing.T) {
	if !helpOpen().showHelp {
		t.Fatal("? did not open the help overlay")
	}
}

func TestHelpClosesWithoutQuitting(t *testing.T) {
	for _, key := range []tea.KeyMsg{keyPress("?"), keyPress("q"), {Type: tea.KeyEsc}} {
		t.Run(key.String(), func(t *testing.T) {
			res, cmd := helpOpen().Update(key)
			if res.(model).showHelp {
				t.Errorf("%s left the help open", key.String())
			}
			if isQuit(cmd) {
				t.Errorf("%s quit instead of closing the help", key.String())
			}
		})
	}
}

// Esc backs out of every view, so a spare press on the graph must not quit.
func TestEscDoesNotQuitFromGraph(t *testing.T) {
	m := testModel()
	m.commits = make([]commit, 25)
	for i := range m.commits {
		m.commits[i].DiffLoaded = true
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc}); isQuit(cmd) {
		t.Error("esc quit from the graph")
	}
	if _, cmd := m.Update(keyPress("q")); !isQuit(cmd) {
		t.Error("q no longer quits from the graph")
	}
}

func TestCtrlCStillQuitsFromHelp(t *testing.T) {
	_, cmd := helpOpen().Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuit(cmd) {
		t.Error("ctrl+c did not quit while the help was open")
	}
}

// The overlay hides the panels, so keys meant for them must not act unseen.
func TestKeysDoNotLeakThroughHelp(t *testing.T) {
	for _, key := range []string{"j", "G", "2", "L", "U"} {
		t.Run(key, func(t *testing.T) {
			before := helpOpen()
			before.latestVersion = "v9.9.9"
			res, _ := before.Update(keyPress(key))
			got := res.(model)
			if !got.showHelp {
				t.Errorf("%s closed the help", key)
			}
			if got.selected != before.selected || got.focusedBox != before.focusedBox ||
				got.colourLanes != before.colourLanes || got.updateState != before.updateState {
				t.Errorf("%s acted on the app behind the help", key)
			}
		})
	}
}

func TestHelpOverlayKeepsScreenSize(t *testing.T) {
	for _, size := range []struct{ w, h int }{{200, 40}, {80, 24}, {40, 12}} {
		m := helpOpen()
		m.windowWidth, m.windowHeight = size.w, size.h
		out := m.View()

		lines := strings.Split(out, "\n")
		if len(lines) != size.h {
			t.Errorf("%dx%d: %d lines, want %d", size.w, size.h, len(lines), size.h)
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > size.w {
				t.Errorf("%dx%d: line %d is %d wide", size.w, size.h, i, w)
			}
		}
		if size.h >= 40 && !strings.Contains(ansi.Strip(out), "Keyboard shortcuts") {
			t.Errorf("%dx%d: help box not drawn", size.w, size.h)
		}
	}
}

func TestStatusLineAdvertisesHelp(t *testing.T) {
	m := testModel()
	if !strings.HasPrefix(ansi.Strip(m.renderStatusLine()), "?: help") {
		t.Error("status line does not lead with the help key")
	}
}

// shortHelp is the help open in a window too short for its list.
func shortHelp() model {
	m := helpOpen()
	m.windowWidth, m.windowHeight = 100, 16
	return m
}

func helpScreen(m model) string {
	return ansi.Strip(m.View())
}

// A window shorter than the list shows the top of it, says there is more,
// and the keys that move through any box move through this one.
func TestHelpScrollsInAShortWindow(t *testing.T) {
	m := shortHelp()
	screen := helpScreen(m)
	if got := strings.Count(screen, "\n") + 1; got != m.windowHeight {
		t.Fatalf("the screen is %d lines, want %d", got, m.windowHeight)
	}
	for _, want := range []string{"Keyboard shortcuts", "toggle this help", "↑/↓: scroll • ? / esc / q: close", strings.TrimSpace(markBelow)} {
		if !strings.Contains(screen, want) {
			t.Errorf("the top of the help does not show %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, strings.TrimSpace(markAbove)) || strings.Contains(screen, "back to the top") {
		t.Errorf("the top of the help shows what is below it, or a mark for what is above:\n%s", screen)
	}

	// A line at a time, with the title and the footer staying put.
	down := press(m, keyPress("j"))
	if down.helpScroll != 1 || strings.Contains(helpScreen(down), "General") {
		t.Errorf("j scrolled to %d; want the first line gone from view", down.helpScroll)
	}
	if s := helpScreen(down); !strings.Contains(s, "Keyboard shortcuts") || !strings.Contains(s, "esc / q: close") || !strings.Contains(s, strings.TrimSpace(markAbove)) {
		t.Errorf("scrolled, the box lost its title, its footer or the mark for what is above:\n%s", s)
	}
	if up := press(down, keyPress("k")); up.helpScroll != 0 {
		t.Errorf("k scrolled to %d, want back at the top", up.helpScroll)
	}

	// The end: the last key of the list, and nothing more below.
	end := press(m, keyPress("G"))
	screen = helpScreen(end)
	if end.helpScroll != end.helpMaxScroll() || !strings.Contains(screen, "back to the top") {
		t.Errorf("G scrolled to %d of %d:\n%s", end.helpScroll, end.helpMaxScroll(), screen)
	}
	if strings.Contains(screen, strings.TrimSpace(markBelow)) {
		t.Errorf("the end of the help says there is more below:\n%s", screen)
	}
	// It stops at both ends.
	if past := press(end, keyPress("j"), tea.KeyMsg{Type: tea.KeyPgDown}); past.helpScroll != end.helpScroll {
		t.Errorf("scrolled past the end, to %d", past.helpScroll)
	}
	if top := press(end, keyPress("g"), keyPress("k"), tea.KeyMsg{Type: tea.KeyPgUp}); top.helpScroll != 0 {
		t.Errorf("g then up scrolled to %d, want the top", top.helpScroll)
	}

	// A page is a screenful less a line, so the eye keeps its place.
	if page := press(m, tea.KeyMsg{Type: tea.KeyPgDown}); page.helpScroll != helpRows(m.windowHeight)-1 {
		t.Errorf("a page scrolled %d lines of %d on screen", page.helpScroll, helpRows(m.windowHeight))
	}
}

// Scrolling from the top to the bottom shows every key of every screen's
// help, which is the point of it: none is out of reach in a small window.
func TestEveryHelpLineCanBeReached(t *testing.T) {
	for name, sections := range map[string][]helpSection{
		"graph": helpSections, "commit view": commitViewHelp, "uncommitted changes": stagingViewHelp,
	} {
		for _, height := range []int{10, 16, 24} {
			seen := ""
			last := len(helpLines(sections)) - helpRows(height)
			for scroll := 0; scroll <= max(0, last); scroll++ {
				box := ansi.Strip(renderHelpSections(sections, scroll, 100, height))
				if rows := strings.Count(box, "\n") + 1; rows > height {
					t.Fatalf("%s at height %d: the box is %d rows", name, height, rows)
				}
				seen += box + "\n"
			}
			for _, s := range sections {
				for _, b := range s.bindings {
					if !strings.Contains(seen, b.desc) {
						t.Errorf("%s at height %d: %q is never on screen", name, height, b.desc)
					}
				}
			}
		}
	}
}

func TestHelpScrollsWithTheWheel(t *testing.T) {
	m := shortHelp()
	selected := m.selected
	down := res(m.Update(wheel(5, 5, tea.MouseButtonWheelDown)))
	if down.helpScroll != mouseScrollLines || down.selected != selected {
		t.Errorf("a notch down scrolled the help to %d and left the graph on %d; want %d and %d",
			down.helpScroll, down.selected, mouseScrollLines, selected)
	}
	if up := res(down.Update(wheel(5, 5, tea.MouseButtonWheelUp))); up.helpScroll != 0 {
		t.Errorf("a notch up scrolled to %d, want the top", up.helpScroll)
	}
	// Past the top is the top, and a click behind the box selects nothing.
	if up := res(m.Update(wheel(5, 5, tea.MouseButtonWheelUp))); up.helpScroll != 0 {
		t.Errorf("a notch up from the top scrolled to %d", up.helpScroll)
	}
	if clicked := res(m.Update(click(5, 6))); clicked.selected != selected || !clicked.showHelp {
		t.Error("a click went through the help to the graph")
	}
}

// With room for the whole list there is nothing to scroll and nothing said
// about scrolling.
func TestHelpThatFitsDoesNotScroll(t *testing.T) {
	m := helpOpen()
	m.windowHeight = 60
	screen := helpScreen(m)
	if strings.Contains(screen, "scroll • ?") || strings.Contains(screen, strings.TrimSpace(markBelow)) {
		t.Errorf("a help that fits offers to scroll:\n%s", screen)
	}
	if !strings.Contains(screen, "toggle this help") || !strings.Contains(screen, "back to the top") {
		t.Errorf("a help that fits does not show its first and last keys:\n%s", screen)
	}
	for _, key := range []string{"j", "G"} {
		if got := press(m, keyPress(key)); got.helpScroll != 0 {
			t.Errorf("%s scrolled a help that fits, to %d", key, got.helpScroll)
		}
	}
	if got := res(m.Update(wheel(5, 5, tea.MouseButtonWheelDown))); got.helpScroll != 0 {
		t.Errorf("the wheel scrolled a help that fits, to %d", got.helpScroll)
	}
}

// Opened again it starts from the top, and a window made taller while it is
// scrolled does not leave it scrolled past its end.
func TestHelpScrollIsNotCarriedOver(t *testing.T) {
	m := press(shortHelp(), keyPress("G"))
	if m.helpScroll == 0 {
		t.Fatal("G did not scroll")
	}
	again := press(m, keyPress("?"), keyPress("?"))
	if !again.showHelp || again.helpScroll != 0 {
		t.Errorf("open=%v scroll=%d after closing and opening, want it open at the top", again.showHelp, again.helpScroll)
	}

	m.windowHeight = 60
	screen := helpScreen(m)
	if !strings.Contains(screen, "toggle this help") || !strings.Contains(screen, "back to the top") {
		t.Errorf("made taller while scrolled, the help does not show all of itself:\n%s", screen)
	}
}

// A window narrower than the list cuts its lines short and keeps the box's
// shape, which a wrapped line would not.
func TestHelpInANarrowWindow(t *testing.T) {
	for _, width := range []int{30, 50} {
		box := ansi.Strip(renderHelpSections(helpSections, 0, width, 16))
		lines := strings.Split(box, "\n")
		if len(lines) != 16 {
			t.Errorf("width %d: the box is %d rows, want 16", width, len(lines))
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > width {
				t.Fatalf("width %d: a line is %d wide:\n%s", width, w, box)
			}
		}
	}
}

// The box is as wide as the widest line of the whole list, wherever it is
// scrolled to: sized to the lines on screen it would change width, and place,
// at every step.
func TestHelpKeepsItsWidthWhileScrolling(t *testing.T) {
	boxWidth := func(box string) int {
		w := 0
		for _, line := range strings.Split(ansi.Strip(box), "\n") {
			w = max(w, ansi.StringWidth(line))
		}
		return w
	}
	for name, sections := range map[string][]helpSection{
		"graph": helpSections, "commit view": commitViewHelp, "uncommitted changes": stagingViewHelp,
	} {
		for _, window := range []struct{ w, h int }{{200, 12}, {200, 16}, {80, 16}, {50, 12}} {
			// What it is with room for every line at once.
			whole := boxWidth(renderHelpSections(sections, 0, window.w, 200))
			last := max(0, len(helpLines(sections))-helpRows(window.h))
			for scroll := 0; scroll <= last; scroll++ {
				box := renderHelpSections(sections, scroll, window.w, window.h)
				if got := boxWidth(box); got != whole {
					t.Fatalf("%s in %dx%d, scrolled to %d: the box is %d wide, want %d as unscrolled",
						name, window.w, window.h, scroll, got, whole)
				}
				for _, line := range strings.Split(ansi.Strip(box), "\n") {
					if w := ansi.StringWidth(line); w != whole {
						t.Fatalf("%s in %dx%d, scrolled to %d: a row is %d wide in a box of %d:\n%s",
							name, window.w, window.h, scroll, w, whole, ansi.Strip(box))
					}
				}
			}
			if whole > window.w {
				t.Errorf("%s in %dx%d: the box is %d wide", name, window.w, window.h, whole)
			}
		}
	}

	// On screen, the box's left edge stays in the same column.
	m := shortHelp()
	edge := func(m model) int {
		for _, line := range strings.Split(helpScreen(m), "\n") {
			if strings.Contains(line, "Keyboard shortcuts") {
				return ansi.StringWidth(line[:strings.Index(line, "Keyboard shortcuts")])
			}
		}
		t.Fatal("no help on screen")
		return 0
	}
	at := edge(m)
	for i := 0; i <= m.helpMaxScroll(); i++ {
		if got := edge(m); got != at {
			t.Fatalf("scrolled to %d, the box starts in column %d, was %d", m.helpScroll, got, at)
		}
		m = press(m, keyPress("j"))
	}
}
