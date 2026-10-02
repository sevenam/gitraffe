package main

import (
	"bytes"
	"errors"
	"flag"
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
		// The flags that do something and exit also answer to a bare word.
		{"update alias", []string{"update"}, cliOptions{repoPath: ".", update: true}},
		{"version alias", []string{"version"}, cliOptions{repoPath: ".", showVersion: true}},
		// Which is why a repository in a directory of either name needs a path.
		{"directory called update", []string{"./update"}, cliOptions{repoPath: "./update"}},
		{"directory called version", []string{"./version"}, cliOptions{repoPath: "./version"}},
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
