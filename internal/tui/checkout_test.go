package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

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

// checkOut presses key, which must start a switch — "C" itself, or the answer
// to its prompt — and feeds back what the switch reports.
func checkOut(t *testing.T, m model, key string) model {
	t.Helper()
	res, cmd := m.Update(keyPress(key))
	if cmd == nil {
		t.Fatalf("%q started no checkout; notice %q", key, res.(model).notice)
	}
	res, _ = res.(model).Update(cmd())
	return res.(model)
}

// One branch leaves nothing to ask.
func TestCheckoutSwitchesWithoutAsking(t *testing.T) {
	dir, _ := branchedRepo(t)
	m := selectMessage(t, loadedModel(t, dir), "first")

	m = checkOut(t, m, "C")
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

// Anything but a number on the list is no, so a stray key never switches.
func TestCheckoutCancelledByAnyOtherKey(t *testing.T) {
	dir, git := branchedRepo(t)
	git("branch", "other", "feature")
	m := selectMessage(t, loadedModel(t, dir), "first")

	m = press(m, keyPress("C"))
	res, cmd := m.Update(keyPress("j"))
	m = res.(model)
	if cmd != nil || m.checkout.open {
		t.Fatal("j did not just close the prompt")
	}
	if m.selected != 1 {
		t.Error("the key that cancelled the prompt also moved the selection")
	}
}

func TestCheckoutOffersEachBranch(t *testing.T) {
	dir, git := branchedRepo(t)
	git("branch", "other", "feature")
	m := selectMessage(t, loadedModel(t, dir), "first")

	m = press(m, keyPress("C"))
	if got := statusLine(m); !strings.Contains(got, "1 feature • 2 other • esc cancel") {
		t.Fatalf("status line = %q, want both branches numbered", got)
	}
	m = checkOut(t, m, "2")
	if !strings.Contains(m.notice, "Switched to other") {
		t.Errorf("notice = %q, want the second branch checked out", m.notice)
	}
}

func TestCheckoutOfACommitWithNoBranchSaysItIsDetached(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	commit("second")
	m := selectMessage(t, loadedModel(t, dir), "first")

	m = checkOut(t, m, "C")
	if !strings.Contains(m.notice, "(detached)") {
		t.Errorf("notice = %q, want it to say HEAD is detached", m.notice)
	}
}

func TestCheckoutOfTheCurrentBranchSaysSo(t *testing.T) {
	dir, _ := branchedRepo(t)
	m := selectMessage(t, loadedModel(t, dir), "second")

	m = press(m, keyPress("C"))
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

	m = checkOut(t, m, "C")
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

	m = press(m, keyPress("C"))
	if m.checkout.open || !strings.Contains(m.notice, "uncommitted changes") {
		t.Errorf("open=%v notice=%q, want no prompt for the uncommitted changes", m.checkout.open, m.notice)
	}
}

// A switch answered after the repository was switched is about another HEAD.
func TestLateCheckoutAnswerIsDropped(t *testing.T) {
	dir, _ := branchedRepo(t)
	m := selectMessage(t, loadedModel(t, dir), "first")
	next, cmd := m.Update(keyPress("C"))
	m = next.(model)
	msg := cmd().(switchFinishedMsg)
	msg.repoPath = "elsewhere"

	if got := res(m.Update(msg)); got.notice != "" || !got.switching {
		t.Errorf("notice=%q switching=%v, want the answer ignored", got.notice, got.switching)
	}
}
