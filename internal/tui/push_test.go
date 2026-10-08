package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/gittest"
)

// pushed presses a key that should start a push, and feeds back what the
// push reports.
func pushed(t *testing.T, m model, key tea.KeyMsg) model {
	t.Helper()
	started, cmd := m.Update(key)
	pushing := started.(model)
	if !pushing.pushing || cmd == nil {
		t.Fatalf("pushing=%v cmd=%v notice=%q; want a push to have started", pushing.pushing, cmd != nil, pushing.notice)
	}
	// The push outlives the keypress, so the status line has to say so.
	if !strings.Contains(ansi.Strip(pushing.renderStatusLine()), "Pushing") {
		t.Error("the status line does not say a push is running")
	}
	return res(pushing.Update(cmd()))
}

// onRemote is what origin holds under ref, or "" when it has no such ref.
func onRemote(t *testing.T, dir, ref string) string {
	t.Helper()
	out, err := git.Run(dir, "ls-remote", "origin", ref)
	if err != nil {
		t.Fatal(err)
	}
	hash, _, _ := strings.Cut(out, "\t")
	return hash
}

// withRemoteTags loads the model and has the remotes say which tags they
// hold, as the program does once it has started.
func withRemoteTags(t *testing.T, dir string) model {
	t.Helper()
	return res(loadedModel(t, dir).Update(loadRemoteTagsCmd(dir)()))
}

func TestPushSendsTheBranch(t *testing.T) {
	dir, _, _, commit := repoWithRemote(t)
	commit("second")
	commit("third")
	m := loadedModel(t, dir)
	if m.ahead != 2 {
		t.Fatalf("ahead = %d, want the two commits not yet pushed", m.ahead)
	}

	m = pushed(t, m, keyPress("P"))
	if m.pushing || !strings.Contains(m.notice, "Pushed 2 commits to origin/main") {
		t.Fatalf("pushing=%v notice=%q; want it to say what was pushed", m.pushing, m.notice)
	}
	// It reloads, like a pull: the counts are only right once it has.
	if m.ready {
		t.Fatal("the repository was not read again after the push")
	}
	m = res(m.Update(loadRepo(dir)()))
	if m.ahead != 0 || m.behind != 0 {
		t.Errorf("ahead/behind %d/%d after the push, want the branch in sync", m.ahead, m.behind)
	}
	if !strings.Contains(m.notice, "Pushed") {
		t.Errorf("notice = %q after the reload, want what was pushed still said", m.notice)
	}
}

func TestPushWithNothingNewSaysSo(t *testing.T) {
	dir, _, _, _ := repoWithRemote(t)
	m := pushed(t, loadedModel(t, dir), keyPress("P"))
	if !strings.Contains(m.notice, "Nothing to push — origin/main already has every commit of main") {
		t.Errorf("notice = %q, want it to say there was nothing to push", m.notice)
	}
}

// The remote's commits are never pushed over. The line says why nothing
// happened and what to do about it.
func TestPushBehindTheRemoteExplainsItself(t *testing.T) {
	dir, origin, _, commit := repoWithRemote(t)
	pushFromElsewhere(t, origin)
	theirs := onRemote(t, dir, "refs/heads/main")
	commit("mine")

	m := pushed(t, loadedModel(t, dir), keyPress("P"))
	if !strings.Contains(m.notice, "Not pushed: origin/main has commits main lacks") || !strings.Contains(m.notice, "pull first") {
		t.Errorf("notice = %q, want it to say the remote is ahead and how to go on", m.notice)
	}
	if got := onRemote(t, dir, "refs/heads/main"); got != theirs {
		t.Error("the remote branch was moved")
	}
	if !m.ready {
		t.Error("the repository was read again though nothing changed")
	}
}

func TestPushNeedsARemote(t *testing.T) {
	m := loadedModel(t, gittest.NewRepo(t))
	got, cmd := m.Update(keyPress("P"))
	if cmd != nil || got.(model).pushing || got.(model).push.open() {
		t.Error("P started a push in a repository with no remote")
	}
	if !strings.Contains(got.(model).notice, "no remote") {
		t.Errorf("notice = %q, want it to say there is no remote", got.(model).notice)
	}
}

func TestPushOnADetachedHeadSaysSo(t *testing.T) {
	dir, _, git, _ := repoWithRemote(t)
	git("switch", "-q", "--detach")
	got, cmd := loadedModel(t, dir).Update(keyPress("P"))
	if cmd != nil || !strings.Contains(got.(model).notice, "HEAD is not on a branch") {
		t.Errorf("cmd=%v notice=%q; want a detached HEAD to have nothing to push", cmd != nil, got.(model).notice)
	}
}

// A branch that tracks nothing is not pushed until a box has asked what to
// call it on the remote, with its own name there to accept.
func TestPushOfANewBranchAsksForItsName(t *testing.T) {
	dir, _, git, commit := repoWithRemote(t)
	git("switch", "-q", "-c", "feature")
	commit("on feature")
	m := loadedModel(t, dir)

	asked, cmd := m.Update(keyPress("P"))
	m = asked.(model)
	if cmd != nil || m.pushing || !m.push.naming {
		t.Fatalf("pushing=%v naming=%v; want the box, and nothing pushed yet", m.pushing, m.push.naming)
	}
	if got := m.push.name.Value(); got != "feature" {
		t.Errorf("the box offers %q, want the branch's own name", got)
	}
	screen := ansi.Strip(m.View())
	for _, want := range []string{"feature has no branch on a remote yet", "Remote  origin", "enter: create it and push"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the box does not say %q:\n%s", want, screen)
		}
	}
	if got := strings.Count(screen, "\n") + 1; got != m.windowHeight {
		t.Errorf("the screen is %d lines with the box open, want %d", got, m.windowHeight)
	}

	// The name can be changed before it is used.
	m = press(m, keyPress("-"), keyPress("2"))
	m = pushed(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.push.open() || !strings.Contains(m.notice, "Pushed feature to origin/feature-2, a new branch it now tracks") {
		t.Fatalf("open=%v notice=%q; want the new branch reported", m.push.open(), m.notice)
	}
	if onRemote(t, dir, "refs/heads/feature-2") == "" {
		t.Error("origin has no branch of the name typed")
	}
	if onRemote(t, dir, "refs/heads/feature") != "" {
		t.Error("origin has a branch of the name that was typed over")
	}

	// It tracks that branch now, so the next push asks nothing.
	commit("more")
	m = res(m.Update(loadRepo(dir)()))
	m = pushed(t, m, keyPress("P"))
	if !strings.Contains(m.notice, "Pushed 1 commit to origin/feature-2") {
		t.Errorf("notice = %q, want the next push to go to the branch it tracks", m.notice)
	}
}

func TestPushNameBoxRefusesABadNameAndCloses(t *testing.T) {
	dir, _, git, _ := repoWithRemote(t)
	git("switch", "-q", "-c", "feature")
	m := res(loadedModel(t, dir).Update(keyPress("P")))

	m = press(m, keyPress(" "), keyPress("x"))
	refused, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = refused.(model)
	if cmd != nil || m.pushing || !m.push.naming || m.push.failure == "" {
		t.Fatalf("pushing=%v naming=%v failure=%q; want the name refused in the box", m.pushing, m.push.naming, m.push.failure)
	}
	if !strings.Contains(ansi.Strip(m.View()), "does not accept that as a branch name") {
		t.Error("the box does not say why the name will not do")
	}
	// Typing again takes the complaint away: it was about the old name.
	if m = press(m, tea.KeyMsg{Type: tea.KeyBackspace}); m.push.failure != "" {
		t.Error("the complaint outlived the name it was about")
	}

	closed, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || closed.(model).push.open() || closed.(model).pushing {
		t.Error("esc did not close the box without pushing")
	}
	if out, _ := git2(t, dir, "ls-remote", "--heads", "origin"); strings.Contains(out, "feature") {
		t.Errorf("something was pushed:\n%s", out)
	}
}

// git2 runs a read-only git command for a check.
func git2(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	return git.Run(dir, args...)
}

// With more than one remote the box says which it would push to, and tab
// moves on to the next.
func TestPushNameBoxCyclesRemotes(t *testing.T) {
	dir, _, git, _ := repoWithRemote(t)
	git("remote", "add", "fork", filepath.Join(t.TempDir(), "fork.git"))
	git("switch", "-q", "-c", "feature")
	m := res(loadedModel(t, dir).Update(keyPress("P")))

	if got := m.push.plan.Remotes[m.push.remote]; got != "origin" {
		t.Fatalf("it starts on %q, want origin", got)
	}
	if !strings.Contains(ansi.Strip(m.View()), "tab: another remote") {
		t.Error("the box does not say tab changes the remote")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.push.plan.Remotes[m.push.remote]; got != "fork" {
		t.Errorf("after tab it is on %q, want fork", got)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Remote  fork") {
		t.Error("the box does not show the remote tab moved to")
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.push.plan.Remotes[m.push.remote]; got != "origin" {
		t.Errorf("after a second tab it is on %q, want origin again", got)
	}
}

// A tag on the selected commit that no remote holds is offered in place of
// the branch, and nothing goes until one is picked.
func TestPushOffersAnUnpushedTag(t *testing.T) {
	dir, _, git, commit := repoWithRemote(t)
	commit("second")
	git("tag", "v1")
	m := withRemoteTags(t, dir)

	asked, cmd := m.Update(keyPress("P"))
	m = asked.(model)
	if cmd != nil || m.pushing || !m.push.asking {
		t.Fatalf("pushing=%v asking=%v; want the question, and nothing pushed yet", m.pushing, m.push.asking)
	}
	if line := ansi.Strip(m.renderStatusLine()); !strings.Contains(line, "t tag v1") || !strings.Contains(line, "b branch main") {
		t.Errorf("status line = %q, want the tag and the branch offered", line)
	}

	m = pushed(t, m, keyPress("t"))
	if !strings.Contains(m.notice, "Pushed tag v1 to origin") {
		t.Fatalf("notice = %q, want the tag reported", m.notice)
	}
	if onRemote(t, dir, "refs/tags/v1") == "" {
		t.Error("origin does not have the tag")
	}
	// Only the tag: the commits under it were not what was picked.
	if m.ahead != 0 {
		t.Fatalf("ahead = %d before the reload has run", m.ahead)
	}
	m = res(m.Update(loadRepo(dir)()))
	if m.ahead != 1 {
		t.Errorf("ahead = %d, want the branch still unpushed", m.ahead)
	}
	// The mark goes at once, without waiting for the remotes to be asked.
	for _, c := range m.commits {
		if len(c.UnpushedTags) > 0 {
			t.Errorf("%s is still marked as having unpushed tags: %v", c.Hash, c.UnpushedTags)
		}
	}
}

func TestPushPromptAnswers(t *testing.T) {
	dir, _, git, commit := repoWithRemote(t)
	commit("second")
	git("tag", "v1")
	git("tag", "v2")
	m := res(withRemoteTags(t, dir).Update(keyPress("P")))
	if line := ansi.Strip(m.renderStatusLine()); !strings.Contains(line, "1 tag v1 • 2 tag v2 • b branch main") {
		t.Fatalf("status line = %q, want the tags numbered", line)
	}

	// A key that picks nothing closes the prompt and sends nothing.
	stray, cmd := m.Update(keyPress("x"))
	if cmd != nil || stray.(model).push.open() || stray.(model).pushing {
		t.Error("a stray key did not just close the prompt")
	}

	second := pushed(t, m, keyPress("2"))
	if !strings.Contains(second.notice, "Pushed tag v2 to origin") || onRemote(t, dir, "refs/tags/v1") != "" {
		t.Errorf("notice = %q; want the second tag pushed and the first left", second.notice)
	}

	branch := pushed(t, m, keyPress("b"))
	if !strings.Contains(branch.notice, "Pushed 1 commit to origin/main") {
		t.Errorf("notice = %q; want b to push the branch", branch.notice)
	}
	if onRemote(t, dir, "refs/tags/v1") != "" {
		t.Error("pushing the branch took a tag with it")
	}
}

// A tag the remote already has is no reason to ask: P pushes the branch.
func TestPushDoesNotAskAboutAPushedTag(t *testing.T) {
	dir, _, git, commit := repoWithRemote(t)
	commit("second")
	git("tag", "v1")
	git("push", "-q", "origin", "v1")
	m := withRemoteTags(t, dir)

	m = pushed(t, m, keyPress("P"))
	if !strings.Contains(m.notice, "Pushed 1 commit to origin/main") {
		t.Errorf("notice = %q, want the branch pushed without a question", m.notice)
	}
}

func TestPushWaitsItsTurn(t *testing.T) {
	dir, _, _, commit := repoWithRemote(t)
	commit("second")
	m := loadedModel(t, dir)

	m.fetching = true
	got, cmd := m.Update(keyPress("P"))
	if cmd != nil || got.(model).pushing || !strings.Contains(got.(model).notice, "A fetch is running") {
		t.Errorf("notice = %q; want P to wait for the fetch", got.(model).notice)
	}
	m.fetching = false

	started, _ := m.Update(keyPress("P"))
	pushing := started.(model)
	for key, want := range map[string]string{"p": "A push is running", "P": ""} {
		got, cmd := pushing.Update(keyPress(key))
		if cmd != nil {
			t.Errorf("%s started something while a push was running", key)
		}
		if want != "" && !strings.Contains(got.(model).notice, want) {
			t.Errorf("%s: notice = %q, want %q", key, got.(model).notice, want)
		}
	}
	if got, cmd := pushing.Update(keyPress("f")); cmd != nil || got.(model).fetching {
		t.Error("f started a fetch beside the push")
	}
	if pushing.canReloadUnasked() {
		t.Error("an unasked reload could replace the model the push will answer to")
	}
}

func TestPushResultForAnotherRepositoryIsDropped(t *testing.T) {
	dir, _, _, _ := repoWithRemote(t)
	m := loadedModel(t, dir)
	m.pushing = true
	got, cmd := m.Update(pushFinishedMsg{repoPath: "somewhere-else", result: git.PushResult{Outcome: git.PushSent}})
	if cmd != nil || got.(model).notice != "" || !got.(model).ready {
		t.Error("a push of another repository was reported on this one")
	}
}

func TestPushFailureSaysWhy(t *testing.T) {
	dir, _, git, commit := repoWithRemote(t)
	commit("second")
	git("remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))

	m := pushed(t, loadedModel(t, dir), keyPress("P"))
	if !strings.HasPrefix(m.notice, "Push failed: ") || len(m.notice) < len("Push failed: ")+5 {
		t.Errorf("notice = %q, want git's reason", m.notice)
	}
	if !m.ready {
		t.Error("the repository was read again after a push that did nothing")
	}
}

func TestPushNameBoxShowsALongName(t *testing.T) {
	dir, _, git, commit := repoWithRemote(t)
	long := "feature/PROJ-1234-make-the-push-box-wide-enough-for-a-long-branch-name"
	git("switch", "-q", "-c", long)
	commit("on the long branch")
	m := loadedModel(t, dir)
	m = press(m, keyPress("P"))
	if !m.push.naming {
		t.Fatal("P did not open the new-branch box")
	}

	screen := ansi.Strip(m.View())
	if !strings.Contains(screen, "Branch  "+long) {
		t.Errorf("the box does not show the whole name %q:\n%s", long, screen)
	}

	// In a narrow window the box still fits, with the name scrolled.
	for _, width := range []int{40, 60, 80} {
		m.windowWidth = width
		for _, line := range strings.Split(ansi.Strip(m.push.render(width)), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Errorf("at %d columns the box is %d wide: %q", width, got, line)
				break
			}
		}
	}
}
