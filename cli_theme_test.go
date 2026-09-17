package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want cliOptions
	}{
		{"nothing", nil, cliOptions{repoPath: "."}},
		{"repo path", []string{`D:\repo`}, cliOptions{repoPath: `D:\repo`}},
		{"theme before path", []string{"-theme", `.\themes\a.yml`, "repo"}, cliOptions{repoPath: "repo", themePath: `.\themes\a.yml`}},
		// Go's flag package alone would stop at "repo" and drop the theme.
		{"theme after path", []string{"repo", "-theme", "a.yml"}, cliOptions{repoPath: "repo", themePath: "a.yml"}},
		{"theme only", []string{"-theme", "a.yml"}, cliOptions{repoPath: ".", themePath: "a.yml"}},
		{"double dash", []string{"--theme", "a.yml"}, cliOptions{repoPath: ".", themePath: "a.yml"}},
		{"equals form", []string{"-theme=a.yml"}, cliOptions{repoPath: ".", themePath: "a.yml"}},
		{"update flag", []string{"-update"}, cliOptions{repoPath: ".", update: true}},
		{"update flag, double dash", []string{"--update"}, cliOptions{repoPath: ".", update: true}},
		{"version flag", []string{"-version"}, cliOptions{repoPath: ".", showVersion: true}},
		{"version flag, double dash", []string{"--version"}, cliOptions{repoPath: ".", showVersion: true}},
		// The bare word predates the flag and stays as an alias for it.
		{"update alias", []string{"update"}, cliOptions{repoPath: ".", update: true}},
		// Which is why a repository in a directory of that name needs a path.
		{"directory called update", []string{"./update"}, cliOptions{repoPath: "./update"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := parseArgs(tc.args, &out)
			if err != nil {
				t.Fatalf("parseArgs(%q) error: %v\n%s", tc.args, err, out.String())
			}
			if got != tc.want {
				t.Errorf("parseArgs(%q) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseArgsRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		says string
	}{
		{"two paths", []string{"a", "b"}, "at most one repository path"},
		{"unknown flag", []string{"-colour", "x"}, "not defined"},
		{"theme without a file", []string{"-theme"}, "needs an argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if _, err := parseArgs(tc.args, &out); err == nil {
				t.Fatalf("parseArgs(%q) succeeded, want an error", tc.args)
			}
			if !strings.Contains(out.String(), tc.says) || !strings.Contains(out.String(), "Usage:") {
				t.Errorf("parseArgs(%q) printed %q, want it to say %q and show usage", tc.args, out.String(), tc.says)
			}
		})
	}

	var out bytes.Buffer
	if _, err := parseArgs([]string{"-h"}, &out); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h returned %v, want flag.ErrHelp so main exits cleanly", err)
	}
	if !strings.Contains(out.String(), "-theme") || !strings.Contains(out.String(), "--theme") {
		t.Errorf("-h output doesn't mention both -theme and --theme: %q", out.String())
	}
}

func TestLoadThemeFromFlag(t *testing.T) {
	saved := currentTheme
	t.Cleanup(func() {
		currentTheme = saved
		initStyles()
	})
	write := func(t *testing.T, body string) string {
		path := filepath.Join(t.TempDir(), "theme.yml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("every bundled theme loads and changes the colours", func(t *testing.T) {
		themes, _ := filepath.Glob("themes/*.yml")
		if len(themes) == 0 {
			t.Fatal("no bundled themes found")
		}
		for _, path := range themes {
			if err := loadTheme(path); err != nil {
				t.Errorf("%s: %v", path, err)
				continue
			}
			if currentTheme == defaultTheme() {
				t.Errorf("%s: loaded but every colour is still the default", path)
			}
		}
	})

	t.Run("a partial file keeps the other defaults", func(t *testing.T) {
		if err := loadTheme(write(t, "colors:\n  branch: \"#123456\"\n")); err != nil {
			t.Fatal(err)
		}
		want := defaultTheme()
		want.Branch = "#123456"
		if currentTheme != want {
			t.Errorf("theme = %+v, want defaults with only branch changed", currentTheme)
		}
	})

	t.Run("an empty file is just the defaults", func(t *testing.T) {
		if err := loadTheme(write(t, "")); err != nil || currentTheme != defaultTheme() {
			t.Errorf("err = %v, defaults = %v", err, currentTheme == defaultTheme())
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
			err := loadTheme(write(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("error = %v, want one mentioning %q", err, tc.says)
			}
		})
	}

	t.Run("fails on a missing file", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope.yml")
		if err := loadTheme(missing); err == nil || !strings.Contains(err.Error(), "nope.yml") {
			t.Errorf("error = %v, want one naming the missing file", err)
		}
	})
}
