package git

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// The list is the whole repository's, named from its top, wherever gitraffe
// was started; untracked and ignored files are not in it.
func TestListFilesNamesEveryTrackedFileFromTheTop(t *testing.T) {
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("top.txt")
	if err := os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "deep", "a file.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-qm", "deep")
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ListFiles(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"sub/deep/a file.go", "top.txt"}
	if !slices.Equal(got, want) {
		t.Errorf("ListFiles from sub/ = %q, want %q", got, want)
	}
}
