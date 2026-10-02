package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadThemeFromFlag(t *testing.T) {
	saved := Current
	t.Cleanup(func() {
		Current = saved
	})
	write := func(t *testing.T, body string) string {
		path := filepath.Join(t.TempDir(), "theme.yml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("every bundled theme loads and changes the colours", func(t *testing.T) {
		themes, _ := filepath.Glob("../../themes/*.yml")
		if len(themes) == 0 {
			t.Fatal("no bundled themes found")
		}
		for _, path := range themes {
			if err := Load(path, ""); err != nil {
				t.Errorf("%s: %v", path, err)
				continue
			}
			if Current == Default() {
				t.Errorf("%s: loaded but every colour is still the default", path)
			}
		}
	})

	t.Run("a partial file keeps the other defaults", func(t *testing.T) {
		if err := Load(write(t, "colors:\n  branch: \"#123456\"\n"), ""); err != nil {
			t.Fatal(err)
		}
		want := Default()
		want.Branch = "#123456"
		if Current != want {
			t.Errorf("theme = %+v, want defaults with only branch changed", Current)
		}
	})

	t.Run("an empty file is just the defaults", func(t *testing.T) {
		if err := Load(write(t, ""), ""); err != nil || Current != Default() {
			t.Errorf("err = %v, defaults = %v", err, Current == Default())
		}
	})

	for _, tc := range []struct {
		name, body, says string
	}{
		{"broken YAML", "colors: [unclosed\n", "theme"},
		// Without strict decoding a typo'd key is silently ignored.
		{"misspelt key", "colors:\n  brnach: \"#123456\"\n", "brnach"},
	} {
		t.Run("fails on "+tc.name, func(t *testing.T) {
			err := Load(write(t, tc.body), "")
			if err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("error = %v, want one mentioning %q", err, tc.says)
			}
		})
	}

	t.Run("fails on a missing file", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope.yml")
		if err := Load(missing, ""); err == nil || !strings.Contains(err.Error(), "nope.yml") {
			t.Errorf("error = %v, want one naming the missing file", err)
		}
	})
}

func TestIsLightColour(t *testing.T) {
	for in, want := range map[string]bool{
		"#e5e7eb": true, "#ffffff": true, "#282c34": false, "#000000": false,
		"": false, "12": false, "#zzzzzz": false,
	} {
		if got := IsLight(in); got != want {
			t.Errorf("IsLight(%q) = %v, want %v", in, got, want)
		}
	}
}
