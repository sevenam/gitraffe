package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// cappedModel loads dir with room for only limit commits, standing in for the
// 5,000 of a real run without needing a repository that long.
func cappedModel(t *testing.T, dir string, limit int) model {
	t.Helper()
	m := testModel()
	m.repoPath = dir
	m.commitLimit = limit
	res, _ := m.Update(loadRepo(dir)())
	got := res.(model)
	if !got.ready {
		t.Fatalf("%s did not load", dir)
	}
	return got
}

func historyOf(t *testing.T, n int) string {
	t.Helper()
	dir, git, commit := gitFixture(t)
	git("init", "-q", "-b", "main")
	for i := range n {
		commit(string(rune('a' + i)))
	}
	return dir
}

func noteRows(m model) []string {
	var out []string
	for _, row := range m.displayRows {
		if row.Note != "" {
			out = append(out, row.Note)
		}
	}
	return out
}

func TestShortHistoryIsNotMarkedCut(t *testing.T) {
	m := cappedModel(t, historyOf(t, 3), 10)
	if m.moreCommits || len(noteRows(m)) != 0 {
		t.Errorf("moreCommits=%v notes=%v; a whole history needs no note", m.moreCommits, noteRows(m))
	}
}

func TestCutHistorySaysSo(t *testing.T) {
	m := cappedModel(t, historyOf(t, 6), 2)
	if len(m.commits) != 2 {
		t.Fatalf("%d commits, want the limit of 2", len(m.commits))
	}
	if !m.moreCommits {
		t.Fatal("the log was cut but nothing says so")
	}
	notes := noteRows(m)
	if len(notes) != 1 || !strings.Contains(notes[0], "press m") {
		t.Fatalf("notes = %v, want one saying which key continues", notes)
	}
	// It belongs at the bottom, where the history stops.
	if m.displayRows[len(m.displayRows)-1].Note == "" {
		t.Error("the note is not the last row")
	}

	m.windowWidth, m.windowHeight = 120, 30
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "more history") {
		t.Errorf("the note is not drawn:\n%s", screen)
	}
}

func TestLoadMoreReadsTheNextBatch(t *testing.T) {
	dir := historyOf(t, 6)
	m := cappedModel(t, dir, 2)
	m.selected = 1
	was := m.commits[1].FullHash

	res, cmd := m.Update(keyPress("m"))
	loading := res.(model)
	if loading.commitLimit != 2+commitBatch || cmd == nil {
		t.Fatalf("limit=%d cmd=%v; want the next batch asked for", loading.commitLimit, cmd != nil)
	}
	res, _ = loading.Update(loadRepo(dir)())
	got := res.(model)

	if len(got.commits) != 6 || got.moreCommits || len(noteRows(got)) != 0 {
		t.Errorf("%d commits, moreCommits=%v, notes=%v; want the whole history and no note",
			len(got.commits), got.moreCommits, noteRows(got))
	}
	// Reading more is a reload, so it keeps your place like one.
	if got.commits[got.selected].FullHash != was {
		t.Errorf("selected %q, want the commit that was selected before", got.commits[got.selected].Message)
	}
}

func TestLoadMoreDoesNothingWhenEverythingIsLoaded(t *testing.T) {
	m := cappedModel(t, historyOf(t, 3), 10)
	res, cmd := m.Update(keyPress("m"))
	if cmd != nil || res.(model).commitLimit != 10 {
		t.Error("m read again although the whole history was already on screen")
	}
}

// Whatever was loaded with "m" has to survive r, or reloading would quietly
// shorten the history again.
func TestReloadKeepsTheLoadedHistory(t *testing.T) {
	dir := historyOf(t, 6)
	m := cappedModel(t, dir, 2)
	res, _ := m.Update(keyPress("m"))
	res, _ = res.(model).Update(loadRepo(dir)())
	expanded := res.(model)

	res, _ = expanded.Update(keyPress("r"))
	res, _ = res.(model).Update(loadRepo(dir)())
	if got := res.(model); got.commitLimit != expanded.commitLimit || len(got.commits) != 6 {
		t.Errorf("limit=%d commits=%d; want the expanded history kept", got.commitLimit, len(got.commits))
	}
}

func TestThousands(t *testing.T) {
	for in, want := range map[int]string{5000: "5,000", 10000: "10,000", 999: "999", 1234567: "1,234,567"} {
		if got := thousands(in); got != want {
			t.Errorf("thousands(%d) = %s, want %s", in, got, want)
		}
	}
}
