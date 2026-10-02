package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// tick runs one turn of the refresh timer: the tick, and the background check
// it starts. The model that comes back has seen the check's answer.
func tick(t *testing.T, m model) model {
	t.Helper()
	res, cmd := m.Update(autoRefreshTickMsg{})
	if cmd == nil {
		t.Fatal("the tick started no check")
	}
	state, ok := cmd().(repoStateMsg)
	if !ok || !state.fromTick {
		t.Fatalf("the tick's command gave %#v, want the timer's own check", state)
	}
	res, _ = res.(model).Update(state)
	return res.(model)
}

func TestAutoRefreshReloadsOnlyWhenSomethingChanged(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")

	m := loadedModel(t, dir)
	m.autoRefresh = time.Minute
	m.windowWidth, m.windowHeight = 100, 24
	m.notice = "something the user was told"
	if m.fingerprint == "" {
		t.Fatal("loading took no fingerprint, so there is nothing to compare against")
	}

	// Nothing changed: the model is left exactly as it was, with no reload
	// under way, so there is nothing for the screen to redraw.
	m = tick(t, m)
	if !m.ready || m.stale != nil {
		t.Fatal("a tick reloaded a repository that had not changed")
	}

	commit("second")
	before := ansi.Strip(m.View())
	m = tick(t, m)
	if m.ready {
		t.Fatal("a tick did not reload a repository that had changed")
	}
	if got := ansi.Strip(m.View()); got != before {
		t.Error("the screen changed before the reload had anything new to show")
	}

	res, _ := m.Update(loadRepo(dir)())
	m = res.(model)
	if got := strings.Join(messages(m), ","); got != "second,first" {
		t.Errorf("commits = %s, want the new commit picked up", got)
	}
	// Unasked, so it says nothing and leaves what was on the bottom line.
	if m.notice != "something the user was told" {
		t.Errorf("notice = %q, want it left alone", m.notice)
	}

	// The reload took its own fingerprint, so the next tick is quiet again.
	if m = tick(t, m); !m.ready {
		t.Error("the tick after a reload reloaded again")
	}
}

func TestAutoRefreshSeesUncommittedChanges(t *testing.T) {
	dir, _, _ := dirtyRepo(t)
	m := loadedModel(t, dir)
	m.autoRefresh = time.Minute

	write(t, dir, "first", "changed")
	if m = tick(t, m); m.ready {
		t.Error("an edit in the working tree went unnoticed")
	}
}

// A reload starts from a fresh model, so one nobody asked for would close
// whatever is open over the graph. It waits instead; the change is still
// there to find on the next tick.
func TestAutoRefreshWaitsWhileSomethingIsOpen(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	loaded := loadedModel(t, dir)
	loaded.autoRefresh = time.Minute
	changed := repoStateMsg{repoPath: dir, fingerprint: "something else", fromTick: true}

	for name, open := range map[string]func(*model){
		"help":          func(m *model) { m.showHelp = true },
		"theme picker":  func(m *model) { m.picker.open = true },
		"repo switcher": func(m *model) { m.switcher.open = true },
		"ref picker":    func(m *model) { m.refs.open = true },
		"search prompt": func(m *model) { m.search.active = true },
		"a fetch":       func(m *model) { m.fetching = true },
		"an update":     func(m *model) { m.updateState = updateConfirming },
	} {
		m := loaded
		open(&m)
		res, cmd := m.Update(changed)
		if !res.(model).ready {
			t.Errorf("%s: reloaded underneath it", name)
		}
		if cmd == nil {
			t.Errorf("%s: the timer was not started again", name)
		}
	}

	res, _ := loaded.Update(changed)
	if res.(model).ready {
		t.Error("with nothing open, a changed repository was not reloaded")
	}
}

// No fingerprint is no evidence of a change. Treating it as one would reload
// on every tick for as long as git could not be asked.
func TestAutoRefreshIgnoresWhatItCannotCompare(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	m := loadedModel(t, dir)
	m.autoRefresh = time.Minute

	for name, msg := range map[string]repoStateMsg{
		"the check failed":   {repoPath: dir},
		"another repository": {repoPath: "/somewhere/else", fingerprint: "x"},
	} {
		if res, _ := m.Update(msg); !res.(model).ready {
			t.Errorf("%s: reloaded", name)
		}
	}

	m.fingerprint = ""
	if res, _ := m.Update(repoStateMsg{repoPath: dir, fingerprint: "x"}); !res.(model).ready {
		t.Error("reloaded with no fingerprint of its own to compare against")
	}
}

func TestAutoRefreshOff(t *testing.T) {
	m := testModel()
	if m.autoRefreshTick() != nil || m.autoFetchTick() != nil {
		t.Error("a timer was started with both intervals at zero")
	}
	if _, cmd := m.Update(tea.FocusMsg{}); cmd != nil {
		t.Error("focus started a check with auto-refresh off")
	}
}

func TestFocusChecksAtOnce(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	m := loadedModel(t, dir)
	m.autoRefresh = time.Minute

	_, cmd := m.Update(tea.FocusMsg{})
	if cmd == nil {
		t.Fatal("coming back to the terminal started no check")
	}
	// Not the timer's check: answering it must not start a second timer.
	if state, ok := cmd().(repoStateMsg); !ok || state.fromTick || state.fingerprint != m.fingerprint {
		t.Errorf("focus gave %#v, want a one-off check matching the loaded state", state)
	}
	if _, cmd := m.Update(repoStateMsg{repoPath: dir, fingerprint: m.fingerprint}); cmd != nil {
		t.Error("a one-off check started the timer again")
	}
}

func TestAutoRefreshSettings(t *testing.T) {
	for _, tc := range []struct {
		name, yml          string
		refresh, autoFetch time.Duration
	}{
		{"nothing set", "", 30 * time.Second, 0},
		{"refresh off", "auto_refresh: 0\n", 0, 0},
		{"refresh slower", "auto_refresh: 120\n", 2 * time.Minute, 0},
		{"refresh too fast is raised", "auto_refresh: 1\n", 2 * time.Second, 0},
		{"nonsense is off", "auto_refresh: -5\n", 0, 0},
		{"fetch on", "auto_fetch: 300\n", 30 * time.Second, 5 * time.Minute},
		{"fetch too fast is raised", "auto_fetch: 5\n", 30 * time.Second, 30 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel()
			m.configDir = t.TempDir()
			writeFile(t, filepath.Join(m.configDir, "settings.yml"), tc.yml)
			m = applyPreferences(m)
			if m.autoRefresh != tc.refresh || m.autoFetch != tc.autoFetch {
				t.Errorf("refresh %v, fetch %v; want %v and %v", m.autoRefresh, m.autoFetch, tc.refresh, tc.autoFetch)
			}
			// The intervals belong to the session, not to a repository.
			next, _ := m.switchRepo(".")
			if next.autoRefresh != tc.refresh || next.autoFetch != tc.autoFetch {
				t.Error("switching repository dropped the intervals")
			}
			// Hand-written, so leaving must not write them back differently.
			if err := savePreferences(m); err != nil {
				t.Fatal(err)
			}
			if again := applyPreferences(m); again.autoRefresh != tc.refresh || again.autoFetch != tc.autoFetch {
				t.Error("saving the other preferences changed the intervals")
			}
		})
	}
}

func TestAutoFetchIsQuietWhenItWorks(t *testing.T) {
	dir, origin, _, _ := repoWithRemote(t)
	m := loadedModel(t, dir)
	m.autoFetch = time.Minute
	pushFromElsewhere(t, origin)

	res, cmd := m.Update(autoFetchTickMsg{})
	m = res.(model)
	if !m.autoFetching || m.fetching || cmd == nil {
		t.Fatalf("autoFetching=%v fetching=%v; want a fetch running that the status line does not announce", m.autoFetching, m.fetching)
	}
	done, ok := cmd().(fetchFinishedMsg)
	if !ok || !done.auto || done.err != nil {
		t.Fatalf("the fetch gave %#v", done)
	}

	res, cmd = m.Update(done)
	m = res.(model)
	if m.autoFetching || m.notice != "" || !m.ready {
		t.Errorf("autoFetching=%v notice=%q ready=%v; want it finished without a word or a reload of its own", m.autoFetching, m.notice, m.ready)
	}
	// What it fetched is shown the way any change is: the check that follows
	// finds the refs moved.
	if cmd == nil {
		t.Fatal("nothing checks whether the fetch brought anything")
	}
	state, _ := checkRepoStateCmd(dir, false)().(repoStateMsg)
	if state.fingerprint == m.fingerprint {
		t.Error("the fetch moved a remote branch but the fingerprint did not change")
	}
	if res, _ = m.Update(state); res.(model).ready {
		t.Error("what the fetch brought was not reloaded")
	}
}

func TestAutoFetchStopsAfterAFailure(t *testing.T) {
	dir, _, _, _ := repoWithRemote(t)
	m := loadedModel(t, dir)
	m.autoFetch = time.Minute
	m.autoFetching = true

	res, _ := m.Update(fetchFinishedMsg{repoPath: dir, auto: true, err: errors.New("exit status 128"), detail: "fatal: no network"})
	m = res.(model)
	if !m.autoFetchStopped || !strings.Contains(m.notice, "Auto-fetch stopped: fatal: no network") {
		t.Fatalf("stopped=%v notice=%q; want it stopped and saying why", m.autoFetchStopped, m.notice)
	}

	res, _ = m.Update(autoFetchTickMsg{})
	if res.(model).autoFetching {
		t.Error("the next tick tried again")
	}

	// A reload is the same repository with the same problem.
	next, _ := m.reloadRepo()
	if !next.autoFetchStopped {
		t.Error("a reload forgot that auto-fetch had stopped")
	}

	// A fetch you ask for that works is the evidence that it is worth trying.
	res, cmd := m.Update(keyPress("f"))
	res, _ = res.(model).Update(cmd())
	if res.(model).autoFetchStopped {
		t.Error("a successful fetch did not start auto-fetch off again")
	}
}

// Pressing f while the timer's fetch is running takes that fetch over, so two
// never run side by side and its result is reported like any you asked for.
func TestPressingFAdoptsARunningAutoFetch(t *testing.T) {
	dir, _, _, _ := repoWithRemote(t)
	m := loadedModel(t, dir)
	m.autoFetching = true

	res, cmd := m.Update(keyPress("f"))
	m = res.(model)
	if !m.fetching || cmd != nil {
		t.Fatalf("fetching=%v cmd=%v; want the running fetch adopted, not a second started", m.fetching, cmd != nil)
	}
	res, _ = m.Update(fetchFinishedMsg{repoPath: dir, auto: true})
	m = res.(model)
	if m.fetching || m.autoFetching || !strings.Contains(m.notice, "Fetched") {
		t.Errorf("fetching=%v autoFetching=%v notice=%q; want it reported as the fetch that was asked for", m.fetching, m.autoFetching, m.notice)
	}
}

func TestAutoFetchWithNoRemoteIsNotAFailure(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	m := loadedModel(t, dir)
	m.autoFetch = time.Minute

	res, cmd := m.Update(autoFetchTickMsg{})
	res, _ = res.(model).Update(cmd())
	if got := res.(model); got.autoFetchStopped || got.autoFetching || got.notice != "" {
		t.Errorf("stopped=%v fetching=%v notice=%q; want nothing to have happened", got.autoFetchStopped, got.autoFetching, got.notice)
	}
}
