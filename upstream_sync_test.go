package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

func TestSyncLabel(t *testing.T) {
	for _, tc := range []struct {
		ahead, behind int
		want          string
	}{
		{0, 0, ""},
		{3, 0, "↑3"},
		{0, 1, "↓1"},
		{3, 1, "↑3 ↓1"},
	} {
		if got := stripANSI(syncLabel(tc.ahead, tc.behind)); got != tc.want {
			t.Errorf("syncLabel(%d, %d) = %q, want %q", tc.ahead, tc.behind, got, tc.want)
		}
	}
}

func TestSyncLabelColours(t *testing.T) {
	withTrueColor(t)
	// The default theme, not loadTheme(): that reads the user's own theme file.
	saved := currentTheme
	currentTheme = defaultTheme()
	initStyles()
	t.Cleanup(func() {
		currentTheme = saved
		initStyles()
	})

	fg := func(s lipgloss.Style) string { return fmt.Sprint(s.GetForeground()) }
	ahead, behind, branch := fg(aheadStyle), fg(behindStyle), fg(localBranchStyle)
	commit := fmt.Sprint(lipgloss.Color(currentTheme.Hash))

	// Each arrow needs its own colour, and neither may blend into the branch
	// name before it or the "Commit:" label after it.
	for _, pair := range [][2]string{
		{"ahead vs behind", ahead + "|" + behind},
		{"ahead vs branch name", ahead + "|" + branch},
		{"behind vs branch name", behind + "|" + branch},
		{"ahead vs commit label", ahead + "|" + commit},
		{"behind vs commit label", behind + "|" + commit},
	} {
		a, b, _ := strings.Cut(pair[1], "|")
		if a == b {
			t.Errorf("%s share the colour %s", pair[0], a)
		}
	}

	label := syncLabel(3, 1)
	if !strings.Contains(label, aheadStyle.Render("↑3")) || !strings.Contains(label, behindStyle.Render("↓1")) {
		t.Errorf("syncLabel(3, 1) = %q, want ↑3 in the ahead style and ↓1 in the behind style", label)
	}
}

func TestSyncColoursFollowBundledThemes(t *testing.T) {
	// Themes don't declare ahead/behind, so they must inherit colours that
	// stay distinct from that theme's branch name (its date colour).
	themes, err := filepath.Glob("themes/*.yml")
	if err != nil || len(themes) == 0 {
		t.Fatalf("no bundled themes found: %v", err)
	}
	for _, path := range themes {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var tf themeFile
		tf.Colors = defaultTheme()
		if err := yaml.Unmarshal(data, &tf); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		c := tf.Colors
		ahead, behind := firstColour(c.Ahead, c.Author), firstColour(c.Behind, c.DiffDel)
		branch := firstColour(c.LocalBranch, c.Date)
		for name, pair := range map[string][2]string{
			"ahead/behind":  {ahead, behind},
			"ahead/branch":  {ahead, branch},
			"behind/branch": {behind, branch},
			"ahead/commit":  {ahead, c.Hash},
			"behind/commit": {behind, c.Hash},
		} {
			if strings.EqualFold(pair[0], pair[1]) {
				t.Errorf("%s: %s both %s", filepath.Base(path), name, pair[0])
			}
		}
	}
}

// gitEnv isolates test repos from the user's git configuration, where settings
// like commit signing or a different default branch would break the fixtures.
func gitEnv(t *testing.T) []string {
	empty := filepath.Join(t.TempDir(), "empty.gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+empty,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
}

func run(t *testing.T, env []string, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func syncOf(dir string) (ahead, behind int) {
	m := &model{repoPath: dir}
	m.loadUpstreamSync()
	return m.ahead, m.behind
}

func TestLoadUpstreamSync(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	env := gitEnv(t)
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	other := filepath.Join(root, "other")

	run(t, env, root, "init", "-q", "--bare", "-b", "main", remote)
	run(t, env, root, "clone", "-q", remote, work)
	run(t, env, work, "commit", "-q", "--allow-empty", "-m", "first")
	run(t, env, work, "push", "-q", "-u", "origin", "main")

	check := func(name string, dir string, wantAhead, wantBehind int) {
		t.Helper()
		if a, b := syncOf(dir); a != wantAhead || b != wantBehind {
			t.Errorf("%s: ahead/behind = %d/%d, want %d/%d", name, a, b, wantAhead, wantBehind)
		}
	}

	check("in sync", work, 0, 0)

	run(t, env, work, "commit", "-q", "--allow-empty", "-m", "local 1")
	run(t, env, work, "commit", "-q", "--allow-empty", "-m", "local 2")
	check("ahead", work, 2, 0)

	// Someone else pushes. Until work fetches, its origin/main hasn't moved.
	run(t, env, root, "clone", "-q", remote, other)
	run(t, env, other, "commit", "-q", "--allow-empty", "-m", "theirs")
	run(t, env, other, "push", "-q", "origin", "main")
	check("remote moved, not yet fetched", work, 2, 0)

	run(t, env, work, "fetch", "-q")
	check("diverged after fetch", work, 2, 1)

	run(t, env, work, "checkout", "-q", "--detach")
	check("detached HEAD", work, 0, 0)

	// Unpushed branches count commits no remote has, and are never "behind".
	run(t, env, work, "checkout", "-q", "-b", "fresh", "origin/main", "--no-track")
	check("new branch, nothing committed yet", work, 0, 0)
	run(t, env, work, "commit", "-q", "--allow-empty", "-m", "feature work")
	check("new branch with one commit, never pushed", work, 1, 0)

	// Pushed, but without -u: the commits are on the remote, so nothing is ahead.
	run(t, env, work, "push", "-q", "origin", "fresh")
	run(t, env, work, "fetch", "-q")
	check("pushed without -u", work, 0, 0)

	// Branched from the diverged local main, whose two commits were never pushed.
	run(t, env, work, "checkout", "-q", "-b", "local-only", "main", "--no-track")
	check("branch with no upstream carrying unpushed commits", work, 2, 0)

	// A tracking branch whose remote branch has since been deleted: its commits
	// are on no remote any more.
	run(t, env, work, "push", "-q", "-u", "origin", "local-only")
	check("tracking and in sync", work, 0, 0)
	run(t, env, other, "push", "-q", "origin", "--delete", "local-only")
	run(t, env, work, "fetch", "-q", "--prune")
	check("upstream deleted", work, 2, 0)

	noRemote := filepath.Join(root, "no-remote")
	run(t, env, root, "init", "-q", "-b", "main", noRemote)
	run(t, env, noRemote, "commit", "-q", "--allow-empty", "-m", "first")
	check("no remote", noRemote, 0, 0)

	check("not a repository", root, 0, 0)
}
