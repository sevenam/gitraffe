package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

func read(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func TestDiscardFile(t *testing.T) {
	dir, _ := stageRepo(t)
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "line two\n", 1))
	write(t, dir, "new.txt", "never seen\n")

	if out, err := DiscardFile(dir, entry(t, dir, "numbered.txt", false)); err != nil {
		t.Fatalf("DiscardFile: %v: %s", err, out)
	}
	if got := read(t, dir, "numbered.txt"); got != numbered(30) {
		t.Errorf("numbered.txt is:\n%s\nwant it as committed", got)
	}
	if out, err := DiscardFile(dir, entry(t, dir, "new.txt", false)); err != nil {
		t.Fatalf("DiscardFile on an untracked file: %v: %s", err, out)
	}
	if exists(dir, "new.txt") {
		t.Error("the untracked file is still there")
	}

	// A file deleted, but not with git rm, comes back.
	os.Remove(filepath.Join(dir, "numbered.txt"))
	if out, err := DiscardFile(dir, entry(t, dir, "numbered.txt", false)); err != nil {
		t.Fatalf("DiscardFile on a deleted file: %v: %s", err, out)
	}
	if got := read(t, dir, "numbered.txt"); got != numbered(30) {
		t.Error("the deleted file did not come back as committed")
	}
}

// Staged changes are kept: discarding a file with both puts it back to what
// is staged, and a staged entry is refused with nothing touched.
func TestDiscardKeepsWhatIsStaged(t *testing.T) {
	dir, git := stageRepo(t)
	staged := strings.Replace(numbered(30), "line 2\n", "line two\n", 1)
	write(t, dir, "numbered.txt", staged)
	git("add", "numbered.txt")
	write(t, dir, "numbered.txt", strings.Replace(staged, "line 29\n", "line twenty-nine\n", 1))

	if _, err := DiscardFile(dir, entry(t, dir, "numbered.txt", true)); !errors.Is(err, ErrStagedChanges) {
		t.Errorf("DiscardFile on the staged entry: err = %v, want ErrStagedChanges", err)
	}
	if out, err := DiscardFile(dir, entry(t, dir, "numbered.txt", false)); err != nil {
		t.Fatalf("DiscardFile: %v: %s", err, out)
	}
	if got := read(t, dir, "numbered.txt"); got != staged {
		t.Errorf("numbered.txt is:\n%s\nwant what is staged", got)
	}
	if got := indexed(t, dir, "numbered.txt"); got != staged {
		t.Error("discarding changed the index")
	}
}

func TestDiscardLines(t *testing.T) {
	dir, _ := stageRepo(t)
	changed := strings.Replace(strings.Replace(numbered(30), "line 2\n", "line two\n", 1), "line 15\n", "line fifteen\nand a half\n", 1)
	write(t, dir, "numbered.txt", changed)

	// One added line out of the middle hunk.
	f := entry(t, dir, "numbered.txt", false)
	at := lineAt(t, f, "+and a half")
	if out, err := DiscardLines(dir, f, at, at); err != nil {
		t.Fatalf("DiscardLines: %v: %s", err, out)
	}
	want := strings.Replace(changed, "and a half\n", "", 1)
	if got := read(t, dir, "numbered.txt"); got != want {
		t.Fatalf("numbered.txt is:\n%s\nwant only that line gone", got)
	}

	// A removed line comes back, beside its replacement which is not picked.
	f = entry(t, dir, "numbered.txt", false)
	at = lineAt(t, f, "-line 15")
	if out, err := DiscardLines(dir, f, at, at); err != nil {
		t.Fatalf("DiscardLines: %v: %s", err, out)
	}
	want = strings.Replace(want, "line fifteen\n", "line 15\nline fifteen\n", 1)
	if got := read(t, dir, "numbered.txt"); got != want {
		t.Fatalf("numbered.txt is:\n%s\nwant line 15 back", got)
	}

	// A whole hunk.
	f = entry(t, dir, "numbered.txt", false)
	first, last := hunkAt(t, f, "+line two")
	if out, err := DiscardLines(dir, f, first, last); err != nil {
		t.Fatalf("DiscardLines: %v: %s", err, out)
	}
	want = strings.Replace(want, "line two\n", "line 2\n", 1)
	if got := read(t, dir, "numbered.txt"); got != want {
		t.Fatalf("numbered.txt is:\n%s\nwant the first hunk discarded", got)
	}
	if got := indexed(t, dir, "numbered.txt"); got != numbered(30) {
		t.Error("discarding lines changed the index")
	}
}

func TestDiscardLinesRefusesWhatItCannotDo(t *testing.T) {
	dir, git := stageRepo(t)
	changed := strings.Replace(numbered(30), "line 2\n", "line two\n", 1)
	write(t, dir, "numbered.txt", changed)
	write(t, dir, "new.txt", "one\ntwo\n")

	f := entry(t, dir, "numbered.txt", false)
	at := lineAt(t, f, "+line two")
	// The file has changed since its diff was read.
	write(t, dir, "numbered.txt", strings.Replace(changed, "line 3\n", "line three\n", 1))
	if _, err := DiscardLines(dir, f, at, at); !errors.Is(err, ErrStaleDiff) {
		t.Errorf("stale diff: err = %v, want ErrStaleDiff", err)
	}
	if got := read(t, dir, "numbered.txt"); !strings.Contains(got, "line three") || !strings.Contains(got, "line two") {
		t.Error("a refused discard changed the file")
	}
	if _, err := DiscardLines(dir, entry(t, dir, "new.txt", false), 0, 0); !errors.Is(err, ErrUntrackedLines) {
		t.Errorf("untracked: err = %v, want ErrUntrackedLines", err)
	}

	git("add", "numbered.txt")
	if _, err := DiscardLines(dir, entry(t, dir, "numbered.txt", true), 0, 5); !errors.Is(err, ErrStagedChanges) {
		t.Errorf("staged: err = %v, want ErrStagedChanges", err)
	}
}

func TestDiscardAll(t *testing.T) {
	dir, git := stageRepo(t)
	write(t, dir, "kept.txt", "staged\n")
	git("add", "kept.txt")
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "line two\n", 1))
	write(t, dir, "new.txt", "never seen\n")
	os.MkdirAll(filepath.Join(dir, "sub", "deeper"), 0o755)
	write(t, dir, "sub/deeper/new.txt", "nor this\n")
	write(t, dir, ".gitignore", "ignored.txt\n")
	git("add", ".gitignore")
	write(t, dir, "ignored.txt", "left alone\n")

	if out, err := DiscardAll(dir); err != nil {
		t.Fatalf("DiscardAll: %v: %s", err, out)
	}
	if got := read(t, dir, "numbered.txt"); got != numbered(30) {
		t.Error("numbered.txt was not put back")
	}
	for _, gone := range []string{"new.txt", "sub"} {
		if exists(dir, gone) {
			t.Errorf("%s is still there", gone)
		}
	}
	for _, kept := range []string{"kept.txt", "ignored.txt", ".gitignore"} {
		if !exists(dir, kept) {
			t.Errorf("%s was deleted", kept)
		}
	}
	if got := describe(WorkingTree(dir, 80).Files); strings.Join(got, " ") != ".gitignore:staged kept.txt:staged" {
		t.Errorf("left = %q, want only the staged files", got)
	}

	// With nothing left to discard it does nothing, and says nothing is wrong.
	if out, err := DiscardAll(dir); err != nil {
		t.Errorf("DiscardAll on nothing: %v: %s", err, out)
	}
}

// Before the first commit the index is empty, which restore given every
// path would fail on.
func TestDiscardBeforeTheFirstCommit(t *testing.T) {
	fresh, git, _ := gittest.Fixture(t)
	git("init", "-q")
	write(t, fresh, "a.txt", "one\n")
	if out, err := DiscardAll(fresh); err != nil {
		t.Fatalf("DiscardAll: %v: %s", err, out)
	}
	if exists(fresh, "a.txt") {
		t.Error("the untracked file is still there")
	}
}

func TestDiscardRefusedDuringAMerge(t *testing.T) {
	dir := conflictedRepo(t)
	f := WorkingTree(dir, 80).Files[0]
	before := read(t, dir, "numbered.txt")
	for name, call := range map[string]func() (string, error){
		"DiscardFile":  func() (string, error) { return DiscardFile(dir, f) },
		"DiscardLines": func() (string, error) { return DiscardLines(dir, f, 0, 5) },
		"DiscardAll":   func() (string, error) { return DiscardAll(dir) },
	} {
		if _, err := call(); !errors.Is(err, ErrInProgress) {
			t.Errorf("%s: err = %v, want ErrInProgress", name, err)
		}
	}
	if read(t, dir, "numbered.txt") != before {
		t.Error("a refused discard changed the file")
	}
}
