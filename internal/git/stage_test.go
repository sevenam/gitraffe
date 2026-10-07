package git

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// stageRepo is a repository on main with one commit holding numbered.txt,
// thirty lines long so that changes at its two ends are two hunks.
func stageRepo(t *testing.T) (dir string, git func(...string)) {
	t.Helper()
	dir, git, _ = gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	gittest.Identify(git)
	// Files are compared byte for byte below; nothing may rewrite line ends.
	git("config", "core.autocrlf", "false")
	write(t, dir, "numbered.txt", numbered(30))
	git("add", "-A")
	git("commit", "-qm", "first")
	return dir, git
}

func numbered(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	return sb.String()
}

// indexed is a file as the index holds it: what a commit made now would.
func indexed(t *testing.T, dir, path string) string {
	t.Helper()
	out, err := rawOutput(dir, "show", ":"+path)
	if err != nil {
		t.Fatalf("%s is not in the index: %v", path, err)
	}
	return out
}

// entry is a file's entry among the uncommitted changes, staged or not.
func entry(t *testing.T, dir, path string, staged bool) FileDiff {
	t.Helper()
	files := WorkingTree(dir, 80).Files
	for _, f := range files {
		if f.Path == path && f.Staged == staged {
			return f
		}
	}
	t.Fatalf("no entry for %q (staged=%v) among %+v", path, staged, describe(files))
	return FileDiff{}
}

// describe is the file list as "path:staged" and the like, for comparing.
func describe(files []FileDiff) []string {
	var out []string
	for _, f := range files {
		kind := "unstaged"
		switch {
		case f.Untracked:
			kind = "untracked"
		case f.Staged:
			kind = "staged"
		}
		out = append(out, f.Path+":"+kind)
	}
	return out
}

// lineAt is the index of the diff line that reads text.
func lineAt(t *testing.T, f FileDiff, text string) int {
	t.Helper()
	for i, line := range strings.Split(f.Body, "\n") {
		if line == text {
			return i
		}
	}
	t.Fatalf("no line %q in:\n%s", text, f.Body)
	return -1
}

// hunkAt is the range of the hunk the line reading text is in: from its
// header to the line before the next.
func hunkAt(t *testing.T, f FileDiff, text string) (first, last int) {
	t.Helper()
	lines := strings.Split(f.Body, "\n")
	at := lineAt(t, f, text)
	first, last = at, len(lines)-1
	for first > 0 && !strings.HasPrefix(lines[first], "@@") {
		first--
	}
	for i := at + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "@@") {
			last = i - 1
			break
		}
	}
	return first, last
}

func mustStage(t *testing.T, dir string, f FileDiff, first, last int) {
	t.Helper()
	if detail, err := StageLines(dir, f, first, last); err != nil {
		t.Fatalf("StageLines(%s, %d..%d): %v\n%s", f.Path, first, last, err, detail)
	}
}

func TestWorkingTreeListsStagedThenUnstagedThenUntracked(t *testing.T) {
	dir, git := stageRepo(t)
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "line two\n", 1))
	git("add", "numbered.txt")
	// A second change to the same file, left out of the index: one file, two
	// entries.
	write(t, dir, "numbered.txt", strings.Replace(strings.Replace(numbered(30), "line 2\n", "line two\n", 1), "line 29\n", "line twenty-nine\n", 1))
	write(t, dir, "added.txt", "new\n")
	git("add", "added.txt")
	write(t, dir, "untracked.txt", "new\n")

	d := WorkingTree(dir, 80)
	want := []string{"added.txt:staged", "numbered.txt:staged", "numbered.txt:unstaged", "untracked.txt:untracked"}
	if got := describe(d.Files); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("files = %q, want %q", got, want)
	}
	if staged := entry(t, dir, "numbered.txt", true); !strings.Contains(staged.Body, "+line two") || strings.Contains(staged.Body, "twenty-nine") {
		t.Errorf("the staged entry does not hold only the staged change:\n%s", staged.Body)
	}
	if unstaged := entry(t, dir, "numbered.txt", false); !strings.Contains(unstaged.Body, "+line twenty-nine") || strings.Contains(unstaged.Body, "+line two") {
		t.Errorf("the unstaged entry does not hold only the unstaged change:\n%s", unstaged.Body)
	}
	if d.State != (WorkingState{}) {
		t.Errorf("state = %+v, want nothing in the way", d.State)
	}
}

// A removed line that began "-- " is written "--- ", which is also how a
// diff's own header starts. It is a line of the diff all the same.
func TestDiffKeepsLinesThatLookLikeHeaders(t *testing.T) {
	dir, git := stageRepo(t)
	write(t, dir, "query.sql", "-- a comment\nselect 1;\n++ odd\n")
	git("add", "-A")
	git("commit", "-qm", "sql")
	write(t, dir, "query.sql", "select 1;\n++ odder\n")

	f := entry(t, dir, "query.sql", false)
	if f.Removed != 2 || f.Added != 1 {
		t.Errorf("counts = +%d -%d, want +1 -2:\n%s", f.Added, f.Removed, f.Body)
	}
	lineAt(t, f, "--- a comment")
	lineAt(t, f, "+++ odder")
}

func TestStageAndUnstageAFile(t *testing.T) {
	dir, git := stageRepo(t)
	// A name full of what a pathspec would read as a pattern.
	odd := "odd [name] é.txt"
	write(t, dir, odd, "one\n")
	write(t, dir, "gone.txt", "soon\n")
	git("add", "-A")
	git("commit", "-qm", "second")

	write(t, dir, odd, "one\ntwo\n")
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "new.txt", "fresh\n")

	for _, path := range []string{odd, "gone.txt", "new.txt"} {
		if detail, err := StageFile(dir, path); err != nil {
			t.Fatalf("StageFile(%q): %v\n%s", path, err, detail)
		}
	}
	want := []string{"gone.txt:staged", "new.txt:staged", odd + ":staged"}
	if got := describe(WorkingTree(dir, 80).Files); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("after staging, files = %q, want %q", got, want)
	}

	for _, path := range []string{odd, "gone.txt", "new.txt"} {
		if detail, err := UnstageFile(dir, path); err != nil {
			t.Fatalf("UnstageFile(%q): %v\n%s", path, err, detail)
		}
	}
	// In the places they had while staged.
	want = []string{"gone.txt:unstaged", "new.txt:untracked", odd + ":unstaged"}
	if got := describe(WorkingTree(dir, 80).Files); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("after unstaging, files = %q, want %q", got, want)
	}
	// Neither touched the files themselves.
	if data, _ := os.ReadFile(filepath.Join(dir, odd)); string(data) != "one\ntwo\n" {
		t.Errorf("%s now reads %q", odd, data)
	}
}

func TestStageAllAndUnstageAll(t *testing.T) {
	dir, _ := stageRepo(t)
	write(t, dir, "numbered.txt", numbered(31))
	write(t, dir, "new.txt", "fresh\n")

	if detail, err := StageAll(dir); err != nil {
		t.Fatalf("StageAll: %v\n%s", err, detail)
	}
	if got := describe(WorkingTree(dir, 80).Files); strings.Join(got, " ") != "new.txt:staged numbered.txt:staged" {
		t.Fatalf("after StageAll, files = %q", got)
	}
	if detail, err := UnstageAll(dir); err != nil {
		t.Fatalf("UnstageAll: %v\n%s", err, detail)
	}
	if got := describe(WorkingTree(dir, 80).Files); strings.Join(got, " ") != "new.txt:untracked numbered.txt:unstaged" {
		t.Fatalf("after UnstageAll, files = %q", got)
	}
}

// Before the first commit there is no HEAD to compare with or restore from.
func TestStagingBeforeTheFirstCommit(t *testing.T) {
	dir, git, _ := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	gittest.Identify(git)
	git("config", "core.autocrlf", "false")
	write(t, dir, "a.txt", "one\ntwo\nthree\n")

	if _, err := StageFile(dir, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if got := describe(WorkingTree(dir, 80).Files); strings.Join(got, " ") != "a.txt:staged" {
		t.Fatalf("files = %q, want the file staged", got)
	}
	if detail, err := UnstageFile(dir, "a.txt"); err != nil {
		t.Fatalf("UnstageFile: %v\n%s", err, detail)
	}
	if got := describe(WorkingTree(dir, 80).Files); strings.Join(got, " ") != "a.txt:untracked" {
		t.Fatalf("files = %q, want the file untracked again", got)
	}

	StageFile(dir, "a.txt")
	r, err := CommitStaged(dir, "the first commit")
	if err != nil || r.Subject != "the first commit" {
		t.Fatalf("commit = %+v, err = %v", r, err)
	}
}

func TestStageOneHunk(t *testing.T) {
	dir, _ := stageRepo(t)
	changed := strings.Replace(strings.Replace(numbered(30), "line 2\n", "line two\n", 1), "line 29\n", "line twenty-nine\n", 1)
	write(t, dir, "numbered.txt", changed)

	f := entry(t, dir, "numbered.txt", false)
	first, last := hunkAt(t, f, "+line twenty-nine")
	mustStage(t, dir, f, first, last)

	want := strings.Replace(numbered(30), "line 29\n", "line twenty-nine\n", 1)
	if got := indexed(t, dir, "numbered.txt"); got != want {
		t.Errorf("the index holds:\n%s\nwant only the second hunk staged", got)
	}
	// The other hunk is what is left, and staging it too leaves nothing.
	rest := entry(t, dir, "numbered.txt", false)
	if !strings.Contains(rest.Body, "+line two") || strings.Contains(rest.Body, "twenty-nine") {
		t.Fatalf("left unstaged:\n%s\nwant only the first hunk", rest.Body)
	}
	first, last = hunkAt(t, rest, "+line two")
	mustStage(t, dir, rest, first, last)
	if got := indexed(t, dir, "numbered.txt"); got != changed {
		t.Errorf("after both hunks the index is not the file")
	}
}

func TestStageSingleLines(t *testing.T) {
	dir, _ := stageRepo(t)
	// One hunk: a line replaced and, next to it, a line added.
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 15\n", "line fifteen\nand a half\n", 1))

	f := entry(t, dir, "numbered.txt", false)
	at := lineAt(t, f, "+and a half")
	mustStage(t, dir, f, at, at)
	// The removal was not picked, so line 15 is still there; neither was its
	// replacement, so that is not.
	want := strings.Replace(numbered(30), "line 15\n", "line 15\nand a half\n", 1)
	if got := indexed(t, dir, "numbered.txt"); got != want {
		t.Fatalf("the index holds:\n%s\nwant only the one added line", got)
	}

	f = entry(t, dir, "numbered.txt", false)
	at = lineAt(t, f, "-line 15")
	mustStage(t, dir, f, at, at)
	want = strings.Replace(numbered(30), "line 15\n", "and a half\n", 1)
	if got := indexed(t, dir, "numbered.txt"); got != want {
		t.Fatalf("the index holds:\n%s\nwant line 15 gone as well", got)
	}
}

func TestUnstageLines(t *testing.T) {
	dir, git := stageRepo(t)
	changed := strings.Replace(strings.Replace(numbered(30), "line 2\n", "line two\n", 1), "line 15\n", "line fifteen\nand a half\n", 1)
	write(t, dir, "numbered.txt", changed)
	git("add", "-A")

	// One line out of the middle hunk.
	f := entry(t, dir, "numbered.txt", true)
	at := lineAt(t, f, "+and a half")
	mustStage(t, dir, f, at, at)
	want := strings.Replace(strings.Replace(numbered(30), "line 2\n", "line two\n", 1), "line 15\n", "line fifteen\n", 1)
	if got := indexed(t, dir, "numbered.txt"); got != want {
		t.Fatalf("the index holds:\n%s\nwant only that line unstaged", got)
	}

	// Then the whole first hunk.
	f = entry(t, dir, "numbered.txt", true)
	first, last := hunkAt(t, f, "+line two")
	mustStage(t, dir, f, first, last)
	want = strings.Replace(numbered(30), "line 15\n", "line fifteen\n", 1)
	if got := indexed(t, dir, "numbered.txt"); got != want {
		t.Fatalf("the index holds:\n%s\nwant the first hunk unstaged too", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "numbered.txt")); string(data) != changed {
		t.Error("unstaging changed the file itself")
	}
}

// Part of a file git has never seen, part of a file being deleted, and part
// of a new file taken back out: a patch of lines cannot say "new" or
// "deleted", so these are where it would go wrong.
func TestStageLinesOfNewAndDeletedFiles(t *testing.T) {
	dir, git := stageRepo(t)

	write(t, dir, "new.txt", "one\ntwo\nthree\n")
	f := entry(t, dir, "new.txt", false)
	if !f.Untracked {
		t.Fatal("new.txt is not listed as untracked")
	}
	at := lineAt(t, f, "+two")
	mustStage(t, dir, f, at, at)
	if got := indexed(t, dir, "new.txt"); got != "two\n" {
		t.Fatalf("the index holds %q of the new file, want only its second line", got)
	}
	if got := describe(WorkingTree(dir, 80).Files); strings.Join(got, " ") != "new.txt:staged new.txt:unstaged" {
		t.Fatalf("files = %q, want the new file half staged", got)
	}

	// All of it staged, then one line taken back.
	git("add", "new.txt")
	f = entry(t, dir, "new.txt", true)
	at = lineAt(t, f, "+three")
	mustStage(t, dir, f, at, at)
	if got := indexed(t, dir, "new.txt"); got != "one\ntwo\n" {
		t.Fatalf("the index holds %q, want the last line unstaged", got)
	}

	if err := os.Remove(filepath.Join(dir, "numbered.txt")); err != nil {
		t.Fatal(err)
	}
	f = entry(t, dir, "numbered.txt", false)
	mustStage(t, dir, f, lineAt(t, f, "-line 1"), lineAt(t, f, "-line 10"))
	if got := indexed(t, dir, "numbered.txt"); got != strings.Join(strings.SplitAfter(numbered(30), "\n")[10:], "") {
		t.Fatalf("the index holds:\n%s\nwant the first ten lines gone and the file still there", got)
	}
}

// Picking every change in a file is staging the file, which git does itself.
func TestStageEveryLineIsStagingTheFile(t *testing.T) {
	dir, _ := stageRepo(t)
	if err := os.Remove(filepath.Join(dir, "numbered.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "new.txt", "one\ntwo\n")

	f := entry(t, dir, "numbered.txt", false)
	mustStage(t, dir, f, 0, strings.Count(f.Body, "\n"))
	f = entry(t, dir, "new.txt", false)
	mustStage(t, dir, f, 0, strings.Count(f.Body, "\n"))

	if got := describe(WorkingTree(dir, 80).Files); strings.Join(got, " ") != "new.txt:staged numbered.txt:staged" {
		t.Fatalf("files = %q, want a staged addition and a staged deletion", got)
	}
	if out, _ := Run(dir, "status", "--porcelain"); out != "A  new.txt\nD  numbered.txt" {
		t.Errorf("status = %q, want the file added and the other deleted", out)
	}
}

func TestStageLinesWithCRLFAndNoFinalNewline(t *testing.T) {
	dir, git := stageRepo(t)
	crlf := strings.ReplaceAll(numbered(30), "\n", "\r\n")
	write(t, dir, "crlf.txt", crlf)
	write(t, dir, "open.txt", strings.TrimSuffix(numbered(30), "\n"))
	git("add", "-A")
	git("commit", "-qm", "second")

	changed := strings.Replace(strings.Replace(crlf, "line 2\r\n", "line two\r\n", 1), "line 29\r\n", "line twenty-nine\r\n", 1)
	write(t, dir, "crlf.txt", changed)
	f := entry(t, dir, "crlf.txt", false)
	first, last := hunkAt(t, f, "+line two")
	mustStage(t, dir, f, first, last)
	if got, want := indexed(t, dir, "crlf.txt"), strings.Replace(crlf, "line 2\r\n", "line two\r\n", 1); got != want {
		t.Errorf("crlf.txt in the index:\n%q\nwant the first hunk staged, line endings kept", got)
	}

	// The last line has no newline and is in the hunk that is not picked.
	open := strings.TrimSuffix(numbered(30), "\n")
	write(t, dir, "open.txt", strings.Replace(strings.Replace(open, "line 2\n", "line two\n", 1), "line 30", "line thirty", 1))
	f = entry(t, dir, "open.txt", false)
	first, last = hunkAt(t, f, "+line two")
	mustStage(t, dir, f, first, last)
	if got, want := indexed(t, dir, "open.txt"), strings.Replace(open, "line 2\n", "line two\n", 1); got != want {
		t.Errorf("open.txt in the index:\n%q\nwant the first hunk staged", got)
	}
	// And the hunk that holds it, a line at a time.
	f = entry(t, dir, "open.txt", false)
	at := lineAt(t, f, "+line thirty")
	mustStage(t, dir, f, at-2, at)
	if got, want := indexed(t, dir, "open.txt"), strings.Replace(strings.Replace(open, "line 2\n", "line two\n", 1), "line 30", "line thirty", 1); got != want {
		t.Errorf("open.txt in the index:\n%q\nwant the last line staged, still without a newline", got)
	}
}

func TestStageLinesRefusesWhatItCannotDo(t *testing.T) {
	dir, _ := stageRepo(t)
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 15\n", "line fifteen\n", 1))
	f := entry(t, dir, "numbered.txt", false)

	// Context only.
	at := lineAt(t, f, " line 14")
	if _, err := StageLines(dir, f, at, at); !errors.Is(err, ErrNoLines) {
		t.Errorf("context lines: err = %v, want ErrNoLines", err)
	}

	// The file was edited after it was listed: line numbers mean other lines.
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 15\n", "line fifteen!\n", 1))
	at = lineAt(t, f, "+line fifteen")
	if _, err := StageLines(dir, f, at, at); !errors.Is(err, ErrStaleDiff) {
		t.Errorf("edited since: err = %v, want ErrStaleDiff", err)
	}
	if got := indexed(t, dir, "numbered.txt"); got != numbered(30) {
		t.Error("a refused call changed the index")
	}

	// An untracked file refused leaves no entry behind in the index.
	write(t, dir, "new.txt", "one\ntwo\n")
	n := entry(t, dir, "new.txt", false)
	if _, err := StageLines(dir, n, 50, 60); !errors.Is(err, ErrNoLines) {
		t.Errorf("lines past the end: err = %v, want ErrNoLines", err)
	}
	if got := entry(t, dir, "new.txt", false); !got.Untracked {
		t.Error("new.txt is no longer untracked after a refused call")
	}
}

// Whatever is picked, the index ends up as the file with exactly those
// changes made: checked against a reading of the diff that knows nothing of
// patches.
func TestStageLinesMatchesWhatWasPicked(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	dir, git := stageRepo(t)
	old := strings.SplitAfter(numbered(60), "\n")
	old = old[:len(old)-1]
	write(t, dir, "numbered.txt", strings.Join(old, ""))
	git("add", "-A")
	git("commit", "-qm", "sixty lines")

	partial := 0
	for round := 0; round < 16; round++ {
		reverse := round%2 == 1
		// Edit at random: drop, replace and insert lines here and there.
		var edited []string
		for i, line := range old {
			switch rng.Intn(9) {
			case 0:
			case 1:
				edited = append(edited, fmt.Sprintf("changed %d in round %d\n", i, round))
			case 2:
				edited = append(edited, line, fmt.Sprintf("inserted after %d in round %d\n", i, round))
			default:
				edited = append(edited, line)
			}
		}
		write(t, dir, "numbered.txt", strings.Join(edited, ""))
		git("reset", "-q")
		if reverse {
			git("add", "-A")
		}

		f := entry(t, dir, "numbered.txt", reverse)
		lines := strings.Split(f.Body, "\n")
		first := rng.Intn(len(lines))
		last := first + rng.Intn(len(lines)-first)

		// The file the diff starts from, with the picked changes made (or,
		// unstaging, the file it ends at with them unmade). Lines between
		// hunks are the same on both sides.
		side := old
		if reverse {
			side = edited
		}
		var want []string
		next := 0 // the next line of side not yet copied
		picked := 0
		for i, line := range lines {
			in := i >= first && i <= last
			switch {
			case strings.HasPrefix(line, "@@"):
				m := hunkHeader.FindStringSubmatch(line)
				start := 0
				if reverse {
					fmt.Sscan(m[3], &start)
				} else {
					fmt.Sscan(m[1], &start)
				}
				for ; next < start-1; next++ {
					want = append(want, side[next])
				}
			case strings.HasPrefix(line, "+"):
				if in {
					picked++
				}
				if reverse {
					if !in {
						want = append(want, side[next])
					}
					next++
				} else if in {
					want = append(want, line[1:]+"\n")
				}
			case strings.HasPrefix(line, "-"):
				if in {
					picked++
				}
				if reverse {
					if in {
						want = append(want, line[1:]+"\n")
					}
				} else {
					if !in {
						want = append(want, side[next])
					}
					next++
				}
			default:
				want = append(want, side[next])
				next++
			}
		}
		want = append(want, side[next:]...)

		detail, err := StageLines(dir, f, first, last)
		if picked == 0 {
			if !errors.Is(err, ErrNoLines) {
				t.Fatalf("round %d: nothing picked, err = %v", round, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("round %d (reverse=%v, lines %d..%d): %v\n%s\n%s", round, reverse, first, last, err, detail, f.Body)
		}
		if got := indexed(t, dir, "numbered.txt"); got != strings.Join(want, "") {
			t.Fatalf("round %d (reverse=%v, lines %d..%d): the index is not the file with the picked lines\n%s", round, reverse, first, last, f.Body)
		}
		if picked < f.Added+f.Removed {
			partial++
		}
	}
	// Picking everything is handed to git whole; it is the rounds that pick
	// some lines and not others that try the patches.
	if partial < 8 {
		t.Errorf("only %d of the rounds staged part of the file; the seed no longer tests much", partial)
	}
}

func TestStagingFromASubdirectory(t *testing.T) {
	dir, git := stageRepo(t)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, sub, "inner.txt", "one\n")
	git("add", "-A")
	git("commit", "-qm", "second")
	write(t, sub, "inner.txt", "one\ntwo\n")
	write(t, sub, "fresh.txt", "new\n")
	write(t, dir, "numbered.txt", numbered(31))

	// Everything is named from the top, wherever it is asked from.
	want := []string{"numbered.txt:unstaged", "sub/fresh.txt:untracked", "sub/inner.txt:unstaged"}
	if got := describe(WorkingTree(sub, 80).Files); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("files = %q, want %q", got, want)
	}
	for _, path := range []string{"numbered.txt", "sub/inner.txt", "sub/fresh.txt"} {
		if detail, err := StageFile(sub, path); err != nil {
			t.Fatalf("StageFile(%q) from sub: %v\n%s", path, err, detail)
		}
	}
	if out, _ := Run(dir, "status", "--porcelain"); out != "M  numbered.txt\nA  sub/fresh.txt\nM  sub/inner.txt" {
		t.Errorf("status = %q, want all three staged", out)
	}
}

func TestCommitStaged(t *testing.T) {
	dir, _ := stageRepo(t)
	write(t, dir, "numbered.txt", numbered(31))
	write(t, dir, "left.txt", "not staged\n")

	if _, err := CommitStaged(dir, "nothing yet"); !errors.Is(err, ErrNothingStaged) {
		t.Fatalf("with nothing staged: err = %v, want ErrNothingStaged", err)
	}

	StageFile(dir, "numbered.txt")
	// A subject, a body, and a line a comment-stripping cleanup would eat.
	message := "\n\nAdd a line\n\n#42 is why\nand a second line   \n\n\n"
	r, err := CommitStaged(dir, message)
	if err != nil {
		t.Fatalf("CommitStaged: %v\n%s", err, r.Output)
	}
	if r.Subject != "Add a line" || len(r.Hash) != 40 || r.Hash != head(t, dir) {
		t.Errorf("result = %+v, want the new HEAD and its subject", r)
	}
	if body, _ := Run(dir, "log", "-1", "--format=%B"); body != "Add a line\n\n#42 is why\nand a second line" {
		t.Errorf("message = %q, want it as typed, less the stray blank lines", body)
	}
	// Only what was staged went in.
	if got := describe(WorkingTree(dir, 80).Files); strings.Join(got, " ") != "left.txt:untracked" {
		t.Errorf("left uncommitted: %q, want only the file that was not staged", got)
	}
	if out, _ := Run(dir, "show", "--format=", "--name-only", "HEAD"); out != "numbered.txt" {
		t.Errorf("the commit holds %q, want only numbered.txt", out)
	}
}

func TestCommitRefusedOnNoBranch(t *testing.T) {
	dir, git := stageRepo(t)
	git("checkout", "-q", "--detach")
	write(t, dir, "numbered.txt", numbered(31))
	// Staging is still allowed: it loses nothing.
	if _, err := StageFile(dir, "numbered.txt"); err != nil {
		t.Fatalf("StageFile on a detached HEAD: %v", err)
	}
	before := head(t, dir)
	if _, err := CommitStaged(dir, "lost"); !errors.Is(err, ErrDetached) {
		t.Fatalf("err = %v, want ErrDetached", err)
	}
	if head(t, dir) != before {
		t.Error("a commit was made on no branch")
	}
	if s := ReadWorkingState(dir); !s.Detached || s.Operation != "" {
		t.Errorf("state = %+v, want detached and nothing else", s)
	}
}

// conflictedRepo is in the middle of a merge that stopped on numbered.txt.
func conflictedRepo(t *testing.T) string {
	t.Helper()
	dir, git := stageRepo(t)
	git("checkout", "-qb", "side")
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "line two, their way\n", 1))
	git("commit", "-qam", "theirs")
	git("checkout", "-q", "main")
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "line two, our way\n", 1))
	git("commit", "-qam", "ours")
	// It fails, which is the point; the fixture's git would stop the test.
	Run(dir, "merge", "side")
	if out, _ := Run(dir, "ls-files", "--unmerged"); out == "" {
		t.Fatal("the merge did not conflict")
	}
	return dir
}

// Staging a conflicted file means "resolved", and a commit concludes the
// merge: neither is what s and c are for, so both are refused.
func TestStagingRefusedDuringAMerge(t *testing.T) {
	dir := conflictedRepo(t)

	d := WorkingTree(dir, 80)
	if d.State.Operation != "merge" {
		t.Fatalf("state = %+v, want a merge in progress", d.State)
	}
	// The file is still listed, once, to be read.
	if got := describe(d.Files); strings.Join(got, " ") != "numbered.txt:unstaged" {
		t.Errorf("files = %q, want the conflicted file listed once", got)
	}

	before, _ := Run(dir, "status", "--porcelain")
	for name, call := range map[string]func() (string, error){
		"StageFile":   func() (string, error) { return StageFile(dir, "numbered.txt") },
		"UnstageFile": func() (string, error) { return UnstageFile(dir, "numbered.txt") },
		"StageAll":    func() (string, error) { return StageAll(dir) },
		"UnstageAll":  func() (string, error) { return UnstageAll(dir) },
		"StageLines":  func() (string, error) { return StageLines(dir, d.Files[0], 0, 5) },
		"CommitStaged": func() (string, error) {
			_, err := CommitStaged(dir, "concluded")
			return "", err
		},
	} {
		if _, err := call(); !errors.Is(err, ErrInProgress) {
			t.Errorf("%s: err = %v, want ErrInProgress", name, err)
		}
	}
	if after, _ := Run(dir, "status", "--porcelain"); after != before {
		t.Errorf("status went from %q to %q, want nothing touched", before, after)
	}
}

// A hook that says no is the repository's own rule, and is reported with
// what it said rather than worked around.
func TestCommitReportsAHookThatRefuses(t *testing.T) {
	dir, _ := stageRepo(t)
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho 'lint: numbered.txt is too long' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(hook, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, dir, "numbered.txt", numbered(31))
	StageFile(dir, "numbered.txt")

	before := head(t, dir)
	r, err := CommitStaged(dir, "Add a line")
	if err == nil {
		t.Fatal("the commit went through a hook that refused it")
	}
	if !strings.Contains(r.Output, "lint: numbered.txt is too long") {
		t.Errorf("output = %q, want what the hook said", r.Output)
	}
	if head(t, dir) != before {
		t.Error("HEAD moved")
	}
	// Still staged, ready for another go once the hook is satisfied.
	if got := describe(WorkingTree(dir, 80).Files); strings.Join(got, " ") != "numbered.txt:staged" {
		t.Errorf("files = %q, want the change still staged", got)
	}
}

// With core.autocrlf the file on disk ends its lines differently from the
// file in the index, which is the usual state of a Windows checkout. The
// patch is of the index's lines, and has to apply to them.
func TestStageLinesWithAutocrlf(t *testing.T) {
	dir, git := stageRepo(t)
	git("config", "core.autocrlf", "true")
	crlf := func(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }
	changed := strings.Replace(strings.Replace(numbered(30), "line 2\n", "line two\n", 1), "line 29\n", "line twenty-nine\n", 1)
	write(t, dir, "numbered.txt", crlf(changed))
	write(t, dir, "new.txt", crlf("one\ntwo\nthree\n"))

	f := entry(t, dir, "numbered.txt", false)
	first, last := hunkAt(t, f, "+line twenty-nine")
	mustStage(t, dir, f, first, last)
	if got, want := indexed(t, dir, "numbered.txt"), strings.Replace(numbered(30), "line 29\n", "line twenty-nine\n", 1); got != want {
		t.Errorf("the index holds:\n%q\nwant the second hunk staged, with the index's own line endings", got)
	}

	n := entry(t, dir, "new.txt", false)
	at := lineAt(t, n, "+two")
	mustStage(t, dir, n, at, at)
	if got := indexed(t, dir, "new.txt"); got != "two\n" {
		t.Errorf("the index holds %q of the new file, want its second line", got)
	}
}

// The patch names the file the way git's own diff did, whatever is in the
// name.
func TestStageLinesOfAFileWithAnOddName(t *testing.T) {
	names := []string{"odd [name] é.txt", "sub dir/with spaces.txt"}
	dir, git := stageRepo(t)
	for _, name := range names {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, dir, name, numbered(30))
	}
	git("add", "-A")
	git("commit", "-qm", "odd names")

	for _, name := range names {
		write(t, dir, name, strings.Replace(strings.Replace(numbered(30), "line 2\n", "line two\n", 1), "line 29\n", "line twenty-nine\n", 1))
		f := entry(t, dir, name, false)
		first, last := hunkAt(t, f, "+line two")
		mustStage(t, dir, f, first, last)
		if got, want := indexed(t, dir, name), strings.Replace(numbered(30), "line 2\n", "line two\n", 1); got != want {
			t.Errorf("%q: the index does not hold just the first hunk", name)
		}
		// And back out again.
		s := entry(t, dir, name, true)
		at := lineAt(t, s, "+line two")
		mustStage(t, dir, s, at, at)
		if got := indexed(t, dir, name); got != strings.Replace(numbered(30), "line 2\n", "", 1) {
			t.Errorf("%q: unstaging the added line left %q", name, got)
		}
	}
}

// Git writes a name holding a quote or a tab in quotes, escaped, and the
// patch has to name the file the same way. Windows cannot hold such a file,
// so these exist in the index only: staged as new, and missing from disk,
// which makes them a staged addition and an unstaged deletion — the two cases
// where the patch's head has to be rewritten as well as copied.
func TestStageLinesOfAFileGitQuotes(t *testing.T) {
	dir, git := stageRepo(t)
	// Git for Windows otherwise refuses names Windows could not check out,
	// even in the index.
	git("config", "core.protectNTFS", "false")
	blob, err := Run(dir, "hash-object", "-w", "numbered.txt")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(numbered(30), "\n")

	for _, name := range []string{"has \"quotes\".txt", "has\ttab.txt"} {
		git("update-index", "--add", "--cacheinfo", "100644,"+blob+","+name)

		// Unstage two of its lines.
		staged := entry(t, dir, name, true)
		mustStage(t, dir, staged, lineAt(t, staged, "+line 2"), lineAt(t, staged, "+line 3"))
		want := lines[0] + strings.Join(lines[3:30], "")
		if got := indexed(t, dir, name); got != want {
			t.Fatalf("%q: after unstaging two lines the index holds\n%q", name, got)
		}

		// Stage the removal of five more: the file is not on disk, so what
		// is unstaged is its deletion.
		gone := entry(t, dir, name, false)
		mustStage(t, dir, gone, lineAt(t, gone, "-line 10"), lineAt(t, gone, "-line 14"))
		want = lines[0] + strings.Join(lines[3:9], "") + strings.Join(lines[14:30], "")
		if got := indexed(t, dir, name); got != want {
			t.Fatalf("%q: after staging five removals the index holds\n%q", name, got)
		}
	}
}

// The list is in path order whichever side of the index a file is on, so
// staging one, or part of one, or taking it back, moves nothing: a file that
// is on both sides has its two entries together, the staged one first.
func TestStagingLeavesTheFilesWhereTheyAre(t *testing.T) {
	dir, git := stageRepo(t)
	write(t, dir, "a.txt", "one\n")
	write(t, dir, "c.txt", "one\n")
	if err := os.Mkdir(filepath.Join(dir, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "dir/e.txt", "one\n")
	git("add", "-A")
	git("commit", "-qm", "second")
	write(t, dir, "a.txt", "one\ntwo\n")
	write(t, dir, "b.txt", "new\n")
	write(t, dir, "c.txt", "one\ntwo\n")
	write(t, dir, "d.txt", "new\n")
	write(t, dir, "dir/e.txt", "one\ntwo\n")

	paths := func() string {
		var out []string
		for _, f := range WorkingTree(dir, 80).Files {
			if len(out) == 0 || out[len(out)-1] != f.Path {
				out = append(out, f.Path)
			}
		}
		return strings.Join(out, " ")
	}
	const order = "a.txt b.txt c.txt d.txt dir/e.txt"
	if got := paths(); got != order {
		t.Fatalf("before staging, paths = %q, want %q", got, order)
	}

	for _, step := range []struct {
		path string
		do   func(dir, path string) (string, error)
		want string
	}{
		{"c.txt", StageFile, "a.txt:unstaged b.txt:untracked c.txt:staged d.txt:untracked dir/e.txt:unstaged"},
		{"d.txt", StageFile, "a.txt:unstaged b.txt:untracked c.txt:staged d.txt:staged dir/e.txt:unstaged"},
		{"dir/e.txt", StageFile, "a.txt:unstaged b.txt:untracked c.txt:staged d.txt:staged dir/e.txt:staged"},
		{"c.txt", UnstageFile, "a.txt:unstaged b.txt:untracked c.txt:unstaged d.txt:staged dir/e.txt:staged"},
		{"d.txt", UnstageFile, "a.txt:unstaged b.txt:untracked c.txt:unstaged d.txt:untracked dir/e.txt:staged"},
	} {
		if detail, err := step.do(dir, step.path); err != nil {
			t.Fatalf("%s: %v\n%s", step.path, err, detail)
		}
		if got := strings.Join(describe(WorkingTree(dir, 80).Files), " "); got != step.want {
			t.Fatalf("after %s, files = %q, want %q", step.path, got, step.want)
		}
	}

	// Edited again after being staged: on both sides, and still in place.
	write(t, dir, "dir/e.txt", "one\ntwo\nthree\n")
	want := "a.txt:unstaged b.txt:untracked c.txt:unstaged d.txt:untracked dir/e.txt:staged dir/e.txt:unstaged"
	if got := strings.Join(describe(WorkingTree(dir, 80).Files), " "); got != want {
		t.Errorf("with a file on both sides, files = %q, want %q", got, want)
	}
}
