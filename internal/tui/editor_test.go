package tui

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
)

// fakeEditor stands in for the editor: nobody can type into one in a test.
// It records the command it was handed and, in place of a person, does
// change to the file it was given.
type fakeEditor struct {
	calls [][]string
	dirs  []string
	write string // written to the file, when not ""
}

func (f *fakeEditor) install(t *testing.T) {
	t.Helper()
	saved := runEditor
	t.Cleanup(func() { runEditor = saved })
	runEditor = func(c *editorProcess, done tea.ExecCallback) tea.Cmd {
		return func() tea.Msg {
			f.calls = append(f.calls, c.Args)
			f.dirs = append(f.dirs, c.Dir)
			if f.write != "" {
				if err := os.WriteFile(c.Args[len(c.Args)-1], []byte(f.write), 0o644); err != nil {
					return done(err)
				}
			}
			return done(nil)
		}
	}
}

// withEditor gives the model a settings.yml naming editor.
func withEditor(t *testing.T, m model, editor string) model {
	t.Helper()
	m.configDir = t.TempDir()
	quoted := "'" + strings.ReplaceAll(editor, "'", "''") + "'"
	writeFile(t, filepath.Join(m.configDir, "settings.yml"), "editor: "+quoted+"\n")
	return m
}

// edit presses e and plays out what follows, as the program would: the
// editor worked out, run, and the changes read again once it closes.
func edit(t *testing.T, m model) model {
	t.Helper()
	var run func(cmd tea.Cmd)
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				run(c)
			}
			return
		}
		switch msg.(type) {
		case editorReadyMsg, editorClosedMsg, stageFinishedMsg:
			next, cmd := m.Update(msg)
			m = next.(model)
			run(cmd)
		}
	}
	next, cmd := m.Update(keyPress("e"))
	m = next.(model)
	run(cmd)
	return m
}

func TestEditOpensTheSelectedFileAndReadsItAgain(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	write(t, dir, "notes.txt", "a note, changed\n")
	editor := &fakeEditor{write: "a note, changed in the editor\n"}
	editor.install(t)

	// "git" is the one program every machine running these tests has.
	m := withEditor(t, openedView(t, dir), "git -c 'x.y=a b' --wait")
	if !strings.Contains(ansi.Strip(m.View()), "e: edit") {
		t.Errorf("the status line does not offer e:\n%s", ansi.Strip(m.View()))
	}
	if got := selectedEntry(t, m); got != "notes.txt:unstaged" {
		t.Fatalf("selected %s", got)
	}

	m = edit(t, m)
	if len(editor.calls) != 1 {
		t.Fatalf("the editor ran %d times", len(editor.calls))
	}
	args := editor.calls[0]
	// The setting split as a shell would, and the file appended whole.
	if len(args) != 5 || args[0] != "git" || args[1] != "-c" || args[2] != "x.y=a b" || args[3] != "--wait" {
		t.Errorf("args = %q", args)
	}
	if file := args[len(args)-1]; !filepath.IsAbs(file) || filepath.Base(file) != "notes.txt" || filepath.Dir(file) != editor.dirs[0] {
		t.Errorf("file %q in %q, want notes.txt by its full path, from the repository's top", file, editor.dirs[0])
	}
	if top, _ := git.Toplevel(dir); editor.dirs[0] != top {
		t.Errorf("ran in %q, want the repository's top %q", editor.dirs[0], top)
	}

	// Read again, the selection where it was.
	if got := selectedEntry(t, m); got != "notes.txt:unstaged" {
		t.Errorf("selected %s after the editor closed", got)
	}
	if m.staging {
		t.Error("still reading the changes")
	}
	if !strings.Contains(m.notice, "Back from the editor: notes.txt") {
		t.Errorf("notice = %q", m.notice)
	}
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "changed in the editor") {
		t.Errorf("the diff is not the file as the editor left it:\n%s", screen)
	}

	// Without the setting it is git's own editor.
	m.configDir = t.TempDir()
	t.Setenv("GIT_EDITOR", "git var")
	m = edit(t, m)
	if len(editor.calls) != 2 || strings.Join(editor.calls[1][:2], " ") != "git var" {
		t.Errorf("without the setting the editor was %q, want git's", editor.calls[len(editor.calls)-1])
	}
}

func TestEditSaysWhyNothingOpened(t *testing.T) {
	dir, gitCmd := stagingRepo(t)
	write(t, dir, "notes.txt", "a note, changed\n")
	editor := &fakeEditor{}
	editor.install(t)

	m := withEditor(t, openedView(t, dir), "no-such-editor-for-gitraffe --wait")
	m = edit(t, m)
	if len(editor.calls) != 0 {
		t.Fatal("an editor that is not there was run")
	}
	if !strings.Contains(m.notice, "could not start “no-such-editor-for-gitraffe --wait”") ||
		!strings.Contains(m.notice, "editor: in settings.yml") {
		t.Errorf("notice = %q", m.notice)
	}

	// A quote left open.
	m = edit(t, withEditor(t, m, `"code --wait`))
	if !strings.Contains(m.notice, "cannot read the editor command") {
		t.Errorf("notice = %q", m.notice)
	}

	// A deleted file has nothing on disk to open.
	gitCmd("rm", "-q", "numbered.txt")
	m = edit(t, press(withEditor(t, openedView(t, dir), "git"), keyPress("j")))
	if got := selectedEntry(t, m); got != "numbered.txt:staged" {
		t.Fatalf("selected %s", got)
	}
	if len(editor.calls) != 0 || !strings.Contains(m.notice, "numbered.txt is not on disk") {
		t.Errorf("calls %d, notice %q", len(editor.calls), m.notice)
	}
}

// The conflict view is where e earns its keep: the file opens with its
// markers, and once they are edited out a marks it resolved.
func TestEditAConflictedFile(t *testing.T) {
	dir, _ := conflictRepo(t)
	editor := &fakeEditor{write: "a\nB, both ways\nc\n"}
	editor.install(t)

	m := withEditor(t, press(withDiff(t, loadedModel(t, dir)), space), "git")
	if !m.inConflict() {
		t.Fatal("not the conflict view")
	}
	m = edit(t, m)
	if len(editor.calls) != 1 || filepath.Base(editor.calls[0][1]) != "both.txt" {
		t.Fatalf("the editor was handed %q", editor.calls)
	}
	// Edited, not resolved: that is still a's to say.
	if f, ok := m.selectedFile(); !ok || f.Path != "both.txt" || !f.Conflicted || f.Resolved {
		t.Errorf("after the editor: %+v", f)
	}
	if screen := ansi.Strip(m.View()); !strings.Contains(screen, "B, both ways") {
		t.Errorf("the view does not show the file as edited:\n%s", screen)
	}
	m = do(t, m, keyPress("a"))
	if f, _ := m.selectedFile(); !f.Resolved {
		t.Errorf("a did not resolve the edited file: notice %q", m.notice)
	}
}

// On a commit there is no file on disk to edit.
func TestEditDoesNothingOnACommit(t *testing.T) {
	dir, gitCmd := stagingRepo(t)
	write(t, dir, "notes.txt", "a note, changed\n")
	gitCmd("commit", "-qam", "second")
	editor := &fakeEditor{}
	editor.install(t)
	m := withEditor(t, openedView(t, dir), "git")
	if m.commitView.workingTree {
		t.Fatal("the view is on the uncommitted changes")
	}
	m = edit(t, m)
	if len(editor.calls) != 0 {
		t.Error("e opened an editor on a commit")
	}
}

// On Windows Bubble Tea reads no input of its own and passes on none; the
// editor must still get the console's, not an empty file.
func TestEditorProcessGetsTheTerminalsInput(t *testing.T) {
	c := &editorProcess{Cmd: exec.Command("git")}
	c.SetStdin(nil)
	if c.Stdin != os.Stdin {
		t.Errorf("stdin = %v, want the terminal's", c.Stdin)
	}
	given := &bytes.Buffer{}
	c = &editorProcess{Cmd: exec.Command("git")}
	c.SetStdin(given)
	if c.Stdin != given {
		t.Error("the input Bubble Tea reads was not passed on")
	}
}
