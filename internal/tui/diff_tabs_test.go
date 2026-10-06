package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// tabbedDiff has an unchanged line, indented with a tab, that fits the box
// only for as long as the tab is counted as no width at all.
func tabbedDiff(width int) string {
	return "@@ -1,3 +1,3 @@\n \t" + strings.Repeat("x", width-1) + "\n-old\n+new\n last"
}

// checkRows fails for a row the box would have to wrap: one wider than the
// box, or holding a tab, whose width is not known until it is drawn.
func checkRows(t *testing.T, rows []string, width int) {
	t.Helper()
	for _, row := range rows {
		if strings.Contains(row, "\t") {
			t.Errorf("a tab is left in the row, to be widened when drawn: %q", ansi.Strip(row))
		}
		if got := ansi.StringWidth(row); got > width {
			t.Errorf("the row is %d wide, the box %d: %q", got, width, ansi.Strip(row))
		}
	}
}

// A line is cut at the edge of the diff box by the width it is drawn at. A
// tab measured as nothing and then drawn as four spaces leaves the line too
// long, and the box wraps it onto a row the diff has no line for.
func TestTabbedDiffLineIsCutNotWrapped(t *testing.T) {
	const width = 40
	c := commit{DiffLoaded: true, DiffFiles: []fileDiff{{Path: "a.go", Body: tabbedDiff(width)}}}
	out, _ := model{}.renderFileDiff(c, width, 20)

	rows := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	checkRows(t, rows, width)
	if want := diffHeaderLines + lineCount(tabbedDiff(width)); len(rows) != want {
		t.Errorf("got %d rows, want %d: one for each line of the diff", len(rows), want)
	}
	// Once in a box, each line is still one row: the row under the long line
	// is the next line of the diff, not the long line's tail.
	box := strings.Split(ansi.Strip(commitBox(out, scrollMarks{}, width+4, len(rows)+2, "diff", false)), "\n")
	if got := box[1+diffHeaderLines+2]; !strings.Contains(got, "-old") {
		t.Errorf("the row under the long line is %q, want the removed line", got)
	}
}

// The details panel cuts its diff the same way.
func TestTabbedDiffLineIsCutInTheDetailsPanel(t *testing.T) {
	m := testModel()
	m.detailsContentWidth = 40
	m.commits = []commit{{Hash: "abc", DiffLoaded: true, DiffBody: "diff --git a/a.go b/a.go\n" + tabbedDiff(40)}}
	content, _ := m.renderCommitDetails()
	checkRows(t, strings.Split(content, "\n"), 40)
}
