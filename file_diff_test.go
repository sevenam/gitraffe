package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// showDiff is what the loader feeds the parser: git's own -p output for HEAD.
func showDiff(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "show", "--format=", "--no-color", "-p", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show: %v", err)
	}
	return strings.ReplaceAll(string(out), "\r", "")
}

func paths(files []fileDiff) []string {
	var out []string
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func findFile(t *testing.T, files []fileDiff, path string) fileDiff {
	t.Helper()
	for _, f := range files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("%q is not among %v", path, paths(files))
	return fileDiff{}
}

// One commit touching several files, which is the case the commit view exists
// for: the whole-commit diff says nothing about where one file ends.
func TestParseFileDiffsSplitsByFile(t *testing.T) {
	dir, git, _ := gitFixture(t)
	git("init", "-q", "-b", "main")
	write(t, dir, "keep.txt", "one\ntwo\nthree\n")
	write(t, dir, "drop.txt", "gone\n")
	git("add", "-A")
	git("commit", "-qm", "first")

	write(t, dir, "keep.txt", "one\nTWO\nthree\nfour\n")
	write(t, dir, "added.txt", "brand new\n")
	os.Remove(filepath.Join(dir, "drop.txt"))
	git("add", "-A")
	git("commit", "-qm", "second")

	files := parseFileDiffs(showDiff(t, dir))
	if got := paths(files); len(got) != 3 {
		t.Fatalf("files = %v, want one entry each for added, drop and keep", got)
	}

	added := findFile(t, files, "added.txt")
	if added.Added != 1 || added.Removed != 0 {
		t.Errorf("added.txt = +%d -%d, want +1 -0", added.Added, added.Removed)
	}
	if !strings.Contains(added.Body, "+brand new") {
		t.Errorf("added.txt body does not hold its own line:\n%s", added.Body)
	}
	// Each file's body is its own: the next file's header ends it.
	if strings.Contains(added.Body, "keep.txt") || strings.Contains(added.Body, "gone") {
		t.Errorf("added.txt body ran into another file:\n%s", added.Body)
	}

	keep := findFile(t, files, "keep.txt")
	if keep.Added != 2 || keep.Removed != 1 {
		t.Errorf("keep.txt = +%d -%d, want +2 -1", keep.Added, keep.Removed)
	}
	if drop := findFile(t, files, "drop.txt"); drop.Removed != 1 || drop.Added != 0 {
		t.Errorf("drop.txt = +%d -%d, want +0 -1", drop.Added, drop.Removed)
	}
}

// The counts are what the file list shows, so they have to mean the same as
// git's own --numstat rather than nearly the same: the "+++"/"---" header
// lines start with a + and a - but are not changes.
func TestFileCountsMatchNumstat(t *testing.T) {
	dir, git, _ := gitFixture(t)
	git("init", "-q", "-b", "main")
	write(t, dir, "a.txt", "1\n2\n3\n4\n5\n")
	git("add", "-A")
	git("commit", "-qm", "first")
	write(t, dir, "a.txt", "1\nchanged\n3\nalso\n5\nextra\n")
	write(t, dir, "b.txt", "new file\n")
	git("add", "-A")
	git("commit", "-qm", "second")

	cmd := exec.Command("git", "show", "--format=", "--numstat", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(strings.ReplaceAll(string(out), "\r", "")), "\n") {
		if parts := strings.Fields(line); len(parts) == 3 {
			want[parts[2]] = parts[0] + "/" + parts[1]
		}
	}

	for _, f := range parseFileDiffs(showDiff(t, dir)) {
		got := fmt.Sprintf("%d/%d", f.Added, f.Removed)
		if want[f.Path] != got {
			t.Errorf("%s = %s, git --numstat says %s", f.Path, got, want[f.Path])
		}
	}
}

func TestParseFileDiffsNamesARename(t *testing.T) {
	dir, git, _ := gitFixture(t)
	git("init", "-q", "-b", "main")
	write(t, dir, "before.txt", "unchanged content\n")
	git("add", "-A")
	git("commit", "-qm", "first")
	git("mv", "before.txt", "after.txt")
	git("commit", "-qm", "rename it")

	files := parseFileDiffs(showDiff(t, dir))
	if len(files) != 1 {
		t.Fatalf("files = %v, want the one renamed file", paths(files))
	}
	if got := files[0].Path; got != "before.txt → after.txt" {
		t.Errorf("path = %q, want both ends of the rename", got)
	}
}

func TestParseFileDiffsMarksBinary(t *testing.T) {
	dir, git, _ := gitFixture(t)
	git("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte{0, 1, 2, 0, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-qm", "add a binary")

	files := parseFileDiffs(showDiff(t, dir))
	if len(files) != 1 {
		t.Fatalf("files = %v, want the one file", paths(files))
	}
	if !files[0].Binary {
		t.Errorf("blob.bin is not marked binary; body = %q", files[0].Body)
	}
}

func TestParseFileDiffsOnNothing(t *testing.T) {
	for _, body := range []string{"", "   \n", "commit 1234\nAuthor: nobody\n"} {
		if got := parseFileDiffs(body); got != nil {
			t.Errorf("parseFileDiffs(%q) = %v, want nothing outside a file", body, paths(got))
		}
	}
}

// A file longer than the cap keeps its head and says it was cut, rather than
// being dropped or taking the whole panel's memory.
func TestLongFileDiffIsCutWithANote(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("diff --git a/big.txt b/big.txt\n")
	sb.WriteString("--- a/big.txt\n+++ b/big.txt\n@@ -0,0 +1,2000 @@\n")
	for i := range 2000 {
		sb.WriteString("+line ")
		sb.WriteString(string(rune('a' + i%26)))
		sb.WriteString("\n")
	}
	files := parseFileDiffs(sb.String())
	if len(files) != 1 {
		t.Fatalf("files = %v, want one", paths(files))
	}
	lines := strings.Split(files[0].Body, "\n")
	if len(lines) != maxFileDiffLines+1 {
		t.Errorf("body is %d lines, want the cap plus the note", len(lines))
	}
	if !strings.Contains(lines[len(lines)-1], "truncated") {
		t.Errorf("last line is %q, want it to say the diff was cut", lines[len(lines)-1])
	}
	// The counts are of the whole file, not of the part that fits.
	if files[0].Added != 2000 {
		t.Errorf("added = %d, want all 2000 counted", files[0].Added)
	}
}
