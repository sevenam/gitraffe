package tui

import (
	"os"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/sevenam/gitraffe/internal/git"
)

// The copy prompt: "y" asks what to copy, and the next key answers it. A
// second "y" takes the hash, the thing most often wanted (for a revert or a
// cherry-pick), so the common case is "yy" as in vim.
//
// It is a prompt on the status line rather than three keys of its own because
// the free letters are few, and h, s and d already mean something to anyone
// who uses vim keys to move around.
const copyPrompt = "Copy: y hash • s subject • d diff • esc cancel"

// copyDiffMsg brings back a diff read for the clipboard.
type copyDiffMsg struct {
	repoPath string // the repository it was read from; see the handler
	patch    string
	err      error
}

// clipboardResult says how text reached a clipboard: the system's own, or the
// terminal's through OSC 52.
type clipboardResult int

const (
	copiedToSystem clipboardResult = iota
	copiedToTerminal
)

// writeClipboard puts text on the clipboard. A variable so tests can see what
// would have been copied without touching the machine's clipboard.
var writeClipboard = func(text string) (clipboardResult, error) {
	if err := clipboard.WriteAll(text); err == nil {
		return copiedToSystem, nil
	}
	// No system clipboard to reach: on Linux without xclip, xsel or
	// wl-copy, and over SSH, where the one that matters is on the other
	// end. OSC 52 asks the terminal to hold the text instead. Whether it
	// did cannot be known — a terminal ignores an escape it doesn't
	// support — so the notice says it was sent, not that it arrived.
	termenv.NewOutput(os.Stdout).Copy(text)
	return copiedToTerminal, nil
}

// openCopyPrompt asks what to copy from the commit on screen.
func (m model) openCopyPrompt() model {
	if !m.ready || m.err != nil {
		return m
	}
	if _, ok := m.commitOnScreen(); !ok {
		return m
	}
	m.copying = true
	return m
}

// answerCopyPrompt copies what the key asks for. Any key it doesn't know
// closes the prompt, so a stray key press costs nothing.
func (m model) answerCopyPrompt(msg tea.KeyMsg) (model, tea.Cmd) {
	m.copying = false
	c, ok := m.commitOnScreen()
	if !ok {
		return m, nil
	}
	switch msg.String() {
	case "y":
		// The uncommitted changes have no hash; saying so beats copying
		// an empty string over whatever the clipboard held.
		if c.WorkingTree {
			m.notice = "Uncommitted changes have no hash to copy"
			return m, nil
		}
		return m.copyText(c.FullHash, "hash "+c.Hash), nil
	case "s":
		if c.WorkingTree {
			m.notice = "Uncommitted changes have no subject to copy"
			return m, nil
		}
		return m.copyText(c.Message, "subject"), nil
	case "d":
		// Read again rather than taken from the details panel, whose copy
		// stops at a few hundred lines: a patch cut short will not apply.
		hash := c.FullHash
		if c.WorkingTree {
			hash = ""
		}
		m.notice = "Reading the diff..."
		return m, copyDiffCmd(m.repoPath, hash)
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func copyDiffCmd(repoPath, hash string) tea.Cmd {
	return func() tea.Msg {
		patch, err := git.Patch(repoPath, hash)
		return copyDiffMsg{repoPath: repoPath, patch: patch, err: err}
	}
}

// finishCopyDiff copies a diff once it has been read.
func (m model) finishCopyDiff(msg copyDiffMsg) model {
	switch {
	case msg.err != nil:
		m.notice = "Could not read the diff: " + msg.err.Error()
	case msg.patch == "":
		// A merge that resolved nothing by hand has no diff of its own.
		m.notice = "This commit's diff is empty, so nothing was copied"
	default:
		m = m.copyText(msg.patch, "diff")
	}
	return m
}

// copyText puts text on the clipboard and says where it went.
func (m model) copyText(text, what string) model {
	how, err := writeClipboard(text)
	switch {
	case err != nil:
		m.notice = "Could not copy the " + what + ": " + err.Error()
	case how == copiedToTerminal:
		m.notice = "Sent the " + what + " to the terminal's clipboard"
	default:
		m.notice = "Copied the " + what
	}
	return m
}
