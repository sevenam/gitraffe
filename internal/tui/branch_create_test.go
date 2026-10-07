package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/gittest"
)

// typed presses each character of s as its own key.
func typed(m model, s string) model {
	for _, r := range s {
		m = press(m, keyPress(string(r)))
	}
	return m
}

// branchRepo has two commits on main and a branch on the older one.
func branchRepo(t *testing.T) (dir string, run func(...string)) {
	t.Helper()
	dir, run, commit := gittest.Fixture(t)
	run("init", "-q", "-b", "main")
	commit("first")
	run("branch", "taken")
	commit("second")
	return dir, run
}

func headBranch(t *testing.T, dir string) string {
	t.Helper()
	out, _ := git.Run(dir, "symbolic-ref", "-q", "--short", "HEAD")
	return out
}

func TestNewBranchIsMadeAtHeadAndSwitchedTo(t *testing.T) {
	dir, _ := branchRepo(t)
	m := loadedModel(t, dir)
	head, _ := git.Run(dir, "rev-parse", "HEAD")
	// The selection is somewhere else: the branch starts at HEAD regardless.
	m.selected = len(m.commits) - 1

	asked, cmd := m.Update(keyPress("b"))
	m = asked.(model)
	if cmd != nil || !m.branchPrompt.open || m.switching {
		t.Fatalf("open=%v switching=%v; want the box, and nothing made yet", m.branchPrompt.open, m.switching)
	}
	screen := ansi.Strip(m.View())
	for _, want := range []string{"New branch", "From  main  " + head[:7] + " second", "enter: create it and switch to it"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the box does not say %q:\n%s", want, screen)
		}
	}
	if got := strings.Count(screen, "\n") + 1; got != m.windowHeight {
		t.Errorf("the screen is %d lines with the box open, want %d", got, m.windowHeight)
	}
	if m.canReloadUnasked() {
		t.Error("an unasked reload could run under the open box")
	}

	// Keys that mean something on the graph are letters of the name here.
	m = typed(m, "feature/qPc")
	started, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = started.(model)
	if cmd == nil || m.branchPrompt.open || !m.switching {
		t.Fatalf("open=%v switching=%v notice=%q; want the branch being made", m.branchPrompt.open, m.switching, m.notice)
	}
	m = res(m.Update(cmd()))
	if m.switching || m.notice != "Created feature/qPc and switched to it" {
		t.Errorf("switching=%v notice=%q", m.switching, m.notice)
	}
	if got := headBranch(t, dir); got != "feature/qPc" {
		t.Errorf("on %q, want feature/qPc", got)
	}
	if got, _ := git.Run(dir, "rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD = %s, want it left at %s", got, head)
	}
}

// Uncommitted changes stop a checkout; they have no reason to stop this.
func TestNewBranchWithUncommittedChanges(t *testing.T) {
	dir, _ := branchRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "first"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := typed(press(loadedModel(t, dir), keyPress("b")), "wip")
	started, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("notice=%q; want the branch being made", started.(model).notice)
	}
	m = res(started.(model).Update(cmd()))
	if got := headBranch(t, dir); got != "wip" {
		t.Errorf("on %q, want wip (notice %q)", got, m.notice)
	}
	if body, _ := os.ReadFile(filepath.Join(dir, "first")); string(body) != "edited" {
		t.Errorf("the edited file now reads %q", body)
	}
}

// A name that will not do is said in the box, which stays open for another.
func TestNewBranchBoxRefusesANameAndStaysOpen(t *testing.T) {
	dir, _ := branchRepo(t)
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	for name, want := range map[string]string{
		"":         "Type a name",
		"a..b":     "Git does not accept",
		"two word": "Git does not accept",
		"taken":    "already a branch called taken",
		"main":     "already a branch called main",
	} {
		m := typed(press(loadedModel(t, dir), keyPress("b")), name)
		got, cmd := m.Update(enter)
		m = got.(model)
		if cmd != nil || !m.branchPrompt.open || m.switching {
			t.Errorf("%q: open=%v switching=%v; want the box still open", name, m.branchPrompt.open, m.switching)
		}
		if !strings.Contains(m.branchPrompt.failure, want) || !strings.Contains(ansi.Strip(m.View()), want) {
			t.Errorf("%q: failure = %q, want %q on screen", name, m.branchPrompt.failure, want)
		}
		// Typing again takes the complaint away.
		if m = press(m, keyPress("x")); m.branchPrompt.failure != "" {
			t.Errorf("%q: the complaint outlived the name it was about", name)
		}
	}
	if got := headBranch(t, dir); got != "main" {
		t.Errorf("on %q, want still main", got)
	}
}

func TestNewBranchBoxCancels(t *testing.T) {
	dir, _ := branchRepo(t)
	m := typed(press(loadedModel(t, dir), keyPress("b")), "feature")
	got, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || got.(model).branchPrompt.open || got.(model).switching {
		t.Error("esc did not close the box and leave it at that")
	}
	if git.BranchExists(dir, "feature") {
		t.Error("a cancelled box made the branch")
	}
}

// On a detached HEAD the box says so, and the branch is the way back.
func TestNewBranchFromADetachedHead(t *testing.T) {
	dir, run := branchRepo(t)
	run("switch", "-q", "--detach", "taken")
	m := press(loadedModel(t, dir), keyPress("b"))
	at, _ := git.Run(dir, "rev-parse", "--short=7", "HEAD")
	// With no branch to name, the commit is all that says where it starts.
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "From  "+m.currentBranch+"  "+at+" first") || !strings.Contains(m.currentBranch, "HEAD") {
		t.Errorf("the box does not say which commit the detached HEAD is at (%q, %s):\n%s", m.currentBranch, at, screen)
	}
	started, cmd := typed(m, "rescued").Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("notice=%q; want the branch being made", started.(model).notice)
	}
	res(started.(model).Update(cmd()))
	if got := headBranch(t, dir); got != "rescued" {
		t.Errorf("on %q, want rescued", got)
	}
	want, _ := git.Run(dir, "rev-parse", "taken")
	if got, _ := git.Run(dir, "rev-parse", "HEAD"); got != want {
		t.Errorf("HEAD = %s, want %s", got, want)
	}
}

func TestNewBranchWaitsItsTurn(t *testing.T) {
	dir, _ := branchRepo(t)
	m := loadedModel(t, dir)
	m.pulling = true
	got, cmd := m.Update(keyPress("b"))
	if cmd != nil || got.(model).branchPrompt.open || !strings.Contains(got.(model).notice, "A pull is running") {
		t.Errorf("open=%v notice=%q; want b to wait for the pull", got.(model).branchPrompt.open, got.(model).notice)
	}

	// Something that started while the box was open is waited for too.
	m.pulling = false
	m = typed(press(m, keyPress("b")), "feature")
	m.committing = true
	got, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || got.(model).switching || !strings.Contains(got.(model).notice, "A commit is running") {
		t.Errorf("notice=%q; want enter to wait for the commit", got.(model).notice)
	}
	if git.BranchExists(dir, "feature") {
		t.Error("the branch was made beside a running commit")
	}
}

func TestNewBranchResultForAnotherRepositoryIsDropped(t *testing.T) {
	dir, _ := branchRepo(t)
	m := loadedModel(t, dir)
	m.switching = true
	got, cmd := m.Update(branchCreatedMsg{repoPath: "elsewhere", name: "feature"})
	if cmd != nil || got.(model).notice != "" || !got.(model).switching {
		t.Error("an answer for another repository was taken for this one's")
	}
}

func TestNewBranchFailureSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		msg  branchCreatedMsg
		want string
	}{
		{branchCreatedMsg{name: "x", err: git.ErrBranchExists}, "No branch made: there is already a branch called x"},
		{branchCreatedMsg{name: "x", err: os.ErrInvalid, detail: "fatal: cannot lock ref\nhint"}, "No branch made: fatal: cannot lock ref"},
	} {
		if got := branchFailedNotice(tc.msg); got != tc.want {
			t.Errorf("notice = %q, want %q", got, tc.want)
		}
	}

	// A merge started in a terminal after the repository was read.
	dir, run := branchRepo(t)
	run("switch", "-q", "-c", "other", "taken")
	if err := os.WriteFile(filepath.Join(dir, "on-other"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "on other")
	run("switch", "-q", "main")
	m := loadedModel(t, dir)
	run("merge", "-q", "--no-commit", "--no-ff", "other")
	started, cmd := typed(press(m, keyPress("b")), "feature").Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("nothing started")
	}
	m = res(started.(model).Update(cmd()))
	if m.switching || !strings.Contains(m.notice, "A merge is in progress") {
		t.Errorf("switching=%v notice=%q; want the merge named", m.switching, m.notice)
	}
}

// The branch's name is kept whole and the commit takes what room is left, so
// a long name or a long subject never widens the box or wraps a line.
func TestNewBranchBoxKeepsItsShape(t *testing.T) {
	short := branchPrompt{from: "main", commit: "4a938a6", subject: "x"}
	want := strings.Count(short.render(60), "\n")
	for _, p := range []branchPrompt{
		{from: "main", commit: "4a938a6", subject: strings.Repeat("a long subject ", 20)},
		{from: strings.Repeat("feature/", 12), commit: "4a938a6", subject: "fix"},
		{from: strings.Repeat("f", 39), commit: "4a938a6", subject: "fix"},
		{from: "main"},
	} {
		box := ansi.Strip(p.render(60))
		if got := strings.Count(box, "\n"); got != want {
			t.Errorf("from %q: the box is %d lines, want %d:\n%s", p.from, got+1, want+1, box)
		}
		if p.from == "main" && p.commit != "" && !strings.Contains(box, "From  main  4a938a6 a long subject") {
			t.Errorf("the commit is missing beside a short name:\n%s", box)
		}
	}
}
