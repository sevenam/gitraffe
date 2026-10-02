package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/gittest"
)

// pull presses p and feeds back what the pull it started reports.
func pull(t *testing.T, m model) model {
	t.Helper()
	res, cmd := m.Update(keyPress("p"))
	pulling := res.(model)
	if !pulling.pulling || cmd == nil {
		t.Fatalf("pulling=%v cmd=%v; want p to start a pull", pulling.pulling, cmd != nil)
	}
	// The pull outlives the keypress, so the status line has to say so.
	if !strings.Contains(ansi.Strip(pulling.renderStatusLine()), "Pulling") {
		t.Error("the status line does not say a pull is running")
	}
	res, _ = pulling.Update(cmd())
	return res.(model)
}

func TestPullBringsTheBranchForward(t *testing.T) {
	dir, origin, _, _ := repoWithRemote(t)
	pushFromElsewhere(t, origin)
	m := loadedModel(t, dir)
	before := len(m.commits)

	m = pull(t, m)
	if m.pulling || !strings.Contains(m.notice, "Pulled 1 commit from origin/main") {
		t.Fatalf("pulling=%v notice=%q; want it to say what was pulled", m.pulling, m.notice)
	}
	// It reloads, like a fetch: the new commit is only on screen once it has.
	if m.ready {
		t.Fatal("the repository was not read again after the pull")
	}
	res, _ := m.Update(loadRepo(dir)())
	m = res.(model)
	if len(m.commits) != before+1 || m.ahead != 0 || m.behind != 0 {
		t.Errorf("%d commits, ahead/behind %d/%d; want the pulled commit and the branch in sync", len(m.commits), m.ahead, m.behind)
	}
	if !strings.Contains(m.notice, "Pulled") {
		t.Errorf("notice = %q after the reload, want what was pulled still said", m.notice)
	}
}

func TestPullWithNothingNewSaysSo(t *testing.T) {
	dir, _, _, _ := repoWithRemote(t)
	m := pull(t, loadedModel(t, dir))
	if !strings.Contains(m.notice, "Already up to date with origin/main") {
		t.Errorf("notice = %q, want it to say there was nothing to pull", m.notice)
	}
}

// The branch is left exactly where it was, and the line says why and what to
// do about it: a key that silently did nothing would look broken.
func TestPullOfADivergedBranchExplainsItself(t *testing.T) {
	dir, origin, _, commit := repoWithRemote(t)
	commit("mine")
	pushFromElsewhere(t, origin)
	m := loadedModel(t, dir)
	was := m.currentCommit

	m = pull(t, m)
	for _, want := range []string{"Not pulled", "diverged", "↑1 ↓1", "terminal"} {
		if !strings.Contains(m.notice, want) {
			t.Errorf("notice = %q, want it to mention %q", m.notice, want)
		}
	}
	res, _ := m.Update(loadRepo(dir)())
	m = res.(model)
	if m.currentCommit != was {
		t.Error("a diverged branch was moved")
	}
	// The fetch half still happened, so the counts are now true.
	if m.ahead != 1 || m.behind != 1 {
		t.Errorf("ahead/behind = %d/%d, want 1/1 shown after the fetch", m.ahead, m.behind)
	}
}

func TestPullThatGitRefusesSaysWhy(t *testing.T) {
	dir, origin, _, _ := repoWithRemote(t)
	m := loadedModel(t, dir)
	// The incoming commit adds a file that already exists here, uncommitted.
	mine := filepath.Join(dir, "from a colleague")
	if err := os.WriteFile(mine, []byte("my edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	pushFromElsewhere(t, origin)

	m = pull(t, m)
	if !strings.HasPrefix(m.notice, "Pull failed: ") || m.notice == "Pull failed: " {
		t.Errorf("notice = %q, want git's own reason", m.notice)
	}
	if body, _ := os.ReadFile(mine); string(body) != "my edit" {
		t.Errorf("the local file was overwritten: %q", body)
	}
}

func TestPullWithNowhereToPullFromSaysSo(t *testing.T) {
	dir, gitCmd, commit := gittest.Fixture(t)
	gitCmd("init", "-q", "-b", "main")
	commit("first")

	m := pull(t, loadedModel(t, dir))
	if !strings.Contains(m.notice, "main tracks no remote branch") || !m.ready {
		t.Errorf("notice=%q ready=%v; want it explained, and no reload since nothing was fetched", m.notice, m.ready)
	}

	gitCmd("checkout", "-q", "--detach")
	m = pull(t, loadedModel(t, dir))
	if !strings.Contains(m.notice, "not on a branch") {
		t.Errorf("notice = %q, want it to say HEAD is detached", m.notice)
	}
}

// A pull fetches, so it must not run beside a fetch, nor a fetch beside it.
func TestPullAndFetchDoNotOverlap(t *testing.T) {
	dir, _, _, _ := repoWithRemote(t)
	loaded := loadedModel(t, dir)

	for name, busy := range map[string]func(*model){
		"a fetch":      func(m *model) { m.fetching = true },
		"an autofetch": func(m *model) { m.autoFetching = true },
	} {
		m := loaded
		busy(&m)
		res, cmd := m.Update(keyPress("p"))
		if got := res.(model); got.pulling || cmd != nil || !strings.Contains(got.notice, "fetch is running") {
			t.Errorf("during %s: pulling=%v notice=%q; want the pull put off with a reason", name, got.pulling, got.notice)
		}
	}

	m := loaded
	m.pulling = true
	if res, cmd := m.Update(keyPress("f")); res.(model).fetching || cmd != nil {
		t.Error("f started a fetch beside a running pull")
	}
	if res, cmd := m.Update(keyPress("p")); cmd != nil || !res.(model).pulling {
		t.Error("p started a second pull beside a running one")
	}
	// Nor does anything unasked start underneath it.
	m.autoRefresh, m.autoFetch = time.Minute, time.Minute
	if m.canReloadUnasked() {
		t.Error("an unasked reload would run during a pull")
	}
	if res, _ := m.Update(autoFetchTickMsg{}); res.(model).autoFetching {
		t.Error("auto-fetch started during a pull")
	}
}

func TestPullResultForAnotherRepositoryIsDropped(t *testing.T) {
	m := testModel()
	m.pulling = true
	res, cmd := m.Update(pullFinishedMsg{repoPath: "/somewhere/else", result: git.PullResult{Fetched: true}})
	if cmd != nil || res.(model).notice != "" {
		t.Error("a pull of a repository since left was acted on")
	}
}

// A pull whose fetch worked is the same evidence a fetch is that the remote
// can be reached again.
func TestPullStartsAutoFetchOffAgain(t *testing.T) {
	m := testModel()
	m.autoFetchStopped = true
	res, _ := m.Update(pullFinishedMsg{repoPath: m.repoPath, result: git.PullResult{Fetched: true, Outcome: git.PullUpToDate}})
	if res.(model).autoFetchStopped {
		t.Error("auto-fetch stayed stopped after a pull reached the remote")
	}

	m.autoFetchStopped = true
	res, _ = m.Update(pullFinishedMsg{repoPath: m.repoPath, err: errors.New("exit status 128"), result: git.PullResult{Detail: "fatal: no network"}})
	if got := res.(model); !got.autoFetchStopped || got.notice != "Pull failed: fatal: no network" {
		t.Errorf("stopped=%v notice=%q; want it still stopped and the failure said", got.autoFetchStopped, got.notice)
	}
}

// p used to open the pull request; that moved to P when p became pull.
func TestCapitalPOpensThePullRequest(t *testing.T) {
	m := loadedModel(t, prRepo(t))
	m.selected = len(m.commits) - 1 // the first commit, which merged nothing
	res, _ := m.Update(keyPress("P"))
	if got := res.(model); !strings.Contains(got.notice, "No pull request") || got.pulling {
		t.Errorf("notice=%q pulling=%v; want P to look for a pull request, not pull", got.notice, got.pulling)
	}
}
