package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/theme"
)

// A checkpoint's colour is the one thing telling it from a branch, so in no
// bundled theme may it be a colour another label or a graph lane already has.
func TestCheckpointColourIsItsOwn(t *testing.T) {
	defer setTheme(theme.DefaultName, theme.Default())

	colour := func(s lipgloss.Style) string {
		if c, ok := s.GetForeground().(lipgloss.Color); ok {
			return strings.ToLower(string(c))
		}
		return ""
	}
	for _, choice := range theme.Available(t.TempDir()) {
		colours, err := choice.Load()
		if err != nil {
			t.Fatalf("%s: %v", choice.Name, err)
		}
		setTheme(choice.Name, colours)

		taken := map[string]string{
			colour(localBranchStyle):  "local branch",
			colour(remoteBranchStyle): "remote branch",
			colour(tagStyle):          "tag",
			colour(mergedBranchStyle): "merged branch",
			colour(aheadStyle):        "ahead",
			colour(behindStyle):       "behind",
		}
		for _, lane := range buildLanePalette() {
			taken[colour(lane)] = "a lane"
		}
		if what, ok := taken[colour(checkpointStyle)]; ok {
			t.Errorf("%s: checkpoints are %s, the colour of %s", choice.Name, colour(checkpointStyle), what)
		}
	}
}
