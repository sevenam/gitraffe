package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// mergedRepo has a merge commit at the top of main: "merge side", with the
// branch it absorbed and the commits either side of it below.
func mergedRepo(t *testing.T) (dir string) {
	t.Helper()
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("base")
	git("checkout", "-qb", "side")
	commit("on side")
	git("checkout", "-q", "main")
	commit("on main")
	git("merge", "-q", "--no-ff", "side", "-m", "merge side")
	return dir
}

func graphOf(m model) string {
	m.windowWidth, m.windowHeight = 100, 24
	return ansi.Strip(m.View())
}

func TestMergeCommitIsDrawnWithItsOwnMarker(t *testing.T) {
	m := loadedModel(t, mergedRepo(t))
	if m.commits[0].Message != "merge side" {
		t.Fatalf("the newest commit is %q, want the merge", m.commits[0].Message)
	}

	// Selected, the merge is ringed like any selected commit — but as a
	// diamond still, or selecting it would hide that it is a merge.
	m.selected = 0
	screen := graphOf(m)
	if !strings.Contains(screen, "◈") || strings.Contains(screen, "◆") || strings.Contains(screen, "◉") {
		t.Errorf("with the merge selected the graph is\n%s\nwant it drawn ◈, and no other commit ringed", screen)
	}

	// Not selected, it is the plain diamond, and the selected commit the
	// ringed dot it always was.
	m.selected = 1
	screen = graphOf(m)
	if !strings.Contains(screen, "◆") || strings.Contains(screen, "◈") || !strings.Contains(screen, "◉") {
		t.Errorf("with another commit selected the graph is\n%s\nwant the merge drawn ◆ and the selection ◉", screen)
	}
	if got := strings.Count(screen, "◆"); got != 1 {
		t.Errorf("%d commits are drawn as merges, want only the one that is", got)
	}
}

// The working-tree row puts its marker in the column of the commit it sits
// on, which it finds by that commit's marker — whichever marker that is.
func TestWorkingTreeRowSitsOnAMergeCommit(t *testing.T) {
	dir := mergedRepo(t)
	write(t, dir, "base", "changed")

	m := loadedModel(t, dir)
	if !m.commits[0].WorkingTree || m.commits[1].Message != "merge side" {
		t.Fatal("want the working tree row directly above the merge commit")
	}
	top := []rune(m.displayRows[0].GraphChars)
	below := []rune(m.displayRows[1].GraphChars)
	column := strings.IndexRune(string(below), '◆')
	if column < 0 {
		t.Fatalf("the merge is drawn %q, want its own marker", string(below))
	}
	at := len([]rune(string(below)[:column]))
	if at >= len(top) || string(top[at]) != workingTreeMarker {
		t.Errorf("working tree row %q does not put its marker above the merge in %q", string(top), string(below))
	}
}
