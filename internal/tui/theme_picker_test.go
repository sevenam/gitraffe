package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/config"
	"github.com/sevenam/gitraffe/internal/theme"
)

func pickerModel(t *testing.T) model {
	t.Helper()
	restoreTheme(t)
	// From a known theme: the picker opens on whichever one is in use, so a
	// theme left behind by another test would move the cursor.
	setTheme(theme.DefaultName, theme.Default())
	m := testModel()

	m.configDir = t.TempDir()
	res, _ := m.Update(keyPress("t"))
	return res.(model)
}

func TestThemePickerOpensOnCurrentTheme(t *testing.T) {
	restoreTheme(t)
	setTheme("tokyo-night-moon", theme.Default())
	m := testModel()
	m = press(m, keyPress("t"))
	if !m.picker.open {
		t.Fatal("t did not open the theme picker")
	}
	if got := m.picker.choices[m.picker.cursor].Name; got != "tokyo-night-moon" {
		t.Errorf("cursor on %q, want the current theme", got)
	}
}

func TestThemePickerPreviewsAndCancels(t *testing.T) {
	m := pickerModel(t)
	m = press(m, keyPress("j"))
	if theme.Current == theme.Default() {
		t.Fatal("moving the cursor did not preview the theme")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.picker.open || theme.Current != theme.Default() || theme.CurrentName != theme.DefaultName {
		t.Errorf("esc left open=%v name=%q; want closed with the defaults back", m.picker.open, theme.CurrentName)
	}
	if config.Load(m.configDir).Theme != "" {
		t.Error("cancelling saved a theme")
	}
}

func TestThemePickerSavesOnEnter(t *testing.T) {
	m := pickerModel(t)
	m = press(m, keyPress("j"), tea.KeyMsg{Type: tea.KeyEnter})
	want := "default-light"
	if m.picker.open || theme.CurrentName != want {
		t.Fatalf("open=%v name=%q; want closed on %s", m.picker.open, theme.CurrentName, want)
	}
	if got := config.Load(m.configDir).Theme; got != want {
		t.Errorf("settings.yml has %q, want %q", got, want)
	}
	if !strings.Contains(m.notice, "saved") {
		t.Errorf("notice = %q, want it to say the theme was saved", m.notice)
	}
}

func TestThemePickerReportsSaveFailure(t *testing.T) {
	m := pickerModel(t)
	m.configDir = ""
	m = press(m, keyPress("j"), tea.KeyMsg{Type: tea.KeyEnter})
	if theme.CurrentName != "default-light" {
		t.Errorf("theme = %q; it should still apply for the session", theme.CurrentName)
	}
	if !strings.Contains(m.notice, "could not save") {
		t.Errorf("notice = %q, want it to report the failure", m.notice)
	}
}

func TestThemePickerRefusesBrokenTheme(t *testing.T) {
	restoreTheme(t)
	m := testModel()
	m.configDir = t.TempDir()
	writeFile(t, filepath.Join(m.configDir, "themes", "zz-broken.yml"), "colors: [unclosed\n")
	m = press(m, keyPress("t"), keyPress("G"))

	if m.picker.loadErr == "" {
		t.Fatal("no error shown for a theme that doesn't parse")
	}
	if theme.Current != theme.Default() {
		t.Error("a broken theme left the previous preview's colours on screen")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.picker.open || config.Load(m.configDir).Theme != "" {
		t.Error("enter on a broken theme closed the picker or saved it")
	}
}

func TestKeysDoNotLeakThroughThemePicker(t *testing.T) {
	m := pickerModel(t)
	m.commits = make([]commit, 25)
	for _, key := range []string{"2", "c", "U", "?"} {
		got := press(m, keyPress(key))
		if !got.picker.open || got.focusedBox != m.focusedBox || got.colourLanes != m.colourLanes ||
			got.updateState != m.updateState || got.showHelp {
			t.Errorf("%s acted on the app behind the picker", key)
		}
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); !isQuit(cmd) {
		t.Error("ctrl+c did not quit from the picker")
	}
}

func TestThemePickerKeepsScreenSize(t *testing.T) {
	restoreTheme(t)
	for _, size := range []struct{ w, h int }{{200, 40}, {80, 24}, {40, 12}} {
		m := testModel()
		m.configDir = t.TempDir()
		// More themes than the smallest screen can list, to exercise scrolling.
		for i := range 20 {
			writeFile(t, filepath.Join(m.configDir, "themes", strings.Repeat("x", i+1)+".yml"), "")
		}
		m.windowWidth, m.windowHeight = size.w, size.h
		m = press(m, keyPress("t"), keyPress("G"))
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
		// Scrolled to the end, the highlighted last theme must be on screen.
		if size.w >= 80 && !strings.Contains(ansi.Strip(out), "> "+strings.Repeat("x", 20)) {
			t.Errorf("%dx%d: the selected theme scrolled out of view", size.w, size.h)
		}
	}
}
