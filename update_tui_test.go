package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestMain(m *testing.M) {
	// View() renders through the package-level styles, which main() normally
	// initialises before starting Bubble Tea. Built from the defaults, never the
	// user's own theme file, so results don't depend on whose machine runs them.
	currentTheme = defaultTheme()
	initStyles()
	os.Exit(m.Run())
}

func keyPress(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func testModel() model {
	m := initialModel(".")
	m.ready = true
	m.windowWidth = 200
	m.windowHeight = 40
	return m
}

func TestUpdateKeyWithNothingToDo(t *testing.T) {
	for _, tc := range []struct {
		name, latest, want string
	}{
		{"already current", "v" + version, "Already on the latest"},
		{"check failed", "", "could not reach GitHub"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel()
			m.latestVersion = tc.latest

			res, _ := m.Update(keyPress("U"))
			got := res.(model)

			if got.updateState != updateIdle {
				t.Errorf("state = %v, want idle", got.updateState)
			}
			if !strings.Contains(got.updateMessage, tc.want) {
				t.Errorf("message = %q, want it to contain %q", got.updateMessage, tc.want)
			}
		})
	}
}

func TestUpdateConfirmFlow(t *testing.T) {
	m := testModel()
	m.latestVersion = "v9.9.9"

	res, _ := m.Update(keyPress("U"))
	confirming := res.(model)
	if confirming.updateState != updateConfirming {
		t.Fatalf("state = %v, want confirming", confirming.updateState)
	}

	t.Run("anything but yes cancels", func(t *testing.T) {
		res, _ := confirming.Update(keyPress("n"))
		if got := res.(model); got.updateState != updateIdle || got.updateMessage != "" {
			t.Errorf("state = %v, message = %q; want idle and empty", got.updateState, got.updateMessage)
		}
	})

	// "j" answers the prompt (as "not yes") and must not also scroll the graph
	// behind it.
	t.Run("navigation does not leak through the prompt", func(t *testing.T) {
		res, _ := confirming.Update(keyPress("j"))
		if got := res.(model); got.selected != 0 {
			t.Errorf("j reached the graph: selected = %d, want 0", got.selected)
		}
	})

	res, cmd := confirming.Update(keyPress("y"))
	downloading := res.(model)
	if downloading.updateState != updateDownloading {
		t.Fatalf("after y: state = %v, want downloading", downloading.updateState)
	}
	if cmd == nil {
		t.Fatal("after y: no download command issued")
	}

	t.Run("navigation is inert while downloading", func(t *testing.T) {
		res, _ := downloading.Update(keyPress("j"))
		if got := res.(model); got.selected != 0 {
			t.Errorf("j reached the graph during download: selected = %d", got.selected)
		}
	})

	t.Run("quitting still works while downloading", func(t *testing.T) {
		_, cmd := downloading.Update(keyPress("q"))
		if cmd == nil || cmd() != (tea.QuitMsg{}) {
			t.Error("q did not quit")
		}
	})
}

func TestUpdateFinished(t *testing.T) {
	t.Run("failure is surfaced and stays in the TUI", func(t *testing.T) {
		m := testModel()
		m.updateState = updateDownloading

		res, cmd := m.Update(updateFinishedMsg{err: errors.New("boom")})
		got := res.(model)

		if got.updateState != updateIdle {
			t.Errorf("state = %v, want idle", got.updateState)
		}
		if !strings.Contains(got.updateMessage, "boom") {
			t.Errorf("message = %q, want it to mention the error", got.updateMessage)
		}
		if got.updatedTo != "" {
			t.Errorf("updatedTo = %q, want empty on failure", got.updatedTo)
		}
		if cmd != nil {
			t.Error("failure should not quit")
		}
	})

	t.Run("success records the version and quits", func(t *testing.T) {
		m := testModel()
		m.updateState = updateDownloading

		res, cmd := m.Update(updateFinishedMsg{version: "v9.9.9"})
		got := res.(model)

		if got.updateState != updateDone {
			t.Errorf("state = %v, want done", got.updateState)
		}
		// main reads updatedTo after Run returns to print the restart notice.
		if got.updatedTo != "v9.9.9" {
			t.Errorf("updatedTo = %q, want v9.9.9", got.updatedTo)
		}
		// Quitting is how the update gets installed on Windows, not just politeness.
		if cmd == nil || cmd() != (tea.QuitMsg{}) {
			t.Error("success did not quit")
		}
	})
}

func TestUpdateNoticeDismissedByNextKey(t *testing.T) {
	m := testModel()
	m.updateMessage = "Update failed: boom"

	res, _ := m.Update(keyPress("1"))
	if got := res.(model).updateMessage; got != "" {
		t.Errorf("message = %q, want it cleared", got)
	}
}

func TestStatusLineFitsAndKeepsLayout(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(m *model)
		want string
	}{
		{"no update available", func(m *model) { m.latestVersion = "v" + version }, "q: quit"},
		{"update available", func(m *model) { m.latestVersion = "v9.9.9" }, "U: update to v9.9.9"},
		{"confirming", func(m *model) {
			m.latestVersion = "v9.9.9"
			m.updateState = updateConfirming
		}, "(y/n)"},
		{"downloading", func(m *model) {
			m.updateState = updateDownloading
			m.updateMessage = "Downloading v9.9.9..."
		}, "Downloading v9.9.9"},
		{"error far longer than the window", func(m *model) {
			m.updateMessage = "Update failed: " + strings.Repeat("very long error text ", 30)
		}, "Update failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 120 columns is narrow enough that the help line plus an update
			// hint no longer fits, which is what makes the hint's position
			// load-bearing.
			m := testModel()
			m.windowWidth = 120
			m.windowHeight = 24
			tc.set(&m)

			lines := strings.Split(m.View(), "\n")
			if len(lines) != m.windowHeight {
				t.Fatalf("rendered %d lines, want exactly %d", len(lines), m.windowHeight)
			}
			if last := stripANSI(lines[len(lines)-1]); !strings.Contains(last, tc.want) {
				t.Errorf("status line = %q, want it to contain %q", last, tc.want)
			}
			for i, l := range lines {
				if w := lipgloss.Width(l); w > m.windowWidth {
					t.Errorf("line %d is %d columns wide, want at most %d", i, w, m.windowWidth)
				}
			}
		})
	}
}

func stripANSI(s string) string {
	return ansi.Strip(s)
}
