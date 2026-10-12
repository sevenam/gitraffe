package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// diffGutter is the column of line numbers drawn in front of a diff. It is
// added as the diff is drawn and is no part of it: lines are still picked,
// staged and copied by their place in the diff git wrote.
//
// By default there is one column, not an old and a new side by side: it holds
// a line's number in the file as it is now, or for a removed line the number
// it had, since that is the only one it has. A second column says something
// more only on the lines that did not change, and costs every line its width.
//
// That is a cost some will pay to read a diff the way other tools show it, so
// the commit view can have both ("#", see toggleLineNumberColumns): the old
// number, then the new, each blank on the lines that have no such number.
type diffGutter struct {
	nums []git.LineNumber
	// digits is the width of a number: that of the largest in this diff, so
	// the column is as narrow as the diff allows and the same on every line.
	digits int
	// both is the old and the new number side by side.
	both bool
}

// newDiffGutter numbers the lines of a diff, in one column or, with both, in
// two. A diff with nothing to number, such as a binary file's or a merge's,
// gets a gutter of no width rather than an empty column.
func newDiffGutter(nums []git.LineNumber, both bool) diffGutter {
	most := 0
	for _, n := range nums {
		most = max(most, n.Old, n.New)
	}
	g := diffGutter{nums: nums, both: both}
	if most > 0 {
		g.digits = len(fmt.Sprint(most))
	}
	return g
}

// width is the room the gutter takes on every line, the gap after it included.
func (g diffGutter) width() int {
	if g.digits == 0 {
		return 0
	}
	if g.both {
		return 2 * (g.digits + 1)
	}
	return g.digits + 1
}

// text is the gutter for line i of the diff, blank where the line has no
// number, as a header has none.
func (g diffGutter) text(i int) string {
	if g.digits == 0 {
		return ""
	}
	var n git.LineNumber
	if i < len(g.nums) {
		n = g.nums[i]
	}
	if g.both {
		return g.number(n.Old) + g.number(n.New)
	}
	if n.New == 0 {
		return g.number(n.Old)
	}
	return g.number(n.New)
}

// number is one number in its column with the gap after it, or that much
// blank for a line that has none.
func (g diffGutter) number(n int) string {
	if n == 0 {
		return strings.Repeat(" ", g.digits+1)
	}
	return fmt.Sprintf("%*d ", g.digits, n)
}

// render is the gutter for line i in the quiet colour of the hints, so the
// numbers can be read without competing with the diff beside them.
func (g diffGutter) render(i int) string {
	if g.digits == 0 {
		return ""
	}
	return helpStyle.Render(g.text(i))
}

// renderPicked is the gutter for a line under the cursor or in the selection.
// It carries the band's background itself: every styled piece ends in a
// reset, so the band would otherwise start after the numbers.
func (g diffGutter) renderPicked(i int) string {
	if g.digits == 0 {
		return ""
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(theme.Current.SelectedBg)).
		Foreground(lipgloss.Color(theme.Current.SelectedFg)).
		Render(g.text(i))
}

// toggleLineNumberColumns is "#" in the commit view: one column of line
// numbers or two, with a word on the status line saying which is which,
// since two columns of numbers do not explain themselves.
func (m model) toggleLineNumberColumns() model {
	m.twoLineNumbers = !m.twoLineNumbers
	m.notice = "Line numbers: one column — # shows old and new"
	if m.twoLineNumbers {
		m.notice = "Line numbers: old, then new — # shows one column"
	}
	return m
}
