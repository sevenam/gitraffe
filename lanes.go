package main

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
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

// graphLanes numbers the lanes of a graph by reading the colours git puts on
// its own graph output.
//
// The colour has to come from git because a lane's column is not its identity.
// Git shuffles lanes sideways to make room — a branch can sit at column 2, be
// pushed out to column 4, and come back several rows later — and it reuses a
// column for an unrelated branch once the first has merged away. Colouring by
// column therefore recolours a branch halfway down and gives two unrelated
// branches the same colour. Git already tracks which lane is which in order to
// draw those shifts at all, so reading its colours is both correct and far
// less work than re-deriving that bookkeeping from the ASCII art.
type graphLanes struct {
	index map[string]int
}

func newGraphLanes() *graphLanes {
	return &graphLanes{index: map[string]int{}}
}

// parse splits one coloured graph line into the text to draw and the colour
// each character of it carries. Commit markers are the only characters git
// leaves uncoloured, so they come back as 0.
func (g *graphLanes) parse(line string) (string, []int) {
	var text strings.Builder
	var lanes []int
	lane := 0
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "\x1b[") {
			end := strings.IndexByte(line[i:], 'm')
			if end < 0 {
				// Not a colour sequence after all; stop rather than dropping
				// the rest of the row into the text.
				break
			}
			lane = g.laneFor(line[i+2 : i+end])
			i += end + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		text.WriteRune(r)
		lanes = append(lanes, lane)
		i += size
	}
	return text.String(), lanes
}

// laneFor numbers a colour, counting a reset as uncoloured. Colours are
// numbered in the order they are first seen rather than by their ANSI code, so
// a custom color.graph setting changes which palette entry a lane draws and
// never whether two lanes stay distinct.
func (g *graphLanes) laneFor(params string) int {
	if params == "" || params == "0" {
		return 0
	}
	if n, ok := g.index[params]; ok {
		return n
	}
	n := len(g.index) + 1
	g.index[params] = n
	return n
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
	if isLightColour(currentTheme.Background) {
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

// commitPaths gives every commit the id of the branch path it sits on. A
// commit's first parent keeps its path — that is what "the same branch" means
// once the branch name is gone — and every further parent of a merge starts a
// new one, because that is a branch that was merged in. Walking in display
// order lets the trunk claim its own chain before anything else can.
func commitPaths(commits []commit) []int {
	paths := make([]int, len(commits))
	for i := range paths {
		paths[i] = -1
	}
	byHash := make(map[string]int, len(commits))
	for i, c := range commits {
		byHash[c.Hash] = i
	}

	next := 0
	claim := func(start, path int) {
		for i := start; paths[i] == -1; {
			paths[i] = path
			if len(commits[i].Parents) == 0 {
				return
			}
			j, ok := byHash[commits[i].Parents[0]]
			if !ok {
				return
			}
			i = j
		}
	}
	for i := range commits {
		if paths[i] == -1 {
			claim(i, next)
			next++
		}
		for _, p := range commits[i].Parents[min(1, len(commits[i].Parents)):] {
			if j, ok := byHash[p]; ok && paths[j] == -1 {
				claim(j, next)
				next++
			}
		}
	}
	return paths
}

// cell is one drawn character of the graph.
type cell struct{ row, col int }

// resolveLanePaths rewrites each row's lane numbers, replacing the colours git
// assigned with the id of the branch the lane belongs to.
//
// Git's colours are not a branch identity on their own: git starts a fresh
// colour for the trunk after every merge, so main alone comes out red, then
// yellow, then blue. They are used only to join a lane's characters to each
// other across the rows where it shifts columns; the resulting run is then
// given the path of the commit it touches, which is what makes a branch one
// colour for its whole length.
func resolveLanePaths(rows []displayRow, commits []commit) {
	paths := commitPaths(commits)

	parent := map[cell]cell{}
	var find func(cell) cell
	find = func(c cell) cell {
		p, ok := parent[c]
		if !ok || p == c {
			return c
		}
		r := find(p)
		parent[c] = r
		return r
	}
	union := func(a, b cell) {
		if ra, rb := find(a), find(b); ra != rb {
			parent[ra] = rb
		}
	}
	ensure := func(c cell) {
		if _, seen := parent[c]; !seen {
			parent[c] = c
		}
	}

	runesOf := make([][]rune, len(rows))
	for r := range rows {
		runesOf[r] = []rune(rows[r].GraphChars)
	}
	colourAt := func(r, c int) (int, bool) {
		if r < 0 || r >= len(rows) || c < 0 || c >= len(rows[r].Lanes) {
			return 0, false
		}
		if c >= len(runesOf[r]) || runesOf[r][c] == ' ' {
			return 0, false
		}
		return rows[r].Lanes[c], true
	}

	// mergingSlash reports whether a character is a diagonal closing into a
	// lane that is already drawn on the same row. The two touch, but the lane
	// it joins was there first and belongs to another branch — and once they
	// converge git gives both the same colour, so the colour alone cannot tell
	// them apart. Treating the contact as a join hands this branch's last
	// segment the other branch's colour.
	mergingSlash := func(r, c int) bool {
		if r < 0 || r >= len(runesOf) || c < 0 || c >= len(runesOf[r]) || runesOf[r][c] != '/' {
			return false
		}
		_, occupied := colourAt(r, c-1)
		return occupied
	}

	// Two characters are the same lane when they are in touching rows, no more
	// than one column apart, and carry the same colour.
	//
	// Following each glyph's own direction instead would be tighter, and was
	// tried: a lane that shifts columns is drawn as a diagonal on one row and a
	// bar on the next, and demanding the two line up splits a branch in half at
	// every shift — the bug this exists to fix.
	for r := range rows {
		for c := range rows[r].Lanes {
			k, ok := colourAt(r, c)
			if !ok || k == 0 {
				continue // a space, or a commit marker
			}
			ensure(cell{r, c})

			if !mergingSlash(r, c) {
				for _, d := range []int{-1, 0, 1} {
					// The cell below is a diagonal merging into a lane that
					// was already drawn, so the only neighbour sharing its
					// lane is the one above and to its right.
					if d != -1 && mergingSlash(r+1, c+d) {
						continue
					}
					if k2, ok := colourAt(r+1, c+d); ok && k2 == k {
						ensure(cell{r + 1, c + d})
						union(cell{r, c}, cell{r + 1, c + d})
					}
				}
				continue
			}

			// A merging diagonal joins only what is above it: the lane it is
			// closing, which is either directly overhead or one column to the
			// right depending on whether it was already stepping across. The
			// link has to be made from here, because the pass over the row
			// above skips the edge into a merging diagonal.
			for _, d := range []int{0, 1} {
				if k2, ok := colourAt(r-1, c+d); ok && k2 == k {
					ensure(cell{r - 1, c + d})
					union(cell{r, c}, cell{r - 1, c + d})
				}
			}
			// Git also closes a lane two columns in one step when another lane
			// is in the way, drawing "| |/" then "|/|": the line crosses the
			// bar between them, which is never drawn. Without this the two
			// halves are separate runs and get coloured independently.
			if k2, ok := colourAt(r-1, c+1); ok && k2 != k {
				if k3, ok := colourAt(r-1, c+2); ok && k3 == k && runesOf[r-1][c+2] == '/' {
					ensure(cell{r - 1, c + 2})
					union(cell{r, c}, cell{r - 1, c + 2})
				}
			}
		}
	}

	// A run can touch a commit at either end: a lane is created one column over
	// from the merge that spawned it, and closes one column over from the
	// commit it merges into. So the candidates are ranked — a commit in the
	// same column is a lane arriving at or leaving its own commit and always
	// wins, and between two diagonal neighbours the one below wins, that being
	// the branch whose commits the lane leads down to rather than the one it
	// forked from.
	//
	// Weakest of all is a merge's own diagonal leaving it for its second
	// parent, "\" below and right of the merge. That line leads to the branch
	// merged in, so it takes that branch's path from the commit at its other
	// end. It usually ends at that branch's commit in the same column and wins
	// anyway; this matters when it ends by closing into another lane, as when
	// main is merged into a feature branch: it then ties with that commit,
	// both being diagonal, and would otherwise draw main's line in the
	// feature's colour.
	const (
		sameColumn = iota
		below
		above
		leavingMerge
	)
	bestRank := map[cell]int{}
	bestPath := map[cell]int{}
	consider := func(c cell, rank, path int) {
		root := find(c)
		if cur, ok := bestRank[root]; !ok || rank < cur {
			bestRank[root] = rank
			bestPath[root] = path
		}
	}
	commitAt := func(r int) (col, path int, ok bool) {
		if r < 0 || r >= len(rows) || rows[r].CommitIdx < 0 {
			return 0, 0, false
		}
		for c, k := range rows[r].Lanes {
			if k == 0 && c < len(runesOf[r]) && runesOf[r][c] != ' ' {
				return c, paths[rows[r].CommitIdx], true
			}
		}
		return 0, 0, false
	}

	for c := range parent {
		// A closing diagonal descends from its own commits into another
		// branch's lane, so the commit above it is its own and the one below
		// belongs to the lane it is joining — the opposite way round from
		// every other character, where the lane leads down to its commits.
		belowRank, aboveRank := below, above
		if mergingSlash(c.row, c.col) {
			belowRank, aboveRank = above, below
		}
		for _, dir := range []struct{ dr, rank int }{{1, belowRank}, {-1, aboveRank}} {
			col, path, ok := commitAt(c.row + dir.dr)
			if !ok || path < 0 {
				continue
			}
			switch col - c.col {
			case 0:
				consider(c, sameColumn, path)
			case -1, 1:
				rank := dir.rank
				if dir.dr == -1 && col == c.col-1 && runesOf[c.row][c.col] == '\\' {
					rank = leavingMerge
				}
				consider(c, rank, path)
			}
		}
	}

	// A commit marker takes its own commit's path; every other character takes
	// the path of the run it belongs to.
	for r := range rows {
		for c := range rows[r].Lanes {
			if c >= len(runesOf[r]) || runesOf[r][c] == ' ' {
				continue
			}
			if rows[r].Lanes[c] == 0 {
				if rows[r].CommitIdx >= 0 {
					rows[r].Lanes[c] = paths[rows[r].CommitIdx]
				}
				continue
			}
			if p, ok := bestPath[find(cell{r, c})]; ok {
				rows[r].Lanes[c] = p
			}
		}
	}
}
