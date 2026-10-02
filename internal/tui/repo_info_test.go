package tui

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/theme"

	"github.com/sevenam/gitraffe/themes"
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
	// The default theme, not theme.Load(): that reads the user's own theme file.
	saved := theme.Current
	theme.Current = theme.Default()
	initStyles()
	t.Cleanup(func() {
		theme.Current = saved
		initStyles()
	})

	fg := func(s lipgloss.Style) string { return fmt.Sprint(s.GetForeground()) }
	ahead, behind, branch := fg(aheadStyle), fg(behindStyle), fg(localBranchStyle)
	commit := fmt.Sprint(lipgloss.Color(theme.Current.Hash))

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
	bundled, err := fs.Glob(themes.FS, "*.yml")
	if err != nil || len(bundled) == 0 {
		t.Fatalf("no bundled themes found: %v", err)
	}
	for _, path := range bundled {
		data, err := themes.FS.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		c, err := theme.Parse(data, false)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
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
				t.Errorf("%s: %s both %s", path, name, pair[0])
			}
		}
	}
}
