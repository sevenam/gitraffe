package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/gittest"
)

// branchedRepo is main with a commit past "first", where feature still is.
func branchedRepo(t *testing.T) (dir string, git func(...string)) {
	t.Helper()
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("branch", "feature")
	commit("second")
	return dir, git
}

func statusLine(m model) string {
	return ansi.Strip(m.renderStatusLine())
}

// checkOut presses key, which must start a switch — "c" itself, or enter on
// its list — and feeds back what the switch reports.
func checkOut(t *testing.T, m model, key tea.KeyMsg) model {
	t.Helper()
	res, cmd := m.Update(key)
	if cmd == nil {
		t.Fatalf("%q started no checkout; notice %q", key.String(), res.(model).notice)
	}
	res, _ = res.(model).Update(cmd())
	return res.(model)
}

var enterKey = tea.KeyMsg{Type: tea.KeyEnter}

// One branch leaves nothing to ask.
func TestCheckoutSwitchesWithoutAsking(t *testing.T) {
	dir, _ := branchedRepo(t)
	m := selectMessage(t, loadedModel(t, dir), "first")

	m = checkOut(t, m, keyPress("c"))
	if m.checkout.open {
		t.Error("a prompt is open though the commit has one branch")
	}
	if !strings.Contains(m.notice, "Switched to feature") {
		t.Errorf("notice = %q, want it to say where HEAD went", m.notice)
	}
	res, _ := m.Update(loadRepo(dir)())
	m = res.(model)
	if m.currentBranch != "feature" {
		t.Errorf("current branch = %q after the reload, want feature", m.currentBranch)
	}
	if c, _ := m.commitOnScreen(); c.Message != "first" {
		t.Errorf("selected %q after the reload, want the commit checked out", c.Message)
	}
}

// Several branches are a list over the graph, which only enter answers.
func TestCheckoutListsEachBranch(t *testing.T) {
	dir, git := branchedRepo(t)
	git("branch", "other", "feature")
	m := selectMessage(t, loadedModel(t, dir), "first")
	selected := m.selected

	asked, cmd := m.Update(keyPress("c"))
	m = asked.(model)
	if cmd != nil || !m.checkout.open || m.switching {
		t.Fatalf("open=%v switching=%v; want the list, and nothing switched yet", m.checkout.open, m.switching)
	}
	screen := ansi.Strip(m.View())
	for _, want := range []string{"Check out", "> feature", "  other", "enter: check out"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the list does not show %q:\n%s", want, screen)
		}
	}
	if got := strings.Count(screen, "\n") + 1; got != m.windowHeight {
		t.Errorf("the screen is %d lines with the list open, want %d", got, m.windowHeight)
	}
	// The question is in the list now, not on the bottom line.
	if got := statusLine(m); strings.Contains(got, "feature") {
		t.Errorf("status line = %q, want the branches left to the list", got)
	}
	if m.canReloadUnasked() {
		t.Error("an unasked reload could run under the open list")
	}

	// The keys move in the list, not in the graph behind it, and stop at
	// its ends.
	m = press(m, keyPress("j"), keyPress("j"))
	if m.checkout.cursor != 1 || m.selected != selected {
		t.Fatalf("cursor=%d selected=%d; want the second row, and the graph left alone", m.checkout.cursor, m.selected)
	}
	m = checkOut(t, m, enterKey)
	if m.checkout.open || !strings.Contains(m.notice, "Switched to other") {
		t.Errorf("open=%v notice=%q, want the second branch checked out", m.checkout.open, m.notice)
	}
}

// A key that is not the list's changes nothing, and esc, q and c close it.
func TestCheckoutListClosesWithoutSwitching(t *testing.T) {
	dir, git := branchedRepo(t)
	git("branch", "other", "feature")
	m := press(selectMessage(t, loadedModel(t, dir), "first"), keyPress("c"))

	for _, key := range []string{"1", "2", "x", "P", "d"} {
		got, cmd := m.Update(keyPress(key))
		if cmd != nil || !got.(model).checkout.open || got.(model).switching {
			t.Errorf("%q did something with the list open", key)
		}
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEsc}, keyPress("q"), keyPress("c")} {
		got, cmd := m.Update(key)
		if cmd != nil || got.(model).checkout.open || got.(model).switching {
			t.Errorf("%q did not just close the list", key.String())
		}
	}
}

// A remote branch with no local one says what picking it makes, and the
// branch already checked out is marked and not where the cursor starts.
func TestCheckoutListSaysWhatEachRowDoes(t *testing.T) {
	dir, _, git, _ := repoWithRemote(t)
	git("push", "-q", "origin", "main:release")
	git("fetch", "-q", "origin")
	git("branch", "other")
	m := loadedModel(t, dir)
	m = press(onCommit(t, m, dir, "HEAD"), keyPress("c"))
	if !m.checkout.open {
		t.Fatalf("notice=%q; want the list", m.notice)
	}

	screen := ansi.Strip(m.View())
	for _, want := range []string{"  main  checked out", "> other", "  origin/release  new local branch release"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the list does not show %q:\n%s", want, screen)
		}
	}
	if strings.Contains(ansi.Strip(m.checkout.render(20, 200)), "origin/main") {
		t.Errorf("origin/main is offered beside the local main it stands for:\n%s", screen)
	}

	m = checkOut(t, press(m, keyPress("G")), enterKey)
	if !strings.Contains(m.notice, "Switched to release, tracking origin/release") {
		t.Errorf("notice = %q, want the remote branch checked out through a local one", m.notice)
	}
}

// More branches than the window has rows for scroll, and names longer than
// it is wide are cut: the box never outgrows the window.
func TestCheckoutListFitsTheWindow(t *testing.T) {
	var targets []git.SwitchTarget
	for i := 0; i < 30; i++ {
		targets = append(targets, git.SwitchTarget{Kind: git.SwitchRemote,
			Name: fmt.Sprintf("origin/feature/a-long-branch-name-%02d", i)})
	}
	p := checkoutPrompt{open: true, targets: targets, cursor: 29}
	for _, window := range []int{20, 40, 60, 120} {
		box := ansi.Strip(p.render(5, window))
		lines := strings.Split(box, "\n")
		// Five rows, the title and footer with a blank line each, the
		// padding and the border.
		if len(lines) != 5+4+4 {
			t.Errorf("window %d: the box is %d lines:\n%s", window, len(lines), box)
		}
		for _, line := range lines {
			if w := ansi.StringWidth(line); w > max(window, 16) {
				t.Fatalf("window %d: a line is %d wide:\n%s", window, w, box)
			}
		}
		if !strings.Contains(box, "> origin/") {
			t.Errorf("window %d: the row the cursor is on is not shown:\n%s", window, box)
		}
	}
	if box := ansi.Strip(p.render(5, 120)); !strings.Contains(box, "> origin/feature/a-long-branch-name-29  new local branch feature/a-long-branch-name-29") {
		t.Errorf("a wide window does not show the last row whole:\n%s", box)
	}
}

func TestCheckoutOfACommitWithNoBranchSaysItIsDetached(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	commit("second")
	m := selectMessage(t, loadedModel(t, dir), "first")

	m = checkOut(t, m, keyPress("c"))
	if !strings.Contains(m.notice, "(detached)") {
		t.Errorf("notice = %q, want it to say HEAD is detached", m.notice)
	}
}

func TestCheckoutOfTheCurrentBranchSaysSo(t *testing.T) {
	dir, _ := branchedRepo(t)
	m := selectMessage(t, loadedModel(t, dir), "second")

	m = press(m, keyPress("c"))
	if m.checkout.open || !strings.Contains(m.notice, "Already on main") {
		t.Errorf("open=%v notice=%q, want no prompt and a note that main is checked out", m.checkout.open, m.notice)
	}
}

func TestCheckoutRefusedWithUncommittedChanges(t *testing.T) {
	dir, _ := branchedRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "first"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := selectMessage(t, loadedModel(t, dir), "first")

	m = checkOut(t, m, keyPress("c"))
	if !strings.Contains(m.notice, "uncommitted changes") {
		t.Errorf("notice = %q, want it to say the changes are in the way", m.notice)
	}
	if !m.ready {
		t.Error("the repository was read again though nothing moved")
	}
}

func TestCheckoutOfTheUncommittedChangesRow(t *testing.T) {
	dir, _ := branchedRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "first"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := loadedModel(t, dir)
	if !m.commits[0].WorkingTree {
		t.Fatal("no uncommitted changes row")
	}
	m.selected = 0

	m = press(m, keyPress("c"))
	if m.checkout.open || !strings.Contains(m.notice, "uncommitted changes") {
		t.Errorf("open=%v notice=%q, want no prompt for the uncommitted changes", m.checkout.open, m.notice)
	}
}

// A switch answered after the repository was switched is about another HEAD.
func TestLateCheckoutAnswerIsDropped(t *testing.T) {
	dir, _ := branchedRepo(t)
	m := selectMessage(t, loadedModel(t, dir), "first")
	next, cmd := m.Update(keyPress("c"))
	m = next.(model)
	msg := cmd().(switchFinishedMsg)
	msg.repoPath = "elsewhere"

	if got := res(m.Update(msg)); got.notice != "" || !got.switching {
		t.Errorf("notice=%q switching=%v, want the answer ignored", got.notice, got.switching)
	}
}

// Checkout moved from "C" to "c"; the capital is left unbound, not an alias.
func TestCapitalCDoesNotCheckOut(t *testing.T) {
	dir, _ := branchedRepo(t)
	m := selectMessage(t, loadedModel(t, dir), "first")
	got, cmd := m.Update(keyPress("C"))
	if cmd != nil || got.(model).checkout.open || got.(model).switching {
		t.Error("C still starts a checkout")
	}
}
