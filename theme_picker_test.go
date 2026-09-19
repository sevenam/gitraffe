package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// restoreTheme puts the default colours back after a test that changes them;
// every other test renders with those.
func restoreTheme(t *testing.T) {
	t.Cleanup(func() { setTheme(defaultThemeName, defaultTheme()) })
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(choices []themeChoice) []string {
	var out []string
	for _, c := range choices {
		out = append(out, c.name)
	}
	return out
}

// The binary carries its own copy, so a theme added to themes/ and left out of
// the embed would be missing for everyone who installed with "go install".
func TestEveryThemeFileIsBundled(t *testing.T) {
	onDisk, _ := filepath.Glob("themes/*.yml")
	if len(onDisk) == 0 {
		t.Fatal("no themes on disk")
	}
	choices := availableThemes("")
	for _, p := range onDisk {
		name := strings.TrimSuffix(filepath.Base(p), ".yml")
		c, ok := findTheme(choices, name)
		if !ok {
			t.Errorf("%s is not bundled", name)
			continue
		}
		colors, err := c.load()
		if err != nil {
			t.Errorf("%s: %v", name, err)
		} else if colors == defaultTheme() {
			t.Errorf("%s: every colour is still the default", name)
		}
	}
}

func TestAvailableThemes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "themes", "mine.yml"), "colors:\n  branch: \"#111111\"\n")
	writeFile(t, filepath.Join(dir, "themes", "tokyo-night-day.yml"), "colors:\n  branch: \"#222222\"\n")
	writeFile(t, filepath.Join(dir, "theme.yml"), "colors:\n  branch: \"#333333\"\n")

	// The default and its light variant first, then the other bundled themes by
	// name (the one replaced keeps its place), then yours.
	want := []string{"default", "default-light"}
	bundled, _ := filepath.Glob("themes/*.yml")
	for _, p := range bundled {
		if name := strings.TrimSuffix(filepath.Base(p), ".yml"); name != "default-light" {
			want = append(want, name)
		}
	}
	want = append(want, "mine", "theme.yml")

	choices := availableThemes(dir)
	if got := strings.Join(names(choices), ","); got != strings.Join(want, ",") {
		t.Fatalf("themes = %s\nwant     %s", got, strings.Join(want, ","))
	}

	// A file of yours with a bundled theme's name takes its place.
	c, _ := findTheme(choices, "tokyo-night-day")
	if colors, _ := c.load(); colors.Branch != "#222222" || c.source != "yours" {
		t.Errorf("tokyo-night-day = %s from %q, want yours with #222222", colors.Branch, c.source)
	}
}

func TestLoadThemeFromConfigDir(t *testing.T) {
	restoreTheme(t)
	custom := "colors:\n  branch: \"#333333\"\n"

	t.Run("theme.yml without a picked theme", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "theme.yml"), custom)
		if err := loadTheme("", dir); err != nil {
			t.Fatal(err)
		}
		if currentTheme.Branch != "#333333" || currentThemeName != customThemeName {
			t.Errorf("got %s (%q), want theme.yml", currentTheme.Branch, currentThemeName)
		}
	})

	t.Run("a picked theme wins over theme.yml", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "theme.yml"), custom)
		if err := saveThemeChoice(dir, "tokyo-night-storm"); err != nil {
			t.Fatal(err)
		}
		if err := loadTheme("", dir); err != nil {
			t.Fatal(err)
		}
		if currentThemeName != "tokyo-night-storm" || currentTheme.Branch == "#333333" {
			t.Errorf("got %q, want tokyo-night-storm", currentThemeName)
		}
	})

	t.Run("a picked theme that is gone falls back to theme.yml", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "theme.yml"), custom)
		if err := saveThemeChoice(dir, "deleted-since"); err != nil {
			t.Fatal(err)
		}
		if err := loadTheme("", dir); err != nil {
			t.Fatal(err)
		}
		if currentThemeName != customThemeName {
			t.Errorf("got %q, want theme.yml", currentThemeName)
		}
	})
}

func pickerModel(t *testing.T) model {
	t.Helper()
	restoreTheme(t)
	m := testModel()
	m.configDir = t.TempDir()
	res, _ := m.Update(keyPress("t"))
	return res.(model)
}

func press(m model, keys ...tea.KeyMsg) model {
	for _, k := range keys {
		res, _ := m.Update(k)
		m = res.(model)
	}
	return m
}

func TestThemePickerOpensOnCurrentTheme(t *testing.T) {
	restoreTheme(t)
	setTheme("tokyo-night-moon", defaultTheme())
	m := testModel()
	m = press(m, keyPress("t"))
	if !m.picker.open {
		t.Fatal("t did not open the theme picker")
	}
	if got := m.picker.choices[m.picker.cursor].name; got != "tokyo-night-moon" {
		t.Errorf("cursor on %q, want the current theme", got)
	}
}

func TestThemePickerPreviewsAndCancels(t *testing.T) {
	m := pickerModel(t)
	m = press(m, keyPress("j"))
	if currentTheme == defaultTheme() {
		t.Fatal("moving the cursor did not preview the theme")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.picker.open || currentTheme != defaultTheme() || currentThemeName != defaultThemeName {
		t.Errorf("esc left open=%v name=%q; want closed with the defaults back", m.picker.open, currentThemeName)
	}
	if loadSettings(m.configDir).Theme != "" {
		t.Error("cancelling saved a theme")
	}
}

func TestThemePickerSavesOnEnter(t *testing.T) {
	m := pickerModel(t)
	m = press(m, keyPress("j"), tea.KeyMsg{Type: tea.KeyEnter})
	want := "default-light"
	if m.picker.open || currentThemeName != want {
		t.Fatalf("open=%v name=%q; want closed on %s", m.picker.open, currentThemeName, want)
	}
	if got := loadSettings(m.configDir).Theme; got != want {
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
	if currentThemeName != "default-light" {
		t.Errorf("theme = %q; it should still apply for the session", currentThemeName)
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
	if currentTheme != defaultTheme() {
		t.Error("a broken theme left the previous preview's colours on screen")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.picker.open || loadSettings(m.configDir).Theme != "" {
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
