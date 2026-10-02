package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// loadedModel opens dir the way the program does, by feeding Update the
// message the load command produces.
func loadedModel(t *testing.T, dir string) model {
	t.Helper()
	m := testModel()
	m.repoPath = dir
	res, _ := m.Update(loadRepo(dir)())
	got := res.(model)
	if !got.ready {
		t.Fatalf("%s did not load", dir)
	}
	return got
}

func messages(m model) []string {
	var out []string
	for _, c := range m.commits {
		out = append(out, c.Message)
	}
	return out
}

func TestReloadPicksUpNewCommits(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	commit("second")

	m := loadedModel(t, dir)
	if got := strings.Join(messages(m), ","); got != "second,first" {
		t.Fatalf("commits = %s, want second,first", got)
	}
	commit("third")

	// The key alone only starts the reload; the graph is read by the command
	// it returns, exactly as at startup.
	res, cmd := m.Update(keyPress("r"))
	reloading := res.(model)
	if reloading.ready || cmd == nil {
		t.Fatalf("ready=%v cmd=%v; want it loading again", reloading.ready, cmd != nil)
	}
	res, _ = reloading.Update(loadRepo(dir)())
	m = res.(model)

	if got := strings.Join(messages(m), ","); got != "third,second,first" {
		t.Errorf("commits after reload = %s, want third,second,first", got)
	}
	if m.notice != "Refreshed" {
		t.Errorf("notice = %q, want Refreshed", m.notice)
	}
}

func TestReloadKeepsYourPlace(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	commit("second")

	m := loadedModel(t, dir)
	m.selected = 1 // "first"
	m.focusedBox = 2
	m.colourLanes = false
	was := m.commits[m.selected].FullHash
	commit("third") // shifts every index down by one

	res, _ := m.Update(keyPress("r"))
	res, _ = res.(model).Update(loadRepo(dir)())
	m = res.(model)

	if m.commits[m.selected].FullHash != was {
		t.Errorf("selected %q, want the commit that was selected before (%q)",
			m.commits[m.selected].Message, was[:7])
	}
	if m.focusedBox != 2 || m.colourLanes {
		t.Errorf("focus=%d colourLanes=%v; want them kept across a reload", m.focusedBox, m.colourLanes)
	}
	if m.reselect != "" {
		t.Error("the kept selection was not cleared after being applied")
	}
}

// An amend or a rebase replaces the commit that was selected.
func TestReloadFallsBackWhenTheCommitIsGone(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	commit("second")

	m := loadedModel(t, dir)
	m.selected = 0
	git("commit", "-q", "--amend", "-m", "amended")

	res, _ := m.Update(keyPress("r"))
	res, _ = res.(model).Update(loadRepo(dir)())
	m = res.(model)

	if m.selected != 0 || m.commits[0].Message != "amended" {
		t.Errorf("selected %d (%q), want the newest commit", m.selected, m.commits[m.selected].Message)
	}
}

func TestReloadWaitsForLoading(t *testing.T) {
	m := testModel()
	m.ready = false
	res, cmd := m.Update(keyPress("r"))
	if cmd != nil || res.(model).notice != "" {
		t.Error("r reloaded while the repository was still loading")
	}
}

func TestF5Reloads(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")

	m := loadedModel(t, dir)
	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF5})
	if got := res.(model); got.ready || cmd == nil {
		t.Errorf("ready=%v cmd=%v; want f5 to reload like r", got.ready, cmd != nil)
	}
}

func TestStatusLineOffersReload(t *testing.T) {
	m := testModel()
	m.windowWidth = 120
	line := ansi.Strip(m.renderStatusLine())
	if !strings.Contains(line, "r: reload") {
		t.Errorf("status line = %q, want it to offer reload", line)
	}
	// It has to survive the width the line is written for.
	if !strings.Contains(line, "q: quit") {
		t.Errorf("status line = %q, want quit still on it at 120 columns", line)
	}
}

// The issue this answers: a refresh used to swap the whole screen for the
// loading page and back, which reads as a flicker. Now the screen that was
// there stays up, and only the bottom line says anything is happening.
func TestRefreshKeepsTheScreenUp(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	commit("second")

	m := loadedModel(t, dir)
	m.windowWidth, m.windowHeight = 100, 24
	before := strings.Split(ansi.Strip(m.View()), "\n")

	res, _ := m.Update(keyPress("r"))
	reloading := res.(model)
	during := strings.Split(ansi.Strip(reloading.View()), "\n")

	if strings.Contains(strings.Join(during, "\n"), "Opening") {
		t.Fatal("the loading page was drawn during a refresh")
	}
	if len(during) != len(before) {
		t.Fatalf("screen is %d lines during a refresh, was %d", len(during), len(before))
	}
	last := len(before) - 1
	for i := range before[:last] {
		if during[i] != before[i] {
			t.Errorf("line %d changed during a refresh:\n was %q\n now %q", i, before[i], during[i])
		}
	}
	if !strings.Contains(during[last], "Refreshing") {
		t.Errorf("bottom line = %q, want it to say the refresh is running", during[last])
	}

	res, _ = reloading.Update(loadRepo(dir)())
	m = res.(model)
	after := strings.Split(ansi.Strip(m.View()), "\n")
	for i := range before[:last] {
		if after[i] != before[i] {
			t.Errorf("line %d changed across a refresh that found nothing new:\n was %q\n now %q", i, before[i], after[i])
		}
	}
	if !strings.Contains(after[last], "Refreshed") || strings.Contains(after[last], "Refreshing") {
		t.Errorf("bottom line = %q, want Refreshed", after[last])
	}
	if m.stale != nil {
		t.Error("the old screen was kept after the new one loaded")
	}
}

// Refreshing again has to show it did something even though the line already
// says "Refreshed": it passes through "Refreshing…" on the way.
func TestRefreshingTwiceStillChangesTheBottomLine(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")

	m := loadedModel(t, dir)
	for range 2 {
		res, _ := m.Update(keyPress("r"))
		if got := res.(model).notice; got != "Refreshing…" {
			t.Fatalf("notice while refreshing = %q, want Refreshing…", got)
		}
		res, _ = res.(model).Update(loadRepo(dir)())
		m = res.(model)
		if m.notice != "Refreshed" {
			t.Fatalf("notice after refreshing = %q, want Refreshed", m.notice)
		}
	}
}

// What a reload would otherwise have to fetch or work out again is carried
// across, so nothing blinks out and back: the split view, the scroll
// positions, the diffs already read and the tag marks.
func TestRefreshCarriesWhatWasOnScreen(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	for i := range 40 {
		commit(fmt.Sprintf("commit %02d", i))
	}

	m := loadedModel(t, dir)
	m.windowWidth, m.windowHeight = 100, 20
	m.maximised = false
	m.selected = 30
	m.graphTop, _ = m.graphWindow()
	top := m.graphTop
	if top == 0 {
		t.Fatal("the graph did not scroll, so this covers nothing")
	}
	m.detailsScroll = 3
	m.commits[30].DiffLoaded = true
	m.commits[30].DiffBody = "the diff that was on screen"
	tags := map[tagRef]bool{{Name: "v1", Commit: m.commits[0].FullHash}: true}
	m.remoteTags = tags

	res, _ := m.Update(keyPress("r"))
	res, cmd := res.(model).Update(loadRepo(dir)())
	m = res.(model)

	if m.maximised {
		t.Error("the split view was maximised again")
	}
	if m.selected != 30 || m.graphTop != top {
		t.Errorf("selected %d with the graph at row %d, want 30 at row %d", m.selected, m.graphTop, top)
	}
	if m.detailsScroll != 3 {
		t.Errorf("details scrolled to %d, want 3 kept", m.detailsScroll)
	}
	if c := m.commits[30]; !c.DiffLoaded || c.DiffBody != "the diff that was on screen" {
		t.Error("the diff already read was dropped, so the panel would show it loading again")
	}
	if cmd != nil {
		t.Error("the selected commit's diff was asked for again")
	}
	if len(m.remoteTags) != 1 {
		t.Error("the remotes' tags were forgotten before they could answer again")
	}
}

// The working tree is what a refresh is usually for, so its diff is read
// again — but the old one stays up until the new one arrives.
func TestRefreshReadsTheWorkingTreeAgain(t *testing.T) {
	dir, _, _ := dirtyRepo(t)
	write(t, dir, "first", "changed")
	m := loadedModel(t, dir)
	if !m.commits[0].WorkingTree {
		t.Fatal("no working tree row")
	}
	m.selected = 0
	m.commits[0].DiffLoaded = true
	m.commits[0].DiffBody = "old changes"

	res, _ := m.Update(keyPress("r"))
	res, cmd := res.(model).Update(loadRepo(dir)())
	m = res.(model)

	if c := m.commits[0]; !c.WorkingTree || !c.DiffLoaded || c.DiffBody != "old changes" {
		t.Errorf("working tree row = loaded %v, body %q; want the old diff kept for now", c.DiffLoaded, c.DiffBody)
	}
	if cmd == nil {
		t.Fatal("the working tree's diff was not read again")
	}
	res, _ = m.Update(cmd())
	if body := res.(model).commits[0].DiffBody; body == "old changes" || !strings.Contains(body, "changed") {
		t.Errorf("diff after the reload = %q, want the working tree as it is now", body)
	}
}
