package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/sevenam/gitraffe/internal/theme"
)

func TestMain(m *testing.M) {
	// View() renders through the package-level styles, which main() normally
	// initialises before starting Bubble Tea. Built from the defaults, never the
	// user's own theme file, so results don't depend on whose machine runs them.
	theme.Current = theme.Default()
	// A real-looking version: main sets it at startup, and the update tests
	// need releases both older and newer than this build to exist.
	Version = "1.0.0"
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

func stripANSI(s string) string {
	return ansi.Strip(s)
}

func press(m model, keys ...tea.KeyMsg) model {
	for _, k := range keys {
		res, _ := m.Update(k)
		m = res.(model)
	}
	return m
}

// res is the model out of an Update, for the checks that want nothing else
// from it.
func res(next tea.Model, _ tea.Cmd) model {
	return next.(model)
}

// restoreTheme puts the default colours back after a test that changes them;
// every other test renders with those.
func restoreTheme(t *testing.T) {
	t.Cleanup(func() { setTheme(theme.DefaultName, theme.Default()) })
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

func withTrueColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func paths(files []fileDiff) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func findFile(t *testing.T, files []fileDiff, path string) fileDiff {
	t.Helper()
	for _, f := range files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("%q is not among %v", path, paths(files))
	return fileDiff{}
}
