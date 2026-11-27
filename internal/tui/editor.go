package tui

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/config"
	"github.com/sevenam/gitraffe/internal/git"
)

// "e" on the uncommitted changes opens the selected file in the user's own
// editor (issue #243). There is no editor in gitraffe and no list of editors
// to keep: it is the one git opens for a commit message, or the editor
// setting in settings.yml for gitraffe alone. Gitraffe stands aside while it
// runs, as git does, and reads the changes again once it closes.

// editorHint is what to do about an editor that could not be found or run.
const editorHint = "set editor: in settings.yml, e.g. editor: code --wait"

// editorReadyMsg is the editor worked out and the file found, or why not.
type editorReadyMsg struct {
	repoPath string
	path     string // the file, as the list names it
	name     string // the editor's command, as written, for the status line
	cmd      *exec.Cmd
	err      error
}

// editorClosedMsg is the editor gone again.
type editorClosedMsg struct {
	repoPath string
	path     string
	name     string
	err      error
}

// openInEditor is "e": the file selected, from either box.
func (m model) openInEditor() (model, tea.Cmd) {
	if m.staging || m.committing || m.aborting {
		return m, nil
	}
	f, ok := m.selectedFile()
	if !ok {
		return m, nil
	}
	// git quotes a path with characters it will not print, and the list
	// shows it as git wrote it; there is no file of that name to open.
	if strings.HasPrefix(f.Path, `"`) {
		m.notice = "Cannot open " + f.Path + ": git wrote its name quoted"
		return m, nil
	}
	repoPath, configDir, path := m.repoPath, m.configDir, f.Path
	return m, func() tea.Msg {
		return findEditor(repoPath, configDir, path)
	}
}

// findEditor works out the command for "e". The setting is read on each
// press rather than at startup, since it is written by hand and the moment
// it matters is just after someone has.
func findEditor(repoPath, configDir, path string) editorReadyMsg {
	msg := editorReadyMsg{repoPath: repoPath, path: path}
	top, err := git.Toplevel(repoPath)
	if err != nil {
		top = repoPath
	}
	// A rename is listed as "old → new"; the file on disk is the new one.
	file := path
	if i := strings.LastIndex(file, " → "); i >= 0 {
		file = file[i+len(" → "):]
	}
	full := filepath.Join(top, filepath.FromSlash(file))
	if info, err := os.Stat(full); err != nil {
		msg.err = errors.New(file + " is not on disk, so there is nothing to open")
		return msg
	} else if info.IsDir() {
		// An untracked directory, or a submodule.
		msg.err = errors.New(file + " is a directory")
		return msg
	}

	msg.name = config.Load(configDir).Editor
	if msg.name == "" {
		msg.name, err = git.Editor(top)
		if err != nil {
			msg.err = errors.New("no editor: " + err.Error() + " — " + editorHint)
			return msg
		}
	}
	words, err := git.SplitCommand(msg.name)
	if err != nil || len(words) == 0 || words[0] == "" {
		msg.err = errors.New("cannot read the editor command " + quoteName(msg.name) + " — " + editorHint)
		return msg
	}
	// The path goes last, as git puts the message file, and as one word
	// whatever is in it: nothing here passes through a shell.
	msg.cmd = exec.Command(words[0], append(words[1:], full)...)
	msg.cmd.Dir = top
	if msg.cmd.Err != nil {
		// Not on the PATH: said now, rather than after the screen has been
		// handed over and taken back for nothing.
		msg.err = errors.New("could not start " + quoteName(msg.name) + ": " + msg.cmd.Err.Error() + " — " + editorHint)
		msg.cmd = nil
	}
	return msg
}

func quoteName(s string) string { return "“" + s + "”" }

// runEditor hands the terminal to the editor until it exits. It is a
// variable so that tests can stand in for an editor, which they cannot type
// into.
var runEditor = func(c *editorProcess, done tea.ExecCallback) tea.Cmd {
	return tea.Exec(c, done)
}

// startEditor runs the editor findEditor found, or says why there is none.
func (m model) startEditor(msg editorReadyMsg) (model, tea.Cmd) {
	if msg.err != nil {
		m.notice = "Not opened: " + msg.err.Error()
		return m, nil
	}
	repoPath, path, name := msg.repoPath, msg.path, msg.name
	return m, runEditor(&editorProcess{Cmd: msg.cmd}, func(err error) tea.Msg {
		return editorClosedMsg{repoPath: repoPath, path: path, name: name, err: err}
	})
}

// finishEditor reads the changes again, with the selection kept on the file:
// whatever the editor did to it, it is the file being looked at.
func (m model) finishEditor(msg editorClosedMsg) (model, tea.Cmd) {
	var exit *exec.ExitError
	switch {
	case msg.err == nil:
		m.notice = "Back from the editor: " + msg.path
	case errors.As(msg.err, &exit):
		// vim's :cq, or an editor that failed part-way; what it saved is
		// on disk all the same, so the list is read again either way.
		m.notice = "The editor " + quoteName(msg.name) + " exited with " + exit.Error()
	default:
		m.notice = "Not opened: could not start " + quoteName(msg.name) + ": " + msg.err.Error() + " — " + editorHint
	}

	// Handing the terminal over turned the mouse off, and Bubble Tea does
	// not turn it back on when it takes the terminal back. This is the same
	// request it makes at startup, so it does not restart the Windows
	// reader that console_input_windows.go warns about.
	mouse := tea.EnableMouseCellMotion
	if !m.commitView.open || !m.commitView.workingTree || m.staging || m.committing || m.aborting {
		return m, mouse
	}
	files := m.viewedFiles()
	v := &m.commitView
	v.want = []fileKey{{msg.path, false}, {msg.path, true}}
	v.want = append(v.want, neighbours(files, v.file)...)
	m.staging = true
	return m, tea.Batch(mouse, stageCmd(m.repoPath, m.diffStatWidth(), func(string) (string, error) {
		return "", nil
	}))
}

// activeConsole is the Windows console reader while it runs, and nil
// otherwise; see console_input_windows.go.
var activeConsole *consoleInput

// editorProcess is the editor as Bubble Tea runs it: an exec.Cmd, with the
// terminal's input given to it even where Bubble Tea reads none itself, and
// gitraffe's own Windows console reader stood down while it runs.
type editorProcess struct{ *exec.Cmd }

func (c *editorProcess) SetStdin(r io.Reader) {
	// On Windows Bubble Tea is given no input (see run.go) and passes on
	// nil, which exec would turn into an empty file: an editor reading
	// nothing at all.
	if r == nil {
		r = os.Stdin
	}
	if c.Stdin == nil {
		c.Stdin = r
	}
}

func (c *editorProcess) SetStdout(w io.Writer) {
	if c.Stdout == nil {
		c.Stdout = w
	}
}

func (c *editorProcess) SetStderr(w io.Writer) {
	if c.Stderr == nil {
		c.Stderr = w
	}
}

func (c *editorProcess) Run() error {
	// Otherwise the reader goes on taking the keys typed into the editor,
	// and gitraffe acts on them once it is back.
	activeConsole.pause()
	defer activeConsole.resume()
	return c.Cmd.Run()
}
