package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/gittest"
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
	dir, git, commit := gittest.Fixture(t)
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
	if len(notes) != 1 || notes[0] != moreHistoryNote {
		t.Fatalf("notes = %v, want the one saying there is more", notes)
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

// Reaching the last row reads the next batch, once: a key pressed while it
// loads asks for nothing more.
func TestReachingTheEndReadsTheNextBatch(t *testing.T) {
	dir := historyOf(t, 6)
	m := cappedModel(t, dir, 2)
	was := m.commits[1].FullHash

	res, cmd := m.Update(keyPress("j"))
	loading := res.(model)
	if loading.commitLimit != 2+commitBatch || cmd == nil {
		t.Fatalf("limit=%d cmd=%v; want the next batch asked for", loading.commitLimit, cmd != nil)
	}
	// The old screen stays up while it loads, saying what is happening.
	if loading.stale == nil || len(noteRows(*loading.stale)) != 1 || noteRows(*loading.stale)[0] != readingHistoryNote {
		t.Fatalf("the screen shown while loading does not say it is reading")
	}
	loading.windowWidth, loading.windowHeight = 120, 30
	if screen := ansi.Strip(loading.View()); !strings.Contains(screen, readingHistoryNote) {
		t.Errorf("the note does not say it is reading:\n%s", screen)
	}

	for _, k := range []string{"j", "G"} {
		again := press(loading, keyPress(k))
		if again.commitLimit != loading.commitLimit {
			t.Errorf("%s while loading asked again: limit %d, want %d", k, again.commitLimit, loading.commitLimit)
		}
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
	// Nothing is left to read, so going down further reads nothing.
	if after := press(got, keyPress("G"), keyPress("j")); after.commitLimit != got.commitLimit || !after.ready {
		t.Error("the bottom of a whole history read again")
	}
}

// Every way down to the last row reads on: the ones that land on it, and
// pushing on from it when something else left the selection there.
func TestEveryWayToTheEndReadsOn(t *testing.T) {
	dir := historyOf(t, 6)
	base := cappedModel(t, dir, 3)
	last := len(base.commits) - 1
	for _, tc := range []struct {
		name  string
		from  int
		input tea.Msg
	}{
		{"G", 0, keyPress("G")},
		{"page down", 0, tea.KeyMsg{Type: tea.KeyPgDown}},
		{"end", 0, tea.KeyMsg{Type: tea.KeyEnd}},
		{"down on the last row", last, tea.KeyMsg{Type: tea.KeyDown}},
		{"j on the last row", last, keyPress("j")},
		{"wheel on the last row", last, wheel(5, firstGraphRow+1, tea.MouseButtonWheelDown)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := base
			m.selected = tc.from
			res, _ := m.Update(tc.input)
			if got := res.(model); got.commitLimit != 3+commitBatch {
				t.Errorf("limit %d, want the next batch asked for", got.commitLimit)
			}
		})
	}
}

func TestClickingTheNoteReadsOn(t *testing.T) {
	m := cappedModel(t, historyOf(t, 6), 2)
	start, _ := m.graphWindow()
	y := firstGraphRow + len(m.displayRows) - 1 - start
	if !m.moreHistoryAt(y) {
		t.Fatalf("row %d is not the note", y)
	}
	res, _ := m.Update(click(5, y))
	if got := res.(model); got.commitLimit != 2+commitBatch {
		t.Errorf("limit %d, want the next batch asked for", got.commitLimit)
	}
}

// A reload that finds its commit on the last row was asked to show the same
// history again, not more of it.
func TestAReloadEndingOnTheLastRowReadsNoMore(t *testing.T) {
	dir := historyOf(t, 6)
	m := cappedModel(t, dir, 2)
	m.selected = 1
	next, _ := m.reloadRepo()
	res, _ := next.Update(loadRepo(dir)())
	if got := res.(model); got.commitLimit != 2 || got.selected != 1 {
		t.Errorf("limit=%d selected=%d; want the same history, same place", got.commitLimit, got.selected)
	}
}

// m is kept for merging, so it no longer reads anything.
func TestMReadsNothing(t *testing.T) {
	m := cappedModel(t, historyOf(t, 6), 2)
	if got := press(m, keyPress("m")); got.commitLimit != 2 || !got.ready {
		t.Error("m read more history")
	}
}

// Whatever was read has to survive r, or reloading would quietly shorten the
// history again.
func TestReloadKeepsTheLoadedHistory(t *testing.T) {
	dir := historyOf(t, 6)
	m := cappedModel(t, dir, 2)
	res, _ := m.Update(keyPress("G"))
	res, _ = res.(model).Update(loadRepo(dir)())
	expanded := res.(model)

	res, _ = expanded.Update(keyPress("r"))
	res, _ = res.(model).Update(loadRepo(dir)())
	if got := res.(model); got.commitLimit != expanded.commitLimit || len(got.commits) != 6 {
		t.Errorf("limit=%d commits=%d; want the expanded history kept", got.commitLimit, len(got.commits))
	}
}

// count runs the background count a loaded model asks for and hands it the
// answer.
func count(t *testing.T, m model) model {
	t.Helper()
	m.historyCountAsked = ""
	cmd := m.countHistory()
	if cmd == nil {
		t.Fatal("no count asked for")
	}
	return res(m.Update(cmd()))
}

// The position is out of the whole history once it is counted, though only
// part of it is loaded.
func TestPositionUsesTheCountedTotal(t *testing.T) {
	dir := historyOf(t, 6)
	m := cappedModel(t, dir, 3)
	m.selected = 1
	if got := m.graphPosition(); got != "2/3+" {
		t.Errorf("before the count, position = %q, want 2/3+", got)
	}
	m = count(t, m)
	if got := m.graphPosition(); got != "2/6 · 33%" {
		t.Errorf("position = %q, want 2/6 · 33%%", got)
	}
}

func TestLoadingACutHistoryAsksForACount(t *testing.T) {
	dir := historyOf(t, 6)
	m := testModel()
	m.repoPath, m.commitLimit = dir, 2
	res, _ := m.Update(loadRepo(dir)())
	if got := res.(model); got.historyCountAsked == "" {
		t.Error("a cut-short history did not ask to be counted")
	}
	m.commitLimit = 100
	res, _ = m.Update(loadRepo(dir)())
	if got := res.(model); got.historyCountAsked != "" {
		t.Error("a whole history asked to be counted; it is all loaded")
	}
}

// The count is the slow part; a reload of an unchanged repository keeps it,
// and one that changed asks again rather than show a total of before.
func TestTheCountIsKeptUntilTheRepositoryChanges(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	for i := range 6 {
		commit(string(rune('a' + i)))
	}
	m := count(t, cappedModel(t, dir, 3))

	next, _ := m.reloadRepo()
	same := res(next.Update(loadRepo(dir)()))
	if same.historyCount != m.historyCount || same.countHistory() != nil {
		t.Errorf("a reload that changed nothing lost the count or asked again")
	}

	commit("g")
	next, _ = same.reloadRepo()
	changed := res(next.Update(loadRepo(dir)()))
	if got := changed.graphPosition(); !strings.HasSuffix(got, "+") {
		t.Errorf("position = %q, want the + form until the new count arrives", got)
	}
	if changed.historyCountAsked == "" {
		t.Error("a changed repository was not counted again")
	}
	changed = count(t, changed)
	if changed.historyCount.total != 7 {
		t.Errorf("counted %d, want 7", changed.historyCount.total)
	}
}

// An answer for another repository, or for the repository as it was, says
// nothing about the history on screen.
func TestALateCountIsDropped(t *testing.T) {
	m := cappedModel(t, historyOf(t, 6), 3)
	key := m.historyKey()
	for _, msg := range []historyCountMsg{
		{repoPath: "elsewhere", key: key, total: 99},
		{repoPath: m.repoPath, key: "an older state", total: 99},
	} {
		if got := m.finishHistoryCount(msg); got.historyCount.total != 0 {
			t.Errorf("%+v was kept", msg)
		}
	}
}

func TestThousands(t *testing.T) {
	for in, want := range map[int]string{5000: "5,000", 10000: "10,000", 999: "999", 1234567: "1,234,567"} {
		if got := thousands(in); got != want {
			t.Errorf("thousands(%d) = %s, want %s", in, got, want)
		}
	}
}
