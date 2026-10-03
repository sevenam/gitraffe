package git

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// filterRepo has a file, notes.txt, edited on main and on a side branch, among
// commits that leave it alone, and a directory with a file of its own.
func filterRepo(t *testing.T) string {
	t.Helper()
	dir, git, commit := gittest.Fixture(t)
	edit := func(path, content, message string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		git("add", "-A")
		git("commit", "-qm", message)
	}
	git("init", "-q", "-b", "main")
	edit("notes.txt", "one", "start notes")
	commit("unrelated")
	git("checkout", "-qb", "side")
	commit("side unrelated")
	edit("notes.txt", "two", "notes on side")
	git("checkout", "-q", "main")
	edit("docs/guide.md", "guide", "add guide")
	edit("notes.txt", "one\nmain", "notes on main")
	commit("later")
	return dir
}

func subjects(commits []Commit) []string {
	var s []string
	for _, c := range commits {
		s = append(s, c.Message)
	}
	return s
}

// A file's history is the commits that changed it, on every branch, and
// nothing else.
func TestFilterByPathKeepsOnlyTheCommitsThatChangedIt(t *testing.T) {
	g, err := LoadGraph(filterRepo(t), 100, Filter{Path: "notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	got := subjects(g.Commits)
	want := []string{"notes on main", "notes on side", "start notes"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("filtered to notes.txt, got %q, want %q", got, want)
	}
}

// Every parent a filtered commit names must be in the list, or its lane would
// run off the bottom of the graph looking for a commit that never comes.
func TestFilteredCommitsNameOnlyParentsInTheList(t *testing.T) {
	g, err := LoadGraph(filterRepo(t), 100, Filter{Path: "notes.txt"})
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, c := range g.Commits {
		listed[c.Hash] = true
	}
	for _, c := range g.Commits {
		for _, p := range c.Parents {
			if !listed[p] {
				t.Errorf("%q names parent %s, which the filter left out", c.Message, p)
			}
		}
	}
	// Both edits branch from the commit that started the file.
	for _, c := range g.Commits {
		if c.Message == "start notes" && len(c.Parents) != 0 {
			t.Errorf("the first commit of the file has parents %v, want none", c.Parents)
		}
	}
}

// A directory's history is that of everything under it, and the path is the
// repository's even when gitraffe was started from a subdirectory.
func TestFilterByDirectoryFromASubdirectory(t *testing.T) {
	dir := filterRepo(t)
	g, err := LoadGraph(filepath.Join(dir, "docs"), 100, Filter{Path: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if got := subjects(g.Commits); !slices.Equal(got, []string{"add guide"}) {
		t.Errorf("filtered to docs from docs/, got %q, want only the guide", got)
	}
}

// The path is a name, not a pattern: a filter on "*.txt" must not read as
// every text file.
func TestFilterPathIsLiteral(t *testing.T) {
	g, err := LoadGraph(filterRepo(t), 100, Filter{Path: "*.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Commits) != 0 {
		t.Errorf("filtered to a file called *.txt, got %q, want nothing", subjects(g.Commits))
	}
}

// The fallback reader answers the same question as the graph.
func TestLoadCommitsFilters(t *testing.T) {
	commits, _, err := LoadCommits(filterRepo(t), 100, Filter{Path: "docs/guide.md"})
	if err != nil {
		t.Fatal(err)
	}
	if got := subjects(commits); !slices.Equal(got, []string{"add guide"}) {
		t.Errorf("got %q, want only the guide", got)
	}
}
