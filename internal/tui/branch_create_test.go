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

// onCommit puts the selection on the commit rev names.
func onCommit(t *testing.T, m model, dir, rev string) model {
	t.Helper()
	full, _ := git.Run(dir, "rev-parse", rev)
	for i, c := range m.commits {
		if c.FullHash == full && !c.WorkingTree {
			m.selected = i
			return m
		}
	}
	t.Fatalf("%s (%s) is not in the graph", rev, full)
	return m
}

// The branch starts at the selected commit, which the box names, and HEAD
// and the files go there with it.
func TestNewBranchIsMadeAtTheSelectedCommit(t *testing.T) {
	dir, _ := branchRepo(t)
	first, _ := git.Run(dir, "rev-parse", "taken")
	main, _ := git.Run(dir, "rev-parse", "main")
	m := onCommit(t, loadedModel(t, dir), dir, "taken")

	m = press(m, keyPress("b"))
	if !m.branchPrompt.open {
		t.Fatalf("notice=%q; want the box", m.notice)
	}
	screen := ansi.Strip(m.View())
	for _, want := range []string{"starts at the selected commit", "From  " + first[:7] + " first"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the box does not say %q:\n%s", want, screen)
		}
	}

	started, cmd := typed(m, "from-first").Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("notice=%q; want the branch being made", started.(model).notice)
	}
	m = res(started.(model).Update(cmd()))
	if m.notice != "Created from-first and switched to it" {
		t.Errorf("notice = %q", m.notice)
	}
	if got := headBranch(t, dir); got != "from-first" {
		t.Errorf("on %q, want from-first", got)
	}
	if got, _ := git.Run(dir, "rev-parse", "HEAD"); got != first {
		t.Errorf("HEAD = %s, want the selected commit %s", got, first)
	}
	if got, _ := git.Run(dir, "rev-parse", "main"); got != main {
		t.Errorf("main = %s, want it left at %s", got, main)
	}
	if _, err := os.Stat(filepath.Join(dir, "second")); !os.IsNotExist(err) {
		t.Error("the later commit's file is still in the working tree")
	}
}

// Anywhere but where you stand it is a checkout as well, and uncommitted
// changes turn it down as they do "c": before the box opens, and again by
// git if they appear while it is open.
func TestNewBranchElsewhereRefusesUncommittedChanges(t *testing.T) {
	dir, _ := branchRepo(t)
	edit := func() {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "second"), []byte("edited"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	clean := onCommit(t, loadedModel(t, dir), dir, "taken")

	edit()
	m := onCommit(t, loadedModel(t, dir), dir, "taken")
	got, cmd := m.Update(keyPress("b"))
	if cmd != nil || got.(model).branchPrompt.open || got.(model).notice != branchElsewhereNotice {
		t.Errorf("open=%v notice=%q; want the box kept shut over the changes", got.(model).branchPrompt.open, got.(model).notice)
	}

	// The same changes, on the commit they were made on, come along.
	m = press(onCommit(t, m, dir, "main"), keyPress("b"))
	if !m.branchPrompt.open || m.branchPrompt.start != "" {
		t.Errorf("open=%v start=%q; want a branch from where you are", m.branchPrompt.open, m.branchPrompt.start)
	}

	// The box opened on a clean tree, and the edit came after.
	if err := os.WriteFile(filepath.Join(dir, "second"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = typed(press(clean, keyPress("b")), "late")
	edit()
	started, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("notice=%q; want git asked", started.(model).notice)
	}
	m = res(started.(model).Update(cmd()))
	if m.switching || m.notice != branchElsewhereNotice {
		t.Errorf("switching=%v notice=%q; want the changes named", m.switching, m.notice)
	}
	if git.BranchExists(dir, "late") || headBranch(t, dir) != "main" {
		t.Error("a refused branch was made, or HEAD moved")
	}
	if body, _ := os.ReadFile(filepath.Join(dir, "second")); string(body) != "edited" {
		t.Errorf("the edited file now reads %q", body)
	}
}

func TestNewBranchWhereYouAre(t *testing.T) {
	dir, _ := branchRepo(t)
	head, _ := git.Run(dir, "rev-parse", "HEAD")
	m := onCommit(t, loadedModel(t, dir), dir, "HEAD")

	asked, cmd := m.Update(keyPress("b"))
	m = asked.(model)
	if cmd != nil || !m.branchPrompt.open || m.switching {
		t.Fatalf("open=%v switching=%v; want the box, and nothing made yet", m.branchPrompt.open, m.switching)
	}
	screen := ansi.Strip(m.View())
	for _, want := range []string{"New branch", "starts where you are", "From  main  " + head[:7] + " second", "enter: create it and switch to it"} {
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
	m := press(onCommit(t, loadedModel(t, dir), dir, "HEAD"), keyPress("b"))
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
	m := onCommit(t, loadedModel(t, dir), dir, "HEAD")
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
		{from: "main", failure: strings.Repeat("no ", 60)},
	} {
		box := ansi.Strip(p.render(60))
		if p.failure != "" {
			want += 2
		}
		if got := strings.Count(box, "\n"); got != want {
			t.Errorf("from %q: the box is %d lines, want %d:\n%s", p.from, got+1, want+1, box)
		}
		if p.from == "main" && p.commit != "" && !strings.Contains(box, "From  main  4a938a6 a long subject") {
			t.Errorf("the commit is missing beside a short name:\n%s", box)
		}
	}
}

// The box grows to show the commit's subject whole where the window has the
// room, and in a narrow window gives up the subject before its own shape.
func TestNewBranchBoxFollowsTheWindow(t *testing.T) {
	subject := "create a branch with b; the branch finder moves to B (#171)"
	p := branchPrompt{from: "new-branch-171", commit: "8d94fb3", subject: subject}
	// A branch from another commit has no name to lead with, and fits too.
	elsewhere := branchPrompt{start: "8d94fb3", commit: "8d94fb3", subject: subject}
	widest := func(box string) int {
		w := 0
		for _, line := range strings.Split(box, "\n") {
			w = max(w, ansi.StringWidth(line))
		}
		return w
	}

	rows := strings.Count(p.render(200), "\n")
	for window := 36; window <= 240; window++ {
		box := ansi.Strip(p.render(window))
		if got := widest(box); got > window {
			t.Fatalf("window %d: the box is %d wide:\n%s", window, got, box)
		}
		if got := strings.Count(box, "\n"); got != rows {
			t.Fatalf("window %d: the box is %d lines, want %d:\n%s", window, got+1, rows+1, box)
		}
		if !strings.Contains(box, "From  new-branch-171") {
			t.Fatalf("window %d: the branch is not named:\n%s", window, box)
		}
		box = ansi.Strip(elsewhere.render(window))
		if widest(box) > window || strings.Count(box, "\n") != rows || !strings.Contains(box, "From  8d94fb3") {
			t.Fatalf("window %d: a branch from another commit draws as:\n%s", window, box)
		}
	}

	if box := ansi.Strip(p.render(120)); !strings.Contains(box, "8d94fb3 "+subject) {
		t.Errorf("a wide window does not show the subject whole:\n%s", box)
	}
	if box := ansi.Strip(p.render(70)); !strings.Contains(box, "8d94fb3 create a branch") || strings.Contains(box, subject) {
		t.Errorf("a narrow window should show the subject cut short:\n%s", box)
	}

	// No wider than its content asks for, nor than is readable.
	short := branchPrompt{from: "main", commit: "8d94fb3", subject: "fix"}
	if a, b := widest(short.render(100)), widest(short.render(240)); a != b {
		t.Errorf("a short line's box is %d wide in one window and %d in a wider one", a, b)
	}
	long := branchPrompt{from: "main", commit: "8d94fb3", subject: strings.Repeat("word ", 80)}
	if got := widest(long.render(240)); got > branchBoxMaxWidth+branchBoxChrome {
		t.Errorf("the box is %d wide, past its limit", got)
	}

	// A name longer than the line above it widens the box as it is typed.
	dir, _ := branchRepo(t)
	m := press(loadedModel(t, dir), keyPress("b"))
	empty := widest(m.branchPrompt.render(200))
	m = typed(m, strings.Repeat("n", 70))
	box := ansi.Strip(m.branchPrompt.render(200))
	if widest(box) <= empty || !strings.Contains(box, strings.Repeat("n", 70)) {
		t.Errorf("the box did not widen for a long name (%d, was %d):\n%s", widest(box), empty, box)
	}
}
