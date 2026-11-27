package git

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// Editor is the editor git opens for a commit message, as `git var
// GIT_EDITOR` works it out: GIT_EDITOR, core.editor, VISUAL, EDITOR, then
// git's built-in fallback. Asking git rather than reading those ourselves
// keeps the order git's, and anyone who has set git up has already chosen.
//
// git refuses, rather than falling back to vi, on a terminal it calls dumb
// with no editor set; the error then carries what git said.
func Editor(dir string) (string, error) {
	cmd := exec.Command("git", "var", "GIT_EDITOR")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if reason := strings.TrimSpace(string(exit.Stderr)); reason != "" {
				return "", errors.New(firstLineOf(reason))
			}
		}
		return "", err
	}
	editor := strings.TrimSpace(string(out))
	if editor == "" {
		return "", errors.New("git names no editor")
	}
	return editor, nil
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// ErrUnclosedQuote is SplitCommand's answer to a quote that never ends.
var ErrUnclosedQuote = errors.New("a quote in the editor command is never closed")

// SplitCommand splits an editor command into its words the way a shell
// would, which is how git reads it: `code --wait` is two words, and
// 'C:/Program Files/Notepad++/notepad++.exe' -multiInst one path and a flag.
// Single quotes keep everything; double quotes keep everything but a
// backslash before ", \, $ or `. Nothing is expanded: there is no shell, so
// $VAR, pipes and ; are only characters.
//
// On Windows a backslash outside quotes is kept as it is, since there it is
// the path separator, C:\Tools\vim.exe, far more often than an escape.
func SplitCommand(s string) ([]string, error) {
	return splitCommand(s, runtime.GOOS == "windows")
}

func splitCommand(s string, windows bool) ([]string, error) {
	var words []string
	var word strings.Builder
	inWord := false
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		case c == '\'':
			inWord = true
			end := indexRune(r, i+1, '\'')
			if end < 0 {
				return nil, ErrUnclosedQuote
			}
			word.WriteString(string(r[i+1 : end]))
			i = end
		case c == '"':
			inWord = true
			i++
			for ; i < len(r) && r[i] != '"'; i++ {
				if r[i] == '\\' && i+1 < len(r) && strings.ContainsRune("\"\\$`", r[i+1]) {
					i++
				}
				word.WriteRune(r[i])
			}
			if i >= len(r) {
				return nil, ErrUnclosedQuote
			}
		case c == '\\' && !windows:
			inWord = true
			if i+1 < len(r) {
				i++
				word.WriteRune(r[i])
			}
		default:
			inWord = true
			word.WriteRune(c)
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, nil
}

func indexRune(r []rune, from int, want rune) int {
	for i := from; i < len(r); i++ {
		if r[i] == want {
			return i
		}
	}
	return -1
}
