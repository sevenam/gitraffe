package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
)

// deleteRows is what the list opened with "d" offers, top to bottom.
func deleteRows(m model) []string {
	var rows []string
	for _, c := range m.deletePicker.choices {
		rows = append(rows, c.label())
	}
	return rows
}

// confirmDelete presses enter on the open list and feeds back what the
// delete reports.
func confirmDelete(t *testing.T, m model) model {
	t.Helper()
	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	deleting := res.(model)
	if !deleting.deleting || cmd == nil {
		t.Fatalf("deleting=%v cmd=%v; want enter to start the delete", deleting.deleting, cmd != nil)
	}
	if !strings.Contains(statusLine(deleting), "Deleting") {
		t.Error("the status line does not say a delete is running")
	}
	res, _ = deleting.Update(cmd())
	return res.(model)
}

func TestDeleteListsLocalRemoteAndBoth(t *testing.T) {
	dir, _, gitCmd, commit := repoWithRemote(t)
	gitCmd("branch", "feature")
	gitCmd("push", "-q", "origin", "feature")
	commit("second")
	gitCmd("push", "-q", "origin", "main")
	m := selectMessage(t, loadedModel(t, dir), "first")

	m = press(m, keyPress("d"))
	want := []string{"local feature", "remote origin/feature", "local feature and remote origin/feature"}
	if got := deleteRows(m); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Delete branch") || !strings.Contains(view, "> local feature") {
		t.Error("the list is not on screen with its first row selected")
	}

	m = confirmDelete(t, press(m, keyPress("j"), keyPress("j")))
	if !strings.Contains(m.notice, "Deleted local feature and remote origin/feature") {
		t.Fatalf("notice = %q, want it to say both went", m.notice)
	}
	if m.ready {
		t.Fatal("the repository was not read again after the delete")
	}
	res, _ := m.Update(loadRepo(dir)())
	m = res.(model)
	if c, _ := m.commitOnScreen(); c.Message != "first" || len(git.DeleteTargets(c.Refs)) != 0 {
		t.Errorf("selected %q with refs %q, want the same commit with no branch left on it", c.Message, c.Refs)
	}
}

// Nothing goes until a row is picked: d, and every way out of the list,
// delete nothing.
func TestDeleteListClosesWithoutDeleting(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEsc}, keyPress("q"), keyPress("d")} {
		dir, _ := branchedRepo(t)
		m := press(selectMessage(t, loadedModel(t, dir), "first"), keyPress("d"))
		if !m.deletePicker.open {
			t.Fatal("d did not open the list")
		}
		res, cmd := m.Update(key)
		if m = res.(model); m.deletePicker.open || m.deleting || cmd != nil {
			t.Errorf("%q: open=%v deleting=%v cmd=%v, want the list closed and nothing started",
				key.String(), m.deletePicker.open, m.deleting, cmd != nil)
		}
		if _, err := git.Run(dir, "rev-parse", "--verify", "--quiet", "refs/heads/feature"); err != nil {
			t.Errorf("%q deleted feature", key.String())
		}
	}
}

// The branch checked out cannot go locally, so only its remote row is left.
func TestDeleteOffersOnlyTheRemoteOfTheCurrentBranch(t *testing.T) {
	dir, _, _, _ := repoWithRemote(t)
	m := press(loadedModel(t, dir), keyPress("d"))
	if got := deleteRows(m); len(got) != 1 || got[0] != "remote origin/main" {
		t.Fatalf("rows = %q, want only the remote branch", got)
	}
}

func TestDeleteWithNothingToDeleteSaysWhy(t *testing.T) {
	dir, gitCmd := branchedRepo(t)
	gitCmd("branch", "-D", "feature")

	m := press(selectMessage(t, loadedModel(t, dir), "second"), keyPress("d"))
	if m.deletePicker.open || !strings.Contains(m.notice, "main is checked out") {
		t.Errorf("open=%v notice=%q, want a note that main is checked out", m.deletePicker.open, m.notice)
	}
	m = press(selectMessage(t, m, "first"), keyPress("d"))
	if m.deletePicker.open || !strings.Contains(m.notice, "No branch on this commit") {
		t.Errorf("open=%v notice=%q, want a note that there is no branch", m.deletePicker.open, m.notice)
	}
}

func TestDeleteRefusedWhenTheCommitsAreOnNoOtherBranch(t *testing.T) {
	dir, gitCmd := branchedRepo(t)
	gitCmd("switch", "-q", "feature")
	m := selectMessage(t, loadedModel(t, dir), "second")

	m = confirmDelete(t, press(m, keyPress("d")))
	if !strings.Contains(m.notice, "Not deleted: its commits are on no other branch") {
		t.Errorf("notice = %q, want it to say why main was kept", m.notice)
	}
	if !m.ready {
		t.Error("the repository was read again though nothing was deleted")
	}
	if _, err := git.Run(dir, "rev-parse", "--verify", "--quiet", "refs/heads/main"); err != nil {
		t.Error("main was deleted")
	}
}

// A delete answered after the repository was switched is about other branches.
func TestLateDeleteAnswerIsDropped(t *testing.T) {
	dir, _ := branchedRepo(t)
	m := press(selectMessage(t, loadedModel(t, dir), "first"), keyPress("d"))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	msg := cmd().(deleteFinishedMsg)
	msg.repoPath = "elsewhere"

	if got := res(m.Update(msg)); got.notice != "" || !got.deleting {
		t.Errorf("notice=%q deleting=%v, want the answer ignored", got.notice, got.deleting)
	}
}
