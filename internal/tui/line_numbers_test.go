package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/config"
)

// hunkAtLine98 crosses from two digits to three, so the column has to be as
// wide as its largest number on every line.
const hunkAtLine98 = "@@ -98,3 +98,3 @@ func f() {\n keep\n-old\n+new\n last"

// A line carries its number in the file as it is now; a removed line, which
// is not in it, the number it had.
func TestDiffLinesAreNumbered(t *testing.T) {
	c := commit{DiffLoaded: true, DiffFiles: []fileDiff{{Path: "a.go", Body: hunkAtLine98}}}
	out, _ := model{}.renderFileDiff(c, 80, 20)

	want := []string{
		"    @@ -98,3 +98,3 @@ func f() {",
		" 98  keep",
		" 99 -old",
		" 99 +new",
		"100  last",
	}
	got := strings.Split(ansi.Strip(out), "\n")[diffHeaderLines:]
	for i, w := range want {
		if i >= len(got) || got[i] != w {
			t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

// The details panel numbers its diff the same way, so a line has one number
// whichever screen it is read on.
func TestDetailsPanelNumbersItsDiff(t *testing.T) {
	c := commit{Hash: "abc", DiffLoaded: true, DiffBody: "diff --git a/a.go b/a.go\n" + hunkAtLine98}
	m := testModel()
	m.commits = []commit{c}
	out, _ := m.renderCommitDetails()
	for _, want := range []string{"\n 99 -old\n", "\n 99 +new\n", "\n100  last\n", "\n    diff --git a/a.go b/a.go\n"} {
		if !strings.Contains(ansi.Strip(out), want) {
			t.Errorf("missing %q in:\n%s", want, ansi.Strip(out))
		}
	}
}

// A diff with nothing to number is drawn as it always was, not pushed in
// behind an empty column.
func TestNoGutterWithoutNumbers(t *testing.T) {
	c := commit{DiffLoaded: true, DiffFiles: []fileDiff{{
		Path: "logo.png", Binary: true, Body: "Binary files a/logo.png and b/logo.png differ",
	}}}
	out, _ := model{}.renderFileDiff(c, 80, 20)
	if got := strings.Split(ansi.Strip(out), "\n")[diffHeaderLines]; !strings.HasPrefix(got, "Binary files") {
		t.Errorf("the line is indented: %q", got)
	}
}

// The band under the staging cursor still spans the box exactly: the numbers
// are part of it, not extra width in front of it.
func TestPickedLineBandIncludesTheGutter(t *testing.T) {
	c := commit{WorkingTree: true, DiffLoaded: true, DiffFiles: []fileDiff{{Path: "a.go", Body: hunkAtLine98}}}
	var m model
	m.commitView.workingTree = true
	m.commitView.focus = commitBoxDiff
	m.commitView.diffCursor = 2

	const width = 60
	out, _ := m.renderFileDiff(c, width, 20)
	line := strings.Split(out, "\n")[diffHeaderLines+2]
	if got := ansi.StringWidth(line); got != width {
		t.Errorf("the band is %d wide, the box %d", got, width)
	}
	if got := ansi.Strip(line); !strings.HasPrefix(got, unstagedMarker+"  99 -old") {
		t.Errorf("the picked line lost its number: %q", got)
	}
}

// "#" gives the commit view a column for each side: the number a line had,
// then the number it has, each blank where the line has no such number.
func TestHashShowsOldAndNewLineNumbers(t *testing.T) {
	c := commit{DiffLoaded: true, DiffFiles: []fileDiff{{Path: "a.go", Body: hunkAtLine98}}}
	m := model{twoLineNumbers: true}
	out, _ := m.renderFileDiff(c, 80, 20)

	want := []string{
		"        @@ -98,3 +98,3 @@ func f() {",
		" 98  98  keep",
		" 99     -old",
		"     99 +new",
		"100 100  last",
	}
	got := strings.Split(ansi.Strip(out), "\n")[diffHeaderLines:]
	for i, w := range want {
		if i >= len(got) || got[i] != w {
			t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

// The key turns the second column on and off from the commit view, says
// which column is which, and leaves the details panel's one column alone.
func TestHashTogglesTheSecondColumn(t *testing.T) {
	m := openedView(t, threeFileRepo(t))
	if m.twoLineNumbers {
		t.Fatal("a first run numbers a diff in two columns")
	}

	m = press(m, keyPress("#"))
	if !m.twoLineNumbers || !strings.Contains(m.notice, "old, then new") {
		t.Errorf("after #: two=%v notice=%q; want two columns and a word on which is which", m.twoLineNumbers, m.notice)
	}
	if !m.commitView.open {
		t.Error("# closed the commit view")
	}
	m = press(m, keyPress("#"))
	if m.twoLineNumbers || !strings.Contains(m.notice, "one column") {
		t.Errorf("after # again: two=%v notice=%q; want one column", m.twoLineNumbers, m.notice)
	}

	// The details panel beside the graph has too little room for a second
	// column, and keeps one whatever the commit view shows.
	c := commit{Hash: "abc", DiffLoaded: true, DiffBody: "diff --git a/a.go b/a.go\n" + hunkAtLine98}
	d := testModel()
	d.twoLineNumbers = true
	d.commits = []commit{c}
	out, _ := d.renderCommitDetails()
	if !strings.Contains(ansi.Strip(out), "\n 99 -old\n") {
		t.Errorf("the details panel does not number its diff in one column:\n%s", ansi.Strip(out))
	}
}

// One column until two are asked for, and the choice is kept.
func TestLineNumberColumnsAreRemembered(t *testing.T) {
	m := initialModel(".")
	m.configDir = t.TempDir()
	if got := applyPreferences(m); got.twoLineNumbers {
		t.Error("an empty settings file gave two columns")
	}
	m.twoLineNumbers = true
	if err := savePreferences(m); err != nil {
		t.Fatal(err)
	}
	if s := config.Load(m.configDir); !s.TwoLineNumbers {
		t.Error("settings do not hold the two columns")
	}
	fresh := initialModel(".")
	fresh.configDir = m.configDir
	if got := applyPreferences(fresh); !got.twoLineNumbers {
		t.Error("the next run forgot the two columns")
	}

	// Another repository in the same session keeps what was chosen.
	if next, _ := m.switchRepo("."); !next.twoLineNumbers {
		t.Error("switching repository went back to one column")
	}
}
