package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/theme"
)

// laneColours are the colours the graph's lanes cycle through when lane
// colouring is on. Lane 0 is deliberately absent: the trunk keeps the theme's
// graph colour, so it looks identical whether colouring is on or off and only
// the branches fanning out gain a colour.
//
// These are fixed rather than theme keys. Every bundled theme already leaves
// the optional ref colours unset (see initStyles), so asking a theme for six
// more colours would in practice mean six more colours nobody sets.
var laneColours = []string{
	"#e06c75", // red
	"#61afef", // blue
	"#98c379", // green
	"#c678dd", // magenta
	"#56b6c2", // cyan
	"#e5c07b", // yellow
}

// laneColoursOnLight are the same six hues darkened for a theme that paints a
// light background, on which the pastels above wash out — the yellow most of
// all. Only a painted background can be known to be light: a theme that leaves
// the terminal's own showing keeps the pastels.
var laneColoursOnLight = []string{
	"#b83240", // red
	"#1f6fb2", // blue
	"#3f7a1e", // green
	"#8e3fa8", // magenta
	"#12808c", // cyan
	"#8a6400", // yellow
}

// buildLanePalette turns the lane colours into styles once per render, so a
// row with many lanes doesn't allocate a style at every colour change.
func buildLanePalette() []lipgloss.Style {
	colours := laneColours
	if theme.IsLight(theme.Current.Background) {
		colours = laneColoursOnLight
	}
	styles := make([]lipgloss.Style, len(colours))
	for i, c := range colours {
		styles[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(c))
	}
	return styles
}

// laneStyle picks a lane's style: the trunk's for lane 0, otherwise one from
// the palette, cycling once a repository has more lanes than there are colours.
func laneStyle(lane int, trunk lipgloss.Style, palette []lipgloss.Style) lipgloss.Style {
	if lane <= 0 || len(palette) == 0 {
		return trunk
	}
	return palette[(lane-1)%len(palette)]
}
