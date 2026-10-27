package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
)

func ticks(b stashBox) string {
	var out []string
	for _, f := range b.files {
		mark := "[ ]"
		if f.ticked {
			mark = "[x]"
		}
		out = append(out, mark+f.path)
	}
	return strings.Join(out, " ")
}

// "s" in the view opens the box on the selected file alone; enter with no
// name stashes that file, called after it.
func TestStashAFileFromTheView(t *testing.T) {
	dir, _ := stagingRepo(t)
	write(t, dir, "notes.txt", "a note, changed\n")
	body := twoHunks(t, dir)
	m := openedView(t, dir)
	if got := selectedEntry(t, m); got != "notes.txt:unstaged" {
		t.Fatalf("selected %q", got)
	}

	m = press(m, keyPress("s"))
	if got := ticks(m.stashBox); got != "[x]notes.txt [ ]numbered.txt" {
		t.Fatalf("the box ticks %q", got)
	}
	if !strings.Contains(ansi.Strip(m.View()), "enter: stash 1 file") {
		t.Fatalf("the box is not on screen:\n%s", ansi.Strip(m.View()))
	}
	m = do(t, m, tea.KeyMsg{Type: tea.KeyEnter})
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
	if !strings.Contains(m.notice, "Stashed 1 file as “notes.txt”") {
		t.Errorf("notice = %q", m.notice)
	}
	if stashes, _ := git.ListStashes(dir); len(stashes) != 1 || !strings.HasSuffix(stashes[0].Subject, ": notes.txt") {
		t.Errorf("stashes = %+v, want one called after the file", stashes)
	}
}

// "S" ticks every file; tab and space untick one, and the name typed is the
// stash's.
func TestStashSomeFilesUnderAName(t *testing.T) {
	dir, _ := stagingRepo(t)
	write(t, dir, "notes.txt", "a note, changed\n")
	twoHunks(t, dir)
	write(t, dir, "new.txt", "fresh\n")
	m := openedView(t, dir)

	m = press(m, keyPress("S"))
	if got := ticks(m.stashBox); got != "[x]new.txt [x]notes.txt [x]numbered.txt" {
		t.Fatalf("the box ticks %q", got)
	}
	m = typeText(m, "the parser")
	m = press(m, keyPress("tab"), keyPress("j"), keyPress(" "))
	if got := ticks(m.stashBox); got != "[x]new.txt [ ]notes.txt [x]numbered.txt" {
		t.Fatalf("after unticking: %q", got)
	}
	if got := m.stashBox.name.Value(); got != "the parser" {
		t.Fatalf("space on the files went to the name: %q", got)
	}
	m = do(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if fileIs(t, dir, "notes.txt") != "a note, changed\n" {
		t.Error("the unticked file went into the stash")
	}
	if fileIs(t, dir, "numbered.txt") != numberedLines(30) || fileIs(t, dir, "new.txt") != "" {
		t.Error("the ticked files are still changed")
	}
	stashes, _ := git.ListStashes(dir)
	if len(stashes) != 1 || !strings.HasSuffix(stashes[0].Subject, ": the parser") {
		t.Errorf("stashes = %+v, want one called “the parser”", stashes)
	}
}

// With nothing ticked, enter says so and leaves the box open.
func TestStashBoxWantsAFile(t *testing.T) {
	dir, _ := stagingRepo(t)
	write(t, dir, "notes.txt", "a note, changed\n")
	m := press(openedView(t, dir), keyPress("s"), keyPress("tab"), keyPress(" "), keyPress("enter"))
	if !m.stashBox.open || m.stashBox.failure == "" {
		t.Errorf("open %v, failure %q", m.stashBox.open, m.stashBox.failure)
	}
	m = press(m, keyPress("esc"))
	if m.stashBox.open || fileIs(t, dir, "notes.txt") != "a note, changed\n" {
		t.Error("esc did not close the box, or stashed")
	}
}

func TestDefaultStashName(t *testing.T) {
	for _, tc := range []struct {
		paths []string
		want  string
	}{
		{[]string{"a.go"}, "a.go"},
		{[]string{"a.go", "b.go"}, "a.go, b.go"},
		{[]string{"internal/tui/stash.go", "internal/tui/stash_test.go", "internal/git/stash.go"},
			"internal/tui/stash.go and 2 more files"},
	} {
		if got := defaultStashName(tc.paths); got != tc.want {
			t.Errorf("defaultStashName(%v) = %q, want %q", tc.paths, got, tc.want)
		}
	}
}

// "S" on the graph opens the box at once, standing for every file until they
// have been read, and puts everything aside; "s" there lists it and pops it
// back with enter.
func TestStashEverythingAndPopFromTheGraph(t *testing.T) {
	dir, _ := stagingRepo(t)
	body := twoHunks(t, dir)
	write(t, dir, "new.txt", "fresh\n")
	m := loadedModel(t, dir)
	m.windowWidth, m.windowHeight = 110, 26

	next, cmd := m.Update(keyPress("S"))
	m = next.(model)
	if !m.stashBox.open || !m.stashBox.waiting || !strings.Contains(ansi.Strip(m.View()), "Every uncommitted change") {
		t.Fatalf("S did not open the box before the files were read:\n%s", ansi.Strip(m.View()))
	}
	m = res(m.Update(cmd()))
	if got := ticks(m.stashBox); got != "[x]new.txt [x]numbered.txt" {
		t.Fatalf("once read, the box ticks %q", got)
	}
	m = typeText(m, "lunch")
	m = do(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if out, _ := git.Run(dir, "status", "--porcelain"); out != "" {
		t.Fatalf("status after S = %q, want nothing", out)
	}
	if !strings.Contains(m.notice, "Stashed all uncommitted changes as “lunch”") {
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

// On a stash's row, "s" opens the list with that stash picked.
func TestStashListOpensOnTheSelectedStash(t *testing.T) {
	dir, gitCmd := stagingRepo(t)
	write(t, dir, "notes.txt", "older\n")
	gitCmd("stash", "push", "-q")
	write(t, dir, "notes.txt", "newer\n")
	gitCmd("stash", "push", "-q")
	stashes, _ := git.ListStashes(dir)

	m := loadedModel(t, dir)
	row := -1
	for i, c := range m.commits {
		if c.FullHash == stashes[0].Hash {
			row = i
		}
	}
	if row < 0 {
		t.Fatal("the newest stash is not drawn on the graph")
	}
	m.selected = row
	_, cmd := m.Update(keyPress("s"))
	msg := cmd().(stashListMsg)
	if msg.selected != stashes[0].Hash {
		t.Fatalf("the list was asked for on %q, want the stash's row", msg.selected)
	}

	// Only the newest stash has a row, so the rest of the list is what
	// says the cursor follows the hash rather than starting at the top.
	msg.selected = stashes[1].Hash
	if got := m.showStashList(msg).stashPicker.cursor; got != 1 {
		t.Errorf("the list opened on row %d, want the stash selected (1)", got)
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
