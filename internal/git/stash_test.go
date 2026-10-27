package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func onlyStash(t *testing.T, dir string) Stash {
	t.Helper()
	stashes, err := ListStashes(dir)
	if err != nil || len(stashes) != 1 {
		t.Fatalf("ListStashes = %+v, %v; want one", stashes, err)
	}
	return stashes[0]
}

// A file stashed takes both its halves with it and leaves every other file
// alone; one git has never seen goes too.
func TestStashFile(t *testing.T) {
	dir, git := stageRepo(t)
	write(t, dir, "other.txt", "other\n")
	git("add", "other.txt")
	git("commit", "-qm", "second")

	staged := strings.Replace(numbered(30), "line 2\n", "line two\n", 1)
	write(t, dir, "numbered.txt", staged)
	git("add", "numbered.txt")
	write(t, dir, "numbered.txt", strings.Replace(staged, "line 9\n", "line nine\n", 1))
	write(t, dir, "other.txt", "changed\n")
	write(t, dir, "new.txt", "never seen\n")

	if out, err := StashFile(dir, "numbered.txt"); err != nil {
		t.Fatalf("StashFile: %v: %s", err, out)
	}
	if got := read(t, dir, "numbered.txt"); got != numbered(30) {
		t.Errorf("numbered.txt is:\n%s\nwant it as committed", got)
	}
	if got := indexed(t, dir, "numbered.txt"); got != numbered(30) {
		t.Error("the staged half of numbered.txt is still staged")
	}
	if got := read(t, dir, "other.txt"); got != "changed\n" {
		t.Error("another file's changes went with it")
	}
	if s := onlyStash(t, dir); !strings.HasSuffix(s.Subject, ": numbered.txt") || s.Ref != "stash@{0}" {
		t.Errorf("the stash is %+v, want it named after the file", s)
	}

	if out, err := StashFile(dir, "new.txt"); err != nil {
		t.Fatalf("StashFile on an untracked file: %v: %s", err, out)
	}
	if exists(dir, "new.txt") {
		t.Error("the untracked file is still there")
	}
}

func TestStashAllAndPop(t *testing.T) {
	dir, _ := stageRepo(t)
	changed := strings.Replace(numbered(30), "line 2\n", "line two\n", 1)
	write(t, dir, "numbered.txt", changed)
	write(t, dir, "new.txt", "never seen\n")

	if out, err := StashAll(dir); err != nil {
		t.Fatalf("StashAll: %v: %s", err, out)
	}
	if read(t, dir, "numbered.txt") != numbered(30) || exists(dir, "new.txt") {
		t.Fatal("StashAll left changes behind")
	}
	if out, err := PopStash(dir, onlyStash(t, dir)); err != nil {
		t.Fatalf("PopStash: %v: %s", err, out)
	}
	if read(t, dir, "numbered.txt") != changed || read(t, dir, "new.txt") != "never seen\n" {
		t.Error("the changes did not come back")
	}
	if stashes, _ := ListStashes(dir); len(stashes) != 0 {
		t.Errorf("the stash was not dropped: %+v", stashes)
	}
}

// A pop that would stop in conflicts, or trip over a file already there, is
// refused before anything is touched, and the stash stays in the list.
func TestPopStashRefusesWhatWouldNotApplyCleanly(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setUp func(t *testing.T, dir string, git func(...string))
		want  error
	}{
		{"a conflict with a later commit", func(t *testing.T, dir string, git func(...string)) {
			write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "stashed\n", 1))
			git("stash", "push", "-q")
			write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "committed\n", 1))
			git("commit", "-qam", "second")
		}, ErrStashConflicts},
		{"a conflict with what is staged", func(t *testing.T, dir string, git func(...string)) {
			write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "stashed\n", 1))
			git("stash", "push", "-q")
			write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "staged\n", 1))
			git("add", "numbered.txt")
		}, ErrStashConflicts},
		{"an untracked file already there", func(t *testing.T, dir string, git func(...string)) {
			write(t, dir, "new.txt", "stashed\n")
			git("stash", "push", "-q", "--include-untracked")
			write(t, dir, "new.txt", "made since\n")
		}, ErrStashUntrackedExists},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, git := stageRepo(t)
			tc.setUp(t, dir, git)
			before, _ := Fingerprint(dir)
			s := onlyStash(t, dir)
			if _, err := PopStash(dir, s); !errors.Is(err, tc.want) {
				t.Fatalf("PopStash = %v, want %v", err, tc.want)
			}
			if after, _ := Fingerprint(dir); after != before {
				t.Error("the working tree or the index changed")
			}
			onlyStash(t, dir)
		})
	}
}

// Changes in the working tree to a file the stash also changes are git's own
// to refuse, and it does so before touching anything.
func TestPopStashOverLocalChanges(t *testing.T) {
	dir, git := stageRepo(t)
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "stashed\n", 1))
	git("stash", "push", "-q")
	local := strings.Replace(numbered(30), "line 29\n", "local\n", 1)
	write(t, dir, "numbered.txt", local)

	if _, err := PopStash(dir, onlyStash(t, dir)); err == nil {
		t.Fatal("PopStash over local changes to the same file succeeded")
	}
	if read(t, dir, "numbered.txt") != local {
		t.Error("the local changes were touched")
	}
	onlyStash(t, dir)
}

// The stash is named by its place in the list, and a list changed since it
// was read must not have pop take whatever is in that place now.
func TestPopStashRefusesAMovedStash(t *testing.T) {
	dir, git := stageRepo(t)
	write(t, dir, "a.txt", "a\n")
	git("stash", "push", "-q", "--include-untracked")
	s := onlyStash(t, dir)
	write(t, dir, "b.txt", "b\n")
	git("stash", "push", "-q", "--include-untracked")

	if _, err := PopStash(dir, s); !errors.Is(err, ErrStashMoved) {
		t.Fatalf("PopStash = %v, want ErrStashMoved", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); err == nil {
		t.Error("the newer stash was popped")
	}
}
