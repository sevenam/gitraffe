package git

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// twoBlockRepo is main in the middle of a merge of feature that stopped on
// blocks.txt with two conflicts, lines 3 and 9 of eleven changed both ways,
// in the given merge.conflictStyle ("" for git's default). eol is each line's
// ending, "\n" or "\r\n".
func twoBlockRepo(t *testing.T, style, eol string) (dir string, git func(...string)) {
	t.Helper()
	dir, git = stageRepo(t)
	if style != "" {
		git("config", "merge.conflictStyle", style)
	}
	file := func(three, nine string) string {
		lines := []string{"one", "two", three, "four", "five", "six", "seven", "eight", nine, "ten", "eleven"}
		return strings.Join(lines, eol) + eol
	}
	write(t, dir, "blocks.txt", file("three", "nine"))
	git("add", "-A")
	git("commit", "-qm", "base")
	git("switch", "-qc", "feature")
	write(t, dir, "blocks.txt", file("THREE, theirs", "NINE, theirs"))
	git("commit", "-qam", "theirs")
	git("switch", "-q", "main")
	write(t, dir, "blocks.txt", file("three, ours", "nine, ours"))
	git("commit", "-qam", "ours")
	Run(dir, "merge", "feature")
	if out, _ := Run(dir, "ls-files", "--unmerged", "blocks.txt"); out == "" {
		t.Fatal("the merge did not conflict")
	}
	return dir, git
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// checkedMarkers is the lines, from 0, git diff --check calls leftover
// conflict markers in text, read in a repository of its own where text is
// a change to an empty file.
func checkedMarkers(t *testing.T, text string, size int) []int {
	t.Helper()
	dir, git, _ := gittest.Fixture(t)
	git("init", "-q")
	git("config", "core.autocrlf", "false")
	if size != DefaultMarkerSize {
		write(t, dir, ".gitattributes", "*.txt conflict-marker-size="+strconv.Itoa(size)+"\n")
	}
	write(t, dir, "f.txt", "")
	git("add", "-A")
	git("commit", "-qm", "empty")
	write(t, dir, "f.txt", text)
	out, _ := runChecked(dir, "-c", "core.whitespace=-trailing-space,-space-before-tab,-indent-with-non-tab,-cr-at-eol,-blank-at-eof",
		"diff", "--check", "--no-color")
	var lines []int
	for _, l := range strings.Split(out, "\n") {
		at, ok := strings.CutSuffix(l, ": leftover conflict marker")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(at[strings.LastIndex(at, ":")+1:])
		if err != nil {
			t.Fatalf("cannot read %q", l)
		}
		lines = append(lines, n-1)
	}
	return lines
}

// markerLines is every marker line of blocks, in order.
func markerLines(blocks []ConflictBlock) []int {
	var lines []int
	for _, b := range blocks {
		lines = append(lines, b.Start)
		if b.Base >= 0 {
			lines = append(lines, b.Base)
		}
		lines = append(lines, b.Middle, b.End)
	}
	sort.Ints(lines)
	return lines
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The blocks of what git writes in each style are the lines git diff --check
// finds, which is what MarkResolved refuses on: when no block is left, the
// file can be marked resolved.
func TestConflictBlocksOfEachStyle(t *testing.T) {
	for _, c := range []struct{ style, eol string }{
		{"", "\n"}, {"merge", "\n"}, {"diff3", "\n"}, {"zdiff3", "\n"}, {"", "\r\n"}, {"diff3", "\r\n"},
	} {
		t.Run(c.style+strconv.Quote(c.eol), func(t *testing.T) {
			dir, _ := twoBlockRepo(t, c.style, c.eol)
			text := readFile(t, dir, "blocks.txt")
			lines := strings.Split(text, "\n")
			blocks := ConflictBlocks(lines, DefaultMarkerSize)
			if len(blocks) != 2 {
				t.Fatalf("%d blocks in\n%s", len(blocks), text)
			}
			if got, want := markerLines(blocks), checkedMarkers(t, text, DefaultMarkerSize); !sameInts(got, want) {
				t.Errorf("markers on lines %v, git diff --check finds %v in\n%s", got, want, text)
			}
			trim := func(ls []string) string { return strings.ReplaceAll(strings.Join(ls, "|"), "\r", "") }
			if got := trim(blocks[0].Ours(lines)); got != "three, ours" {
				t.Errorf("first block's ours = %q", got)
			}
			if got := trim(blocks[1].Theirs(lines)); got != "NINE, theirs" {
				t.Errorf("second block's theirs = %q", got)
			}
			hasBase := c.style == "diff3" || c.style == "zdiff3"
			if (blocks[0].Base >= 0) != hasBase {
				t.Errorf("base marker at %d, want one: %v", blocks[0].Base, hasBase)
			} else if hasBase {
				if got := trim(blocks[0].BaseLines(lines)); got != "three" {
					t.Errorf("first block's base = %q", got)
				}
			}
		})
	}
}

func TestConflictBlocksPassOverWhatIsNotAMarker(t *testing.T) {
	lines := []string{
		"=======",           // 0: no conflict open
		">>>>>>> stray",     // 1
		"<<<<<<<x",          // 2: no space after it
		"<<<<<<<< eight",    // 3: longer than the file's markers
		"<<<<<<< HEAD",      // 4: a block, started again below
		"text of the file",  // 5
		"<<<<<<< HEAD",      // 6: the block
		"ours",              // 7
		"<<<<<<<<< inner",   // 8: a merged base's own conflict, as text
		"||||||| base",      // 9
		"base",              // 10
		"=======",           // 11
		"theirs",            // 12
		"=======",           // 13: in the incoming side only >>>>>>> counts
		"<<<<<<< not a new", // 14
		">>>>>>> feature",   // 15
		"after",             // 16
		"<<<<<<< never",     // 17: never closed
		"=======",           // 18
	}
	blocks := ConflictBlocks(lines, DefaultMarkerSize)
	want := []ConflictBlock{{Start: 6, Base: 9, Middle: 11, End: 15}}
	if len(blocks) != len(want) || blocks[0] != want[0] {
		t.Fatalf("blocks = %+v, want %+v", blocks, want)
	}
	// Each marker read is one git diff --check finds too; it finds more,
	// since it reads a line at a time.
	checked := map[int]bool{}
	for _, n := range checkedMarkers(t, strings.Join(lines, "\n")+"\n", DefaultMarkerSize) {
		checked[n] = true
	}
	for _, n := range markerLines(blocks) {
		if !checked[n] {
			t.Errorf("line %d %q is read as a marker, and git diff --check does not", n, lines[n])
		}
	}
	for _, n := range []int{2, 3, 8} {
		if checked[n] {
			t.Errorf("git diff --check takes line %d %q for a marker", n, lines[n])
		}
	}

	// With conflict-marker-size=9, the inner line is a marker, and the
	// seven-long ones are not.
	nine := []string{"<<<<<<<<< a", "<<<<<<< x", "=========", "=======", ">>>>>>>>> b"}
	if got := ConflictBlocks(nine, 9); len(got) != 1 || got[0] != (ConflictBlock{0, -1, 2, 4}) {
		t.Errorf("size 9: %+v", got)
	}
	if got, want := markerLines(ConflictBlocks(nine, 9)), checkedMarkers(t, strings.Join(nine, "\n")+"\n", 9); !sameInts(got, want) {
		t.Errorf("size 9: markers %v, git diff --check %v", got, want)
	}
}

func TestResolveBlockText(t *testing.T) {
	lines := []string{"a", "<<<<<<< HEAD", "o1", "o2", "||||||| base", "b", "=======", "t1", ">>>>>>> x", "z"}
	b := ConflictBlocks(lines, DefaultMarkerSize)[0]
	for pick, want := range map[Pick]string{
		PickOurs:            "a o1 o2 z",
		PickTheirs:          "a t1 z",
		PickBoth:            "a o1 o2 t1 z",
		PickBothTheirsFirst: "a t1 o1 o2 z",
	} {
		if got := strings.Join(ResolveBlockText(lines, b, pick), " "); got != want {
			t.Errorf("pick %d: %q, want %q", pick, got, want)
		}
	}
}

// shownBlock is block i of a file as the conflict view shows it.
func shownBlock(t *testing.T, dir, path string, i int) []string {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(readFile(t, dir, path), "\r", ""), "\n")
	b := ConflictBlocks(lines, DefaultMarkerSize)[i]
	return lines[b.Start : b.End+1]
}

func TestResolveBlockOneAtATimeThenUndo(t *testing.T) {
	dir, _ := twoBlockRepo(t, "diff3", "\r\n")
	start := readFile(t, dir, "blocks.txt")

	second, err := ResolveBlock(dir, "blocks.txt", 1, shownBlock(t, dir, "blocks.txt", 1), PickTheirs)
	if err != nil {
		t.Fatal(err)
	}
	if second.Left != 1 || second.Resolved || second.Before != start {
		t.Errorf("result = %+v", second)
	}
	now := readFile(t, dir, "blocks.txt")
	if now != second.After || !strings.Contains(now, "eight\r\nNINE, theirs\r\nten\r\n") || !strings.Contains(now, "<<<<<<< HEAD\r\nthree, ours") {
		t.Errorf("after the second block:\n%q", now)
	}
	if !isUnmerged(dir, "blocks.txt") {
		t.Error("the file was marked resolved with a block left")
	}

	// A block that is not as shown is refused, and the file left alone.
	stale := shownBlock(t, dir, "blocks.txt", 0)
	stale[1] = "something else"
	if _, err := ResolveBlock(dir, "blocks.txt", 0, stale, PickOurs); !errors.Is(err, ErrBlockChanged) {
		t.Errorf("stale block: err = %v", err)
	}
	if _, err := ResolveBlock(dir, "blocks.txt", 1, stale, PickOurs); !errors.Is(err, ErrBlockChanged) {
		t.Errorf("a block that is gone: err = %v", err)
	}
	if readFile(t, dir, "blocks.txt") != now {
		t.Error("a refused block changed the file")
	}

	first, err := ResolveBlock(dir, "blocks.txt", 0, shownBlock(t, dir, "blocks.txt", 0), PickBothTheirsFirst)
	if err != nil {
		t.Fatal(err)
	}
	want := "one\r\ntwo\r\nTHREE, theirs\r\nthree, ours\r\nfour\r\nfive\r\nsix\r\nseven\r\neight\r\nNINE, theirs\r\nten\r\neleven\r\n"
	if got := readFile(t, dir, "blocks.txt"); got != want || first.After != want {
		t.Errorf("after both blocks:\n%q\nwant\n%q", got, want)
	}
	if !first.Resolved || first.Left != 0 || first.MarkErr != nil || isUnmerged(dir, "blocks.txt") {
		t.Errorf("the last block did not mark the file resolved: %+v", first)
	}
	if indexed(t, dir, "blocks.txt") != want {
		t.Error("what was staged is not the file")
	}

	// Undo, the last first: the file is unmerged again, with the first
	// block back, and then as git left it.
	if out, err := RestoreConflictText(dir, "blocks.txt", first.Before, first.After); err != nil {
		t.Fatal(out, err)
	}
	if readFile(t, dir, "blocks.txt") != now || !isUnmerged(dir, "blocks.txt") {
		t.Error("the undo of the last block did not put the file back in conflict as it was")
	}
	// The text it would undo is no longer on disk.
	if _, err := RestoreConflictText(dir, "blocks.txt", first.Before, first.After); !errors.Is(err, ErrResolvedByHand) {
		t.Errorf("undo of a text that is gone: err = %v", err)
	}
	if _, err := RestoreConflictText(dir, "blocks.txt", second.Before, second.After); err != nil {
		t.Fatal(err)
	}
	if readFile(t, dir, "blocks.txt") != start {
		t.Error("the file is not as git left it")
	}
}

func TestResolveBlockLeavesTheFileUnresolvedOverStrayMarkers(t *testing.T) {
	dir, _ := twoBlockRepo(t, "", "\n")
	text := readFile(t, dir, "blocks.txt")
	write(t, dir, "blocks.txt", "=======\n"+text)
	for i := 0; i < 2; i++ {
		r, err := ResolveBlock(dir, "blocks.txt", 0, shownBlock(t, dir, "blocks.txt", 0), PickOurs)
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 && (r.Resolved || !errors.Is(r.MarkErr, ErrConflictMarkers)) {
			t.Errorf("the last block, a stray marker left: %+v", r)
		}
	}
	if !isUnmerged(dir, "blocks.txt") {
		t.Error("a file with a marker left was marked resolved")
	}
}

func TestRecreateConflicts(t *testing.T) {
	dir, _ := twoBlockRepo(t, "", "\n")
	if _, err := ResolveBlock(dir, "blocks.txt", 0, shownBlock(t, dir, "blocks.txt", 0), PickOurs); err != nil {
		t.Fatal(err)
	}
	if out, err := RecreateConflicts(dir, "blocks.txt"); err != nil {
		t.Fatal(out, err)
	}
	// git checkout -m labels the sides "ours" and "theirs", where the
	// merge named them; the blocks are what count.
	now := readFile(t, dir, "blocks.txt")
	if n := len(ConflictBlocks(strings.Split(now, "\n"), DefaultMarkerSize)); n != 2 {
		t.Errorf("%d blocks back, want 2:\n%s", n, now)
	}
	if _, err := RecreateConflicts(dir, "numbered.txt"); !errors.Is(err, ErrNotUnmerged) {
		t.Errorf("a file not in conflict: err = %v", err)
	}
}

func TestMarkerSizes(t *testing.T) {
	dir, _ := stageRepo(t)
	write(t, dir, ".gitattributes", "*.md conflict-marker-size=12\n")
	got := MarkerSizes(dir, "a.md", "b.txt")
	if got["a.md"] != 12 || got["b.txt"] != DefaultMarkerSize {
		t.Errorf("sizes = %v", got)
	}
}
