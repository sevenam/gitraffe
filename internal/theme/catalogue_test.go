package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sevenam/gitraffe/internal/config"
)

// restoreTheme puts the default colours back after a test that changes them.
func restoreTheme(t *testing.T) {
	t.Cleanup(func() { Current, CurrentName = Default(), DefaultName })
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(choices []Choice) []string {
	var out []string
	for _, c := range choices {
		out = append(out, c.Name)
	}
	return out
}

// The binary carries its own copy, so a theme added to themes/ and left out of
// the embed would be missing for everyone who installed with "go install".
func TestEveryThemeFileIsBundled(t *testing.T) {
	onDisk, _ := filepath.Glob("../../themes/*.yml")
	if len(onDisk) == 0 {
		t.Fatal("no themes on disk")
	}
	choices := Available("")
	for _, p := range onDisk {
		name := strings.TrimSuffix(filepath.Base(p), ".yml")
		c, ok := Find(choices, name)
		if !ok {
			t.Errorf("%s is not bundled", name)
			continue
		}
		colors, err := c.Load()
		if err != nil {
			t.Errorf("%s: %v", name, err)
		} else if colors == Default() {
			t.Errorf("%s: every colour is still the default", name)
		}
	}
}

func TestAvailableThemes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "themes", "mine.yml"), "colors:\n  branch: \"#111111\"\n")
	writeFile(t, filepath.Join(dir, "themes", "tokyo-night-day.yml"), "colors:\n  branch: \"#222222\"\n")
	writeFile(t, filepath.Join(dir, "theme.yml"), "colors:\n  branch: \"#333333\"\n")

	// The default and its light variant first, then the other bundled themes by
	// name (the one replaced keeps its place), then yours.
	want := []string{"default", "default-light"}
	bundled, _ := filepath.Glob("../../themes/*.yml")
	for _, p := range bundled {
		if name := strings.TrimSuffix(filepath.Base(p), ".yml"); name != "default-light" {
			want = append(want, name)
		}
	}
	want = append(want, "mine", "theme.yml")

	choices := Available(dir)
	if got := strings.Join(names(choices), ","); got != strings.Join(want, ",") {
		t.Fatalf("themes = %s\nwant     %s", got, strings.Join(want, ","))
	}

	// A file of yours with a bundled theme's name takes its place.
	c, _ := Find(choices, "tokyo-night-day")
	if colors, _ := c.Load(); colors.Branch != "#222222" || c.Source != "yours" {
		t.Errorf("tokyo-night-day = %s from %q, want yours with #222222", colors.Branch, c.Source)
	}
}

func TestLoadThemeFromConfigDir(t *testing.T) {
	restoreTheme(t)
	custom := "colors:\n  branch: \"#333333\"\n"

	t.Run("theme.yml without a picked theme", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "theme.yml"), custom)
		if err := Load("", dir); err != nil {
			t.Fatal(err)
		}
		if Current.Branch != "#333333" || CurrentName != CustomName {
			t.Errorf("got %s (%q), want theme.yml", Current.Branch, CurrentName)
		}
	})

	t.Run("a picked theme wins over theme.yml", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "theme.yml"), custom)
		if err := config.SaveTheme(dir, "tokyo-night-storm"); err != nil {
			t.Fatal(err)
		}
		if err := Load("", dir); err != nil {
			t.Fatal(err)
		}
		if CurrentName != "tokyo-night-storm" || Current.Branch == "#333333" {
			t.Errorf("got %q, want tokyo-night-storm", CurrentName)
		}
	})

	t.Run("a picked theme that is gone falls back to theme.yml", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "theme.yml"), custom)
		if err := config.SaveTheme(dir, "deleted-since"); err != nil {
			t.Fatal(err)
		}
		if err := Load("", dir); err != nil {
			t.Fatal(err)
		}
		if CurrentName != CustomName {
			t.Errorf("got %q, want theme.yml", CurrentName)
		}
	})
}
