package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

func fileIs(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return string(data)
}

func TestDiscardAFileAsksFirst(t *testing.T) {
	dir, _ := stagingRepo(t)
	write(t, dir, "notes.txt", "a note, changed\n")
	twoHunks(t, dir)
	m := openedView(t, dir)
	if got := listed(m); got != "notes.txt:unstaged numbered.txt:unstaged" {
		t.Fatalf("files = %q", got)
	}

	// Anything but y leaves it alone.
	m = do(t, m, keyPress("d"))
	if !strings.Contains(ansi.Strip(m.View()), "The unstaged changes to notes.txt") {
		t.Fatalf("d did not ask:\n%s", ansi.Strip(m.View()))
	}
	m = do(t, m, keyPress("n"))
	if fileIs(t, dir, "notes.txt") != "a note, changed\n" || m.notice != "Nothing discarded" {
		t.Fatalf("n discarded, or did not say it had not (notice %q)", m.notice)
	}

	m = do(t, m, keyPress("d"))
	m = do(t, m, keyPress("y"))
	if got := fileIs(t, dir, "notes.txt"); got != "a note\n" {
		t.Errorf("notes.txt is %q, want it as committed", got)
	}
	if got, sel := listed(m), selectedEntry(t, m); got != "numbered.txt:unstaged" || sel != "numbered.txt:unstaged" {
		t.Errorf("after discarding: files = %q, selected %q", got, sel)
	}
	if !strings.Contains(m.notice, "Discarded the changes to notes.txt") {
		t.Errorf("notice = %q", m.notice)
	}
}

// What is staged is out of a discard's reach: refused on a staged row, and
// left as it is when the file's unstaged half goes.
func TestDiscardKeepsWhatIsStaged(t *testing.T) {
	dir, gitCmd := stagingRepo(t)
	staged := strings.Replace(numberedLines(30), "line 2\n", "line two\n", 1)
	write(t, dir, "numbered.txt", staged)
	gitCmd("add", "numbered.txt")
	write(t, dir, "numbered.txt", strings.Replace(staged, "line 29\n", "line twenty-nine\n", 1))

	m := openedView(t, dir)
	if got := selectedEntry(t, m); got != "numbered.txt:staged" {
		t.Fatalf("selected %q", got)
	}
	m = do(t, m, keyPress("d"))
	if m.discard.asking || m.notice != keptNotice {
		t.Fatalf("d on a staged row: asking %v, notice %q", m.discard.asking, m.notice)
	}

	// D in the diff box takes the file's unstaged half, from either row.
	m = press(m, keyPress("3"))
	m = do(t, m, keyPress("D"))
	m = do(t, m, keyPress("y"))
	if got := fileIs(t, dir, "numbered.txt"); got != staged {
		t.Errorf("numbered.txt is not what is staged:\n%s", got)
	}
	if got := indexed(t, dir, "numbered.txt"); got != staged {
		t.Error("the discard changed the index")
	}
	if got := listed(m); got != "numbered.txt:staged" {
		t.Errorf("files = %q, want the staged half alone", got)
	}
}

func TestDiscardAHunkFromTheDiff(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	m := openedView(t, dir)
	m = press(m, keyPress("3"))
	m = onHunk(t, m, "+line twenty-nine")

	m = do(t, m, keyPress("d"))
	if !strings.Contains(ansi.Strip(m.View()), "This hunk of numbered.txt") {
		t.Fatalf("d did not ask about the hunk:\n%s", ansi.Strip(m.View()))
	}
	m = do(t, m, keyPress("y"))
	want := strings.Replace(numberedLines(30), "line 2\n", "line two\n", 1)
	if got := fileIs(t, dir, "numbered.txt"); got != want {
		t.Errorf("numbered.txt is:\n%s\nwant only the first hunk left", got)
	}
	if out, _ := git.Run(dir, "diff", "--cached", "--name-only"); out != "" {
		t.Errorf("staged %q, want nothing", out)
	}
}

// When nothing uncommitted is left, the view has nothing to show and the
// graph loses its row, so the view closes and the repository is read again.
func TestDiscardingEverythingClosesTheView(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	write(t, dir, "new.txt", "fresh\n")
	m := openedView(t, dir)

	next, cmd := m.Update(keyPress("D"))
	m = next.(model)
	if !strings.Contains(m.discard.what, "2 files") {
		t.Fatalf("what = %q, want the files counted", m.discard.what)
	}
	next, cmd = m.Update(keyPress("y"))
	m = next.(model)
	next, cmd = m.Update(cmd())
	m = next.(model)
	if m.commitView.open || cmd == nil {
		t.Errorf("view open %v, reload %v: want it closed and the repository read again", m.commitView.open, cmd != nil)
	}
	if out, _ := git.Run(dir, "status", "--porcelain"); out != "" {
		t.Errorf("status = %q, want nothing left", out)
	}
}

func TestDiscardFromTheGraph(t *testing.T) {
	dir, gitCmd := stagingRepo(t)
	write(t, dir, "notes.txt", "staged\n")
	gitCmd("add", "notes.txt")
	twoHunks(t, dir)
	write(t, dir, "new.txt", "fresh\n")
	m := withDiff(t, loadedModel(t, dir))

	m = press(m, keyPress("D"))
	if !strings.Contains(ansi.Strip(m.View()), "The unstaged changes to 2 files") {
		t.Fatalf("D did not ask:\n%s", ansi.Strip(m.View()))
	}
	m = do(t, m, keyPress("y"))
	if out, _ := git.Run(dir, "status", "--porcelain"); out != "M  notes.txt" {
		t.Errorf("status = %q, want only the staged file left", out)
	}

	// With nothing uncommitted, there is nothing to ask about.
	clean, _ := stagingRepo(t)
	m = press(loadedModel(t, clean), keyPress("D"))
	if m.discard.asking || !strings.Contains(m.notice, "Nothing to discard") {
		t.Errorf("asking %v, notice %q on a clean working tree", m.discard.asking, m.notice)
	}
}

// The keys stand out from the words beside them.
func TestDiscardBoxHighlightsItsKeys(t *testing.T) {
	withTrueColor(t)
	box := discardPrompt{title: "Discard changes", what: "This hunk of a.txt"}.render(100)
	key := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.SelectedFg))
	for _, k := range []string{"y", "esc"} {
		if !strings.Contains(box, key.Render(k)) {
			t.Errorf("%q is not drawn in the key colour:\n%q", k, box)
		}
	}
	if !strings.Contains(ansi.Strip(box), "y: discard • esc: cancel") {
		t.Errorf("the keys line reads:\n%s", ansi.Strip(box))
	}
}
