package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

// divergedModel is a repository whose main has a commit of its own while
// origin/main has another, loaded; git is the repository's own git.
func divergedModel(t *testing.T) (dir string, run func(...string), m model) {
	t.Helper()
	dir, origin, run, commit := repoWithRemote(t)
	gittest.Identify(run)
	commit("mine")
	pushFromElsewhere(t, origin)
	return dir, run, loadedModel(t, dir)
}

// A diverged branch is not moved by the pull, but the merge box is opened
// for its upstream once the reload has read what the fetch brought; enter
// there makes the merge commit, as m on origin/main would.
func TestPullOfADivergedBranchOffersAMerge(t *testing.T) {
	dir, _, m := divergedModel(t)
	was := m.currentCommit

	m = pull(t, m)
	for _, want := range []string{"diverged", "↑1 ↓1", "enter merges origin/main in"} {
		if !strings.Contains(m.notice, want) {
			t.Errorf("notice = %q, want it to mention %q", m.notice, want)
		}
	}
	if m.merge.open {
		t.Error("the box opened before the reload had read the fetched commits")
	}
	res, _ := m.Update(loadRepo(dir)())
	m = res.(model)
	if m.currentCommit != was {
		t.Error("a diverged branch was moved by the pull itself")
	}
	if m.ahead != 1 || m.behind != 1 {
		t.Errorf("ahead/behind = %d/%d, want 1/1 shown after the fetch", m.ahead, m.behind)
	}
	if !m.merge.open || m.merge.ref != "refs/remotes/origin/main" || m.merge.into != "main" {
		t.Fatalf("merge box = %+v; want origin/main into main", m.merge)
	}
	m.windowWidth, m.windowHeight = 120, 30
	screen := ansi.Strip(m.View())
	for _, want := range []string{"origin/main  into  main", "as a merge commit", "Merge remote-tracking branch 'origin/main'"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the box does not say %q:\n%s", want, screen)
		}
	}

	m, reload := step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if reload == nil || !strings.HasPrefix(m.notice, "Merged origin/main into main as ") {
		t.Fatalf("notice = %q, want the merge made", m.notice)
	}
	parents, _ := git.Run(dir, "rev-list", "--parents", "-n", "1", "main")
	upstream, _ := git.Run(dir, "rev-parse", "origin/main")
	if f := strings.Fields(parents); len(f) != 3 || !strings.HasPrefix(f[1], was) || f[2] != upstream {
		t.Errorf("main is %q, want a merge of %s and origin/main's %s", parents, was, upstream)
	}
}

// Esc at the box the pull opened leaves everything as it was.
func TestPullMergeOfferCanBeDeclined(t *testing.T) {
	dir, _, m := divergedModel(t)
	was := m.currentCommit
	m = pull(t, m)
	res, _ := m.Update(loadRepo(dir)())
	m = press(res.(model), tea.KeyMsg{Type: tea.KeyEsc})
	if m.merge.open || m.notice != "Nothing merged" {
		t.Errorf("open=%v notice=%q; want the box closed and nothing merged", m.merge.open, m.notice)
	}
	if head, _ := git.Run(dir, "rev-parse", "main"); !strings.HasPrefix(head, was) {
		t.Errorf("main moved to %s", head)
	}
}

// Where git pull would not merge, neither is a merge offered: the line says
// what it always said, and the branch is left where it was.
func TestPullOfADivergedBranchRespectsPullSettings(t *testing.T) {
	for name, config := range map[string][]string{
		"pull.ff=only":       {"pull.ff", "only"},
		"pull.rebase=true":   {"pull.rebase", "true"},
		"pull.rebase=merges": {"pull.rebase", "merges"},
		"branch rebase":      {"branch.main.rebase", "true"},
	} {
		t.Run(name, func(t *testing.T) {
			dir, run, m := divergedModel(t)
			run(append([]string{"config"}, config...)...)
			was := m.currentCommit
			m = pull(t, m)
			for _, want := range []string{"Not pulled", "diverged", "↑1 ↓1", "terminal"} {
				if !strings.Contains(m.notice, want) {
					t.Errorf("notice = %q, want it to mention %q", m.notice, want)
				}
			}
			res, _ := m.Update(loadRepo(dir)())
			m = res.(model)
			if m.merge.open {
				t.Error("a merge was offered against the repository's pull settings")
			}
			if m.currentCommit != was {
				t.Error("a diverged branch was moved")
			}
		})
	}
}

// A merge is refused over uncommitted changes, so none is offered; the line
// says what would let it be.
func TestPullOfADivergedBranchWithChangesSaysWhatWouldMerge(t *testing.T) {
	dir, _, m := divergedModel(t)
	if err := os.WriteFile(filepath.Join(dir, "mine"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = pull(t, m)
	if !strings.Contains(m.notice, "commit or stash your changes, and p offers a merge") {
		t.Errorf("notice = %q, want it to say how to get the merge offered", m.notice)
	}
	res, _ := m.Update(loadRepo(dir)())
	if res.(model).merge.open {
		t.Error("a merge was offered over uncommitted changes")
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

// The pull request key has been p and then P; it is o now, p pulls, and P is
// left free.
func TestLowerCaseOOpensThePullRequest(t *testing.T) {
	m := loadedModel(t, prRepo(t))
	m.selected = len(m.commits) - 1 // the first commit, which merged nothing
	if got, _ := pullRequest(t, m); !strings.Contains(got.notice, "No pull request") || got.pulling {
		t.Errorf("notice=%q pulling=%v; want o to look for a pull request, not pull", got.notice, got.pulling)
	}

	m.selected = 0 // the merge of a pull request
	for name, at := range map[string]model{"the graph": m, "the commit view": res(m.openCommitView())} {
		next, cmd := at.Update(keyPress("P"))
		if got := next.(model); cmd != nil || got.pulling || got.notice != "" {
			t.Errorf("P on %s: cmd=%v pulling=%v notice=%q, want it to do nothing", name, cmd != nil, got.pulling, got.notice)
		}
		if _, cmd := at.Update(keyPress("o")); cmd == nil {
			t.Errorf("o on %s started nothing", name)
		}
	}
}
