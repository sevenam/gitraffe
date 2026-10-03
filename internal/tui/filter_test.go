package tui

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/gittest"
)

// filterRepo has notes.txt changed by some commits and not others, and a
// commit that changes two files, so a file list has more than one entry.
func filterRepo(t *testing.T) string {
	t.Helper()
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "notes.txt"), "one")
	git("add", "-A")
	git("commit", "-qm", "start notes")
	commit("unrelated")
	writeFile(t, filepath.Join(dir, "a-first.txt"), "a")
	writeFile(t, filepath.Join(dir, "notes.txt"), "two")
	git("add", "-A")
	git("commit", "-qm", "notes and more")
	commit("later")
	return dir
}

// finishReload feeds the model the answer the reload it started is waiting for.
func finishReload(t *testing.T, m model) model {
	t.Helper()
	if m.ready {
		t.Fatal("no reload was started")
	}
	got := res(m.Update(loadRepo(m.repoPath)()))
	if !got.ready {
		t.Fatal("the reload did not finish")
	}
	return got
}

func TestFilterPromptShowsOnlyTheFilesHistory(t *testing.T) {
	dir := filterRepo(t)
	m := loadedModel(t, dir)
	// Uncommitted work elsewhere: its row would read as a change to the file.
	writeFile(t, filepath.Join(dir, "unrelated"), "edited")

	m = press(m, keyPress("F"))
	if !m.filterPrompt.active {
		t.Fatal("F did not open the filter prompt")
	}
	if !strings.Contains(stripANSI(m.renderStatusLine()), "filter:") {
		t.Errorf("the status line does not show the prompt: %q", stripANSI(m.renderStatusLine()))
	}
	m = typeText(m, "notes.txt")
	m = finishReload(t, press(m, tea.KeyMsg{Type: tea.KeyEnter}))

	if got, want := messages(m), []string{"notes and more", "start notes"}; !slices.Equal(got, want) {
		t.Errorf("filtered graph = %q, want %q", got, want)
	}
	if got := stripANSI(m.renderRepoInfo()); !strings.Contains(got, "Filter: notes.txt") {
		t.Errorf("the top box does not say what the graph is filtered to: %q", got)
	}
	if got := stripANSI(m.renderStatusLine()); !strings.Contains(got, "History of notes.txt") {
		t.Errorf("status line after filtering = %q", got)
	}
}

// filterTo filters the graph the way a user does, with the prompt.
func filterTo(t *testing.T, m model, path string) model {
	t.Helper()
	m = typeText(press(m, keyPress("F")), path)
	return finishReload(t, press(m, tea.KeyMsg{Type: tea.KeyEnter}))
}

func selectMessage(t *testing.T, m model, message string) model {
	t.Helper()
	for i, c := range m.commits {
		if c.Message == message {
			m.selected = i
			return m
		}
	}
	t.Fatalf("no commit %q in %q", message, messages(m))
	return m
}

// Esc on the graph gives the whole history back, still on the commit you were
// looking at.
func TestEscClearsTheFilterAndKeepsYourPlace(t *testing.T) {
	m := filterTo(t, loadedModel(t, filterRepo(t)), "notes.txt")
	m = selectMessage(t, m, "start notes")

	m = finishReload(t, press(m, tea.KeyMsg{Type: tea.KeyEsc}))
	if !m.filter.IsZero() {
		t.Errorf("filter after esc = %+v, want none", m.filter)
	}
	if len(m.commits) != 4 {
		t.Errorf("after esc the graph has %q, want all four commits", messages(m))
	}
	if got := selectedMessage(m); got != "start notes" {
		t.Errorf("after esc %q is selected, want the commit you were on", got)
	}

	// With no filter on, esc on the graph does nothing: it must not reload.
	if again := press(m, tea.KeyMsg{Type: tea.KeyEsc}); !again.ready {
		t.Error("esc with no filter started a reload")
	}
}

// Esc in the prompt abandons it: nothing is read and the filter is unchanged.
func TestEscInTheFilterPromptChangesNothing(t *testing.T) {
	m := loadedModel(t, filterRepo(t))
	m = press(typeText(press(m, keyPress("F")), "notes.txt"), tea.KeyMsg{Type: tea.KeyEsc})
	if m.filterPrompt.active || !m.ready || !m.filter.IsZero() {
		t.Errorf("after esc: prompt open %v, ready %v, filter %+v; want closed, ready, none",
			m.filterPrompt.active, m.ready, m.filter)
	}
}

// "r" means "show me this again", which is the filtered graph.
func TestReloadKeepsTheFilter(t *testing.T) {
	m := filterTo(t, loadedModel(t, filterRepo(t)), "notes.txt")
	m = finishReload(t, press(m, keyPress("r")))
	if m.filter.Path != "notes.txt" || len(m.commits) != 2 {
		t.Errorf("after r: filter %+v, commits %q; want notes.txt's two", m.filter, messages(m))
	}
}

// Opening another repository drops the filter: the path was this one's.
func TestSwitchingRepositoryDropsTheFilter(t *testing.T) {
	m := filterTo(t, loadedModel(t, filterRepo(t)), "notes.txt")
	next, _ := m.switchRepo(gittest.NewRepo(t))
	if !next.filter.IsZero() {
		t.Errorf("filter after switching = %+v, want none", next.filter)
	}
}

// h in the commit view's file list is the way to the history of a file you
// have found in a commit, without typing its path.
func TestCommitViewHShowsTheSelectedFilesHistory(t *testing.T) {
	m := loadedModel(t, filterRepo(t))
	m = selectMessage(t, m, "notes and more")
	m = press(m, keyPress(" "))
	m = res(m.Update(loadDiffCmd(m.repoPath, m.commits[m.selected].FullHash, m.selected, 80)()))
	files := m.viewedFiles()
	if len(files) != 2 {
		t.Fatalf("files = %+v, want two", files)
	}
	for m.viewedFiles()[m.commitView.file].Path != "notes.txt" {
		m = press(m, keyPress("j"))
	}

	m = finishReload(t, press(m, keyPress("h")))
	if m.commitView.open {
		t.Error("the commit view is still open over the filtered graph")
	}
	if m.filter.Path != "notes.txt" {
		t.Errorf("filter = %+v, want notes.txt", m.filter)
	}
	if got := selectedMessage(m); got != "notes and more" {
		t.Errorf("%q is selected, want the commit h was pressed on", got)
	}

	// Opened again on a filtered graph, the view starts on the filtered file
	// rather than on whichever sorts first.
	m = press(m, keyPress(" "))
	m = res(m.Update(loadDiffCmd(m.repoPath, m.commits[m.selected].FullHash, m.selected, 80)()))
	if got := m.viewedFiles()[m.commitView.file].Path; got != "notes.txt" {
		t.Errorf("the commit view opened on %q, want notes.txt", got)
	}
}

// h only means something where a file is what is selected.
func TestCommitViewHOutsideTheFileListDoesNothing(t *testing.T) {
	m := loadedModel(t, filterRepo(t))
	m = press(m, keyPress(" "))
	m = res(m.Update(loadDiffCmd(m.repoPath, m.commits[m.selected].FullHash, m.selected, 80)()))
	m.commitView.focus = commitBoxDiff
	m = press(m, keyPress("h"))
	if !m.commitView.open || !m.ready || !m.filter.IsZero() {
		t.Error("h in the diff box filtered the graph")
	}
}

// A path no commit changed is an empty graph, and the screen says why.
func TestFilterWithNoCommitsSaysSo(t *testing.T) {
	m := filterTo(t, loadedModel(t, filterRepo(t)), "missing.txt")
	if len(m.commits) != 0 {
		t.Errorf("commits = %q, want none", messages(m))
	}
	if got := stripANSI(m.renderStatusLine()); !strings.Contains(got, "No commit changed missing.txt") {
		t.Errorf("status line = %q, want it to say nothing changed the file", got)
	}
	// And the screen still draws, at its full height.
	if got := strings.Count(m.View(), "\n") + 1; got != m.windowHeight {
		t.Errorf("View is %d lines, want %d", got, m.windowHeight)
	}
}

// What is typed becomes the path git is given, whichever way it was written.
func TestFilterPathIsTheRepositorysOwn(t *testing.T) {
	root := t.TempDir()
	m := testModel()
	m.repoRoot = root
	for typed, want := range map[string]string{
		"  notes.txt ":                      "notes.txt",
		`docs\guide.md`:                     "docs/guide.md",
		"./docs/":                           "docs",
		".":                                 "",
		"":                                  "",
		filepath.Join(root, "docs", "a.md"): "docs/a.md",
	} {
		if got := m.filterPath(typed); got != want {
			t.Errorf("filterPath(%q) = %q, want %q", typed, got, want)
		}
	}
}

// A rename is listed as "old → new"; its history is the file's under its new
// name.
func TestCurrentPathOfARename(t *testing.T) {
	if got := currentPath("old.go → new.go"); got != "new.go" {
		t.Errorf("currentPath = %q, want new.go", got)
	}
	m := testModel()
	m.filter = git.Filter{Path: "pkg"}
	files := []fileDiff{{Path: "README.md"}, {Path: "pkg/a.go"}}
	if got := m.filteredFile(files); got != 1 {
		t.Errorf("filteredFile for a directory filter = %d, want the file under it", got)
	}
}

// The auto-refresh must not reload under an open prompt, or the prompt would
// close mid-word.
func TestAutoRefreshWaitsForTheFilterPrompt(t *testing.T) {
	m := loadedModel(t, filterRepo(t))
	m = press(m, keyPress("F"))
	if m.canReloadUnasked() {
		t.Error("an unasked reload may run while the filter prompt is open")
	}
}
