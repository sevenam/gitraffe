package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestMarksFor(t *testing.T) {
	for _, tc := range []struct {
		name                string
		total, offset, rows int
		want                scrollMarks
	}{
		{"fits", 5, 0, 10, scrollMarks{}},
		{"fits exactly", 10, 0, 10, scrollMarks{}},
		{"more below", 11, 0, 10, scrollMarks{below: true}},
		{"both ways", 30, 5, 10, scrollMarks{above: true, below: true}},
		{"at the end", 30, 20, 10, scrollMarks{above: true}},
		{"no room to draw in", 30, 0, 0, scrollMarks{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := marksFor(tc.total, tc.offset, tc.rows); got != tc.want {
				t.Errorf("marksFor(%d, %d, %d) = %+v, want %+v", tc.total, tc.offset, tc.rows, got, tc.want)
			}
		})
	}
}

// The newline after the last line of a diff leaves an empty line, which must
// not read as more to scroll to.
func TestTrailingBlankLinesAreNotMore(t *testing.T) {
	if got := textLines([]string{"a", "b", "", "  "}); got != 2 {
		t.Errorf("textLines = %d, want 2", got)
	}
	if got := textLines([]string{""}); got != 0 {
		t.Errorf("textLines of nothing = %d, want 0", got)
	}
}

func TestMarkBorderSitsInTheMiddle(t *testing.T) {
	line := "╰" + strings.Repeat("─", 30) + "╯"
	got := markBorder(line, markBelow, lipgloss.NewStyle())
	if w := ansi.StringWidth(got); w != 32 {
		t.Errorf("marked line is %d wide, want the border's 32", w)
	}
	if !strings.HasPrefix(got, "╰") || !strings.HasSuffix(got, "╯") {
		t.Errorf("marked line %q lost its corners", got)
	}
	left, right, _ := strings.Cut(got, markBelow)
	if l, r := ansi.StringWidth(left), ansi.StringWidth(right); l-r > 1 || r-l > 1 {
		t.Errorf("mark is off centre in %q (%d left, %d right)", got, l, r)
	}
}

// The label shares the top border, and a mark must never write over it.
func TestMarkBorderStepsAroundTheLabel(t *testing.T) {
	line := "╭[2]-commit-details" + strings.Repeat("─", 6) + "╮"
	got := markBorder(line, markAbove, lipgloss.NewStyle())
	if !strings.HasPrefix(got, "╭[2]-commit-details") {
		t.Errorf("label overwritten: %q", got)
	}
	if !strings.Contains(got, "⇡") {
		t.Errorf("no mark in %q, though there was room after the label", got)
	}
	if w := ansi.StringWidth(got); w != ansi.StringWidth(line) {
		t.Errorf("marked line is %d wide, want %d", w, ansi.StringWidth(line))
	}
}

func TestMarkBorderLeavesATooNarrowBoxAlone(t *testing.T) {
	for _, line := range []string{"╰──╯", "╭[3]-diff─╮"} {
		if got := markBorder(line, markBelow, lipgloss.NewStyle()); got != line {
			t.Errorf("markBorder(%q) = %q, want it untouched", line, got)
		}
	}
}

// borders are the top and bottom border lines of the commit view's diff box.
func diffBorders(t *testing.T, m model) (top, bottom string) {
	t.Helper()
	for _, line := range strings.Split(diffBoxOf(t, m), "\n") {
		if strings.HasPrefix(line, "╭[3]-diff") {
			top = line
		}
		if strings.HasPrefix(line, "╰") {
			bottom = line
		}
	}
	if top == "" || bottom == "" {
		t.Fatalf("diff box borders not found:\n%s", diffBoxOf(t, m))
	}
	return top, bottom
}

func TestDiffBoxMarksWhereThereIsMore(t *testing.T) {
	m := openedView(t, longDiffRepo(t))
	m.commitView.focus = commitBoxDiff

	for _, tc := range []struct {
		name         string
		keys         []string
		above, below bool
	}{
		{"at the top", nil, false, true},
		{"part way", []string{"j", "j"}, true, true},
		{"at the end", []string{"G"}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := m
			for _, k := range tc.keys {
				at = press(at, keyPress(k))
			}
			top, bottom := diffBorders(t, at)
			if got := strings.Contains(top, "⇡"); got != tc.above {
				t.Errorf("⇡ shown = %v, want %v: %q", got, tc.above, top)
			}
			if got := strings.Contains(bottom, "⇣"); got != tc.below {
				t.Errorf("⇣ shown = %v, want %v: %q", got, tc.below, bottom)
			}
		})
	}
}

func TestShortDiffHasNoMarks(t *testing.T) {
	top, bottom := diffBorders(t, openedView(t, threeFileRepo(t)))
	if strings.ContainsAny(top+bottom, "⇡⇣") {
		t.Errorf("a diff that fits is marked:\n%s\n%s", top, bottom)
	}
}

// The main screen's details panel holds the whole diff under the message, so
// it is the box most often cut short.
func TestDetailsPanelMarksWhereThereIsMore(t *testing.T) {
	m := withDiff(t, loadedModel(t, longDiffRepo(t)))
	m.windowWidth, m.windowHeight = 110, 26
	m.maximised = false

	lines := strings.Split(ansi.Strip(m.View()), "\n")
	top, bottom := lines[3], lines[len(lines)-2]
	if !strings.Contains(bottom, "⇣") || strings.Contains(top, "⇡") {
		t.Errorf("at the top, want only ⇣:\n%s\n%s", top, bottom)
	}

	m.detailsScroll = 5
	lines = strings.Split(ansi.Strip(m.View()), "\n")
	if top := lines[3]; !strings.Contains(top, "⇡") {
		t.Errorf("scrolled down, want ⇡ on top: %q", top)
	}
}
