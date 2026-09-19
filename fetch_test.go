package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// repoWithRemote is a repository whose origin is a bare repository next door,
// so a fetch is a real fetch that never leaves the machine.
func repoWithRemote(t *testing.T) (dir, origin string, git func(...string), commit func(string)) {
	t.Helper()
	dir, git, commit = gitFixture(t)
	origin = filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", origin).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	git("init", "-q", "-b", "main")
	commit("first")
	git("remote", "add", "origin", origin)
	git("push", "-q", "-u", "origin", "main")
	return dir, origin, git, commit
}

// pushFromElsewhere puts a commit on the remote behind this repository's back,
// the way a colleague would.
func pushFromElsewhere(t *testing.T, origin string) {
	t.Helper()
	clone, git, commit := gitFixture(t)
	if out, err := exec.Command("git", "clone", "-q", origin, clone).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	commit("from a colleague")
	git("push", "-q", "origin", "main")
}

func TestFetchNeedsARemote(t *testing.T) {
	m := loadedModel(t, newRepo(t))
	got, cmd := m.Update(keyPress("f"))
	if cmd != nil || got.(model).fetching {
		t.Error("f started a fetch in a repository with no remote")
	}
	if !strings.Contains(got.(model).notice, "no remote") {
		t.Errorf("notice = %q, want it to say there is no remote", got.(model).notice)
	}
}

func TestFetchBringsInNewCommits(t *testing.T) {
	dir, origin, _, _ := repoWithRemote(t)
	pushFromElsewhere(t, origin)

	m := loadedModel(t, dir)
	if m.behind != 0 {
		t.Fatalf("behind = %d before fetching, want 0: the new commit is only on the remote", m.behind)
	}

	res, cmd := m.Update(keyPress("f"))
	fetching := res.(model)
	if !fetching.fetching || cmd == nil {
		t.Fatalf("fetching=%v cmd=%v; want f to start a fetch", fetching.fetching, cmd != nil)
	}
	// The status line has to say so: the fetch outlives the keypress.
	if !strings.Contains(ansi.Strip(fetching.renderStatusLine()), "Fetching") {
		t.Error("the status line does not say a fetch is running")
	}

	msg, ok := cmd().(fetchFinishedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("fetch returned %v (%v)", cmd(), msg.detail)
	}
	res, cmd = fetching.Update(msg)
	done := res.(model)
	if done.fetching || !strings.Contains(done.notice, "Fetched") {
		t.Errorf("fetching=%v notice=%q; want it finished and said so", done.fetching, done.notice)
	}
	if cmd == nil {
		t.Fatal("a successful fetch did not reload the repository")
	}

	// Reloading is what turns the fetched refs into counts on screen.
	res, _ = done.Update(loadRepo(dir)())
	if got := res.(model); got.behind != 1 {
		t.Errorf("behind = %d after fetching, want 1", got.behind)
	}
}

func TestFetchReportsFailure(t *testing.T) {
	dir, origin, git, _ := repoWithRemote(t)
	git("remote", "set-url", "origin", origin+"-gone")

	m := loadedModel(t, dir)
	res, cmd := m.Update(keyPress("f"))
	msg := cmd().(fetchFinishedMsg)
	if msg.err == nil {
		t.Fatal("fetching a remote that isn't there succeeded")
	}
	res, cmd = res.(model).Update(msg)
	got := res.(model)
	if got.fetching || cmd != nil {
		t.Errorf("fetching=%v cmd=%v; want it stopped with nothing reloaded", got.fetching, cmd != nil)
	}
	if !strings.HasPrefix(got.notice, "Fetch failed") {
		t.Errorf("notice = %q, want it to report the failure", got.notice)
	}
}

func TestFetchRunsOneAtATime(t *testing.T) {
	dir, _, _, _ := repoWithRemote(t)
	m := loadedModel(t, dir)

	res, _ := m.Update(keyPress("f"))
	busy := res.(model)
	if _, cmd := busy.Update(keyPress("f")); cmd != nil {
		t.Error("a second f started another fetch while one was running")
	}
}

// A fetch of a repository you have since left says nothing about this one.
func TestFetchOfAnotherRepoIsIgnored(t *testing.T) {
	m := loadedModel(t, newRepo(t))
	m.fetching = true

	res, cmd := m.Update(fetchFinishedMsg{repoPath: "/somewhere/else"})
	got := res.(model)
	if cmd != nil || !got.fetching || got.notice != "" {
		t.Errorf("fetching=%v notice=%q cmd=%v; want the stray answer ignored",
			got.fetching, got.notice, cmd != nil)
	}
}

func TestFetchErrorTextPrefersGitsOwnWords(t *testing.T) {
	m := loadedModel(t, newRepo(t))
	m.fetching = true
	res, _ := m.Update(fetchFinishedMsg{
		repoPath: m.repoPath,
		err:      errors.New("exit status 128"),
		detail:   "fatal: 'origin' does not appear to be a git repository",
	})
	if got := res.(model).notice; !strings.Contains(got, "does not appear to be a git repository") {
		t.Errorf("notice = %q, want git's own message", got)
	}

	m.fetching = true
	res, _ = m.Update(fetchFinishedMsg{repoPath: m.repoPath, err: errors.New("no answer after 1m0s")})
	if got := res.(model).notice; !strings.Contains(got, "no answer") {
		t.Errorf("notice = %q, want the error when git said nothing", got)
	}
}
