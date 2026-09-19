package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// dirtyRepo is a repository with one commit, plus whatever the caller writes
// into it afterwards.
func dirtyRepo(t *testing.T) (dir string, git func(...string), commit func(string)) {
	t.Helper()
	dir, git, commit = gitFixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	return dir, git, commit
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCleanRepoHasNoWorkingTreeRow(t *testing.T) {
	dir, _, _ := dirtyRepo(t)
	m := loadedModel(t, dir)
	if m.commits[0].WorkingTree {
		t.Error("a clean working tree still got a row")
	}
}

func TestWorkingTreeRowCounts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, dir string, git func(...string))
		want  string
	}{
		{"a modified file", func(t *testing.T, dir string, _ func(...string)) {
			write(t, dir, "first", "changed")
		}, "1 changed"},
		{"an untracked file", func(t *testing.T, dir string, _ func(...string)) {
			write(t, dir, "new.txt", "hello")
		}, "1 untracked"},
		{"both at once", func(t *testing.T, dir string, _ func(...string)) {
			write(t, dir, "first", "changed")
			write(t, dir, "new.txt", "hello")
		}, "1 changed, 1 untracked"},
		// Staged or not, it is work in progress: one number covers both.
		{"a staged file", func(t *testing.T, dir string, git func(...string)) {
			write(t, dir, "staged.txt", "hello")
			git("add", "staged.txt")
		}, "1 changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, git, _ := dirtyRepo(t)
			tc.setup(t, dir, git)

			m := loadedModel(t, dir)
			if !m.commits[0].WorkingTree {
				t.Fatal("no working tree row")
			}
			if m.commits[0].Message != tc.want {
				t.Errorf("row says %q, want %q", m.commits[0].Message, tc.want)
			}
		})
	}
}

// Every row of the graph points at a commit by index, so inserting one at the
// top has to move all of them along.
func TestWorkingTreeRowKeepsTheGraphLinedUp(t *testing.T) {
	dir, _, commit := dirtyRepo(t)
	commit("second")
	write(t, dir, "first", "changed")

	m := loadedModel(t, dir)
	for _, row := range m.displayRows {
		if row.CommitIdx < 0 {
			continue
		}
		if row.CommitIdx >= len(m.commits) {
			t.Fatalf("row points at commit %d of %d", row.CommitIdx, len(m.commits))
		}
	}
	// The first two rows are the working tree and the commit it sits on.
	if m.displayRows[0].CommitIdx != 0 || !m.commits[0].WorkingTree {
		t.Errorf("the top row is not the working tree")
	}
	if got := m.commits[m.displayRows[1].CommitIdx].Message; got != "second" {
		t.Errorf("the row below it is %q, want the newest commit", got)
	}
	if !strings.Contains(m.displayRows[0].GraphChars, workingTreeMarker) {
		t.Errorf("graph row = %q, want the hollow marker", m.displayRows[0].GraphChars)
	}
}

func TestWorkingTreeRowIsDrawnAndDescribed(t *testing.T) {
	dir, _, _ := dirtyRepo(t)
	write(t, dir, "first", "changed")
	write(t, dir, "new.txt", "hello")

	m := loadedModel(t, dir)
	m.windowWidth, m.windowHeight = 120, 30
	screen := ansi.Strip(m.View())

	if !strings.Contains(screen, "1 changed, 1 untracked") {
		t.Errorf("the counts are not in the graph:\n%s", screen)
	}
	// Selected by default, so the details panel describes it.
	if !strings.Contains(screen, "Uncommitted changes") {
		t.Errorf("the details panel does not describe the working tree:\n%s", screen)
	}
}

func TestWorkingTreeDiffCoversStagedAndUntracked(t *testing.T) {
	dir, git, _ := dirtyRepo(t)
	write(t, dir, "first", "changed\n")
	write(t, dir, "staged.txt", "staged\n")
	git("add", "staged.txt")
	write(t, dir, "new.txt", "hello\n")

	m := loadedModel(t, dir)
	m.detailsContentWidth = 80
	cmd := m.maybeLoadDiff()
	if cmd == nil {
		t.Fatal("no diff was loaded for the working tree")
	}
	msg, ok := cmd().(diffLoadedMsg)
	if !ok {
		t.Fatalf("got %T, want a diff", cmd())
	}

	if !strings.Contains(msg.diffBody, "changed") {
		t.Error("the unstaged change is missing from the diff")
	}
	if !strings.Contains(msg.diffBody, "staged.txt") {
		t.Error("the staged file is missing from the diff")
	}
	// Untracked files have no diff, so they are listed by name instead.
	if !strings.Contains(msg.diffBody, "Untracked files:") || !strings.Contains(msg.diffBody, "new.txt") {
		t.Error("untracked files are not listed")
	}
}

func TestReloadPicksUpNewChanges(t *testing.T) {
	dir, _, _ := dirtyRepo(t)
	m := loadedModel(t, dir)
	if m.commits[0].WorkingTree {
		t.Fatal("the repository started dirty")
	}

	write(t, dir, "first", "changed")
	res, _ := m.Update(keyPress("r"))
	res, _ = res.(model).Update(loadRepo(dir)())
	if got := res.(model); !got.commits[0].WorkingTree {
		t.Error("reloading did not pick up the uncommitted change")
	}
}
