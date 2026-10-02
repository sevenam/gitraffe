package git

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/sevenam/gitraffe/internal/gittest"
)

func TestFingerprintChangesWithWhatAReloadWouldShow(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")

	last := ""
	step := func(what string, change func()) {
		t.Helper()
		change()
		got, err := Fingerprint(dir)
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		if got == last {
			t.Errorf("%s: the fingerprint did not change", what)
		}
		last = got
	}
	file := filepath.Join(dir, "first")
	edit := func(body string, at time.Time) {
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(file, at, at); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()

	step("as committed", func() {})
	step("a new commit", func() { commit("second") })
	step("a new branch", func() { git("branch", "side") })
	step("another branch checked out at the same commit", func() { git("checkout", "-q", "side") })
	step("a tag", func() { git("tag", "v1") })
	step("an untracked file", func() { os.WriteFile(filepath.Join(dir, "new file.txt"), []byte("x"), 0o644) })
	step("an edit", func() { edit("one", now.Add(time.Hour)) })
	// The status line reads " M first" before and after, and the size is the
	// same: only the time says it was touched again.
	step("a second edit of the same file", func() { edit("two", now.Add(2*time.Hour)) })
	step("staging it", func() { git("add", "first") })

	again, err := Fingerprint(dir)
	if err != nil || again != last {
		t.Errorf("asking twice with nothing changed gave %q then %q (%v)", last, again, err)
	}
}

func TestFingerprintOutsideARepositoryIsAnError(t *testing.T) {
	if got, err := Fingerprint(t.TempDir()); err == nil {
		t.Errorf("got %q, want an error: there is nothing to watch", got)
	}
}

func TestStatusPaths(t *testing.T) {
	out := "# branch.oid abc\x00# branch.head main\x00" +
		"1 .M N... 100644 100644 100644 aaa bbb with space.txt\x00" +
		"2 R. N... 100644 100644 100644 aaa bbb R100 new name.txt\x00old name.txt\x00" +
		"u UU N... 100644 100644 100644 100644 aaa bbb ccc conflict.txt\x00" +
		"? untracked dir/\x00"
	want := []string{"with space.txt", "new name.txt", "conflict.txt", "untracked dir/"}
	if got := statusPaths([]byte(out)); !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %q, want %q", got, want)
	}
}
