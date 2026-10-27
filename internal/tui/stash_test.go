package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
)

// "s" in the view takes the selected file aside, and only that file.
func TestStashAFileFromTheView(t *testing.T) {
	dir, _ := stagingRepo(t)
	write(t, dir, "notes.txt", "a note, changed\n")
	body := twoHunks(t, dir)
	m := openedView(t, dir)
	if got := selectedEntry(t, m); got != "notes.txt:unstaged" {
		t.Fatalf("selected %q", got)
	}

	m = do(t, m, keyPress("s"))
	// A stash moves refs/stash, so the graph is read again behind the view.
	m = res(m.Update(loadRepo(dir)()))
	m = withDiff(t, m)
	if got := fileIs(t, dir, "notes.txt"); got != "a note\n" {
		t.Errorf("notes.txt is %q, want it as committed", got)
	}
	if fileIs(t, dir, "numbered.txt") != body {
		t.Error("another file went into the stash")
	}
	if got := listed(m); got != "numbered.txt:unstaged" || !m.commitView.open {
		t.Errorf("files = %q, view open %v", got, m.commitView.open)
	}
	if !strings.Contains(m.notice, "Stashed notes.txt") {
		t.Errorf("notice = %q", m.notice)
	}
	if stashes, _ := git.ListStashes(dir); len(stashes) != 1 {
		t.Errorf("stashes = %+v, want one", stashes)
	}
}

// "S" on the graph puts everything aside, and "s" there lists it and pops it
// back with enter.
func TestStashEverythingAndPopFromTheGraph(t *testing.T) {
	dir, _ := stagingRepo(t)
	body := twoHunks(t, dir)
	write(t, dir, "new.txt", "fresh\n")
	m := withDiff(t, loadedModel(t, dir))

	m = do(t, m, keyPress("S"))
	if out, _ := git.Run(dir, "status", "--porcelain"); out != "" {
		t.Fatalf("status after S = %q, want nothing", out)
	}
	if !strings.Contains(m.notice, "Stashed all uncommitted changes") {
		t.Errorf("notice = %q", m.notice)
	}

	m = loadedModel(t, dir)
	m.windowWidth, m.windowHeight = 110, 26
	m = do(t, m, keyPress("s"))
	if !m.stashPicker.open || !strings.Contains(ansi.Strip(m.View()), "Pop stash") {
		t.Fatalf("s did not list the stash:\n%s", ansi.Strip(m.View()))
	}
	m = do(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if fileIs(t, dir, "numbered.txt") != body || fileIs(t, dir, "new.txt") != "fresh\n" {
		t.Error("the changes did not come back")
	}
	if !strings.HasPrefix(m.notice, "Popped stash@{0}") {
		t.Errorf("notice = %q", m.notice)
	}
	if stashes, _ := git.ListStashes(dir); len(stashes) != 0 {
		t.Errorf("the stash is still there: %+v", stashes)
	}
}

// A pop that would leave conflicts is refused, says why, and keeps the stash.
func TestPopThatWouldConflictIsRefused(t *testing.T) {
	dir, gitCmd := stagingRepo(t)
	write(t, dir, "notes.txt", "stashed\n")
	gitCmd("stash", "push", "-q")
	write(t, dir, "notes.txt", "committed\n")
	gitCmd("commit", "-qam", "second")

	m := loadedModel(t, dir)
	m = do(t, m, keyPress("s"))
	m = do(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.notice, "Not popped: it would conflict") {
		t.Errorf("notice = %q", m.notice)
	}
	if fileIs(t, dir, "notes.txt") != "committed\n" {
		t.Error("notes.txt was touched")
	}
	if stashes, _ := git.ListStashes(dir); len(stashes) != 1 {
		t.Errorf("stashes = %+v, want the one still there", stashes)
	}
}

func TestNothingToStashOrPop(t *testing.T) {
	dir, _ := stagingRepo(t)
	m := do(t, loadedModel(t, dir), keyPress("s"))
	if m.stashPicker.open || !strings.Contains(m.notice, "No stashes") {
		t.Errorf("open %v, notice %q with no stashes", m.stashPicker.open, m.notice)
	}
	m = do(t, m, keyPress("S"))
	if !strings.Contains(m.notice, "Nothing to stash") {
		t.Errorf("notice %q on a clean working tree", m.notice)
	}
}
