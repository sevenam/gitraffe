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
		{"P", "push this branch, or this commit's unpushed tag"},
		{"c", "check out this commit's branch"},
		{"b", "new branch at this commit, and switch to it"},
		{"d", "delete this commit's branch: local, remote or both"},
		{"o", "open this commit's pull request in a browser, or start one"},
		{"y", "copy this commit's hash (y), subject (s) or diff (d)"},
		{"B", "jump to a branch or tag"},
		{"tab / shift+tab", "cycle focus"},
		{"/", "search messages, authors and hashes"},
		{"n / N", "next / previous match"},
		{"h", "pick a file or directory and show only its history"},
		{"esc", "clear the filter"},
		{"L", "toggle graph lane colours"},
		{"w", "show or hide whitespace in diffs"},
		{"t", "choose a colour theme"},
		{"O", "open another repository"},
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
		{"o", "open this commit's pull request in a browser, or start one"},
		{"y", "copy this commit's hash (y), subject (s) or diff (d)"},
		{"w", "show or hide whitespace in the diff"},
		{"h", "files: show the selected file's history in the graph"},
		{"space / esc / q", "back to the graph"},
		{"ctrl+c", "quit"},
	}},
}

// stagingViewHelp is commitViewHelp for the uncommitted changes, where the
// view has keys a commit has no use for. It is a list of its own rather than
// a section added to the other: the two together are taller than a
// 24-row terminal, and the keys cut off would be the new ones. What only
// applies to a commit is left out to make the room.
var stagingViewHelp = []helpSection{
	{"Uncommitted changes", []keyBinding{
		{"s", "files: stage or unstage the file"},
		{"s", "diff: on a hunk's @@ line the hunk, below it the line"},
		{"v", "diff: pick lines for s, from here to where you move; esc lets go"},
		{"S", "files: stage or unstage everything; diff: this file"},
		{"c", "commit what is staged, or everything when nothing is"},
		{"1 / 2 / 3  tab", "focus commit / files / diff, or cycle"},
		{"↑ / ↓  k / j", "move in the focused box"},
		{"pgup/pgdn  g / G", "move by ten; first / last"},
		{"wheel / click", "move in, or select, what is under the pointer"},
		{"y / w", "copy the diff (d) / show or hide whitespace"},
		{"h", "files: show the selected file's history in the graph"},
		{"?", "toggle this help"},
		{"space / esc / q", "back to the graph"},
		{"ctrl+c", "quit"},
	}},
}

// The help is a box over the screen, and a window shorter than the list
// would cut it off at the bottom with no way to reach the rest. So the list
// scrolls inside the box, between a title and a footer that stay put, and the
// box is never taller than the window.

// helpChromeRows is what the box draws around the list: the border, the
// padding, the title and the footer, and the blank line under one and over
// the other.
const helpChromeRows = 8

// helpChromeCols is the border and the padding on both sides.
const helpChromeCols = 6

// currentHelp is the sections "?" shows on the screen that is open: each
// screen lists the keys it answers to.
func (m model) currentHelp() []helpSection {
	switch {
	case m.commitView.open && m.commitView.workingTree:
		return stagingViewHelp
	case m.commitView.open:
		return commitViewHelp
	}
	return helpSections
}

// helpLines is the list itself, a line per section title and binding, with a
// blank line between sections.
func helpLines(sections []helpSection) []string {
	keyWidth := 0
	for _, s := range sections {
		for _, b := range s.bindings {
			keyWidth = max(keyWidth, ansi.StringWidth(b.keys))
		}
	}

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.SectionHeader))
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Branch))

	var lines []string
	for i, s := range sections {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, sectionStyle.Render(s.title))
		for _, b := range s.bindings {
			// Pad before styling: the escape codes would otherwise count toward
			// the width and misalign the descriptions.
			lines = append(lines, "  "+
				keyStyle.Render(b.keys+strings.Repeat(" ", keyWidth-ansi.StringWidth(b.keys)))+
				"   "+b.desc)
		}
	}
	return lines
}

// helpRows is how many lines of the list a window this tall has room for.
func helpRows(windowHeight int) int {
	return max(1, windowHeight-helpChromeRows)
}

// helpMaxScroll is how far the list on screen can be scrolled: nothing, when
// the window holds all of it. The renderer, the keys and the wheel all ask
// here, so none can scroll past what another would draw.
func (m model) helpMaxScroll() int {
	return max(0, len(helpLines(m.currentHelp()))-helpRows(m.windowHeight))
}

// scrollHelp moves the list by delta lines and stops at its ends.
func (m model) scrollHelp(delta int) model {
	m.helpScroll = max(0, min(m.helpScroll+delta, m.helpMaxScroll()))
	return m
}

// helpKey is the keyboard while the help is open: the keys that scroll a box
// anywhere else scroll this one. The last result is false for a key that is
// not one of them.
func (m model) helpKey(key string) (model, bool) {
	page := max(1, helpRows(m.windowHeight)-1)
	switch key {
	case "j", "down":
		return m.scrollHelp(1), true
	case "k", "up":
		return m.scrollHelp(-1), true
	case "ctrl+d", "pgdown":
		return m.scrollHelp(page), true
	case "ctrl+u", "pgup":
		return m.scrollHelp(-page), true
	case "g", "home", "ctrl+home":
		return m.scrollHelp(-m.helpScroll), true
	case "G", "end", "ctrl+end":
		return m.scrollHelp(m.helpMaxScroll()), true
	}
	return m, false
}

// renderHelp draws the key reference for the screen that is open, as a
// bordered box no larger than the window.
func (m model) renderHelp() string {
	return renderHelpSections(m.currentHelp(), m.helpScroll, m.windowWidth, m.windowHeight)
}

// renderHelpSections renders one or more sections of the reference, so a
// screen can show only the keys it answers to. In a window too short for the
// list it draws the part from scroll on, and marks the border at whichever
// end has more, as the commit view's boxes do; in one too narrow it cuts the
// lines short. A window of no size is taken to have room for everything.
func renderHelpSections(sections []helpSection, scroll, windowWidth, windowHeight int) string {
	lines := helpLines(sections)
	footer := "? / esc / q: close"
	var marks scrollMarks
	if windowHeight > 0 && len(lines) > helpRows(windowHeight) {
		rows := helpRows(windowHeight)
		scroll = max(0, min(scroll, len(lines)-rows))
		marks = marksFor(len(lines), scroll, rows)
		lines = lines[scroll : scroll+rows]
		// Only when there is something to scroll: the hint is one more thing
		// to read on a screen that is already short of room.
		footer = "↑/↓: scroll • " + footer
	}

	content := append([]string{titleStyle.Padding(0).Render("Keyboard shortcuts"), ""}, lines...)
	content = append(content, "", helpStyle.Render(footer))
	if room := windowWidth - helpChromeCols; windowWidth > 0 {
		for i, line := range content {
			content[i] = ansi.Truncate(line, max(1, room), "…")
		}
	}

	border := lipgloss.Color(theme.Current.BorderActive)
	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(1, 2).
		Render(strings.Join(content, "\n"))
	return markScroll(box, marks, border)
}
