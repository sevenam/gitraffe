package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// busyRepo has enough commits, branches and authors for every optional column
// to be wanted, so widening the graph has something to spend the room on.
func busyRepo(t *testing.T) string {
	t.Helper()
	dir, git, commit := gitFixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("checkout", "-qb", "a-rather-long-branch-name")
	commit("second")
	git("checkout", "-q", "main")
	git("merge", "-q", "--no-ff", "a-rather-long-branch-name", "-m", "merge the branch")
	return dir
}

func panelsIn(screen string) (graph, details bool) {
	plain := ansi.Strip(screen)
	return strings.Contains(plain, "[1]-git-graph"), strings.Contains(plain, "[2]-commit-details")
}

// splitView puts a model back in the two-panel view. Gitraffe starts
// maximised, so a test that wants both panels has to ask for them.
func splitView(m model) model {
	m.maximised = false
	return m
}

func TestEnterTogglesMaximised(t *testing.T) {
	m := loadedModel(t, busyRepo(t))
	if !m.maximised {
		t.Fatal("a fresh model does not start maximised")
	}
	m = press(m, enter)
	if m.maximised {
		t.Fatal("enter did not go back to both panels")
	}
	if m = press(m, enter); !m.maximised {
		t.Error("enter again did not maximise the focused panel")
	}
}

func TestMaximisedDrawsOnlyTheFocusedPanel(t *testing.T) {
	base := splitView(loadedModel(t, busyRepo(t)))
	base.windowWidth, base.windowHeight = 120, 30

	if graph, details := panelsIn(base.View()); !graph || !details {
		t.Fatalf("split, both panels show; graph=%v details=%v", graph, details)
	}

	for _, tc := range []struct {
		focus                 int
		wantGraph, wantDetail bool
	}{
		{1, true, false},
		{2, false, true},
	} {
		m := base
		m.focusedBox = tc.focus
		m = press(m, enter)
		screen := m.View()

		if graph, details := panelsIn(screen); graph != tc.wantGraph || details != tc.wantDetail {
			t.Errorf("focus %d maximised: graph=%v details=%v, want %v and %v",
				tc.focus, graph, details, tc.wantGraph, tc.wantDetail)
		}
		lines := strings.Split(screen, "\n")
		if len(lines) != m.windowHeight {
			t.Errorf("focus %d maximised: %d lines, want %d", tc.focus, len(lines), m.windowHeight)
		}
		// The panel takes the whole width: its border reaches both edges.
		var widest int
		for _, l := range lines {
			widest = max(widest, ansi.StringWidth(l))
		}
		if widest != m.windowWidth {
			t.Errorf("focus %d maximised: widest line is %d, want the full %d", tc.focus, widest, m.windowWidth)
		}
	}
}

// The point of maximising the graph is the extra room, so the columns that had
// to be dropped beside the details panel should come back.
func TestMaximisedGraphGetsTheWholeWidth(t *testing.T) {
	const window = 100
	split := computePanelLayout(window, 6, 30, len(dateColumnFormat), 24)
	full := computeMaximisedLayout(window, 6, 30, len(dateColumnFormat), 24)

	if full.leftWidth != window || full.rightWidth != 0 {
		t.Errorf("widths = %d/%d, want the whole window and no details panel", full.leftWidth, full.rightWidth)
	}
	if full.branchCol < split.branchCol || full.authorCol <= split.authorCol {
		t.Errorf("columns maximised = branch %d author %d, split = branch %d author %d; want more room",
			full.branchCol, full.authorCol, split.branchCol, split.authorCol)
	}
}

func TestMaximisedIsRemembered(t *testing.T) {
	dir := t.TempDir()
	m := initialModel(".")
	m.configDir = dir
	m.maximised = true
	m.focusedBox = 2
	if err := savePreferences(m); err != nil {
		t.Fatal(err)
	}

	fresh := initialModel(".")
	fresh.configDir = dir
	if got := applyPreferences(fresh); !got.maximised || got.focusedBox != 2 {
		t.Errorf("maximised=%v focus=%d, want the maximised details panel back", got.maximised, got.focusedBox)
	}
}

// The interesting direction now that maximised is the default: leaving
// gitraffe split has to survive, which an absent key cannot say.
func TestSplitViewIsRemembered(t *testing.T) {
	dir := t.TempDir()
	m := initialModel(".")
	m.configDir = dir
	m.maximised = false
	if err := savePreferences(m); err != nil {
		t.Fatal(err)
	}

	fresh := initialModel(".")
	fresh.configDir = dir
	if applyPreferences(fresh).maximised {
		t.Error("a saved split view came back maximised")
	}
}

// An empty or absent settings.yml keeps the built-in default.
func TestNoSettingsStartsMaximised(t *testing.T) {
	fresh := initialModel(".")
	fresh.configDir = t.TempDir()
	if !applyPreferences(fresh).maximised {
		t.Error("with nothing saved, gitraffe did not start maximised")
	}
}

// Enter belongs to whatever box is open before it reaches the panels. Split
// to start with, so a stray toggle shows up as a maximised panel.
func TestEnterInABoxDoesNotMaximise(t *testing.T) {
	m := splitView(loadedModel(t, busyRepo(t)))
	m.configDir = t.TempDir()

	picker := press(m, keyPress("t"), tea.KeyMsg{Type: tea.KeyDown}, enter)
	if picker.maximised {
		t.Error("enter picked a theme and maximised a panel")
	}
	switcher := press(m, keyPress("o"), enter)
	if switcher.maximised {
		t.Error("enter in the repository box maximised a panel")
	}
}
