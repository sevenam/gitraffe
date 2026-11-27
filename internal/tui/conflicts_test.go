package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// conflictRepo is main in the middle of a merge of feature/login, started as
// a terminal would start it, that stopped on both.txt and gone.txt (deleted
// by feature/login, changed on main); added.txt came in cleanly.
func conflictRepo(t *testing.T) (dir string, gitCmd func(...string)) {
	t.Helper()
	dir, gitCmd = stagingRepo(t)
	write(t, dir, "both.txt", "a\nb\nc\n")
	write(t, dir, "gone.txt", "kept\n")
	gitCmd("add", "-A")
	gitCmd("commit", "-qm", "base")

	gitCmd("switch", "-qc", "feature/login")
	write(t, dir, "both.txt", "a\nB, their way\nc\n")
	write(t, dir, "added.txt", "new on the branch\n")
	gitCmd("rm", "-q", "gone.txt")
	gitCmd("add", "-A")
	gitCmd("commit", "-qm", "theirs")

	gitCmd("switch", "-q", "main")
	write(t, dir, "both.txt", "a\nB, our way\nc\n")
	write(t, dir, "gone.txt", "changed on main\n")
	gitCmd("commit", "-qam", "ours")

	// It fails, which is the point.
	git.Run(dir, "merge", "feature/login")
	if out, _ := git.Run(dir, "ls-files", "--unmerged"); out == "" {
		t.Fatal("the merge did not conflict")
	}
	return dir, gitCmd
}

// reloadedView reads the repository again, as auto-refresh or r would,
// with the commit view left open.
func reloadedView(t *testing.T, m model, dir string) model {
	t.Helper()
	m, _ = m.reloadRepo()
	m = res(m.Update(loadRepo(dir)()))
	// The reload draws the changes as they were until they are read again,
	// which is the command it hands back.
	return res(m.Update(loadWorkingDiffCmd(dir, m.diffStatWidth())()))
}

func TestConflictViewShowsWhereTheMergeStands(t *testing.T) {
	dir, _ := conflictRepo(t)
	m := withDiff(t, loadedModel(t, dir))
	m.windowWidth, m.windowHeight = 120, 30

	if got := m.commits[0].Message; got != "merge in progress — 2 conflicts" {
		t.Errorf("the graph's top row says %q", got)
	}
	if !strings.Contains(ansi.Strip(m.View()), "merge in progress — 2 conflicts") {
		t.Error("the graph does not show the merge in progress")
	}

	m = press(m, space)
	if !m.inConflict() {
		t.Fatal("the view is not the conflict view")
	}
	screen := ansi.Strip(m.View())
	for _, want := range []string{
		"Merging feature/login into main — 2 of 2 files still in conflict",
		"> " + unresolvedMarker + " both.txt",
		unresolvedMarker + " gone.txt",
		"deleted by feature/log", // cut to fit the column
		stagedMarker + " added.txt",
		"both.txt  in conflict — both modified",
		"<<<<<<< HEAD",
		"B, our way",
		"=======",
		"B, their way",
		">>>>>>> feature/login",
		"o: keep main • t: take feature/login • e: edit • a: mark resolved • A: abort",
	} {
		if !strings.Contains(screen, want) {
			t.Errorf("the screen lacks %q:\n%s", want, screen)
		}
	}
	// Conflicted files first, whatever their names; then what merged.
	if got := strings.Join(paths(m.viewedFiles()), " "); got != "both.txt gone.txt added.txt" {
		t.Errorf("files = %s", got)
	}
	if strings.Index(screen, "both.txt  ") > strings.Index(screen, "added.txt") {
		t.Errorf("both.txt is not listed first:\n%s", screen)
	}

	// The keys that would stage, discard or stash say how resolving is
	// done, and change nothing.
	for _, key := range []string{"d", "s", "v"} {
		next := press(m, keyPress(key))
		if !strings.Contains(next.notice, "o / t takes a side") || next.staging || next.discard.asking || next.stashBox.open {
			t.Errorf("%s: notice %q, staging=%v", key, next.notice, next.staging)
		}
	}
	if out, _ := git.Run(dir, "ls-files", "--unmerged"); out == "" {
		t.Error("a key resolved a conflict")
	}
	// The help is the conflict view's.
	if help := ansi.Strip(press(m, keyPress("?")).View()); !strings.Contains(help, "abort the merge, after asking") {
		t.Errorf("the help does not list A:\n%s", help)
	}
}

func TestConflictViewColoursTheSides(t *testing.T) {
	withTrueColor(t)
	restoreTheme(t)
	colours := theme.Default()
	colours.ConflictOurs, colours.ConflictTheirs, colours.ConflictBase = "#112233", "#445566", "#778899"
	setTheme(theme.DefaultName, colours)

	dir, gitCmd := conflictRepo(t)
	// diff3 writes the base between the two.
	git.Run(dir, "merge", "--abort")
	gitCmd("config", "merge.conflictStyle", "diff3")
	git.Run(dir, "merge", "feature/login")

	m := press(withDiff(t, loadedModel(t, dir)), space)
	m = press(m, keyPress("3"), keyPress("j"), keyPress("j"), keyPress("j"), keyPress("j"),
		keyPress("j"), keyPress("j"), keyPress("j"), keyPress("j"))
	screen := m.View()
	for _, c := range []struct{ text, rgb string }{
		{"B, our way", "38;2;17;34;51"},
		{"B, their way", "38;2;68;85;102"},
		{"|||||||", "38;2;119;136;153"},
	} {
		found := false
		for _, line := range strings.Split(screen, "\n") {
			if strings.Contains(ansi.Strip(line), c.text) && strings.Contains(line, c.rgb) {
				found = true
			}
		}
		if !found {
			t.Errorf("%q is not drawn in %s", c.text, c.rgb)
		}
	}
}

func TestConflictRegions(t *testing.T) {
	lines := []string{
		"=======", // in the file itself, outside any conflict
		"<<<<<<< HEAD",
		"ours",
		"||||||| base",
		"base",
		"=======",
		"theirs",
		">>>>>>> feature",
		"after",
		"<<<<<<<<< HEAD", // longer than the file's markers: text
		"=======",
		">>>>>>>>> x",
	}
	want := []conflictLine{
		{regionOutside, false},
		{regionOurs, true}, {regionOurs, false},
		{regionBase, true}, {regionBase, false},
		{regionTheirs, true}, {regionTheirs, false}, {regionTheirs, true},
		{regionOutside, false},
		{regionOutside, false}, {regionOutside, false}, {regionOutside, false},
	}
	got := blockRegions(git.ConflictBlocks(lines, git.DefaultMarkerSize), len(lines))
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d %q: %+v, want %+v", i, lines[i], got[i], want[i])
		}
	}
}

func TestConflictViewShowsAFileResolvedInATerminal(t *testing.T) {
	dir, gitCmd := conflictRepo(t)
	m := press(withDiff(t, loadedModel(t, dir)), space)

	m = press(m, keyPress("c"))
	if m.commitPrompt.open || !strings.Contains(m.notice, "2 files still in conflict") {
		t.Errorf("c with conflicts left: prompt open=%v, notice %q", m.commitPrompt.open, m.notice)
	}

	write(t, dir, "both.txt", "a\nB, both ways\nc\n")
	gitCmd("add", "both.txt")
	m = reloadedView(t, m, dir)
	if got := m.commits[0].Message; got != "merge in progress — 1 conflict" {
		t.Errorf("the graph's top row says %q", got)
	}
	screen := ansi.Strip(m.View())
	for _, want := range []string{
		"1 of 2 files still in conflict",
		"> " + resolvedMarker + " both.txt",
		unresolvedMarker + " gone.txt",
		"both.txt  resolved — both modified",
		"+B, both ways",
	} {
		if !strings.Contains(screen, want) {
			t.Errorf("the screen lacks %q:\n%s", want, screen)
		}
	}
}

func TestConflictViewCommitsTheMerge(t *testing.T) {
	dir, gitCmd := conflictRepo(t)
	m := press(withDiff(t, loadedModel(t, dir)), space)

	write(t, dir, "both.txt", "a\nB, both ways\nc\n")
	gitCmd("add", "both.txt")
	gitCmd("rm", "-q", "gone.txt")
	m = reloadedView(t, m, dir)
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "all 2 conflicts resolved — c commits") {
		t.Errorf("the header does not say it is ready:\n%s", screen)
	}

	m = do(t, m, keyPress("c"))
	p := m.commitPrompt
	if !p.open || !p.merge || p.subject.Value() != "Merge branch 'feature/login'" || p.body.Value() != "" {
		t.Fatalf("prompt open=%v merge=%v subject %q body %q, want git's message", p.open, p.merge, p.subject.Value(), p.body.Value())
	}
	if box := ansi.Strip(m.View()); !strings.Contains(box, "Merge commit  feature/login into main") {
		t.Errorf("the box does not name the merge:\n%s", box)
	}
	// Esc keeps what was typed, and c finds it again.
	m.commitPrompt.body.SetValue("Both ways it is.")
	m = press(m, keyPress("esc"))
	m = do(t, m, keyPress("c"))
	if m.commitPrompt.body.Value() != "Both ways it is." {
		t.Errorf("body %q after esc and c, want what was typed", m.commitPrompt.body.Value())
	}

	m = do(t, m, keyPress("enter"))
	if m.committing || !strings.HasPrefix(m.notice, "Committed ") {
		t.Fatalf("committing=%v notice %q", m.committing, m.notice)
	}
	if git.ReadWorkingState(dir).Operation != "" {
		t.Error("the merge is still in progress")
	}
	if body, _ := git.Run(dir, "log", "-1", "--format=%B"); body != "Merge branch 'feature/login'\n\nBoth ways it is." {
		t.Errorf("message = %q", body)
	}
	if parents, _ := git.Run(dir, "log", "-1", "--format=%P"); len(strings.Fields(parents)) != 2 {
		t.Errorf("parents = %q, want a merge commit", parents)
	}
	if m.commitPrompt.merge {
		t.Error("the box still holds the merge's message")
	}
}

func TestConflictViewAbortsTheMergeAfterAsking(t *testing.T) {
	dir, _ := conflictRepo(t)
	before, _ := git.Run(dir, "rev-parse", "HEAD")
	m := press(withDiff(t, loadedModel(t, dir)), space)

	m = press(m, keyPress("A"))
	if !m.abortMerge.asking {
		t.Fatal("A did not ask")
	}
	screen := ansi.Strip(m.View())
	for _, want := range []string{"Abort the merge?", "Merging feature/login into main stops.",
		"Everything goes back to how it was before", "enter: abort", "esc: keep going"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the question lacks %q:\n%s", want, screen)
		}
	}
	// Anything but yes keeps the merge.
	kept := press(m, keyPress("n"))
	if kept.abortMerge.asking || kept.aborting || !kept.commitView.open || git.ReadWorkingState(dir).Operation != "merge" {
		t.Errorf("n: asking=%v aborting=%v, merge %q", kept.abortMerge.asking, kept.aborting, git.ReadWorkingState(dir).Operation)
	}

	m = do(t, m, keyPress("enter"))
	if m.commitView.open || m.notice != "Merge aborted — main is back as it was" {
		t.Errorf("view open=%v, notice %q", m.commitView.open, m.notice)
	}
	if git.ReadWorkingState(dir).Operation != "" {
		t.Error("the merge is still in progress")
	}
	if after, _ := git.Run(dir, "rev-parse", "HEAD"); after != before {
		t.Errorf("HEAD moved from %s to %s", before, after)
	}
	if out, _ := git.Run(dir, "status", "--porcelain"); out != "" {
		t.Errorf("status = %q, want clean", out)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "both.txt"))
	if string(data) != "a\nB, our way\nc\n" {
		t.Errorf("both.txt = %q, want main's", data)
	}
}

func TestConflictViewCherryPick(t *testing.T) {
	dir, _ := conflictRepo(t)
	git.Run(dir, "merge", "--abort")
	git.Run(dir, "cherry-pick", "feature/login")
	m := withDiff(t, loadedModel(t, dir))
	if got := m.commits[0].Message; got != "cherry-pick in progress — 2 conflicts" {
		t.Errorf("the graph's top row says %q", got)
	}
	m = press(m, space)
	screen := ansi.Strip(m.View())
	if !strings.Contains(screen, "Cherry-picking feature/login onto main — 2 of 2 files still in conflict") {
		t.Errorf("header:\n%s", screen)
	}
	if !strings.Contains(ansi.Strip(press(m, keyPress("A")).View()), "Abort the cherry-pick?") {
		t.Error("A does not ask about the cherry-pick")
	}
}

// A merge with nothing changed still has its row, to be committed or
// aborted from.
func TestConflictViewOnAMergeWithNothingToShow(t *testing.T) {
	dir, gitCmd := stagingRepo(t)
	gitCmd("switch", "-qc", "side")
	write(t, dir, "notes.txt", "the same\n")
	gitCmd("commit", "-qam", "side")
	gitCmd("switch", "-q", "main")
	write(t, dir, "notes.txt", "the same\n")
	gitCmd("commit", "-qam", "main")
	// Both made the same change, so the merge changes no file.
	gitCmd("merge", "--no-commit", "-q", "side")

	m := withDiff(t, loadedModel(t, dir))
	if !m.commits[0].WorkingTree || m.commits[0].Message != "merge in progress — conflicts resolved" {
		t.Fatalf("top row = %+v", m.commits[0].Message)
	}
	m = press(m, space)
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "Merging side into main — no conflicts — c commits") {
		t.Errorf("header:\n%s", screen)
	}
}

func TestConflictViewTakesASideAndPutsItBack(t *testing.T) {
	dir, _ := conflictRepo(t)
	m := press(withDiff(t, loadedModel(t, dir)), space)
	m.windowWidth, m.windowHeight = 120, 30

	// both.txt: main's version, from the file list.
	m = do(t, m, keyPress("o"))
	if !strings.Contains(m.notice, "both.txt: main's version taken") {
		t.Errorf("notice %q", m.notice)
	}
	if got := readFile(t, dir, "both.txt"); got != "a\nB, our way\nc\n" {
		t.Errorf("both.txt is %q", got)
	}
	if f, _ := m.selectedFile(); f.Path != "both.txt" || !f.Resolved {
		t.Errorf("selected %+v, want both.txt resolved", f)
	}
	// The notice stands in for the hints until the next key.
	shown := m
	shown.notice = ""
	screen := ansi.Strip(shown.View())
	for _, want := range []string{"Merging feature/login into main — 1 of 2 files still in conflict",
		resolvedMarker + " both.txt", "u: back in conflict"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the screen lacks %q:\n%s", want, screen)
		}
	}
	// A file resolved is not resolved again over the top.
	if next := press(m, keyPress("t")); next.staging || !strings.Contains(next.notice, "u puts it back") {
		t.Errorf("t on a resolved file: staging=%v notice %q", next.staging, next.notice)
	}

	// u: in conflict again, the markers back on disk.
	m = do(t, m, keyPress("u"))
	if !strings.Contains(m.notice, "both.txt is in conflict again") {
		t.Errorf("notice %q", m.notice)
	}
	if got := readFile(t, dir, "both.txt"); !strings.Contains(got, "=======") {
		t.Errorf("both.txt has no markers after u:\n%s", got)
	}
	if f, _ := m.selectedFile(); f.Path != "both.txt" || f.Resolved {
		t.Errorf("selected %+v, want both.txt in conflict", f)
	}

	// a refuses while the markers are there.
	m = do(t, m, keyPress("a"))
	if !strings.Contains(m.notice, "conflict markers are left") {
		t.Errorf("a with markers: notice %q", m.notice)
	}
	// Then, edited, it takes the file as it is.
	write(t, dir, "both.txt", "a\nB, both ways\nc\n")
	m = do(t, m, keyPress("a"))
	if !strings.Contains(m.notice, "both.txt marked resolved") {
		t.Errorf("notice %q", m.notice)
	}
	// And u will not write over what was edited.
	m = do(t, m, keyPress("u"))
	if !strings.Contains(m.notice, "resolved by hand") || readFile(t, dir, "both.txt") != "a\nB, both ways\nc\n" {
		t.Errorf("u on a hand resolution: notice %q", m.notice)
	}

	// gone.txt, deleted by feature/login: the hint says t deletes it, and it
	// does, from the diff box too.
	m = press(m, keyPress("j"))
	if f, _ := m.selectedFile(); f.Path != "gone.txt" {
		t.Fatalf("selected %s", f.Path)
	}
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "o: keep main • t: delete, as feature/login") {
		t.Errorf("the hints for a deletion:\n%s", screen)
	}
	m = press(m, keyPress("3"))
	m = do(t, m, keyPress("t"))
	if !strings.Contains(m.notice, "gone.txt deleted, as on feature/login") {
		t.Errorf("notice %q", m.notice)
	}
	if _, err := os.Stat(filepath.Join(dir, "gone.txt")); !os.IsNotExist(err) {
		t.Error("gone.txt is still there")
	}
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "all 2 conflicts resolved — c commits") {
		t.Errorf("the header does not say it is ready:\n%s", screen)
	}

	// A file that merged cleanly has nothing to take.
	m = press(m, keyPress("2"), keyPress("j"))
	if f, _ := m.selectedFile(); f.Path != "added.txt" {
		t.Fatalf("selected %s", f.Path)
	}
	if next := press(m, keyPress("o")); next.staging || !strings.Contains(next.notice, "merged without a conflict") {
		t.Errorf("o on a clean file: notice %q", next.notice)
	}
}

func readFile(t *testing.T, dir, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
