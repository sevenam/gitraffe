package git

import "slices"

// Gitraffe lays the graph out itself rather than showing the one "git log
// --graph" draws. Git draws a connection on rows of its own, between the
// commits, and closes a lane the moment its branch has no more commits: a
// branch then seems to end beside the commit that merged it and to start out
// of the side of some line, rows above the commit it was actually branched
// from. Here every connection is drawn on the row of the commit it belongs to:
//
//	◆─╮      a merge: the branch on the right was merged into this commit
//	│ ●      a commit on that branch
//	│ │ ●    a commit on another
//	●─┴─╯    both branches start at this commit
//
// A lane therefore stays open, in its own column, from the commit that needs a
// parent down to the row of that parent — however many other commits are
// listed in between — and that is where it visibly joins.

// lane is one column of the graph while it is in use: a line running down
// towards a commit that has not been listed yet.
type lane struct {
	want   string // short hash of the commit the line leads to; "" when the column is free
	colour int    // the branch path the line is drawn in; see commitPaths
}

// connection is how a column other than the commit's own takes part in the
// commit's row.
type connection struct {
	kind   connKind
	colour int
}

type connKind int

const (
	connEnd  connKind = iota // a lane arriving at this commit: a branch that started here
	connOpen                 // a lane opened for a merge's further parent
	connJoin                 // a lane running past that the commit's stroke meets
)

// layoutGraph draws one row per commit, in the order given, which has to be
// one that lists every commit before its parents (git's --topo-order).
//
// Each character's lane number is the path of the branch it belongs to, so a
// branch is one colour for its whole length; see commitPaths for what a path
// is. A line takes the path of the commit at its upper end, except the line a
// merge sends to a branch it merged in, which takes that branch's — it is the
// branch's line, arriving.
func layoutGraph(commits []Commit) (rows []DisplayRow, maxWidth int) {
	paths := commitPaths(commits)
	byHash := make(map[string]int, len(commits))
	for i, c := range commits {
		byHash[c.Hash] = i
	}

	var lanes []lane
	grow := func() int {
		lanes = append(lanes, lane{})
		return len(lanes) - 1
	}

	rows = make([]DisplayRow, 0, len(commits))
	for i, c := range commits {
		path := paths[i]
		before := slices.Clone(lanes)

		// The lanes leading here. One of them is this commit's own line —
		// the one already drawn in its colour, when there is one — and the
		// commit sits in its column. The rest are branches that started at
		// this commit, and end on this row.
		var arriving []int
		for x, l := range lanes {
			if l.want == c.Hash {
				arriving = append(arriving, x)
			}
		}
		col := -1
		for _, x := range arriving {
			if lanes[x].colour == path {
				col = x
				break
			}
		}
		switch {
		case col >= 0:
		case len(arriving) > 0:
			col = arriving[0]
		default:
			// A tip: nothing listed so far descends from it.
			col = slices.IndexFunc(lanes, func(l lane) bool { return l.want == "" })
			if col < 0 {
				col = grow()
			}
		}

		conns := map[int]connection{}
		// ending columns are free once this row is drawn, and until then can
		// be taken over, once each, by a lane this commit opens.
		taken := map[int]bool{}
		ending := func(x int) bool { return x != col && slices.Contains(arriving, x) && !taken[x] }
		for _, x := range arriving {
			if x != col {
				conns[x] = connection{connEnd, lanes[x].colour}
			}
		}
		// place finds the column for a lane this commit opens: to the right
		// first, because that is the side a merged branch is usually drawn on,
		// and as near the commit as there is room.
		//
		// A column where another lane ends on this very row counts as room.
		// The two then meet the commit in one stroke, and successive branches
		// stack in one column instead of stepping sideways each time. When
		// several end here the outermost is taken, which leaves the ones
		// nearer the commit drawn as plain ends.
		place := func() int {
			side := func(from, to, step int) int {
				nearestFree, nearestEnd, outerEnd := -1, -1, -1
				for x := from; x != to; x += step {
					switch {
					case ending(x):
						if nearestEnd < 0 {
							nearestEnd = x
						}
						outerEnd = x
					case lanes[x].want == "" && nearestFree < 0:
						nearestFree = x
					}
				}
				before := func(a, b int) bool { return (a-b)*step < 0 }
				if nearestFree >= 0 && (nearestEnd < 0 || before(nearestFree, nearestEnd)) {
					return nearestFree
				}
				return outerEnd
			}
			if x := side(col+1, len(lanes), 1); x >= 0 {
				return x
			}
			if x := side(col-1, -1, -1); x >= 0 {
				return x
			}
			return grow()
		}

		// The first parent carries on down this commit's column. If another
		// lane already leads to the same parent the two are left side by
		// side: joining them here is what would hide where each began.
		if len(c.Parents) == 0 {
			lanes[col] = lane{}
		} else {
			lanes[col] = lane{want: c.Parents[0], colour: path}
		}
		for n, p := range c.Parents {
			if n == 0 || slices.Contains(c.Parents[:n], p) {
				continue
			}
			existing := -1
			for x, l := range lanes {
				if l.want == p && x != col {
					existing = x
					break
				}
			}
			if existing >= 0 {
				conns[existing] = connection{connJoin, lanes[existing].colour}
				continue
			}
			colour := path
			if j, ok := byHash[p]; ok {
				colour = paths[j]
			}
			x := place()
			kind := connOpen
			if ending(x) {
				kind = connJoin // a lane above and a lane below, both meeting the commit
				taken[x] = true
			}
			lanes[x] = lane{want: p, colour: colour}
			conns[x] = connection{kind, colour}
		}
		for _, x := range arriving {
			if x != col && !taken[x] {
				lanes[x] = lane{}
			}
		}

		marker := CommitMarker
		if len(c.Parents) > 1 {
			marker = MergeMarker
		}
		chars, colours := drawRow(col, marker, path, before, lanes, conns)
		rows = append(rows, DisplayRow{
			GraphChars: string(chars),
			Lanes:      colours,
			CommitIdx:  i,
			GraphWidth: len(chars),
		})
		maxWidth = max(maxWidth, len(chars))

		// Columns freed at the right edge are given back, so the graph is
		// only ever as wide as the lanes in use.
		for len(lanes) > 0 && lanes[len(lanes)-1].want == "" {
			lanes = lanes[:len(lanes)-1]
		}
	}
	return rows, maxWidth
}

// drawRow draws one commit's row: a character per column with one between each
// pair, which is either part of a connection or a space.
func drawRow(col int, marker rune, path int, before, after []lane, conns map[int]connection) ([]rune, []int) {
	lo, hi := col, col
	for x := range conns {
		lo, hi = min(lo, x), max(hi, x)
	}
	active := func(ls []lane, x int) bool { return x < len(ls) && ls[x].want != "" }

	// outward is the connection a horizontal stroke at or beyond column x
	// belongs to: the nearest one on the way out from the commit. Strokes
	// nearer the commit than that are shared with the connections beyond it,
	// and are drawn in the colour of the nearest.
	outward := func(x, step int) connection {
		for ; x >= lo && x <= hi; x += step {
			if c, ok := conns[x]; ok {
				return c
			}
		}
		return connection{}
	}

	columns := max(len(before), len(after), hi+1)
	var chars []rune
	var colours []int
	put := func(r rune, colour int) {
		chars = append(chars, r)
		colours = append(colours, colour)
	}

	for x := 0; x < columns; x++ {
		conn, connected := conns[x]
		through := x > lo && x < hi // a stroke passes this column on its way further out
		right := x > col

		switch {
		case x == col:
			put(marker, path)
		case connected:
			put(connectionGlyph(conn.kind, right, through), conn.colour)
		case active(before, x):
			// A lane that has nothing to do with this commit. Where a
			// connection has to cross it the lane is drawn unbroken and the
			// stroke resumes on its far side: a cross would read as the
			// two being joined.
			put('│', before[x].colour)
		case through:
			step := 1
			if !right {
				step = -1
			}
			put('─', outward(x, step).colour)
		default:
			put(' ', 0)
		}

		if x == columns-1 {
			break
		}
		switch {
		case x >= col && x < hi:
			put('─', outward(x+1, 1).colour)
		case x < col && x >= lo:
			put('─', outward(x, -1).colour)
		default:
			put(' ', 0)
		}
	}

	for len(chars) > 0 && chars[len(chars)-1] == ' ' {
		chars, colours = chars[:len(chars)-1], colours[:len(colours)-1]
	}
	return chars, colours
}

// connectionGlyph picks the box-drawing character for a column joined to the
// commit on its row. right says which side of the commit the column is on, and
// through whether the stroke carries on past it to a column further out.
func connectionGlyph(kind connKind, right, through bool) rune {
	switch kind {
	case connEnd: // the lane comes down from above and turns in to the commit
		switch {
		case through:
			return '┴'
		case right:
			return '╯'
		}
		return '╰'
	case connOpen: // the lane leaves the commit and turns down
		switch {
		case through:
			return '┬'
		case right:
			return '╮'
		}
		return '╭'
	}
	// connJoin: the lane runs past, and the commit's stroke meets it
	switch {
	case through:
		return '┼'
	case right:
		return '┤'
	}
	return '├'
}

// commitPaths gives every commit the id of the branch path it sits on. A
// commit's first parent keeps its path — that is what "the same branch" means
// once the branch name is gone — and every further parent of a merge starts a
// new one, because that is a branch that was merged in. Walking in display
// order lets the trunk claim its own chain before anything else can.
func commitPaths(commits []Commit) []int {
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
