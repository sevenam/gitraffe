package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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
	dir, git, commit := gitFixture(t)
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
	if !strings.Contains(m.notice, "Reloaded") {
		t.Errorf("notice = %q, want it to say the repository was reloaded", m.notice)
	}
}

func TestReloadKeepsYourPlace(t *testing.T) {
	dir, git, commit := gitFixture(t)
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
	dir, git, commit := gitFixture(t)
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
	dir, git, commit := gitFixture(t)
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
	if !strings.Contains(line, "q/esc: quit") {
		t.Errorf("status line = %q, want quit still on it at 120 columns", line)
	}
}
