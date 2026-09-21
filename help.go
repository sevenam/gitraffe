package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
		{"p", "open this commit's pull request in a browser"},
		{"b", "jump to a branch or tag"},
		{"tab / shift+tab", "cycle focus"},
		{"/", "search messages, authors and hashes"},
		{"n / N", "next / previous match"},
		{"c", "toggle graph lane colours"},
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
		{"g / home", "first commit"},
		{"G / end", "last commit"},
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
		{"g / G", "first / last"},
		{"mouse wheel", "scroll the box under the pointer"},
		{"click", "select the file under the pointer"},
		{"?", "toggle this help"},
		{"p", "open this commit's pull request in a browser"},
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

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.SectionHeader))
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Branch))

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
		BorderForeground(lipgloss.Color(currentTheme.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}

// overlayCentre draws box over the middle of screen, leaving the screen visible
// around it so the help reads as a layer over the app rather than a new page.
// lipgloss v1 has no compositing, so each line is spliced by display column:
// the screen's left part, the box's line, then the screen's right part. The
// box is clipped when the screen is smaller than it.
func overlayCentre(screen, box string, width, height int) string {
	lines := strings.Split(screen, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	boxLines := strings.Split(box, "\n")
	if len(boxLines) > height {
		boxLines = boxLines[:height]
	}
	boxWidth := 0
	for _, l := range boxLines {
		boxWidth = max(boxWidth, ansi.StringWidth(l))
	}
	boxWidth = min(boxWidth, width)

	top := (height - len(boxLines)) / 2
	left := (width - boxWidth) / 2
	for i, bl := range boxLines {
		row := top + i
		bg := lines[row]
		// Pad a short screen line so the right-hand cut lands in the same column.
		if w := ansi.StringWidth(bg); w < width {
			bg += strings.Repeat(" ", width-w)
		}
		bl = ansi.Truncate(bl, boxWidth, "")
		if w := ansi.StringWidth(bl); w < boxWidth {
			bl += strings.Repeat(" ", boxWidth-w)
		}
		// The resets stop a colour left open in the screen's left part bleeding
		// into the box, and the box's colour into the screen's right part.
		lines[row] = ansi.Truncate(bg, left, "") + "\x1b[0m" + bl + "\x1b[0m" + ansi.TruncateLeft(bg, left+boxWidth, "")
	}
	return strings.Join(lines, "\n")
}
