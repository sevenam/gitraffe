package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/gittest"
)

var tab = tea.KeyMsg{Type: tea.KeyTab}

func numberedLines(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	return sb.String()
}

// stagingRepo has a commit holding numbered.txt, thirty lines long so that
// changes at its two ends are two hunks, and notes.txt beside it.
func stagingRepo(t *testing.T) (dir string, gitCmd func(...string)) {
	t.Helper()
	dir, gitCmd, _ = gittest.Fixture(t)
	gitCmd("init", "-q", "-b", "main")
	gittest.Identify(gitCmd)
	// The index is compared byte for byte; nothing may rewrite line ends.
	gitCmd("config", "core.autocrlf", "false")
	write(t, dir, "numbered.txt", numberedLines(30))
	write(t, dir, "notes.txt", "a note\n")
	gitCmd("add", "-A")
	gitCmd("commit", "-qm", "first")
	return dir, gitCmd
}

// twoHunks edits numbered.txt near its top and near its end.
func twoHunks(t *testing.T, dir string) string {
	t.Helper()
	body := strings.Replace(strings.Replace(numberedLines(30), "line 2\n", "line two\n", 1), "line 29\n", "line twenty-nine\n", 1)
	write(t, dir, "numbered.txt", body)
	return body
}

// do presses a key and feeds back what the command it started reports, which
// is what the program does between one keystroke and the next.
func do(t *testing.T, m model, key tea.KeyMsg) model {
	t.Helper()
	next, cmd := m.Update(key)
	m = next.(model)
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	next, _ = m.Update(msg)
	return next.(model)
}

// indexed is a file as the index holds it: what a commit made now would.
func indexed(t *testing.T, dir, path string) string {
	t.Helper()
	out, err := git.Run(dir, "show", ":"+path)
	if err != nil {
		t.Fatalf("%s is not in the index: %v", path, err)
	}
	return out + "\n"
}

// listed is the file list as "path:staged" and the like.
func listed(m model) string {
	var out []string
	for _, f := range m.viewedFiles() {
		kind := "unstaged"
		switch {
		case f.Untracked:
			kind = "untracked"
		case f.Staged:
			kind = "staged"
		}
		out = append(out, f.Path+":"+kind)
	}
	return strings.Join(out, " ")
}

func selectedEntry(t *testing.T, m model) string {
	t.Helper()
	f, ok := m.selectedFile()
	if !ok {
		t.Fatal("no file is selected")
	}
	if f.Staged {
		return f.Path + ":staged"
	}
	return f.Path + ":unstaged"
}

// cursorOn moves the diff's cursor to the first line of the box that reads
// text.
func cursorOn(t *testing.T, m model, text string) model {
	t.Helper()
	d := m.stagingDiff()
	for i, line := range d.lines {
		if line.text == text {
			m.commitView.diffCursor = i
			return m
		}
	}
	t.Fatalf("no line %q in the diff box:\n%s", text, shown(d))
	return m
}

// onHunk moves the diff's cursor to the header of the hunk holding the first
// line of the box that reads text.
func onHunk(t *testing.T, m model, text string) model {
	t.Helper()
	m = cursorOn(t, m, text)
	d := m.stagingDiff()
	for i := m.commitView.diffCursor; i >= 0; i-- {
		if strings.HasPrefix(d.lines[i].text, "@@") {
			m.commitView.diffCursor = i
			return m
		}
	}
	t.Fatalf("no hunk header above %q:\n%s", text, shown(d))
	return m
}

// shown is the diff box's lines with their marks, as "● +text".
func shown(d stagingDiff) string {
	var out []string
	for _, l := range d.lines {
		mark := " "
		if l.marked() {
			mark = unstagedMarker
			if l.staged {
				mark = stagedMarker
			}
		}
		out = append(out, mark+" "+l.text)
	}
	return strings.Join(out, "\n")
}

// underCursor is the line of the diff box the cursor is on, with its mark.
func underCursor(t *testing.T, m model) string {
	t.Helper()
	d := m.stagingDiff()
	l, ok := m.cursorLine(d)
	if !ok {
		t.Fatal("the diff box has no lines")
	}
	return shown(stagingDiff{lines: []stagingLine{l}})
}

func TestStagingAFileFromTheList(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	write(t, dir, "notes.txt", "a note, changed\n")
	write(t, dir, "new.txt", "fresh\n")

	m := openedView(t, dir)
	// In path order, whatever side of the index each is on.
	const before = "new.txt:untracked notes.txt:unstaged numbered.txt:unstaged"
	if got := listed(m); got != before {
		t.Fatalf("files = %q", got)
	}
	m = press(m, keyPress("j"))
	screen := ansi.Strip(m.View())
	if !strings.Contains(screen, "> "+unstagedMarker+" notes.txt") || !strings.Contains(screen, "0-of-3-staged") {
		t.Errorf("the list does not mark what is staged:\n%s", screen)
	}
	if !strings.Contains(screen, "s: stage file") || !strings.Contains(screen, "c: commit") {
		t.Errorf("the status line does not say how to stage and commit:\n%s", screen)
	}

	m = do(t, m, keyPress("s"))
	// The file is where it was, and so is the selection.
	if got := listed(m); got != "new.txt:untracked notes.txt:staged numbered.txt:unstaged" {
		t.Fatalf("after s, files = %q, want notes.txt staged in place", got)
	}
	if got := selectedEntry(t, m); got != "notes.txt:staged" {
		t.Errorf("selected %q after staging, want the file just staged", got)
	}
	screen = ansi.Strip(m.View())
	if !strings.Contains(screen, stagedMarker+" notes.txt") || !strings.Contains(screen, "1-of-3-staged") {
		t.Errorf("the list does not show notes.txt staged:\n%s", screen)
	}
	if out, _ := git.Run(dir, "status", "--porcelain"); !strings.Contains(out, "M  notes.txt") {
		t.Errorf("status = %q, want notes.txt staged in the repository", out)
	}

	// s again, on what is now a staged entry, takes it back out.
	if !strings.Contains(ansi.Strip(m.View()), "s: unstage file") {
		t.Error("the status line does not say s would unstage")
	}
	m = do(t, m, keyPress("s"))
	if got := listed(m); got != before {
		t.Fatalf("after unstaging, files = %q", got)
	}
	if got := selectedEntry(t, m); got != "notes.txt:unstaged" {
		t.Errorf("selected %q, want the file where it was", got)
	}

	// An untracked file is staged and taken back in place too, though git
	// calls it something else on each side.
	m = press(m, keyPress("g"))
	m = do(t, m, keyPress("s"))
	if got, sel := listed(m), selectedEntry(t, m); got != "new.txt:staged notes.txt:unstaged numbered.txt:unstaged" || sel != "new.txt:staged" {
		t.Errorf("after staging the untracked file: files = %q, selected %q", got, sel)
	}
	m = do(t, m, keyPress("s"))
	if got, sel := listed(m), selectedEntry(t, m); got != before || sel != "new.txt:unstaged" {
		t.Errorf("after taking it back: files = %q, selected %q", got, sel)
	}
}

func TestStagingEverything(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	write(t, dir, "new.txt", "fresh\n")
	m := openedView(t, dir)

	m = do(t, m, keyPress("S"))
	if got := listed(m); got != "new.txt:staged numbered.txt:staged" {
		t.Fatalf("after S, files = %q, want everything staged", got)
	}
	// With nothing left to stage, the same key takes it all back.
	m = do(t, m, keyPress("S"))
	if got := listed(m); got != "new.txt:untracked numbered.txt:unstaged" {
		t.Fatalf("after S again, files = %q, want nothing staged", got)
	}
	// The row's own summary follows: a new file is "untracked" again.
	if c, _ := m.viewedCommit(); c.Message != "1 changed, 1 untracked" {
		t.Errorf("summary = %q, want it counted again", c.Message)
	}
}

func TestStagingAHunkFromTheDiff(t *testing.T) {
	dir, _ := stagingRepo(t)
	changed := twoHunks(t, dir)
	m := openedView(t, dir)
	m = press(m, keyPress("3"))
	if !strings.Contains(ansi.Strip(m.View()), "s: stage hunk") {
		t.Error("the status line does not say s stages a hunk here")
	}

	firstOnly := strings.Replace(numberedLines(30), "line 2\n", "line two\n", 1)
	rows := len(m.stagingDiff().lines)

	// The cursor starts on the first hunk's header, which stands for the hunk.
	if l, _ := m.cursorLine(m.stagingDiff()); !strings.HasPrefix(l.text, "@@") {
		t.Fatalf("the cursor starts on %q, want the first hunk's header", l.text)
	}
	m = do(t, m, keyPress("s"))
	if got := indexed(t, dir, "numbered.txt"); got != firstOnly {
		t.Fatalf("the index holds:\n%s\nwant only the first hunk", got)
	}
	if got := listed(m); got != "numbered.txt:staged numbered.txt:unstaged" {
		t.Fatalf("files = %q, want the file listed on both sides", got)
	}
	// The hunk has not gone anywhere: it is under the cursor still, with
	// the staged mark, above the one that is left.
	if got := underCursor(t, m); !strings.HasPrefix(got, stagedMarker+" @@") {
		t.Errorf("under the cursor: %q, want the header of the hunk just staged, marked so", got)
	}
	d := m.stagingDiff()
	if len(d.lines) != rows || !strings.Contains(shown(d), stagedMarker+" +line two") ||
		!strings.Contains(shown(d), unstagedMarker+" +line twenty-nine") {
		t.Errorf("the diff box does not show both hunks, each with its mark:\n%s", shown(d))
	}
	body := diffBoxOf(t, m)
	for _, want := range []string{"partly staged", stagedMarker + "  2 +line two", unstagedMarker + " 29 +line twenty-nine"} {
		if !strings.Contains(body, want) {
			t.Errorf("the diff box does not draw %q:\n%s", want, body)
		}
	}
	if !strings.Contains(ansi.Strip(m.View()), "s: unstage hunk") {
		t.Error("the status line does not say s would take the hunk back")
	}

	// s again on the same hunk is its undo.
	m = do(t, m, keyPress("s"))
	if got := indexed(t, dir, "numbered.txt"); got != numberedLines(30) {
		t.Fatalf("the index holds:\n%s\nwant the hunk taken back", got)
	}
	if got := underCursor(t, m); !strings.HasPrefix(got, unstagedMarker+" @@") || listed(m) != "numbered.txt:unstaged" {
		t.Errorf("under the cursor: %q of %q, want the hunk back where it was", got, listed(m))
	}

	// Both hunks, one after the other.
	m = do(t, m, keyPress("s"))
	m = onHunk(t, m, "+line twenty-nine")
	m = do(t, m, keyPress("s"))
	if got := indexed(t, dir, "numbered.txt"); got != changed {
		t.Fatal("after both hunks the index is not the file")
	}
	// Nothing is left on the unstaged side, so the selection is on the file's
	// staged row; the box and the cursor in it are as they were.
	if got := selectedEntry(t, m); got != "numbered.txt:staged" || listed(m) != "numbered.txt:staged" {
		t.Errorf("selected %q of %q, want the staged file", got, listed(m))
	}
	if got := underCursor(t, m); !strings.HasPrefix(got, stagedMarker+" @@") {
		t.Errorf("under the cursor: %q, want the header of the hunk just staged", got)
	}
	if strings.Contains(shown(m.stagingDiff()), unstagedMarker) {
		t.Errorf("something is still marked as not staged:\n%s", shown(m.stagingDiff()))
	}

	// And back: s on a staged hunk unstages it, and only it.
	m = do(t, m, keyPress("s"))
	if got := indexed(t, dir, "numbered.txt"); got != firstOnly {
		t.Fatalf("the index holds:\n%s\nwant the second hunk unstaged again", got)
	}
	if got := underCursor(t, m); !strings.HasPrefix(got, unstagedMarker+" @@") {
		t.Errorf("under the cursor: %q, want the header of the hunk taken back", got)
	}
}

// Below a hunk's header, s takes the one line the cursor is on: the header
// is for the hunk, and a line is for itself.
func TestStagingOneLineFromTheDiff(t *testing.T) {
	dir, _ := stagingRepo(t)
	// One hunk of three changed lines.
	write(t, dir, "numbered.txt", strings.Replace(numberedLines(30), "line 15\n", "line fifteen\nand a half\n", 1))
	m := press(openedView(t, dir), keyPress("3"))
	if !strings.Contains(ansi.Strip(m.View()), "s: stage hunk") {
		t.Error("on the header, the status line does not say s stages the hunk")
	}

	changed := strings.Replace(numberedLines(30), "line 15\n", "line fifteen\nand a half\n", 1)

	// The hunk a line at a time, the cursor never leaving it: after each s
	// it is on the line below the one that went, which is the next to take.
	m = cursorOn(t, m, "-line 15")
	if !strings.Contains(ansi.Strip(m.View()), "s: stage line") {
		t.Error("on a line, the status line does not say s stages the line")
	}
	for _, step := range []struct{ index, next string }{
		{strings.Replace(numberedLines(30), "line 15\n", "", 1), unstagedMarker + " +line fifteen"},
		{strings.Replace(numberedLines(30), "line 15\n", "line fifteen\n", 1), unstagedMarker + " +and a half"},
	} {
		m = do(t, m, keyPress("s"))
		if got := indexed(t, dir, "numbered.txt"); got != step.index {
			t.Fatalf("the index holds:\n%s\nwant:\n%s", got, step.index)
		}
		if got := underCursor(t, m); got != step.next {
			t.Fatalf("under the cursor: %q, want the line below the one staged, %q", got, step.next)
		}
	}
	// What went is still in the box, marked staged, with what is left.
	d := m.stagingDiff()
	for _, want := range []string{stagedMarker + " -line 15", stagedMarker + " +line fifteen", unstagedMarker + " +and a half"} {
		if !strings.Contains(shown(d), want) {
			t.Errorf("the diff box does not show %q:\n%s", want, shown(d))
		}
	}
	m = do(t, m, keyPress("s"))
	if got := indexed(t, dir, "numbered.txt"); got != changed {
		t.Fatalf("the index holds:\n%s\nwant the whole change, a line at a time", got)
	}

	// Taking lines back works down a staged hunk the same way.
	m = cursorOn(t, m, "-line 15")
	if !strings.Contains(ansi.Strip(m.View()), "s: unstage line") {
		t.Error("the status line does not say s would take the line back")
	}
	m = do(t, m, keyPress("s"))
	if got, want := indexed(t, dir, "numbered.txt"), strings.Replace(numberedLines(30), "line 15\n", "line 15\nline fifteen\nand a half\n", 1); got != want {
		t.Fatalf("the index holds:\n%s\nwant the removal taken back", got)
	}
	if got := underCursor(t, m); got != stagedMarker+" +line fifteen" {
		t.Errorf("under the cursor: %q, want the next staged line", got)
	}

	// A line that changes nothing has nothing to stage, and says so; the
	// cursor stays on it.
	m = do(t, cursorOn(t, m, " line 14"), keyPress("s"))
	if !strings.Contains(m.notice, "No changed lines") {
		t.Errorf("notice = %q, want it to say nothing there can be staged", m.notice)
	}
	if got := underCursor(t, m); got != "   line 14" {
		t.Errorf("under the cursor: %q after a key that moved nothing", got)
	}
}

// An untracked file is drawn as its contents, with no header to press for
// all of it, so s anywhere in it still takes the file.
func TestStagingAnUntrackedFileFromTheDiffTakesItWhole(t *testing.T) {
	dir, _ := stagingRepo(t)
	write(t, dir, "new.txt", "one\ntwo\nthree\n")
	m := openedView(t, dir)
	for i, f := range m.viewedFiles() {
		if f.Path == "new.txt" {
			m.commitView.file = i
			m.showFile()
		}
	}
	m = cursorOn(t, press(m, keyPress("3")), "+two")
	if !strings.Contains(ansi.Strip(m.View()), "s: stage hunk") {
		t.Error("the status line does not say s takes more than the line")
	}
	m = do(t, m, keyPress("s"))
	if got := indexed(t, dir, "new.txt"); got != "one\ntwo\nthree\n" {
		t.Fatalf("the index holds %q, want the whole file", got)
	}
}

func TestStagingPickedLines(t *testing.T) {
	dir, _ := stagingRepo(t)
	// One hunk: a line replaced and a line added next to it.
	write(t, dir, "numbered.txt", strings.Replace(numberedLines(30), "line 15\n", "line fifteen\nand a half\n", 1))
	m := openedView(t, dir)

	// v belongs to the diff; from the list it says where to go.
	m = press(m, keyPress("v"))
	if m.commitView.selecting || !strings.Contains(m.notice, "press 3") {
		t.Fatalf("selecting=%v notice=%q, want v from the file list to point at the diff", m.commitView.selecting, m.notice)
	}

	m = cursorOn(t, press(m, keyPress("3")), "+line fifteen")
	m = press(m, keyPress("v"), keyPress("j"))
	if !m.commitView.selecting || !m.linePicked(m.stagingDiff(), m.commitView.diffCursor) || !m.linePicked(m.stagingDiff(), m.commitView.diffCursor-1) {
		t.Fatal("v then j did not pick two lines")
	}
	if !strings.Contains(ansi.Strip(m.View()), "s: stage these lines") {
		t.Error("the status line does not say what s would stage")
	}

	// Esc lets go of the lines without leaving the view.
	m = press(m, esc)
	if m.commitView.selecting || !m.commitView.open {
		t.Fatalf("selecting=%v open=%v after esc, want the selection dropped and the view kept", m.commitView.selecting, m.commitView.open)
	}

	// One line: v and s with the cursor where it is.
	m = cursorOn(t, m, "+and a half")
	m = do(t, press(m, keyPress("v")), keyPress("s"))
	want := strings.Replace(numberedLines(30), "line 15\n", "line 15\nand a half\n", 1)
	if got := indexed(t, dir, "numbered.txt"); got != want {
		t.Fatalf("the index holds:\n%s\nwant only the one added line", got)
	}
	if m.commitView.selecting {
		t.Error("the selection outlived the lines it picked")
	}

	// The line is still in the box, marked staged, beside the one that was
	// left; the cursor has not been sent back to the top.
	d := m.stagingDiff()
	for _, want := range []string{stagedMarker + " +and a half", unstagedMarker + " +line fifteen", unstagedMarker + " -line 15"} {
		if !strings.Contains(shown(d), want) {
			t.Errorf("the diff box does not show %q:\n%s", want, shown(d))
		}
	}
	// The cursor is still in the hunk the line left, on the line below it;
	// the line itself can be taken back from where it is now.
	if got := underCursor(t, m); got != "   line 16" {
		t.Fatalf("under the cursor: %q, want the line below the one staged", got)
	}
	m = cursorOn(t, m, "+and a half")
	m = do(t, press(m, keyPress("v")), keyPress("s"))
	if got := indexed(t, dir, "numbered.txt"); got != numberedLines(30) {
		t.Fatalf("the index holds:\n%s\nwant the line taken back", got)
	}
}

// Lines picked across a staged hunk and one that is not cannot go both ways
// at once: the side the selection was started on is the one that moves.
func TestStagingPickedLinesKeepsToOneSide(t *testing.T) {
	dir, _ := stagingRepo(t)
	changed := twoHunks(t, dir)
	m := press(openedView(t, dir), keyPress("3"))
	m = do(t, onHunk(t, m, "+line two"), keyPress("s"))

	// From a line of the hunk that is left, up over the staged one to the top.
	m = cursorOn(t, m, "+line twenty-nine")
	m = press(m, keyPress("v"), keyPress("g"))
	d := m.stagingDiff()
	for i, l := range d.lines {
		if got := m.linePicked(d, i); got != (!l.staged && i <= m.commitView.diffAnchor) {
			t.Fatalf("line %d (%q, staged=%v): picked=%v, want only the side the selection started on", i, l.text, l.staged, got)
		}
	}
	if !strings.Contains(ansi.Strip(m.View()), "s: stage these lines") {
		t.Error("the status line does not go by the side the selection started on")
	}
	m = do(t, m, keyPress("s"))
	if got := indexed(t, dir, "numbered.txt"); got != changed {
		t.Fatalf("the index holds:\n%s\nwant the second hunk staged and the first left staged", got)
	}
}

func hunk(header string, lines ...string) string {
	return strings.Join(append([]string{header}, lines...), "\n")
}

// The two diffs' hunks are laid out by where they are in the index, which is
// the one file both are counted in.
func TestMergeSidesOrdersHunksAsTheFileDoes(t *testing.T) {
	staged := &fileDiff{Path: "a", Staged: true, Body: strings.Join([]string{
		hunk("@@ -2 +2 @@", "-two", "+TWO"),
		hunk("@@ -20,0 +21 @@", "+twenty and a half"),
	}, "\n")}
	unstaged := &fileDiff{Path: "a", Body: strings.Join([]string{
		hunk("@@ -10 +10 @@", "-ten", "+TEN"),
		hunk("@@ -21 +21,2 @@", " twenty and a half", "+and three quarters"),
		hunk("@@ -40 +41 @@", "-forty", "+FORTY"),
	}, "\n")}

	var got []string
	for _, l := range mergeSides(staged, unstaged) {
		if strings.HasPrefix(l.text, "@@") {
			side := "unstaged"
			if l.staged {
				side = "staged"
			}
			got = append(got, fmt.Sprintf("%s %s at %d", side, l.text, l.at))
		}
	}
	want := []string{
		"staged @@ -2 +2 @@ at 0",
		"unstaged @@ -10 +10 @@ at 0",
		// Both start on line 21 of the index: what is staged comes first.
		"staged @@ -20,0 +21 @@ at 3",
		"unstaged @@ -21 +21,2 @@ at 3",
		"unstaged @@ -40 +41 @@ at 6",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("hunks:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// Each line remembers its place in the diff it came from, which is how
	// it is staged.
	for _, l := range mergeSides(staged, unstaged) {
		from := unstaged
		if l.staged {
			from = staged
		}
		if lines := strings.Split(from.Body, "\n"); lines[l.at] != l.text {
			t.Fatalf("%q says it is line %d of its diff, which reads %q", l.text, l.at, lines[l.at])
		}
	}
}

// One side alone is drawn as git wrote it, whatever its headers say, and a
// side with nothing textual adds nothing.
func TestMergeSidesLeavesOneSideAlone(t *testing.T) {
	odd := strings.Join([]string{
		hunk("@@ -30 +30 @@", "-b", "+B"),
		hunk("@@@ -1,2 -1,2 +1,3 @@@", "  a", "++b"),
		hunk("@@ -3 +3 @@", "-a", "+A"),
	}, "\n")
	for _, lines := range [][]stagingLine{
		mergeSides(nil, &fileDiff{Path: "a", Body: odd}),
		mergeSides(&fileDiff{Path: "a", Staged: true, Body: "  \n"}, &fileDiff{Path: "a", Body: odd}),
	} {
		var got []string
		for _, l := range lines {
			got = append(got, l.text)
		}
		if strings.Join(got, "\n") != odd {
			t.Errorf("drawn as:\n%s\nwant it as written:\n%s", strings.Join(got, "\n"), odd)
		}
	}
	if lines := mergeSides(nil, nil); len(lines) != 0 {
		t.Errorf("no sides drew %d lines", len(lines))
	}
}

func TestStagingContextLinesSaysSo(t *testing.T) {
	dir, _ := stagingRepo(t)
	write(t, dir, "numbered.txt", strings.Replace(numberedLines(30), "line 15\n", "line fifteen\n", 1))
	m := cursorOn(t, press(openedView(t, dir), keyPress("3")), " line 14")

	m = do(t, press(m, keyPress("v")), keyPress("s"))
	if !strings.Contains(m.notice, "No changed lines") {
		t.Errorf("notice = %q, want it to say nothing there can be staged", m.notice)
	}
	if got := indexed(t, dir, "numbered.txt"); got != numberedLines(30) {
		t.Error("the index changed")
	}
}

// The file was edited between reading it and pressing s: the line numbers on
// screen are of a diff that no longer exists.
func TestStagingAFileThatChangedShowsItAgain(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	m := press(openedView(t, dir), keyPress("3"))
	write(t, dir, "numbered.txt", strings.Replace(numberedLines(30), "line 2\n", "line two, again\n", 1))

	m = do(t, m, keyPress("s"))
	if !strings.Contains(m.notice, "changed since it was read") {
		t.Errorf("notice = %q, want it to say the file changed", m.notice)
	}
	if got := indexed(t, dir, "numbered.txt"); got != numberedLines(30) {
		t.Error("something was staged from a diff that had changed")
	}
	if body := diffBoxOf(t, m); !strings.Contains(body, "+line two, again") {
		t.Errorf("the diff box does not show the file as it is now:\n%s", body)
	}
}

func TestDiffCursorStaysOnScreen(t *testing.T) {
	dir, _ := stagingRepo(t)
	write(t, dir, "long.txt", numberedLines(200))
	write(t, dir, "notes.txt", "a note, changed\n")
	m := openedView(t, dir)
	for i, f := range m.viewedFiles() {
		if f.Path == "long.txt" {
			m.commitView.file = i
			m.showFile()
		}
	}
	m = press(m, keyPress("3"))

	for range 60 {
		m = press(m, keyPress("j"))
	}
	if m.commitView.diffCursor != 60 {
		t.Fatalf("cursor on line %d after 60 presses, want 60", m.commitView.diffCursor)
	}
	if body := diffBoxOf(t, m); !strings.Contains(body, "+line 61") {
		t.Errorf("the line under the cursor is not on screen:\n%s", body)
	}
	// The row drawn there is the row a click finds.
	l := m.currentCommitViewLayout()
	for y := commitViewTopRow + 1; y < commitViewTopRow+l.rows-1; y++ {
		if m.diffLineAt(y) == m.commitView.diffCursor {
			line := strings.Split(ansi.Strip(m.View()), "\n")[y]
			if !strings.Contains(line, "+line 61") {
				t.Errorf("row %d is the cursor's but reads %q", y, line)
			}
		}
	}

	m = press(m, keyPress("G"))
	if m.commitView.diffCursor != 199 || !strings.Contains(diffBoxOf(t, m), "+line 200") {
		t.Errorf("G put the cursor on line %d, want the last, on screen", m.commitView.diffCursor)
	}

	// A click puts the cursor on the line clicked; the wheel moves it.
	res, _ := m.Update(click(l.leftWidth+4, commitViewTopRow+3))
	clicked := res.(model)
	if want := m.diffLineAt(commitViewTopRow + 3); clicked.commitView.diffCursor != want {
		t.Errorf("cursor on %d after a click, want %d", clicked.commitView.diffCursor, want)
	}
	res, _ = m.Update(wheel(l.leftWidth+4, 8, tea.MouseButtonWheelUp))
	if got := res.(model).commitView.diffCursor; got != 199-mouseScrollLines {
		t.Errorf("cursor on %d after a notch up, want %d", got, 199-mouseScrollLines)
	}

	// Another file starts from its top.
	m = press(m, keyPress("2"), keyPress("j"))
	if m.commitView.diffCursor != 0 || m.commitView.diffScroll != 0 {
		t.Errorf("cursor %d, scroll %d on another file, want both at the top", m.commitView.diffCursor, m.commitView.diffScroll)
	}
}

// On a commit there is nothing to stage, and the keys must not pretend.
func TestStagingKeysDoNothingOnACommit(t *testing.T) {
	m := openedView(t, threeFileRepo(t))
	before := ansi.Strip(m.View())
	for _, key := range []string{"s", "S", "v", "c"} {
		next, cmd := m.Update(keyPress(key))
		got := next.(model)
		if cmd != nil || got.staging || got.commitPrompt.open || got.commitView.selecting || got.notice != "" {
			t.Errorf("%q did something on a commit: cmd=%v staging=%v prompt=%v notice=%q",
				key, cmd != nil, got.staging, got.commitPrompt.open, got.notice)
		}
		if after := ansi.Strip(got.View()); after != before {
			t.Errorf("%q changed the screen", key)
		}
	}
}

// commitWith types a message into the prompt and commits, feeding back what the
// commit reports.
func commitWith(t *testing.T, m model, subject, body string) model {
	t.Helper()
	m = press(m, keyPress("c"))
	if !m.commitPrompt.open {
		t.Fatalf("c did not open the message box; notice %q", m.notice)
	}
	m = typeText(m, subject)
	if body != "" {
		m = typeText(press(m, tab), body)
		m = press(m, tab)
	}
	next, cmd := m.Update(enter)
	m = next.(model)
	if !m.committing || cmd == nil {
		t.Fatalf("committing=%v cmd=%v; want enter to start the commit", m.committing, cmd != nil)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Committing") {
		t.Error("the status line does not say a commit is running")
	}
	next, _ = m.Update(cmd())
	return next.(model)
}

func TestCommitWhatIsStaged(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	write(t, dir, "notes.txt", "a note, changed\n")
	m := openedView(t, dir)

	// Nothing staged: c says so rather than committing everything.
	m = press(m, keyPress("c"))
	if m.commitPrompt.open || !strings.Contains(m.notice, "Nothing staged") {
		t.Fatalf("open=%v notice=%q, want c to say nothing is staged", m.commitPrompt.open, m.notice)
	}

	m = do(t, m, keyPress("s")) // notes.txt
	m = press(m, keyPress("c"))
	screen := ansi.Strip(m.View())
	for _, want := range []string{"Commit", "1 staged file to main", "Subject", "Body", "enter: commit"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the message box does not show %q:\n%s", want, screen)
		}
	}
	m = press(m, esc)

	m = commitWith(t, m, "Reword the note", "It said too little.\nNow it says more.")
	if !strings.Contains(m.notice, "Committed") || !strings.Contains(m.notice, "Reword the note") {
		t.Fatalf("notice = %q, want the new commit named", m.notice)
	}
	if got, _ := git.Run(dir, "log", "-1", "--format=%B"); got != "Reword the note\n\nIt said too little.\nNow it says more." {
		t.Errorf("message = %q", got)
	}
	if got, _ := git.Run(dir, "show", "--format=", "--name-only", "HEAD"); got != "notes.txt" {
		t.Errorf("the commit holds %q, want only the staged file", got)
	}

	// It reloads, and with changes left the view stays on them, ready for
	// the next commit, with an empty message box.
	if m.ready {
		t.Fatal("the repository was not read again after the commit")
	}
	next, cmd := m.Update(loadRepo(dir)())
	m = next.(model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(model)
	}
	if c, _ := m.viewedCommit(); !m.commitView.open || !c.WorkingTree {
		t.Fatalf("open=%v on the uncommitted changes=%v, want the view still on what is left", m.commitView.open, c.WorkingTree)
	}
	if got := listed(m); got != "numbered.txt:unstaged" {
		t.Errorf("files = %q, want what was not committed", got)
	}
	if len(m.commits) < 2 || m.commits[1].Message != "Reword the note" {
		t.Errorf("the new commit is not under the uncommitted changes: %q", messages(m))
	}
	if !strings.Contains(m.notice, "Committed") {
		t.Errorf("notice = %q after the reload, want the commit still named", m.notice)
	}
	m = do(t, m, keyPress("S"))
	if m = press(m, keyPress("c")); m.commitPrompt.subject.Value() != "" || m.commitPrompt.body.Value() != "" {
		t.Error("the last commit's message is still in the box")
	}
}

// With nothing left uncommitted there is nothing for the view to show.
func TestCommitOfEverythingReturnsToTheGraph(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	m := do(t, openedView(t, dir), keyPress("S"))

	m = commitWith(t, m, "Spell out two numbers", "")
	next, cmd := m.Update(loadRepo(dir)())
	m = next.(model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(model)
	}
	if m.commitView.open {
		t.Fatal("the view is still open with nothing uncommitted")
	}
	if c, _ := m.commitOnScreen(); c.WorkingTree || c.Message != "Spell out two numbers" {
		t.Errorf("selected %q, want the commit just made", c.Message)
	}
}

func TestCommitMessageBox(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	m := do(t, openedView(t, dir), keyPress("S"))
	before, _ := git.Run(dir, "rev-parse", "HEAD")

	// Enter with no subject commits nothing and says why.
	m = press(m, keyPress("c"))
	next, cmd := m.Update(enter)
	m = next.(model)
	if cmd != nil || m.committing || !m.commitPrompt.open || !strings.Contains(ansi.Strip(m.View()), "needs a subject") {
		t.Fatalf("cmd=%v committing=%v open=%v, want an empty subject refused in the box", cmd != nil, m.committing, m.commitPrompt.open)
	}

	// Letters that are keys elsewhere are text here, and enter in the body
	// is a new line, not the commit.
	// The space bar arrives as a key of its own, which closes the view when
	// the box is not open.
	m = press(typeText(m, "s"), tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	m = typeText(m, "S c q")
	m = typeText(press(m, tab), "first")
	next, cmd = m.Update(enter)
	m = typeText(next.(model), "second")
	if cmd != nil && m.committing {
		t.Fatal("enter in the body committed")
	}
	if got := m.commitPrompt.message(); got != "s S c q\n\nfirst\nsecond" {
		t.Fatalf("message = %q", got)
	}
	if !strings.Contains(ansi.Strip(m.View()), "enter: new line") {
		t.Error("the box does not say what enter does in the body")
	}

	// Esc closes it without committing, and c finds the message as it was.
	m = press(m, esc)
	if m.commitPrompt.open || !m.commitView.open {
		t.Fatalf("prompt open=%v, view open=%v after esc", m.commitPrompt.open, m.commitView.open)
	}
	m = press(m, keyPress("c"))
	if got := m.commitPrompt.message(); got != "s S c q\n\nfirst\nsecond" {
		t.Errorf("message = %q after reopening, want it kept", got)
	}
	if after, _ := git.Run(dir, "rev-parse", "HEAD"); after != before {
		t.Error("something was committed along the way")
	}

	// The box fits whatever window it is drawn over.
	for _, size := range [][2]int{{110, 26}, {80, 24}, {60, 12}, {40, 10}} {
		m.windowWidth, m.windowHeight = size[0], size[1]
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		if len(lines) != m.windowHeight {
			t.Errorf("%dx%d: %d lines, want %d", size[0], size[1], len(lines), m.windowHeight)
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > m.windowWidth {
				t.Errorf("%dx%d: line %d is %d wide", size[0], size[1], i, w)
			}
		}
	}

	// A reload nobody asked for would take the box away mid-sentence.
	if m.canReloadUnasked() {
		t.Error("an unasked reload is allowed while the message is being typed")
	}
}

// A hook that refuses is the repository's own rule: its words are shown, and
// the message is still there to try again with.
func TestCommitRefusedByAHook(t *testing.T) {
	dir, _ := stagingRepo(t)
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	writeFile(t, hook, "#!/bin/sh\necho 'lint: numbered.txt has a typo' >&2\nexit 1\n")
	if runtime.GOOS != "windows" {
		if err := os.Chmod(hook, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	twoHunks(t, dir)
	m := do(t, openedView(t, dir), keyPress("S"))
	before, _ := git.Run(dir, "rev-parse", "HEAD")

	m = commitWith(t, m, "Spell out two numbers", "")
	if after, _ := git.Run(dir, "rev-parse", "HEAD"); after != before {
		t.Fatal("the commit went through a hook that refused it")
	}
	if !m.ready || !m.commitPrompt.open || m.committing {
		t.Fatalf("ready=%v open=%v committing=%v, want the box back and nothing reloaded", m.ready, m.commitPrompt.open, m.committing)
	}
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "lint: numbered.txt has a typo") {
		t.Errorf("the box does not show what the hook said:\n%s", screen)
	}
	if m.commitPrompt.subject.Value() != "Spell out two numbers" {
		t.Errorf("subject = %q, want the message kept", m.commitPrompt.subject.Value())
	}
	if got := listed(m); got != "numbered.txt:staged" {
		t.Errorf("files = %q, want the change still staged", got)
	}
}

func TestCommitRefusedOnNoBranch(t *testing.T) {
	dir, gitCmd := stagingRepo(t)
	gitCmd("checkout", "-q", "--detach")
	twoHunks(t, dir)
	m := do(t, openedView(t, dir), keyPress("S"))
	if got := listed(m); got != "numbered.txt:staged" {
		t.Fatalf("files = %q, want staging to work on no branch", got)
	}

	m = press(m, keyPress("c"))
	if m.commitPrompt.open || !strings.Contains(m.notice, "on no branch") {
		t.Errorf("open=%v notice=%q, want c refused with the reason", m.commitPrompt.open, m.notice)
	}
}

// During a merge, staging a file means "this conflict is resolved" and a
// commit concludes the merge. Neither is what these keys are for.
func TestStagingRefusedDuringAMerge(t *testing.T) {
	dir, gitCmd := stagingRepo(t)
	gitCmd("checkout", "-qb", "side")
	write(t, dir, "numbered.txt", strings.Replace(numberedLines(30), "line 2\n", "line two, their way\n", 1))
	gitCmd("commit", "-qam", "theirs")
	gitCmd("checkout", "-q", "main")
	write(t, dir, "numbered.txt", strings.Replace(numberedLines(30), "line 2\n", "line two, our way\n", 1))
	gitCmd("commit", "-qam", "ours")
	git.Run(dir, "merge", "side") // fails, on purpose
	before, _ := git.Run(dir, "status", "--porcelain")
	if !strings.HasPrefix(before, "UU") {
		t.Fatalf("status = %q, want a conflict", before)
	}

	m := openedView(t, dir)
	if got := listed(m); got != "numbered.txt:unstaged" {
		t.Fatalf("files = %q, want the conflicted file listed once", got)
	}
	for _, key := range []string{"s", "S", "c"} {
		next, cmd := m.Update(keyPress(key))
		got := next.(model)
		if cmd != nil || got.staging || got.commitPrompt.open || !strings.Contains(got.notice, "merge is in progress") {
			t.Errorf("%q: cmd=%v staging=%v prompt=%v notice=%q, want it refused with the reason",
				key, cmd != nil, got.staging, got.commitPrompt.open, got.notice)
		}
	}
	if after, _ := git.Run(dir, "status", "--porcelain"); after != before {
		t.Errorf("status went from %q to %q", before, after)
	}
}

// Answers that arrive after the repository was switched are about another
// repository's index.
func TestLateStagingAnswersAreDropped(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	m := openedView(t, dir)

	next, cmd := m.Update(keyPress("s"))
	staged := cmd().(stageFinishedMsg)
	staged.repoPath = "elsewhere"
	if got := res(next.(model).Update(staged)); !got.staging || listed(got) != "numbered.txt:unstaged" {
		t.Errorf("staging=%v files=%q, want the answer ignored", got.staging, listed(got))
	}

	got := res(m.Update(commitFinishedMsg{repoPath: "elsewhere", result: git.CommitResult{Hash: "abc", Subject: "x"}}))
	if got.notice != "" || !got.ready {
		t.Errorf("notice=%q ready=%v, want a commit elsewhere ignored", got.notice, got.ready)
	}
}

// A second s before the first has answered would be staging from a list that
// is about to change.
func TestStagingWaitsForTheLastChange(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	write(t, dir, "notes.txt", "a note, changed\n")
	m := openedView(t, dir)

	next, first := m.Update(keyPress("s"))
	for _, key := range []string{"s", "S", "c"} {
		after, cmd := next.(model).Update(keyPress(key))
		if cmd != nil || after.(model).commitPrompt.open {
			t.Errorf("%q started something while a change was running", key)
		}
	}
	if first == nil {
		t.Fatal("the first s started nothing")
	}
}

// The view of the uncommitted changes has keys a commit's does not, and its
// help has to show every one of them on a terminal of the usual height.
func TestHelpOnUncommittedChangesFitsAndListsTheKeys(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	m := openedView(t, dir)
	m.windowWidth, m.windowHeight = 80, 24
	screen := ansi.Strip(press(m, keyPress("?")).View())

	for _, want := range []string{"stage or unstage the file", "on a hunk's @@ line the hunk, below it the line", "pick lines", "stage or unstage everything; diff: this file", "commit what is staged", "back to the graph", "quit"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the help does not show %q on an 80x24 terminal:\n%s", want, screen)
		}
	}
	// A commit's help is the one without them.
	if on := ansi.Strip(press(openedView(t, threeFileRepo(t)), keyPress("?")).View()); strings.Contains(on, "stage") {
		t.Error("the help on a commit lists staging keys")
	}
}

// In the diff box S reaches as far as the box does: the one file shown, all
// of it, and no other.
func TestCapitalSInTheDiffTakesOnlyTheFileShown(t *testing.T) {
	dir, _ := stagingRepo(t)
	changed := twoHunks(t, dir)
	write(t, dir, "notes.txt", "a note, changed\n")
	write(t, dir, "new.txt", "fresh\n")
	m := openedView(t, dir)
	for i, f := range m.viewedFiles() {
		if f.Path == "numbered.txt" {
			m.commitView.file = i
			m.showFile()
		}
	}
	const others = "new.txt:untracked notes.txt:unstaged "

	// With one hunk staged already, S takes the rest of the file.
	m = do(t, press(m, keyPress("3")), keyPress("s"))
	if got := listed(m); got != others+"numbered.txt:staged numbered.txt:unstaged" {
		t.Fatalf("files = %q, want numbered.txt on both sides", got)
	}
	if !strings.Contains(ansi.Strip(m.View()), "S: file") {
		t.Error("the status line does not say S is the file's here")
	}
	cursor := m.commitView.diffCursor
	m = do(t, m, keyPress("S"))
	if got := indexed(t, dir, "numbered.txt"); got != changed {
		t.Fatalf("the index holds:\n%s\nwant all of the file", got)
	}
	if got := listed(m); got != others+"numbered.txt:staged" {
		t.Fatalf("after S, files = %q, want only numbered.txt staged", got)
	}
	// Nothing jumped: the same file, the same row of its diff.
	if got := selectedEntry(t, m); got != "numbered.txt:staged" || m.commitView.diffCursor != cursor {
		t.Errorf("selected %q, cursor %d; want the file shown and the cursor on row %d", got, m.commitView.diffCursor, cursor)
	}
	if !strings.Contains(ansi.Strip(m.View()), "S: unstage file") {
		t.Error("the status line does not say S would take the file back")
	}

	// With none of it left to stage, S takes all of it back, and only it.
	m = do(t, m, keyPress("S"))
	if got := indexed(t, dir, "numbered.txt"); got != numberedLines(30) {
		t.Fatalf("the index holds:\n%s\nwant none of the file", got)
	}
	if got := listed(m); got != others+"numbered.txt:unstaged" {
		t.Fatalf("after S again, files = %q", got)
	}
	if got := selectedEntry(t, m); got != "numbered.txt:unstaged" {
		t.Errorf("selected %q, want the file shown", got)
	}

	// From the file list the same key is still every file.
	m = do(t, press(m, keyPress("2")), keyPress("S"))
	if got := listed(m); got != "new.txt:staged notes.txt:staged numbered.txt:staged" {
		t.Errorf("after S on the list, files = %q, want everything staged", got)
	}
}
