package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
)

// blocksRepo is main in the middle of a merge of feature that stopped on
// blocks.txt, with two conflict blocks (lines 3 and 9 changed both ways), and
// on other.txt with one.
func blocksRepo(t *testing.T) (dir string, gitCmd func(...string)) {
	t.Helper()
	dir, gitCmd = stagingRepo(t)
	file := func(three, nine string) string {
		return "one\ntwo\n" + three + "\nfour\nfive\nsix\nseven\neight\n" + nine + "\nten\neleven\n"
	}
	write(t, dir, "blocks.txt", file("three", "nine"))
	write(t, dir, "other.txt", "x\n")
	gitCmd("add", "-A")
	gitCmd("commit", "-qm", "base")
	gitCmd("switch", "-qc", "feature")
	write(t, dir, "blocks.txt", file("THREE", "NINE"))
	write(t, dir, "other.txt", "X, theirs\n")
	gitCmd("commit", "-qam", "theirs")
	gitCmd("switch", "-q", "main")
	write(t, dir, "blocks.txt", file("three, ours", "nine, ours"))
	write(t, dir, "other.txt", "x, ours\n")
	gitCmd("commit", "-qam", "ours")
	git.Run(dir, "merge", "feature")
	if out, _ := git.Run(dir, "ls-files", "--unmerged"); out == "" {
		t.Fatal("the merge did not conflict")
	}
	return dir, gitCmd
}

func unmerged(t *testing.T, dir, path string) bool {
	t.Helper()
	out, _ := git.Run(dir, "ls-files", "--unmerged", "--", path)
	return out != ""
}

func TestConflictBlocksArePickedOneAtATime(t *testing.T) {
	dir, _ := blocksRepo(t)
	m := press(withDiff(t, loadedModel(t, dir)), space)
	m.windowWidth, m.windowHeight = 120, 30
	if f, _ := m.selectedFile(); f.Path != "blocks.txt" {
		t.Fatalf("selected %s", f.Path)
	}
	start := readFile(t, dir, "blocks.txt")

	// n from the file list: the first block, in the diff box.
	m = press(m, keyPress("n"))
	if m.commitView.focus != commitBoxDiff || m.commitView.diffCursor != 2 {
		t.Fatalf("n: focus %v, cursor %d", m.commitView.focus, m.commitView.diffCursor)
	}
	if hints := ansi.Strip(m.View()); !strings.Contains(hints, "o: keep main • t: take feature • b / B: both") {
		t.Errorf("the status line on a block:\n%s", hints)
	}
	// n and N go from block to block, and N back from inside one.
	if next := press(m, keyPress("n")); next.commitView.diffCursor != 12 {
		t.Errorf("n again: cursor %d, want the second block's first line, 12", next.commitView.diffCursor)
	}
	if next := press(m, keyPress("j"), keyPress("j"), keyPress("N")); next.commitView.file == m.commitView.file {
		t.Errorf("N from inside the first block stayed on %d of the same file", next.commitView.diffCursor)
	}

	// Off any block in the diff box, o takes nothing.
	off := press(m, keyPress("k"))
	if next := press(off, keyPress("o")); next.staging || !strings.Contains(next.notice, "on no conflict block") {
		t.Errorf("o off a block: staging=%v, notice %q", next.staging, next.notice)
	}

	// o on the first block: that block only, the file still in conflict, the
	// cursor on to the next block.
	m = do(t, m, keyPress("o"))
	if !strings.Contains(m.notice, "main's lines kept — 1 block left") {
		t.Errorf("notice %q", m.notice)
	}
	afterFirst := readFile(t, dir, "blocks.txt")
	if !strings.HasPrefix(afterFirst, "one\ntwo\nthree, ours\nfour\n") || !strings.Contains(afterFirst, "<<<<<<< HEAD\nnine, ours") {
		t.Errorf("after o:\n%s", afterFirst)
	}
	if !unmerged(t, dir, "blocks.txt") {
		t.Error("blocks.txt was marked resolved with a block left")
	}
	blocks := conflictBlocks(m.stagingDiff())
	if len(blocks) != 1 || m.commitView.diffCursor != blocks[0].Start {
		t.Errorf("cursor %d, blocks %+v", m.commitView.diffCursor, blocks)
	}

	// B on the last: both, feature's first; the file resolved, and the
	// cursor on the next file in conflict.
	m = do(t, m, keyPress("B"))
	want := "one\ntwo\nthree, ours\nfour\nfive\nsix\nseven\neight\nNINE\nnine, ours\nten\neleven\n"
	if got := readFile(t, dir, "blocks.txt"); got != want {
		t.Errorf("after B:\n%s", got)
	}
	if unmerged(t, dir, "blocks.txt") || indexed(t, dir, "blocks.txt") != want {
		t.Error("the last block did not mark blocks.txt resolved")
	}
	if !strings.Contains(m.notice, "the last block — marked resolved") {
		t.Errorf("notice %q", m.notice)
	}
	if f, _ := m.selectedFile(); f.Path != "other.txt" || m.commitView.diffCursor != 0 {
		t.Errorf("selected %s, cursor %d; want other.txt's block", f.Path, m.commitView.diffCursor)
	}

	// Back on blocks.txt, u undoes the last choice first: in conflict
	// again, with the second block back; then the first.
	m = press(m, keyPress("2"), keyPress("k"))
	if f, _ := m.selectedFile(); f.Path != "blocks.txt" {
		t.Fatalf("selected %s", f.Path)
	}
	m = do(t, m, keyPress("u"))
	if got := readFile(t, dir, "blocks.txt"); got != afterFirst || !unmerged(t, dir, "blocks.txt") {
		t.Errorf("after one u (notice %q):\n%s", m.notice, got)
	}
	m = do(t, m, keyPress("u"))
	if got := readFile(t, dir, "blocks.txt"); got != start {
		t.Errorf("after two u (notice %q):\n%s", m.notice, got)
	}
	if m.commitView.focus != commitBoxDiff || m.commitView.diffCursor != 2 {
		t.Errorf("after u: focus %v, cursor %d; want the block put back", m.commitView.focus, m.commitView.diffCursor)
	}

	// Nothing left to undo: u asks before writing the conflicts back.
	m = press(m, keyPress("u"))
	if !m.abortMerge.asking || m.abortMerge.reconflict != "blocks.txt" {
		t.Fatalf("u with nothing to undo: %+v", m.abortMerge)
	}
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "Bring back every conflict in blocks.txt?") {
		t.Errorf("the question:\n%s", screen)
	}
	if next := press(m, keyPress("esc")); next.abortMerge.asking || !strings.Contains(next.notice, "Nothing put back") {
		t.Errorf("esc: notice %q", next.notice)
	}
	m = do(t, m, keyPress("enter"))
	if !strings.Contains(m.notice, "every conflict is back") || !unmerged(t, dir, "blocks.txt") {
		t.Errorf("enter: notice %q", m.notice)
	}
	if out, _ := git.Run(dir, "rev-parse", "-q", "--verify", "MERGE_HEAD"); out == "" {
		t.Error("the question aborted the merge")
	}
}

func TestConflictBlockUndoRefusesAFileChangedSince(t *testing.T) {
	dir, _ := blocksRepo(t)
	m := press(withDiff(t, loadedModel(t, dir)), space)
	m = do(t, press(m, keyPress("n")), keyPress("t"))
	if !strings.Contains(readFile(t, dir, "blocks.txt"), "THREE") {
		t.Fatalf("t did not take the block (notice %q)", m.notice)
	}
	// Edited in the meantime: u will not write over it.
	edited := strings.Replace(readFile(t, dir, "blocks.txt"), "one\n", "ONE, by hand\n", 1)
	write(t, dir, "blocks.txt", edited)
	m = reloadedView(t, m, dir)
	m = do(t, m, keyPress("u"))
	if got := readFile(t, dir, "blocks.txt"); got != edited {
		t.Errorf("u wrote over an edit:\n%s", got)
	}
	if !strings.Contains(m.notice, "changed since the last block choice") {
		t.Errorf("notice %q", m.notice)
	}
	// What is left is the question.
	if m = press(m, keyPress("u")); !m.abortMerge.asking {
		t.Error("u did not ask")
	}
}

func TestConflictBlockPickedFromAStaleView(t *testing.T) {
	dir, _ := blocksRepo(t)
	m := press(withDiff(t, loadedModel(t, dir)), space)
	m = press(m, keyPress("n"))
	// The block changed on disk, unseen.
	write(t, dir, "blocks.txt", strings.Replace(readFile(t, dir, "blocks.txt"), "three, ours", "three, edited", 1))
	m = do(t, m, keyPress("o"))
	if !strings.Contains(m.notice, "changed since it was read") {
		t.Errorf("notice %q", m.notice)
	}
	if !strings.Contains(readFile(t, dir, "blocks.txt"), "three, edited") {
		t.Error("the stale pick changed the file")
	}
}
