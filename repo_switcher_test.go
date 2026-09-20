package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// newRepo makes an empty git repository and returns its root as git reports
// it, which is the form the switcher stores and compares.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	root, err := gitToplevel(dir)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func typeText(m model, s string) model {
	for _, r := range s {
		m = press(m, keyPress(string(r)))
	}
	return m
}

var (
	enter = tea.KeyMsg{Type: tea.KeyEnter}
	esc   = tea.KeyMsg{Type: tea.KeyEsc}
	up    = tea.KeyMsg{Type: tea.KeyUp}
	down  = tea.KeyMsg{Type: tea.KeyDown}
	space = tea.KeyMsg{Type: tea.KeySpace}
)

func TestResolveRepo(t *testing.T) {
	root := newRepo(t)
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	notRepo := t.TempDir()

	for _, tc := range []struct {
		name, input, base, want string
	}{
		{"the root", root, "", root},
		{"a folder inside opens the whole repository", sub, "", root},
		{"relative to the open repository", filepath.Join("a", "b"), root, root},
		{"pasted with quotes", `"` + root + `"`, "", root},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveRepo(tc.input, tc.base)
			if err != nil || !samePath(got, tc.want) {
				t.Errorf("resolveRepo(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
			}
		})
	}

	for _, tc := range []struct{ name, input, says string }{
		{"missing folder", filepath.Join(notRepo, "nope"), "no folder"},
		{"not a repository", notRepo, "not in a git repository"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolveRepo(tc.input, ""); err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("error = %v, want one saying %q", err, tc.says)
			}
		})
	}
}

func TestRememberCurrentRepo(t *testing.T) {
	config := t.TempDir()
	if err := saveThemeChoice(config, "default-light"); err != nil {
		t.Fatal(err)
	}
	repos := make([]string, maxRecentRepos+2)
	for i := range repos {
		repos[i] = newRepo(t)
	}
	remember := func(root string) {
		m := initialModel(root)
		m.configDir = config
		m.rememberCurrentRepo()
		if !samePath(m.repoRoot, root) {
			t.Fatalf("repoRoot = %q, want %q", m.repoRoot, root)
		}
	}
	for _, r := range repos {
		remember(r)
	}
	remember(repos[5]) // opened again: moves to the top, not listed twice

	s := loadSettings(config)
	if len(s.RecentRepos) != maxRecentRepos {
		t.Fatalf("%d recent repos, want the cap of %d", len(s.RecentRepos), maxRecentRepos)
	}
	if !samePath(s.RecentRepos[0], repos[5]) || !samePath(s.RecentRepos[1], repos[len(repos)-1]) {
		t.Errorf("order = %v, want the one reopened first, then the newest", s.RecentRepos[:2])
	}
	seen := map[string]bool{}
	for _, p := range s.RecentRepos {
		if seen[strings.ToLower(p)] {
			t.Errorf("%s listed twice", p)
		}
		seen[strings.ToLower(p)] = true
	}
	if s.Theme != "default-light" {
		t.Errorf("theme = %q; saving the recent list dropped it", s.Theme)
	}
}

// switcherModel is a loaded model showing current, with others as recents.
func switcherModel(t *testing.T, current string, others ...string) model {
	t.Helper()
	m := testModel()
	m.configDir = t.TempDir()
	for i := len(others) - 1; i >= 0; i-- {
		o := initialModel(others[i])
		o.configDir = m.configDir
		o.rememberCurrentRepo()
	}
	m.repoPath = current
	m.rememberCurrentRepo()
	return press(m, keyPress("o"))
}

func TestSwitcherListsOtherRecentRepos(t *testing.T) {
	current, a, b := newRepo(t), newRepo(t), newRepo(t)
	gone := newRepo(t)
	m := switcherModel(t, current, a, gone, b)
	if err := os.RemoveAll(gone); err != nil {
		t.Skipf("can't remove %s: %v", gone, err)
	}
	m = press(m, esc, keyPress("o"))

	if !m.switcher.open {
		t.Fatal("r did not open the switcher")
	}
	// Not the one already open, and not one that no longer exists.
	if got := m.switcher.recents; len(got) != 2 || !samePath(got[0], a) || !samePath(got[1], b) {
		t.Errorf("recents = %v, want [%s %s]", got, a, b)
	}
}

func TestSwitcherTypesEveryPrintableKey(t *testing.T) {
	m := switcherModel(t, newRepo(t))
	m = typeText(m, "qtor?c")
	if !m.switcher.open || m.switcher.input.Value() != "qtor?c" {
		t.Errorf("open=%v value=%q; letters that are shortcuts elsewhere must type here",
			m.switcher.open, m.switcher.input.Value())
	}
	if m.showHelp || m.picker.open || !m.colourLanes {
		t.Error("a key typed into the path acted on the app")
	}
	if m = press(m, esc); m.switcher.open {
		t.Error("esc did not close the switcher")
	}
}

func TestSwitcherOpensAHighlightedRecent(t *testing.T) {
	current, a, b := newRepo(t), newRepo(t), newRepo(t)
	m := switcherModel(t, current, a, b)

	m = press(m, down, down, up)
	if m.switcher.cursor != 0 || m.switcher.input.Value() != "" {
		t.Fatalf("cursor=%d value=%q; want the first recent highlighted and the path untouched",
			m.switcher.cursor, m.switcher.input.Value())
	}
	res, _ := m.Update(enter)
	if got := res.(model); !samePath(got.repoPath, a) {
		t.Errorf("enter opened %q, want %s", got.repoPath, a)
	}
}

// folderTree lays out a folder to type paths into: two folders starting "al"
// in different cases, one of them a repository, another folder, a hidden one
// and a file.
func folderTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"beta", "Alpha", ".hidden"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(dir, "album.txt"), "")
	cmd := exec.Command("git", "init", "-q", "alpine")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func itemLabels(s repoSwitcher) string {
	var out []string
	for _, it := range s.items {
		l := it.label
		if it.repo {
			l += "(repo)"
		}
		out = append(out, l)
	}
	return strings.Join(out, ",")
}

const sep = string(os.PathSeparator)

func TestSwitcherSuggestsFolders(t *testing.T) {
	dir := folderTree(t)
	m := switcherModel(t, newRepo(t))

	for _, tc := range []struct{ typed, want string }{
		// Case ignored, sorted, files and hidden folders left out, repositories marked.
		{dir + sep + "al", "Alpha,alpine(repo)"},
		{dir + sep, "Alpha,alpine(repo),beta"},
		{dir + sep + "AL", "Alpha,alpine(repo)"},
		// Hidden folders once the name starts with a dot.
		{dir + sep + ".", ".hidden"},
		{dir + sep + "zz", ""},
	} {
		got := typeText(press(m, esc, keyPress("o")), tc.typed)
		if labels := itemLabels(got.switcher); labels != tc.want {
			t.Errorf("typing %q suggested %q, want %q", tc.typed, labels, tc.want)
		}
	}

	got := typeText(press(m, esc, keyPress("o")), filepath.Join(dir, "missing", "x"))
	if len(got.switcher.items) != 0 || !strings.Contains(got.switcher.empty, "No folder at") {
		t.Errorf("a missing folder gave %q / %q", itemLabels(got.switcher), got.switcher.empty)
	}
}

func TestSwitcherTabCompletes(t *testing.T) {
	dir := folderTree(t)
	m := typeText(switcherModel(t, newRepo(t)), dir+sep+"al")

	// Nothing highlighted: the first match.
	m = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if want := dir + sep + "Alpha" + sep; m.switcher.input.Value() != want {
		t.Errorf("tab gave %q, want %q", m.switcher.input.Value(), want)
	}
	if !strings.Contains(m.switcher.heading, "Alpha") {
		t.Errorf("heading = %q; the list didn't move into the completed folder", m.switcher.heading)
	}

	// Highlighted: that one.
	m = typeText(press(m, esc, keyPress("o")), dir+sep+"al")
	m = press(m, down, down, tea.KeyMsg{Type: tea.KeyTab})
	if want := dir + sep + "alpine" + sep; m.switcher.input.Value() != want {
		t.Errorf("tab on the highlighted folder gave %q, want %q", m.switcher.input.Value(), want)
	}
}

func TestSwitcherEnterGoesIntoFoldersAndOpensRepos(t *testing.T) {
	dir := folderTree(t)
	m := typeText(switcherModel(t, newRepo(t)), dir+sep+"al")

	into := press(m, down, enter) // Alpha, a plain folder
	if !into.switcher.open || into.switcher.input.Value() != dir+sep+"Alpha"+sep {
		t.Errorf("enter on a folder: open=%v value=%q; want it to go into the folder",
			into.switcher.open, into.switcher.input.Value())
	}

	res, _ := press(m, down, down).Update(enter) // alpine, a repository
	want, err := gitToplevel(filepath.Join(dir, "alpine"))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.(model); !samePath(got.repoPath, want) {
		t.Errorf("enter on a repository opened %q, want %s", got.repoPath, want)
	}
}

func TestSwitcherRelativePaths(t *testing.T) {
	current := newRepo(t)
	if err := os.Mkdir(filepath.Join(current, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := typeText(switcherModel(t, current), "do")
	if labels := itemLabels(m.switcher); labels != "docs" {
		t.Errorf("a relative path suggested %q, want the open repository's docs folder", labels)
	}
}

func TestSplitPathInput(t *testing.T) {
	cases := map[string][2]string{
		"":              {"", ""},
		"repo":          {"", "repo"},
		"~":             {"~" + sep, ""},
		"a/b/c":         {"a/b/", "c"},
		"/abs/":         {"/abs/", ""},
		`"quoted/start`: {"quoted/", "start"},
	}
	if runtime.GOOS == "windows" {
		cases[`D:`] = [2]string{`D:\`, ""}
		cases[`D:\git\se`] = [2]string{`D:\git\`, "se"}
		cases[`D:\git/mixed`] = [2]string{`D:\git/`, "mixed"}
	}
	for in, want := range cases {
		if dir, prefix := splitPathInput(in); dir != want[0] || prefix != want[1] {
			t.Errorf("splitPathInput(%q) = %q, %q; want %q, %q", in, dir, prefix, want[0], want[1])
		}
	}
}

func TestSwitcherRefusesWhatIsNotARepo(t *testing.T) {
	m := switcherModel(t, newRepo(t))
	m = typeText(m, t.TempDir())
	m = press(m, enter)
	if !m.switcher.open || !strings.Contains(m.switcher.err, "not in a git repository") {
		t.Errorf("open=%v err=%q; want it kept open with the reason", m.switcher.open, m.switcher.err)
	}
	if m = typeText(m, "x"); m.switcher.err != "" {
		t.Error("the error outlived editing the path")
	}
}

func TestSwitcherOpensTheRepo(t *testing.T) {
	current, other := newRepo(t), newRepo(t)
	m := switcherModel(t, current)
	m.colourLanes = false
	m.latestVersion = "v9.9.9"
	m.commits = make([]commit, 3)
	m.selected = 2

	m = typeText(m, other)
	res, cmd := m.Update(enter)
	got := res.(model)

	if !samePath(got.repoPath, other) || got.ready || cmd == nil {
		t.Fatalf("repoPath=%q ready=%v cmd=%v; want %s loading", got.repoPath, got.ready, cmd != nil, other)
	}
	if got.commits != nil || got.selected != 0 || got.switcher.open {
		t.Error("the previous repository's state carried over")
	}
	if got.windowWidth != m.windowWidth || got.colourLanes || got.latestVersion != "v9.9.9" || got.configDir != m.configDir {
		t.Error("session settings were not carried over")
	}
}

func TestSwitcherOnTheOpenRepoJustCloses(t *testing.T) {
	current := newRepo(t)
	m := switcherModel(t, current)
	m = press(typeText(m, current), enter)
	if m.switcher.open || !m.ready || !strings.Contains(m.notice, "Already showing") {
		t.Errorf("open=%v ready=%v notice=%q", m.switcher.open, m.ready, m.notice)
	}
}

func TestSwitcherWaitsForLoading(t *testing.T) {
	m := testModel()
	m.ready = false
	if m = press(m, keyPress("o")); m.switcher.open {
		t.Error("r opened the switcher while a repository was loading")
	}
}

// Answers for a repository switched away from arrive late; applied to the new
// one they would put the old one's diff and tags on its commits.
func TestLateAnswersFromThePreviousRepoAreDropped(t *testing.T) {
	m := testModel()
	m.repoPath = "/new"
	m.commits = []commit{{Hash: "aaaaaaa", Refs: "tag: refs/tags/v1"}}

	res, _ := m.Update(diffLoadedMsg{repoPath: "/old", commitIdx: 0, diffBody: "old diff"})
	if got := res.(model); got.commits[0].DiffLoaded {
		t.Error("a diff from the previous repository was applied")
	}
	res, _ = m.Update(remoteTagsMsg{repoPath: "/old", tags: map[tagRef]bool{}})
	if got := res.(model); got.remoteTags != nil {
		t.Error("remote tags from the previous repository were applied")
	}
	res, _ = m.Update(diffLoadedMsg{repoPath: "/new", commitIdx: 0, diffBody: "new diff"})
	if got := res.(model); got.commits[0].DiffBody != "new diff" {
		t.Error("the current repository's diff was dropped")
	}
}

func TestSwitcherKeepsScreenSize(t *testing.T) {
	for _, size := range []struct{ w, h int }{{200, 40}, {80, 24}, {40, 12}} {
		m := switcherModel(t, newRepo(t), newRepo(t), newRepo(t))
		m.windowWidth, m.windowHeight = size.w, size.h
		m = typeText(m, strings.Repeat("long/path/", 20))
		for name, out := range map[string]string{"graph": m.View(), "error screen": func() string {
			m.err = os.ErrNotExist
			return m.View()
		}()} {
			lines := strings.Split(out, "\n")
			if name == "graph" && len(lines) != size.h {
				t.Errorf("%s %dx%d: %d lines, want %d", name, size.w, size.h, len(lines), size.h)
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w > size.w {
					t.Errorf("%s %dx%d: line %d is %d wide", name, size.w, size.h, i, w)
				}
			}
			if size.h >= 24 && !strings.Contains(ansi.Strip(out), "Open repository") {
				t.Errorf("%s %dx%d: switcher not drawn", name, size.w, size.h)
			}
		}
	}
}
