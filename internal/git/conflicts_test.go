package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// conflictKindsRepo is main in the middle of a merge of feature/login that
// stopped on one file of each common kind:
//
//	both.txt     both modified
//	added.txt    both added
//	ours-del.txt deleted by us (main), changed by them
//	their-del.txt deleted by them, changed by us
//
// numbered.txt is merged cleanly. The merge is started by the fixture's
// own git, as a terminal would.
func conflictKindsRepo(t *testing.T) (dir string, git func(...string)) {
	t.Helper()
	dir, git = stageRepo(t)
	write(t, dir, "both.txt", "a\nb\nc\n")
	write(t, dir, "ours-del.txt", "kept by them\n")
	write(t, dir, "their-del.txt", "kept by us\n")
	git("add", "-A")
	git("commit", "-qm", "base")

	git("switch", "-qc", "feature/login")
	write(t, dir, "both.txt", "a\nB, their way\nc\n")
	write(t, dir, "added.txt", "theirs\n")
	write(t, dir, "ours-del.txt", "changed by them\n")
	git("rm", "-q", "their-del.txt")
	write(t, dir, "numbered.txt", strings.Replace(numbered(30), "line 25\n", "line twenty-five\n", 1))
	git("add", "-A")
	git("commit", "-qm", "theirs")

	git("switch", "-q", "main")
	write(t, dir, "both.txt", "a\nB, our way\nc\n")
	write(t, dir, "added.txt", "ours\n")
	git("rm", "-q", "ours-del.txt")
	write(t, dir, "their-del.txt", "changed by us\n")
	git("add", "-A")
	git("commit", "-qm", "ours")

	// It fails, which is the point; the fixture's git would stop the test.
	Run(dir, "merge", "feature/login")
	if out, _ := Run(dir, "ls-files", "--unmerged"); out == "" {
		t.Fatal("the merge did not conflict")
	}
	return dir, git
}

func TestConflictsListsEachKind(t *testing.T) {
	dir, _ := conflictKindsRepo(t)
	// From a folder below the top, too: paths are the top's.
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := Conflicts(sub)
	if err != nil {
		t.Fatal(err)
	}
	if s.Operation != "merge" || s.Ours != "main" || s.Theirs != "feature/login" {
		t.Errorf("state = %+v, want a merge of feature/login into main", s)
	}
	want := map[string]ConflictKind{
		"added.txt":     BothAdded,
		"both.txt":      BothModified,
		"ours-del.txt":  DeletedByUs,
		"their-del.txt": DeletedByThem,
	}
	if len(s.Files) != len(want) {
		t.Fatalf("files = %+v, want %d", s.Files, len(want))
	}
	for i, f := range s.Files {
		if kind, ok := want[f.Path]; !ok || kind != f.Kind {
			t.Errorf("%s is %v, want %v", f.Path, f.Kind, kind)
		}
		if i > 0 && s.Files[i-1].Path >= f.Path {
			t.Errorf("files not in path order: %+v", s.Files)
		}
	}
}

// A path is read as it is: spaces, and nothing quoted.
func TestConflictsPathWithSpaces(t *testing.T) {
	dir, git := stageRepo(t)
	write(t, dir, "a file é.txt", "base\n")
	git("add", "-A")
	git("commit", "-qm", "base")
	git("switch", "-qc", "side")
	write(t, dir, "a file é.txt", "side\n")
	git("commit", "-qam", "side")
	git("switch", "-q", "main")
	write(t, dir, "a file é.txt", "main\n")
	git("commit", "-qam", "main")
	Run(dir, "merge", "side")

	s, err := Conflicts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Files) != 1 || s.Files[0].Path != "a file é.txt" || s.Files[0].Kind != BothModified {
		t.Errorf("files = %+v", s.Files)
	}
}

// A message of the user's own names nothing, and the other side is named by
// the branch its commit is the tip of; a commit further down by its hash.
func TestConflictsTheirNameWithoutGitsMessage(t *testing.T) {
	dir, _ := conflictKindsRepo(t)
	Run(dir, "merge", "--abort")
	Run(dir, "merge", "-m", "my own words", "feature/login")
	if s, _ := Conflicts(dir); s.Theirs != "feature/login" {
		t.Errorf("theirs = %q, want feature/login", s.Theirs)
	}

	// side~1 is the tip of no branch.
	dir2, git := stageRepo(t)
	git("switch", "-qc", "side")
	write(t, dir2, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "two, side\n", 1))
	git("commit", "-qam", "side one")
	wantShort, _ := Run(dir2, "rev-parse", "--short", "HEAD")
	write(t, dir2, "extra.txt", "x\n")
	git("add", "-A")
	git("commit", "-qm", "side two")
	git("switch", "-q", "main")
	write(t, dir2, "numbered.txt", strings.Replace(numbered(30), "line 2\n", "two, main\n", 1))
	git("commit", "-qam", "main")
	Run(dir2, "merge", "-m", "mine", "side~1")
	if s, _ := Conflicts(dir2); s.Theirs != wantShort {
		t.Errorf("theirs = %q, want the short hash %s", s.Theirs, wantShort)
	}
}

func TestNameInMergeMessage(t *testing.T) {
	for msg, want := range map[string]string{
		"Merge branch 'feature/login'\n\n# Conflicts:\n#\tboth.txt\n": "feature/login",
		"Merge branch 'x' into main":                                  "x",
		"Merge remote-tracking branch 'origin/x'":                     "origin/x",
		"Merge tag 'v1.2'":                                            "v1.2",
		"Merge branches 'a' and 'b'":                                  "a",
		"my own words, 'quoted'":                                      "",
		"Merge without quotes":                                        "",
	} {
		if got := nameInMergeMessage(msg); got != want {
			t.Errorf("%q: got %q, want %q", msg, got, want)
		}
	}
}

func TestConflictsCherryPick(t *testing.T) {
	dir := conflictedRepo(t) // a merge of side into main, stopped
	Run(dir, "merge", "--abort")
	Run(dir, "cherry-pick", "side")
	s, err := Conflicts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Operation != "cherry-pick" || s.Ours != "main" || s.Theirs != "side" {
		t.Errorf("state = %+v, want a cherry-pick of side onto main", s)
	}
	if len(s.Files) != 1 || s.Files[0].Path != "numbered.txt" {
		t.Errorf("files = %+v", s.Files)
	}
}

func TestAbortMergeRestoresHeadAndTree(t *testing.T) {
	dir, _ := conflictKindsRepo(t)
	before, _ := Run(dir, "rev-parse", "ORIG_HEAD")
	if out, err := AbortMerge(dir); err != nil {
		t.Fatalf("AbortMerge: %v\n%s", err, out)
	}
	if head(t, dir) != before {
		t.Error("HEAD moved")
	}
	if s := ReadWorkingState(dir); s.Operation != "" {
		t.Errorf("still in a %s", s.Operation)
	}
	if dirty, _ := LocalChanges(dir); dirty {
		out, _ := Run(dir, "status", "--short")
		t.Errorf("the working tree is not as it was:\n%s", out)
	}
	if body, _ := os.ReadFile(filepath.Join(dir, "both.txt")); string(body) != "a\nB, our way\nc\n" {
		t.Errorf("both.txt = %q, want ours back", body)
	}

	if _, err := AbortMerge(dir); !errors.Is(err, ErrNoMerge) {
		t.Errorf("with nothing under way: err = %v, want ErrNoMerge", err)
	}
}

func TestAbortCherryPick(t *testing.T) {
	dir := conflictedRepo(t)
	Run(dir, "merge", "--abort")
	before := head(t, dir)
	Run(dir, "cherry-pick", "side")
	if ReadWorkingState(dir).Operation != "cherry-pick" {
		t.Fatal("the cherry-pick did not stop")
	}
	if out, err := AbortMerge(dir); err != nil {
		t.Fatalf("AbortMerge: %v\n%s", err, out)
	}
	if head(t, dir) != before || ReadWorkingState(dir).Operation != "" {
		t.Error("the cherry-pick was not undone")
	}
	if dirty, _ := LocalChanges(dir); dirty {
		t.Error("the working tree was left changed")
	}
}

// Unmerged files left by a stash are not a merge to abort or continue.
func TestAbortAndContinueRefusedForOtherConflicts(t *testing.T) {
	dir := conflictedRepo(t)
	if err := os.Remove(filepath.Join(dir, ".git", "MERGE_HEAD")); err != nil {
		t.Fatal(err)
	}
	if ReadWorkingState(dir).Operation != "conflict" {
		t.Fatal("want a conflict with no merge")
	}
	if _, err := AbortMerge(dir); !errors.Is(err, ErrNoMerge) {
		t.Errorf("abort: err = %v, want ErrNoMerge", err)
	}
	if _, err := ContinueMerge(dir, ""); !errors.Is(err, ErrNoMerge) {
		t.Errorf("continue: err = %v, want ErrNoMerge", err)
	}
	if _, err := MarkResolved(dir, "numbered.txt"); !errors.Is(err, ErrNoMerge) {
		t.Errorf("mark resolved: err = %v, want ErrNoMerge", err)
	}
}

func TestContinueMergeOnceResolved(t *testing.T) {
	dir, git := conflictKindsRepo(t)
	ours := head(t, dir)
	theirs, _ := Run(dir, "rev-parse", "feature/login")

	if _, err := ContinueMerge(dir, ""); !errors.Is(err, ErrUnmerged) {
		t.Fatalf("with files unmerged: err = %v, want ErrUnmerged", err)
	}

	// Markers left in: refused, and nothing staged.
	if _, err := MarkResolved(dir, "both.txt"); !errors.Is(err, ErrConflictMarkers) {
		t.Errorf("both.txt with markers: err = %v, want ErrConflictMarkers", err)
	}
	if s, _ := Conflicts(dir); len(s.Files) != 4 {
		t.Errorf("files = %+v, want all four still unmerged", s.Files)
	}
	if _, err := MarkResolved(dir, "numbered.txt"); !errors.Is(err, ErrNotUnmerged) {
		t.Errorf("numbered.txt: err = %v, want ErrNotUnmerged", err)
	}

	// Each resolved: an edit, a side's version, and a deletion kept.
	write(t, dir, "both.txt", "a\nB, both ways\nc\n")
	write(t, dir, "added.txt", "ours and theirs\n")
	if err := os.Remove(filepath.Join(dir, "ours-del.txt")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"both.txt", "added.txt", "ours-del.txt", "their-del.txt"} {
		if out, err := MarkResolved(dir, path); err != nil {
			t.Fatalf("MarkResolved %s: %v\n%s", path, err, out)
		}
	}
	if s, _ := Conflicts(dir); len(s.Files) != 0 || s.Operation != "merge" {
		t.Fatalf("state = %+v, want a merge with nothing unmerged", s)
	}

	// A file staged by hand with its markers still in is refused too.
	write(t, dir, "added.txt", "<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> feature/login\n")
	git("add", "added.txt")
	if _, err := ContinueMerge(dir, ""); !errors.Is(err, ErrConflictMarkers) || !strings.Contains(err.Error(), "added.txt") {
		t.Fatalf("with markers staged: err = %v, want ErrConflictMarkers naming added.txt", err)
	}
	write(t, dir, "added.txt", "ours and theirs\n")
	git("add", "added.txt")

	if got := MergeMessage(dir); got != "Merge branch 'feature/login'" {
		t.Errorf("MergeMessage = %q, want git's, less the list of conflicts", got)
	}
	r, err := ContinueMerge(dir, "")
	if err != nil {
		t.Fatalf("ContinueMerge: %v\n%s", err, r.Output)
	}
	if r.Hash != head(t, dir) || r.Subject != "Merge branch 'feature/login'" {
		t.Errorf("result = %+v", r)
	}
	if body, _ := Run(dir, "log", "-1", "--format=%B"); body != "Merge branch 'feature/login'" {
		t.Errorf("message = %q, want no comment lines", body)
	}
	if p := parents(t, dir); len(p) != 2 || p[0] != ours || p[1] != theirs {
		t.Errorf("parents = %v, want %s and %s", p, ours, theirs)
	}
	if ReadWorkingState(dir).Operation != "" {
		t.Error("still merging")
	}
	if out, _ := Run(dir, "ls-files", "ours-del.txt", "their-del.txt"); out != "their-del.txt" {
		t.Errorf("tracked: %q, want the deletion kept and the other file kept", out)
	}
}

func TestContinueMergeWithAMessageOfItsOwn(t *testing.T) {
	dir := conflictedRepo(t)
	write(t, dir, "numbered.txt", numbered(30))
	if out, err := MarkResolved(dir, "numbered.txt"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	r, err := ContinueMerge(dir, "Merge side\n\n#7 is why\n\n\n")
	if err != nil {
		t.Fatalf("%v\n%s", err, r.Output)
	}
	if body, _ := Run(dir, "log", "-1", "--format=%B"); body != "Merge side\n\n#7 is why" {
		t.Errorf("message = %q, want it as typed", body)
	}
}

// A cherry-pick continued keeps the picked commit's author and message.
func TestContinueCherryPick(t *testing.T) {
	dir := conflictedRepo(t)
	Run(dir, "merge", "--abort")
	Run(dir, "cherry-pick", "side")
	write(t, dir, "numbered.txt", numbered(30))
	if out, err := MarkResolved(dir, "numbered.txt"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	r, err := ContinueMerge(dir, "")
	if err != nil {
		t.Fatalf("%v\n%s", err, r.Output)
	}
	if r.Subject != "theirs" {
		t.Errorf("subject = %q, want the picked commit's", r.Subject)
	}
	if p := parents(t, dir); len(p) != 1 {
		t.Errorf("parents = %v, want one", p)
	}
	picked, _ := Run(dir, "log", "-1", "--format=%an %at", "side")
	if got, _ := Run(dir, "log", "-1", "--format=%an %at"); got != picked {
		t.Errorf("author = %q, want the picked commit's %q", got, picked)
	}
	if ReadWorkingState(dir).Operation != "" {
		t.Error("still cherry-picking")
	}
}

// A hook that refuses the merge commit is reported, and the merge is left
// as it was, to be continued once the hook is satisfied.
func TestContinueMergeReportsAHookThatRefuses(t *testing.T) {
	dir := conflictedRepo(t)
	write(t, dir, "numbered.txt", numbered(30))
	MarkResolved(dir, "numbered.txt")
	hooks := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "commit-msg"), []byte("#!/bin/sh\necho not today >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := head(t, dir)
	r, err := ContinueMerge(dir, "")
	if err == nil {
		t.Fatal("the commit went through a hook that refused it")
	}
	if !strings.Contains(r.Output, "not today") {
		t.Errorf("output = %q, want the hook's words", r.Output)
	}
	if head(t, dir) != before || ReadWorkingState(dir).Operation != "merge" {
		t.Error("the merge was not left as it was")
	}
}

// Trailing spaces are diff --check's business too, but not a conflict.
func TestConflictMarkersIgnoresWhitespace(t *testing.T) {
	dir, _ := stageRepo(t)
	write(t, dir, "numbered.txt", numbered(30)+"trailing   \n")
	if got, err := ConflictMarkers(dir); err != nil || len(got) != 0 {
		t.Errorf("got %q, %v; want nothing", got, err)
	}
	write(t, dir, "numbered.txt", numbered(30)+"<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> side\n")
	if got, _ := ConflictMarkers(dir); len(got) != 1 || got[0] != "numbered.txt" {
		t.Errorf("got %q, want numbered.txt", got)
	}
}

func TestContinueMergeRefusedOnNoBranch(t *testing.T) {
	dir := conflictedRepo(t)
	write(t, dir, "numbered.txt", numbered(30))
	MarkResolved(dir, "numbered.txt")
	h := head(t, dir)
	// Detach without leaving the merge: HEAD written as a bare hash.
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte(h+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ContinueMerge(dir, ""); !errors.Is(err, ErrDetached) {
		t.Errorf("err = %v, want ErrDetached", err)
	}
	if head(t, dir) != h {
		t.Error("a commit was made on no branch")
	}
}

// conflictEntry finds path among the conflicted files of a WorkingTree.
func conflictEntry(t *testing.T, files []FileDiff, path string) FileDiff {
	t.Helper()
	for _, f := range files {
		if f.Path == path && f.Conflicted {
			return f
		}
	}
	t.Fatalf("%s is not listed as conflicted in %+v", path, files)
	return FileDiff{}
}

func TestWorkingTreeListsConflictsFirst(t *testing.T) {
	dir, git := conflictKindsRepo(t)
	d := WorkingTree(dir, 80)
	if d.Conflict.Operation != "merge" || d.Conflict.Theirs != "feature/login" || d.Conflict.Ours != "main" {
		t.Fatalf("conflict = %+v", d.Conflict)
	}
	var got []string
	for _, f := range d.Files {
		mark := " "
		switch {
		case f.Conflicted && f.Resolved:
			mark = "✓"
		case f.Conflicted:
			mark = "✗"
		}
		got = append(got, mark+f.Path)
	}
	want := []string{"✗added.txt", "✗both.txt", "✗ours-del.txt", "✗their-del.txt", " numbered.txt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("files = %v, want %v", got, want)
	}
	both := conflictEntry(t, d.Files, "both.txt")
	if both.Kind != BothModified || both.Staged ||
		both.Body != "a\n<<<<<<< HEAD\nB, our way\n=======\nB, their way\n>>>>>>> feature/login\nc" {
		t.Errorf("both.txt = %+v, want its contents on disk", both)
	}
	if nums := both.LineNumbers(); len(nums) != 7 || nums[0].New != 1 || nums[6].New != 7 {
		t.Errorf("both.txt numbered %v, want 1 to 7", nums)
	}
	// Deleted by us: git leaves their version on disk.
	if f := conflictEntry(t, d.Files, "ours-del.txt"); f.Kind != DeletedByUs || f.Body != "changed by them" {
		t.Errorf("ours-del.txt = %+v", f)
	}
	if f := d.Files[4]; !f.Staged || f.Conflicted || !strings.Contains(f.Body, "+line twenty-five") {
		t.Errorf("numbered.txt = %+v, want staged, as merged cleanly", f)
	}
	// What git writes for an unmerged path is in no file's diff.
	for _, f := range d.Files {
		if strings.Contains(f.Body, "Unmerged path") || strings.Contains(f.Body, "diff --cc") {
			t.Errorf("%s holds another file's conflict: %q", f.Path, f.Body)
		}
	}

	// Resolved in a terminal: an edit, our side as it was, and a deletion.
	write(t, dir, "both.txt", "a\nB, both ways\nc\n")
	git("add", "both.txt")
	git("checkout", "--ours", "added.txt")
	git("add", "added.txt")
	git("rm", "-q", "their-del.txt")
	d = WorkingTree(dir, 80)
	got = nil
	for _, f := range d.Files {
		got = append(got, f.Path)
	}
	if strings.Join(got, ",") != strings.Join([]string{"added.txt", "both.txt", "ours-del.txt", "their-del.txt", "numbered.txt"}, ",") {
		t.Fatalf("after resolving, files = %v, want the same rows", got)
	}
	if f := conflictEntry(t, d.Files, "both.txt"); !f.Resolved || !f.Staged || f.Kind != BothModified || !strings.Contains(f.Body, "+B, both ways") {
		t.Errorf("both.txt = %+v, want resolved, as its staged diff", f)
	}
	if f := conflictEntry(t, d.Files, "added.txt"); !f.Resolved || f.Kind != BothAdded || f.Body != "" {
		t.Errorf("added.txt = %+v, want resolved, with nothing HEAD lacks", f)
	}
	if f := conflictEntry(t, d.Files, "their-del.txt"); !f.Resolved || f.Kind != DeletedByThem || f.Removed != 1 {
		t.Errorf("their-del.txt = %+v, want resolved as a deletion", f)
	}
	if f := conflictEntry(t, d.Files, "ours-del.txt"); f.Resolved {
		t.Errorf("ours-del.txt = %+v, want still unmerged", f)
	}
}

func TestWorkingTreeForgetsAnEarlierMergesConflicts(t *testing.T) {
	dir, git := conflictKindsRepo(t)
	write(t, dir, "both.txt", "a\nB, both ways\nc\n")
	write(t, dir, "added.txt", "ours and theirs\n")
	git("rm", "-q", "ours-del.txt")
	git("add", "-A")
	git("commit", "-q", "--no-edit")

	// The index still remembers those conflicts; this merge has others.
	git("switch", "-qc", "next", "HEAD~1")
	write(t, dir, "numbered.txt", "next's\n")
	git("add", "-A")
	git("commit", "-qm", "next")
	git("switch", "-q", "main")
	write(t, dir, "numbered.txt", "main's\n")
	git("commit", "-qam", "main's")
	Run(dir, "merge", "next")

	d := WorkingTree(dir, 80)
	for _, f := range d.Files {
		if f.Conflicted && f.Path != "numbered.txt" {
			t.Errorf("%s is listed as conflicted: %+v", f.Path, f)
		}
	}
	if f := conflictEntry(t, d.Files, "numbered.txt"); f.Resolved {
		t.Errorf("numbered.txt = %+v", f)
	}
}

func TestWorkingTreeCherryPickConflicts(t *testing.T) {
	dir := conflictedRepo(t)
	Run(dir, "merge", "--abort")
	Run(dir, "cherry-pick", "side")
	d := WorkingTree(dir, 80)
	if d.Conflict.Operation != "cherry-pick" || d.Conflict.Theirs != "side" {
		t.Errorf("conflict = %+v", d.Conflict)
	}
	if f := conflictEntry(t, d.Files, "numbered.txt"); f.Resolved || !strings.Contains(f.Body, "<<<<<<<") {
		t.Errorf("numbered.txt = %+v", f)
	}
}

func TestKindOfStages(t *testing.T) {
	for _, c := range []struct {
		base, ours, theirs bool
		want               ConflictKind
	}{
		{true, true, true, BothModified},
		{false, true, true, BothAdded},
		{true, false, true, DeletedByUs},
		{true, true, false, DeletedByThem},
		{false, true, false, AddedByUs},
		{false, false, true, AddedByThem},
		{true, false, false, BothDeleted},
	} {
		if got := kindOfStages(c.base, c.ours, c.theirs); got != c.want {
			t.Errorf("stages %v %v %v: %v, want %v", c.base, c.ours, c.theirs, got, c.want)
		}
	}
}
