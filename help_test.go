package main

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

func TestCtrlCStillQuitsFromHelp(t *testing.T) {
	_, cmd := helpOpen().Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuit(cmd) {
		t.Error("ctrl+c did not quit while the help was open")
	}
}

// The overlay hides the panels, so keys meant for them must not act unseen.
func TestKeysDoNotLeakThroughHelp(t *testing.T) {
	for _, key := range []string{"j", "G", "2", "c", "U"} {
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
