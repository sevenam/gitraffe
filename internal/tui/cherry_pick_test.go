package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/gittest"
)

// pickRepo has main checked out and a feature branch off its first commit
// with a commit of its own; each commit adds a file of its own, so the
// feature's commit applies to main cleanly.
func pickRepo(t *testing.T) (dir string, run func(...string)) {
	t.Helper()
	dir, run, commit := gittest.Fixture(t)
	run("init", "-q", "-b", "main")
	gittest.Identify(run)
	commit("first")
	run("switch", "-q", "-c", "feature")
	commit("on feature")
	run("switch", "-q", "main")
	commit("on main")
	return dir, run
}

// C asks, naming the commit and the branch; enter makes the new commit on
// the branch checked out, and the reload is to select it.
func TestCherryPickAsksThenPicks(t *testing.T) {
	dir, _ := pickRepo(t)
	before, _ := git.Run(dir, "rev-parse", "main")
	feature, _ := git.Run(dir, "rev-parse", "feature")
	m := onCommit(t, loadedModel(t, dir), dir, "feature")

	m = press(m, keyPress("C"))
	if !m.cherryPick.open {
		t.Fatalf("notice=%q; want the question", m.notice)
	}
	screen := ansi.Strip(m.View())
	for _, want := range []string{"Cherry-pick", feature[:7] + "  on feature", "onto main, as a new commit", "enter: cherry-pick"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the box does not say %q:\n%s", want, screen)
		}
	}
	if got, _ := git.Run(dir, "rev-parse", "main"); got != before {
		t.Fatal("main moved before the question was answered")
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("notice=%q; want the cherry-pick running", next.(model).notice)
	}
	next, reload := next.(model).Update(cmd())
	m = next.(model)
	head, _ := git.Run(dir, "rev-parse", "HEAD")
	if parent, _ := git.Run(dir, "rev-parse", "HEAD^"); parent != before {
		t.Errorf("HEAD^ = %s, want the old main %s", parent, before)
	}
	if subject, _ := git.Run(dir, "log", "-1", "--format=%s"); subject != "on feature" {
		t.Errorf("the new commit is %q", subject)
	}
	if want := "Cherry-picked " + feature[:7] + " onto main as " + head[:7]; m.notice != want {
		t.Errorf("notice = %q, want %q", m.notice, want)
	}
	if reload == nil {
		t.Fatal("no reload after the cherry-pick")
	}
	if m.reselect != head {
		t.Errorf("reselect = %q, want the new commit %s", m.reselect, head)
	}
}

// Esc, or any key but enter and y, leaves the branch as it was.
func TestCherryPickCancelled(t *testing.T) {
	dir, _ := pickRepo(t)
	before, _ := git.Run(dir, "rev-parse", "main")
	for _, k := range []tea.KeyMsg{{Type: tea.KeyEsc}, keyPress("n")} {
		m := press(onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("C"))
		next, cmd := m.Update(k)
		if cmd != nil || next.(model).cherryPick.open {
			t.Errorf("%s: the question is still open or something ran", k)
		}
		if got := next.(model).notice; got != "Nothing cherry-picked" {
			t.Errorf("%s: notice = %q", k, got)
		}
	}
	if got, _ := git.Run(dir, "rev-parse", "main"); got != before {
		t.Error("main moved")
	}
}

// What can be told before asking is said instead of the question.
func TestCherryPickRefusedBeforeAsking(t *testing.T) {
	dir, run := pickRepo(t)
	m := loadedModel(t, dir)

	m = press(onCommit(t, m, dir, "main"), keyPress("C"))
	if m.cherryPick.open || !strings.Contains(m.notice, "is the commit checked out") {
		t.Errorf("on HEAD: open=%v notice=%q", m.cherryPick.open, m.notice)
	}

	writeFile(t, dir+"/first", "changed")
	m = press(onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("C"))
	if m.cherryPick.open || m.notice != cherryPickDirtyNotice {
		t.Errorf("with changes: open=%v notice=%q", m.cherryPick.open, m.notice)
	}
	run("checkout", "-q", "--", "first")

	run("merge", "-q", "--no-edit", "--no-ff", "feature")
	m = press(onCommit(t, loadedModel(t, dir), dir, "main"), keyPress("C"))
	if m.cherryPick.open || !strings.Contains(m.notice, "is a merge") {
		t.Errorf("on a merge: open=%v notice=%q", m.cherryPick.open, m.notice)
	}
}

// A conflict found by the trial is said, with nothing done.
func TestCherryPickConflictSaid(t *testing.T) {
	dir, run := pickRepo(t)
	run("switch", "-q", "feature")
	writeFile(t, dir+"/clash", "feature's")
	run("add", "clash")
	run("commit", "-qm", "clash on feature")
	run("switch", "-q", "main")
	writeFile(t, dir+"/clash", "main's")
	run("add", "clash")
	run("commit", "-qm", "clash on main")
	before, _ := git.Run(dir, "rev-parse", "main")

	m := press(onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("C"))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("notice=%q; want the cherry-pick tried", next.(model).notice)
	}
	m = res(next.(model).Update(cmd()))
	if !strings.Contains(m.notice, "would conflict with main") {
		t.Errorf("notice = %q", m.notice)
	}
	if got, _ := git.Run(dir, "rev-parse", "main"); got != before {
		t.Error("main moved")
	}
}
