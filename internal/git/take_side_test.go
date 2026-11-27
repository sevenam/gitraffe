package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// onDisk is a file's contents in the working tree, or "<absent>".
func onDisk(t *testing.T, dir, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, path))
	if os.IsNotExist(err) {
		return "<absent>"
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func unmergedPaths(t *testing.T, dir string) string {
	t.Helper()
	files, err := unmergedFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return strings.Join(paths, " ")
}

// Each side of each kind, taken, put back with Unresolve, then the other
// side taken: the round trip a user changing their mind makes.
func TestTakeSideAndUnresolveEachKind(t *testing.T) {
	dir, _ := conflictKindsRepo(t)
	want := map[Side]map[string]string{
		Ours: {
			"both.txt":      "a\nB, our way\nc\n",
			"added.txt":     "ours\n",
			"ours-del.txt":  "<absent>", // main deleted it
			"their-del.txt": "changed by us\n",
		},
		Theirs: {
			"both.txt":      "a\nB, their way\nc\n",
			"added.txt":     "theirs\n",
			"ours-del.txt":  "changed by them\n",
			"their-del.txt": "<absent>", // feature/login deleted it
		},
	}
	paths := []string{"added.txt", "both.txt", "ours-del.txt", "their-del.txt"}
	for _, side := range []Side{Ours, Theirs} {
		for _, path := range paths {
			if out, err := TakeSide(dir, path, side); err != nil {
				t.Fatalf("TakeSide(%s, %v): %v: %s", path, side, err, out)
			}
			if got := onDisk(t, dir, path); got != want[side][path] {
				t.Errorf("side %v: %s is %q, want %q", side, path, got, want[side][path])
			}
		}
		if left := unmergedPaths(t, dir); left != "" {
			t.Fatalf("side %v: still unmerged: %s", side, left)
		}
		// Each listed as a conflict resolved, of the kind it was.
		files, state := conflictFiles(dir)
		for _, path := range paths {
			if f := conflictEntry(t, files, path); !f.Resolved {
				t.Errorf("side %v: %s is not listed as resolved", side, path)
			}
		}
		if len(state.Files) != 0 {
			t.Errorf("side %v: state still lists %+v", side, state.Files)
		}

		for _, path := range paths {
			if out, err := Unresolve(dir, path); err != nil {
				t.Fatalf("Unresolve(%s) after %v: %v: %s", path, side, err, out)
			}
		}
		if got := unmergedPaths(t, dir); got != strings.Join(paths, " ") {
			t.Fatalf("after Unresolve, unmerged = %q", got)
		}
		s, _ := Conflicts(dir)
		for _, f := range s.Files {
			if f.Path == "ours-del.txt" && f.Kind != DeletedByUs || f.Path == "their-del.txt" && f.Kind != DeletedByThem {
				t.Errorf("%s came back as %v", f.Path, f.Kind)
			}
		}
		// Back as git left them: markers where both changed the file, and
		// the side that kept it where one deleted it.
		both := onDisk(t, dir, "both.txt")
		for _, line := range []string{"<<<<<<< ", "B, our way", "=======", "B, their way", ">>>>>>> "} {
			if !strings.Contains(both, line) {
				t.Errorf("both.txt after Unresolve lacks %q:\n%s", line, both)
			}
		}
		if got := onDisk(t, dir, "ours-del.txt"); got != "changed by them\n" {
			t.Errorf("ours-del.txt after Unresolve is %q", got)
		}
		if got := onDisk(t, dir, "their-del.txt"); got != "changed by us\n" {
			t.Errorf("their-del.txt after Unresolve is %q", got)
		}
	}
}

func TestTakeSideRefusals(t *testing.T) {
	dir, _ := conflictKindsRepo(t)
	if _, err := TakeSide(dir, "numbered.txt", Ours); !errors.Is(err, ErrNotUnmerged) {
		t.Errorf("TakeSide on a file merged cleanly: %v", err)
	}
	if _, err := TakeSide(dir, "nowhere.txt", Theirs); !errors.Is(err, ErrNotUnmerged) {
		t.Errorf("TakeSide on no file: %v", err)
	}
	Run(dir, "merge", "--abort")
	if _, err := TakeSide(dir, "both.txt", Ours); !errors.Is(err, ErrNoMerge) {
		t.Errorf("TakeSide with no merge: %v", err)
	}
}

// Unresolve writes the conflict over the file, so it does so only where
// that loses nothing: the file is one side's version whole, as TakeSide
// left it.
func TestUnresolveRefusesToWriteOverWork(t *testing.T) {
	dir, _ := conflictKindsRepo(t)

	for path, want := range map[string]error{
		"both.txt":     ErrNotResolved, // still unmerged
		"numbered.txt": ErrNotResolved, // merged cleanly
	} {
		if _, err := Unresolve(dir, path); !errors.Is(err, want) {
			t.Errorf("Unresolve(%s) = %v, want %v", path, err, want)
		}
	}

	// Resolved by hand, then marked: the hand's work is not either side.
	write(t, dir, "both.txt", "a\nB, both ways\nc\n")
	if out, err := MarkResolved(dir, "both.txt"); err != nil {
		t.Fatalf("MarkResolved: %v: %s", err, out)
	}
	if _, err := Unresolve(dir, "both.txt"); !errors.Is(err, ErrResolvedByHand) {
		t.Errorf("Unresolve of a hand resolution = %v", err)
	}
	if got := onDisk(t, dir, "both.txt"); got != "a\nB, both ways\nc\n" {
		t.Errorf("both.txt was written over: %q", got)
	}

	// A side taken, then changed on disk.
	if out, err := TakeSide(dir, "added.txt", Ours); err != nil {
		t.Fatalf("TakeSide: %v: %s", err, out)
	}
	write(t, dir, "added.txt", "ours, then more\n")
	if _, err := Unresolve(dir, "added.txt"); !errors.Is(err, ErrResolvedByHand) {
		t.Errorf("Unresolve of a file changed since = %v", err)
	}

	// A deletion taken, then a new file in its place.
	if out, err := TakeSide(dir, "their-del.txt", Theirs); err != nil {
		t.Fatalf("TakeSide: %v: %s", err, out)
	}
	write(t, dir, "their-del.txt", "new\n")
	if _, err := Unresolve(dir, "their-del.txt"); !errors.Is(err, ErrResolvedByHand) {
		t.Errorf("Unresolve over a new file = %v", err)
	}
	if got := onDisk(t, dir, "their-del.txt"); got != "new\n" {
		t.Errorf("their-del.txt was written over: %q", got)
	}
}

func TestTakeSideCherryPick(t *testing.T) {
	dir := conflictedRepo(t) // a merge of side into main, stopped
	Run(dir, "merge", "--abort")
	Run(dir, "cherry-pick", "side")
	before := onDisk(t, dir, "numbered.txt")
	if out, err := TakeSide(dir, "numbered.txt", Theirs); err != nil {
		t.Fatalf("TakeSide: %v: %s", err, out)
	}
	side, _ := Run(dir, "show", "side:numbered.txt")
	if got := strings.TrimSuffix(onDisk(t, dir, "numbered.txt"), "\n"); got != side {
		t.Errorf("numbered.txt is not side's:\n%s", got)
	}
	if out, err := Unresolve(dir, "numbered.txt"); err != nil {
		t.Fatalf("Unresolve: %v: %s", err, out)
	}
	if unmergedPaths(t, dir) != "numbered.txt" {
		t.Error("numbered.txt is not unmerged again")
	}
	// The markers are labelled by checkout -m rather than by the pick, so
	// only the lines between them are compared.
	strip := func(s string) string {
		var keep []string
		for _, l := range strings.Split(s, "\n") {
			if !strings.HasPrefix(l, "<<<<<<<") && !strings.HasPrefix(l, ">>>>>>>") {
				keep = append(keep, l)
			}
		}
		return strings.Join(keep, "\n")
	}
	if got := onDisk(t, dir, "numbered.txt"); strip(got) != strip(before) {
		t.Errorf("numbered.txt came back as\n%s\nwant\n%s", got, before)
	}
}
