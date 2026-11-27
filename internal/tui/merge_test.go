package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/gittest"
)

// step sends one key, and the answer to the command it starts, if it starts
// one: the plan "m" asks for, or the merge enter starts.
func step(t *testing.T, m model, k tea.KeyMsg) (model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(k)
	m = next.(model)
	if cmd == nil {
		return m, nil
	}
	next, cmd = m.Update(cmd())
	return next.(model), cmd
}

// ffRepo has main checked out one commit behind feature, so merging feature
// is a fast-forward.
func ffRepo(t *testing.T) (dir string, run func(...string)) {
	t.Helper()
	dir, run, commit := gittest.Fixture(t)
	run("init", "-q", "-b", "main")
	gittest.Identify(run)
	commit("first")
	run("switch", "-q", "-c", "feature")
	commit("on feature")
	run("switch", "-q", "main")
	return dir, run
}

// m says the fast-forward, enter does it, and the reload selects where the
// branch went.
func TestMergeFastForwards(t *testing.T) {
	dir, _ := ffRepo(t)
	feature, _ := git.Run(dir, "rev-parse", "feature")
	m, _ := step(t, onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("m"))
	if !m.merge.open {
		t.Fatalf("notice=%q; want the box", m.notice)
	}
	screen := ansi.Strip(m.View())
	for _, want := range []string{"Merge", "feature  into  main", "fast-forward main to " + feature[:7], "enter: merge • tab: merge commit • esc: cancel"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the box does not say %q:\n%s", want, screen)
		}
	}

	m, reload := step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if head, _ := git.Run(dir, "rev-parse", "main"); head != feature {
		t.Fatalf("main is at %s, want feature's %s (notice %q)", head, feature, m.notice)
	}
	if want := "Fast-forwarded main to " + feature[:7]; m.notice != want {
		t.Errorf("notice = %q, want %q", m.notice, want)
	}
	if reload == nil || m.reselect != feature {
		t.Errorf("reload=%v reselect=%q; want the new HEAD selected", reload != nil, m.reselect)
	}
}

// Tab turns the fast-forward into a merge commit, says so, and offers the
// message; tab again turns it back.
func TestMergeTabMakesAMergeCommit(t *testing.T) {
	dir, _ := ffRepo(t)
	before, _ := git.Run(dir, "rev-parse", "main")
	m, _ := step(t, onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("m"))

	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	screen := ansi.Strip(m.View())
	for _, want := range []string{"as a merge commit (--no-ff)", "Merge branch 'feature'", "tab: fast-forward", "e: message"} {
		if !strings.Contains(screen, want) {
			t.Errorf("after tab the box does not say %q:\n%s", want, screen)
		}
	}
	if back := press(m, tea.KeyMsg{Type: tea.KeyTab}); !back.merge.fastForward() {
		t.Error("tab again did not go back to the fast-forward")
	}

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	head, _ := git.Run(dir, "rev-parse", "HEAD")
	if parents, _ := git.Run(dir, "rev-list", "--parents", "-n1", "HEAD"); len(strings.Fields(parents)) != 3 {
		t.Fatalf("HEAD %s is not a merge (notice %q)", parents, m.notice)
	}
	if first, _ := git.Run(dir, "rev-parse", "HEAD^1"); first != before {
		t.Errorf("HEAD^1 = %s, want the old main %s", first, before)
	}
	if want := "Merged feature into main as " + head[:7]; m.notice != want {
		t.Errorf("notice = %q, want %q", m.notice, want)
	}
}

// Diverged branches make a merge commit, with nothing to choose; "e" opens
// the message box with git's message in it, and enter there merges with
// what was written.
func TestMergeCommitWithEditedMessage(t *testing.T) {
	dir, _ := pickRepo(t)
	m, _ := step(t, onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("m"))
	if !m.merge.open {
		t.Fatalf("notice=%q; want the box", m.notice)
	}
	screen := ansi.Strip(m.View())
	if !strings.Contains(screen, "as a merge commit\n") && !strings.Contains(screen, "as a merge commit ") {
		t.Errorf("the box does not say it makes a merge commit:\n%s", screen)
	}
	if strings.Contains(screen, "tab:") {
		t.Errorf("tab is offered with nothing to choose:\n%s", screen)
	}

	m = press(m, keyPress("e"))
	if !m.merge.editing || m.merge.editor.subject.Value() != "Merge branch 'feature'" {
		t.Fatalf("editing=%v subject=%q", m.merge.editing, m.merge.editor.subject.Value())
	}
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "enter: merge") {
		t.Errorf("the message box does not say enter merges:\n%s", screen)
	}
	m.merge.editor.subject.SetValue("Bring the feature in")
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if subject, _ := git.Run(dir, "log", "-1", "--format=%s"); subject != "Bring the feature in" {
		t.Errorf("the merge commit's subject is %q (notice %q)", subject, m.notice)
	}
	if parents, _ := git.Run(dir, "rev-list", "--parents", "-n1", "HEAD"); len(strings.Fields(parents)) != 3 {
		t.Errorf("HEAD %s is not a merge", parents)
	}
}

// Esc, or any key but enter, y, tab and e, leaves the branch as it was.
func TestMergeCancelled(t *testing.T) {
	dir, _ := ffRepo(t)
	before, _ := git.Run(dir, "rev-parse", "main")
	for _, k := range []tea.KeyMsg{{Type: tea.KeyEsc}, keyPress("n")} {
		m, _ := step(t, onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("m"))
		next, cmd := m.Update(k)
		if cmd != nil || next.(model).merge.open {
			t.Errorf("%s: the box is still open or something ran", k)
		}
		if got := next.(model).notice; got != "Nothing merged" {
			t.Errorf("%s: notice = %q", k, got)
		}
	}
	if got, _ := git.Run(dir, "rev-parse", "main"); got != before {
		t.Error("main moved")
	}
}

// What can be told before the box opens is said instead of opening it.
func TestMergeRefusedBeforeAsking(t *testing.T) {
	dir, run := pickRepo(t)

	m, _ := step(t, onCommit(t, loadedModel(t, dir), dir, "main"), keyPress("m"))
	if m.merge.open || !strings.Contains(m.notice, "is the commit checked out") {
		t.Errorf("on HEAD: open=%v notice=%q", m.merge.open, m.notice)
	}

	m, _ = step(t, onCommit(t, loadedModel(t, dir), dir, "main~1"), keyPress("m"))
	if m.merge.open || !strings.Contains(m.notice, "is already on main") {
		t.Errorf("on an ancestor: open=%v notice=%q", m.merge.open, m.notice)
	}

	writeFile(t, dir+"/first", "changed")
	m, _ = step(t, onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("m"))
	if m.merge.open || m.notice != mergeDirtyNotice {
		t.Errorf("with changes: open=%v notice=%q", m.merge.open, m.notice)
	}
	run("checkout", "-q", "--", "first")

	run("switch", "-q", "--detach", "main")
	m, _ = step(t, onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("m"))
	if m.merge.open || m.notice != mergeDetachedNotice {
		t.Errorf("detached: open=%v notice=%q", m.merge.open, m.notice)
	}
}

// A conflict found by the plan is said, and the box never opens.
func TestMergeConflictSaid(t *testing.T) {
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

	m, _ := step(t, onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("m"))
	if want := "Not merged: feature would conflict with main — resolve that in a terminal"; m.merge.open || m.notice != want {
		t.Errorf("open=%v notice=%q, want %q", m.merge.open, m.notice, want)
	}
	if got, _ := git.Run(dir, "rev-parse", "main"); got != before {
		t.Error("main moved")
	}
}

// With merge.ff=only and no fast-forward, enter has nothing to do and is not
// offered; tab asks for a merge commit, which git's --no-ff makes anyway.
func TestMergeFFOnlyNeedsTab(t *testing.T) {
	dir, run := pickRepo(t)
	run("config", "merge.ff", "only")
	before, _ := git.Run(dir, "rev-parse", "main")
	m, _ := step(t, onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("m"))
	screen := ansi.Strip(m.View())
	if !strings.Contains(screen, "merge.ff is only") || strings.Contains(screen, "enter: merge") {
		t.Errorf("the box does not say enter does nothing:\n%s", screen)
	}
	after, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || !after.(model).merge.open {
		t.Fatal("enter did something")
	}
	next, _ := step(t, press(after.(model), tea.KeyMsg{Type: tea.KeyTab}), tea.KeyMsg{Type: tea.KeyEnter})
	if parent, _ := git.Run(dir, "rev-parse", "HEAD^1"); parent != before {
		t.Errorf("no merge commit on main (notice %q)", next.notice)
	}
}

// With several branches on the commit, the list "c" shows asks which, the
// branch checked out left off it.
func TestMergeAsksWhichBranch(t *testing.T) {
	dir, run := ffRepo(t)
	run("branch", "also", "feature")
	m, _ := step(t, onCommit(t, loadedModel(t, dir), dir, "feature"), keyPress("m"))
	if !m.checkout.open || !m.checkout.merge || len(m.checkout.targets) != 2 {
		t.Fatalf("list open=%v merge=%v targets=%v notice=%q", m.checkout.open, m.checkout.merge, m.checkout.targets, m.notice)
	}
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "Merge into main") || !strings.Contains(screen, "enter: merge") {
		t.Errorf("the list does not say it merges:\n%s", screen)
	}
	m = press(m, keyPress("j"))
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.merge.open || m.merge.name != "feature" || m.merge.ref != "refs/heads/feature" {
		t.Errorf("box open=%v name=%q ref=%q", m.merge.open, m.merge.name, m.merge.ref)
	}
}
