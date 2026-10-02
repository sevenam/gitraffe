package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/sevenam/gitraffe/internal/theme"
)

// paintBackground fills the screen with the theme's background colour, if it
// has one.
//
// Setting it on each style instead would take a background on every piece of
// text in the app, and would still miss the gaps between them: every styled
// piece ends in a full SGR reset, which clears the background along with the
// colour. So it is applied once, to the finished screen: set at the start of
// each line, set again after every reset, and each line padded to the full
// width so it reaches the right edge.
//
// The text colour is set alongside it. Plenty of text is drawn with no colour
// of its own (the repository name, diff context lines, the stats), which means
// the terminal's default, chosen to suit the terminal's background rather than
// the theme's: near-white on a dark terminal, and unreadable on a light theme.
func paintBackground(screen string, width int) string {
	base := baseSequence()
	if base == "" {
		return screen
	}
	lines := strings.Split(screen, "\n")
	for i, l := range lines {
		l = strings.ReplaceAll(l, "\x1b[0m", "\x1b[0m"+base)
		l = strings.ReplaceAll(l, "\x1b[m", "\x1b[m"+base)
		if pad := width - ansi.StringWidth(l); pad > 0 {
			l += strings.Repeat(" ", pad)
		}
		lines[i] = base + l + "\x1b[0m"
	}
	return strings.Join(lines, "\n")
}

// baseSequence is the escape code setting the theme's background and plain text
// colour in the terminal's colour profile. Empty when the theme sets no
// background, or the terminal shows no colour, in which case nothing is
// painted.
func baseSequence() string {
	profile := lipgloss.ColorProfile()
	bg := profile.Color(theme.Current.Background)
	if bg == nil || bg.Sequence(true) == "" {
		return ""
	}
	seq := termenv.CSI + bg.Sequence(true) + "m"
	if fg := profile.Color(textColour()); fg != nil && fg.Sequence(false) != "" {
		seq += termenv.CSI + fg.Sequence(false) + "m"
	}
	return seq
}

// textColour is the colour of text that has none of its own. It only applies
// with a painted background; defaults to the commit message colour, which is
// already the theme's main text colour.
func textColour() string {
	return firstColour(theme.Current.Foreground, theme.Current.Message)
}
