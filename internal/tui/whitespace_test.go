package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/config"
	"github.com/sevenam/gitraffe/internal/gittest"
)

func TestStyleDiffLineSpellsOutWhitespace(t *testing.T) {
	for _, tc := range []struct{ line, want string }{
		// The column git writes is not part of the file, and stays a blank.
		{" a b", " a·b"},
		{"+\tindented", "+→   indented"},
		{"-two  spaces ", "-two··spaces·"},
		{"+ \t mixed", "+·→   ·mixed"},
		{" ", " "},
		{"+", "+"},
		// Not lines of a file: left as they are.
		{"@@ -1,2 +1,3 @@ func main() {", "@@ -1,2 +1,3 @@ func main() {"},
		{"diff --git a/x y b/x y", "diff --git a/x y b/x y"},
		{"--- a/some file", "--- a/some file"},
		{"+++ b/some file", "+++ b/some file"},
		{"\\ No newline at end of file", "\\ No newline at end of file"},
		{"... (truncated)", "... (truncated)"},
	} {
		if got := ansi.Strip(styleDiffLine(tc.line, true)); got != tc.want {
			t.Errorf("with marks, %q is drawn %q, want %q", tc.line, got, tc.want)
		}
		// A mark takes the columns of what it stands for, so nothing that
		// counts widths has to know whether they are on.
		on, off := ansi.StringWidth(styleDiffLine(tc.line, true)), ansi.StringWidth(styleDiffLine(tc.line, false))
		if on != off {
			t.Errorf("%q is %d wide with marks and %d without", tc.line, on, off)
		}
		if got := ansi.Strip(styleDiffLine(tc.line, false)); strings.ContainsAny(got, spaceMark+"→") {
			t.Errorf("without marks, %q is drawn %q", tc.line, got)
		}
	}
}

// The band under the cursor carries the marks too, and is still exactly as
// wide as the box.
func TestPickedDiffLineSpellsOutWhitespace(t *testing.T) {
	const width = 30
	got := pickedDiffLine("+\ta b ", width, true)
	if text := ansi.Strip(got); !strings.HasPrefix(text, "+→   a·b·") || ansi.StringWidth(got) != width {
		t.Errorf("the band reads %q, %d wide; want the marks and %d", text, ansi.StringWidth(got), width)
	}
	if text := ansi.Strip(pickedDiffLine("+\ta b ", width, false)); strings.Contains(text, spaceMark) {
		t.Errorf("without marks the band reads %q", text)
	}
	// A header under the cursor is no line of a file.
	if text := ansi.Strip(pickedDiffLine("@@ -1 +1 @@ a b", width, true)); !strings.HasPrefix(text, "@@ -1 +1 @@ a b") {
		t.Errorf("a header in the band reads %q", text)
	}
}

// whitespaceRepo has one commit that turns a tab into spaces and leaves a
// space at the end of a line.
func whitespaceRepo(t *testing.T) string {
	t.Helper()
	dir, git, _ := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "code.go"), "func f() {\n\treturn one\n}\n")
	git("add", "-A")
	git("commit", "-qm", "first")
	writeFile(t, filepath.Join(dir, "code.go"), "func f() {\n    return one \n}\n")
	git("add", "-A")
	git("commit", "-qm", "spaces for the tab")
	return dir
}

// w turns the marks on and off in both places a diff is drawn, says which,
// and does not change the size of anything.
func TestWTogglesWhitespaceInBothDiffViews(t *testing.T) {
	dir := whitespaceRepo(t)
	m := withDiff(t, selectMessage(t, loadedModel(t, dir), "spaces for the tab"))
	m.maximised = false
	const tabbed, spaced = "-→   return·one", "+····return·one·"

	plain := stripANSI(m.View())
	if strings.Contains(plain, "return·one") || !strings.Contains(plain, "+    return one") {
		t.Fatalf("with the marks off the details panel reads:\n%s", plain)
	}

	m = press(m, keyPress("w"))
	if !m.showWhitespace || !strings.Contains(m.notice, "Whitespace shown") {
		t.Fatalf("show=%v notice=%q after w", m.showWhitespace, m.notice)
	}
	marked := stripANSI(m.View())
	for _, want := range []string{tabbed, spaced, "func·f()·{"} {
		if !strings.Contains(marked, want) {
			t.Errorf("the details panel does not show %q:\n%s", want, marked)
		}
	}
	if a, b := strings.Count(plain, "\n"), strings.Count(marked, "\n"); a != b {
		t.Errorf("the screen is %d lines with marks and %d without", b+1, a+1)
	}

	// The commit view draws the same commit the same way, and has the key.
	m, _ = m.openCommitView()
	view := stripANSI(m.View())
	for _, want := range []string{tabbed, spaced} {
		if !strings.Contains(view, want) {
			t.Errorf("the commit view does not show %q:\n%s", want, view)
		}
	}
	m = press(m, keyPress("w"))
	if m.showWhitespace || !m.commitView.open || !strings.Contains(m.notice, "Whitespace hidden") {
		t.Fatalf("show=%v open=%v notice=%q after w in the commit view", m.showWhitespace, m.commitView.open, m.notice)
	}
	if view := stripANSI(m.View()); strings.Contains(view, "return·one") || !strings.Contains(view, "+    return one") {
		t.Errorf("with the marks off the commit view reads:\n%s", view)
	}
}

// The marks are on until they are turned off, and the choice is kept.
func TestWhitespaceChoiceIsRemembered(t *testing.T) {
	if !initialModel(".").showWhitespace {
		t.Error("a first run does not show whitespace")
	}

	m := initialModel(".")
	m.configDir = t.TempDir()
	if got := applyPreferences(m); !got.showWhitespace {
		t.Error("an empty settings file turned the marks off")
	}
	m.showWhitespace = false
	if err := savePreferences(m); err != nil {
		t.Fatal(err)
	}
	if s := config.Load(m.configDir); s.Whitespace == nil || *s.Whitespace {
		t.Errorf("settings hold %v, want the marks saved as off", s.Whitespace)
	}
	fresh := initialModel(".")
	fresh.configDir = m.configDir
	if got := applyPreferences(fresh); got.showWhitespace {
		t.Error("the next run forgot the marks were off")
	}

	// Another repository in the same session keeps what was chosen.
	next, _ := m.switchRepo(".")
	if next.showWhitespace {
		t.Error("switching repository turned the marks back on")
	}
}
