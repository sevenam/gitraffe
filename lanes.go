package main

import "github.com/charmbracelet/lipgloss"

// laneColours are the colours the graph's lanes cycle through when lane
// colouring is on. Lane 0 is deliberately absent: the leftmost lane keeps the
// theme's own graph colour, so the trunk looks identical whether colouring is
// on or off and only the branches fanning out to the right gain a colour.
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

// laneAt maps a column of a "git log --graph" line to the lane it belongs to.
// Git draws lane n at column 2n and keeps the odd columns between them for the
// diagonals that create and collapse lanes.
//
// A diagonal takes the lane on its right — the one it is splitting off to, or
// merging in from — rather than the lane it leans against. That keeps a branch
// one colour along its whole length including the row where it leaves the
// trunk; taking the left lane instead would draw that first row in the trunk's
// colour and make the branch look like it starts a row late.
func laneAt(col int, ch rune) int {
	if ch == '/' || ch == '\\' {
		return (col + 1) / 2
	}
	return col / 2
}

// buildLanePalette turns laneColours into styles once per render, so a row
// with many lanes doesn't allocate a style at every colour change.
func buildLanePalette() []lipgloss.Style {
	styles := make([]lipgloss.Style, len(laneColours))
	for i, c := range laneColours {
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
