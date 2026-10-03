package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/theme"
)

// keyBinding is one row of the help overlay.
type keyBinding struct {
	keys, desc string
}

// helpSection groups bindings by where they apply. The same letter means
// different things per panel (j moves the selection in the graph but scrolls the
// details), so a flat list would have to repeat keys with qualifiers.
type helpSection struct {
	title    string
	bindings []keyBinding
}

// helpSections is written by hand rather than derived from Update's switch, so
// it has to be kept in step with it: a key added there and not here is a key
// nobody finds.
var helpSections = []helpSection{
	{"General", []keyBinding{
		{"?", "toggle this help"},
		{"1 / 2", "focus graph / details"},
		{"enter", "one panel or both"},
		{"space", "open the commit view"},
		{"p", "pull: fetch, then fast-forward this branch"},
		{"C", "check out this commit's branch"},
		{"P", "open this commit's pull request in a browser"},
		{"y", "copy this commit's hash (y), subject (s) or diff (d)"},
		{"b", "jump to a branch or tag"},
		{"tab / shift+tab", "cycle focus"},
		{"/", "search messages, authors and hashes"},
		{"n / N", "next / previous match"},
		{"h", "pick a file or directory and show only its history"},
		{"esc", "clear the filter"},
		{"L", "toggle graph lane colours"},
		{"t", "choose a colour theme"},
		{"o", "open another repository"},
		{"r / f5", "reload this repository"},
		{"f", "fetch from the remote, then reload"},
		{"m", "read more of a long history"},
		{"U", "update to the latest release"},
		{"mouse wheel", "scroll the panel under the pointer"},
		{"click", "select the commit under the pointer"},
		{"q / ctrl+c", "quit"},
	}},
	{"[1] git graph", []keyBinding{
		{"↑ / ↓  k / j", "previous / next commit"},
		{"ctrl+u / ctrl+d  pgup / pgdn", "move 10 commits"},
		{"home / end", "top / bottom of the screen"},
		{"g / ctrl+home", "first commit"},
		{"G / ctrl+end", "last commit"},
	}},
	{"[2] commit details", []keyBinding{
		{"↑ / ↓  k / j", "scroll one line"},
		{"ctrl+u / ctrl+d  pgup / pgdn", "scroll 10 lines"},
		{"g / home", "back to the top"},
	}},
}

// commitViewHelp is what "?" shows while the commit view is open: that
// screen's own keys rather than the graph's.
//
// The full reference is long enough to be clipped on a short terminal, and a
// section below the fold is no help to someone reading the screen it belongs
// to. The graph's list says what space does; the rest is here.
var commitViewHelp = []helpSection{
	{"Commit view", []keyBinding{
		{"1 / 2 / 3", "focus commit / files / diff"},
		{"tab / shift+tab", "cycle focus"},
		{"↑ / ↓  k / j", "move in the focused box"},
		{"pgup/pgdn  ctrl+u / ctrl+d", "move by ten"},
		{"home / end", "files: top / bottom of the screen; text: start / end"},
		{"g / G  ctrl+home / ctrl+end", "first / last"},
		{"mouse wheel", "scroll the box under the pointer"},
		{"click", "select the file under the pointer"},
		{"?", "toggle this help"},
		{"P", "open this commit's pull request in a browser"},
		{"y", "copy this commit's hash (y), subject (s) or diff (d)"},
		{"h", "files: show the selected file's history in the graph"},
		{"space / esc / q", "back to the graph"},
		{"ctrl+c", "quit"},
	}},
}

// renderHelpBox renders the whole key reference as a bordered box.
func renderHelpBox() string {
	return renderHelpSections(helpSections)
}

// renderHelpSections renders one or more sections of the reference, so a
// screen can show only the keys it answers to.
func renderHelpSections(sections []helpSection) string {
	keyWidth := 0
	for _, s := range sections {
		for _, b := range s.bindings {
			keyWidth = max(keyWidth, ansi.StringWidth(b.keys))
		}
	}

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.SectionHeader))
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Branch))

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Keyboard shortcuts"))
	for _, s := range sections {
		sb.WriteString("\n\n")
		sb.WriteString(sectionStyle.Render(s.title))
		for _, b := range s.bindings {
			sb.WriteString("\n  ")
			// Pad before styling: the escape codes would otherwise count toward
			// the width and misalign the descriptions.
			sb.WriteString(keyStyle.Render(b.keys + strings.Repeat(" ", keyWidth-ansi.StringWidth(b.keys))))
			sb.WriteString("   ")
			sb.WriteString(b.desc)
		}
	}
	sb.WriteString("\n\n")
	sb.WriteString(helpStyle.Render("? / esc / q: close"))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}
