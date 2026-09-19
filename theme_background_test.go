package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// withColour renders in true colour for the test: under "go test" there is no
// terminal, so lipgloss would otherwise print no escape codes at all.
func withColour(t *testing.T) {
	saved := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(saved) })
}

// The background and, straight after it, the text colour (the message colour
// by default), which unstyled text such as the repository name relies on.
const lightGreyBase = "\x1b[48;2;229;231;235m\x1b[38;2;46;52;64m" // #e5e7eb, #2e3440

func TestBackgroundCoversTheWholeScreen(t *testing.T) {
	withColour(t)
	restoreTheme(t)
	light := defaultTheme()
	light.Background = "#e5e7eb"
	light.Message = "#2e3440"
	setTheme("test", light)

	m := testModel()
	m.windowWidth, m.windowHeight = 80, 24
	// With the help open too, so the overlay's own resets are covered.
	m = press(m, keyPress("?"))
	lines := strings.Split(m.View(), "\n")

	if len(lines) != 24 {
		t.Fatalf("%d lines, want 24", len(lines))
	}
	for i, l := range lines {
		if !strings.HasPrefix(l, lightGreyBase) {
			t.Errorf("line %d doesn't start with the background", i)
		}
		if w := ansi.StringWidth(l); w != 80 {
			t.Errorf("line %d is %d wide, want the full 80", i, w)
		}
		// Every reset but the line's last must be followed by the background,
		// or the rest of the line falls back to the terminal's.
		body := strings.TrimSuffix(l, "\x1b[0m")
		if n := strings.Count(body, "\x1b[0m"); n != strings.Count(body, "\x1b[0m"+lightGreyBase) {
			t.Errorf("line %d has a reset without the background after it", i)
		}
	}
}

func TestNoBackgroundLeavesTheScreenAlone(t *testing.T) {
	withColour(t)
	m := testModel()
	if strings.Contains(m.View(), "\x1b[48;") {
		t.Error("the default theme painted a background")
	}
}

func TestLightBackgroundDarkensTheLanes(t *testing.T) {
	restoreTheme(t)
	light := defaultTheme()
	light.Background = "#e5e7eb"
	light.Message = "#2e3440"
	setTheme("test", light)
	got := buildLanePalette()[5].GetForeground()
	if got != lipgloss.Color(laneColoursOnLight[5]) {
		t.Errorf("yellow lane = %v on a light background, want %s", got, laneColoursOnLight[5])
	}

	setTheme(defaultThemeName, defaultTheme())
	if got := buildLanePalette()[5].GetForeground(); got != lipgloss.Color(laneColours[5]) {
		t.Errorf("yellow lane = %v with no background, want %s", got, laneColours[5])
	}
}

func TestIsLightColour(t *testing.T) {
	for in, want := range map[string]bool{
		"#e5e7eb": true, "#ffffff": true, "#282c34": false, "#000000": false,
		"": false, "12": false, "#zzzzzz": false,
	} {
		if got := isLightColour(in); got != want {
			t.Errorf("isLightColour(%q) = %v, want %v", in, got, want)
		}
	}
}
