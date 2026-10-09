package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
)

// Discarding: "d" and "D" in the commit view of the uncommitted changes throw
// away what "s" and "S" there would stage, and "D" on the graph throws away
// all of it. It is the one thing gitraffe does that loses work nothing can
// bring back, so every discard asks on the status line first and only "y"
// answers yes; any other key leaves everything as it was.
//
// Only unstaged changes are reached (see internal/git/discard.go): what is
// staged is kept, so staging something is how to put it out of reach of a
// discard, and "s" again is how to make it discardable.

// discardPrompt is the question a discard waits on, and what it does once
// answered.
type discardPrompt struct {
	asking   bool
	question string
	done     string // the status line once it is done
	change   func(dir string) (string, error)
	want     []fileKey // where the file list's selection goes afterwards
}

// canDiscard is canStage for the graph as well as the commit view: nothing
// else is changing the index, and the repository is in no operation a
// discard would get in the middle of.
func (m *model) canDiscard() bool {
	if m.staging || m.committing || m.commitPrompt.open {
		return false
	}
	if op := m.workingState.Operation; op != "" {
		m.notice = inProgressNotice(op)
		return false
	}
	return true
}

const keptNotice = "Staged changes are kept — unstage them with s first to discard them"

// discardSelected asks to discard what the focused box has selected, the
// counterpart of stageSelected.
func (m model) discardSelected() model {
	if !m.canStage() {
		return m
	}
	f, ok := m.selectedFile()
	if !ok {
		return m
	}
	files := m.viewedFiles()
	v := m.commitView

	if v.focus != commitBoxDiff {
		if f.Staged {
			m.notice = keptNotice
			return m
		}
		return m.askDiscardFile(f)
	}

	d := m.stagingDiff()
	entry, first, last := m.pickedLines(d, f)
	switch {
	case entry.Staged:
		m.notice = keptNotice
		return m
	case entry.Untracked:
		m.notice = "Git has never seen this file, so it goes whole or not at all — D deletes it"
		return m
	}
	what := "this line"
	switch {
	case v.selecting:
		what = "these lines"
	case m.cursorTakesHunk(d):
		what = "this hunk"
	}
	m.discard = discardPrompt{
		asking:   true,
		question: "Discard " + what + " of " + entry.Path + "? It cannot be undone (y/n)",
		done:     "Discarded " + what + " of " + entry.Path,
		change: func(dir string) (string, error) {
			return git.DiscardLines(dir, entry, first, last)
		},
		want: append([]fileKey{keyOf(entry), {entry.Path, true}}, neighbours(files, v.file)...),
	}
	return m
}

// discardShownFile asks to discard the unstaged half of the file the diff
// box shows, whichever of its rows is selected.
func (m model) discardShownFile() model {
	if !m.canStage() {
		return m
	}
	entry, ok := m.stagingDiff().entry(false)
	if !ok {
		m.notice = keptNotice
		return m
	}
	return m.askDiscardFile(entry)
}

func (m model) askDiscardFile(f fileDiff) model {
	p := discardPrompt{
		asking:   true,
		question: "Discard the changes to " + f.Path + "? It cannot be undone (y/n)",
		done:     "Discarded the changes to " + f.Path,
		change: func(dir string) (string, error) {
			return git.DiscardFile(dir, f)
		},
		// Its staged half if it has one, which is what is left of it.
		want: append([]fileKey{{f.Path, true}}, neighbours(m.viewedFiles(), m.commitView.file)...),
	}
	if f.Untracked {
		p.question = "Delete " + f.Path + ", which git has never seen? It cannot be undone (y/n)"
		p.done = "Deleted " + f.Path
	}
	m.discard = p
	return m
}

// discardEverything asks to discard every unstaged change: "D" from the
// commit view's file list, and from the graph, where there is no file to
// pick.
func (m model) discardEverything() model {
	if len(m.commits) == 0 || !m.commits[0].WorkingTree {
		m.notice = "Nothing to discard — there are no uncommitted changes"
		return m
	}
	if !m.canDiscard() {
		return m
	}
	c := m.commits[0]
	question := "Discard every unstaged change and untracked file? Staged changes are kept. It cannot be undone (y/n)"
	if c.DiffLoaded {
		unstaged := 0
		for _, f := range c.DiffFiles {
			if !f.Staged {
				unstaged++
			}
		}
		if unstaged == 0 {
			m.notice = "Nothing unstaged to discard — staged changes are kept"
			return m
		}
		question = "Discard the unstaged changes to " + plural(unstaged, "file") +
			"? Staged changes are kept. It cannot be undone (y/n)"
	}
	m.discard = discardPrompt{
		asking:   true,
		question: question,
		done:     "Discarded all unstaged changes",
		change:   git.DiscardAll,
	}
	return m
}

// answerDiscard discards on "y" and on nothing else: a key pressed by
// mistake costs nothing.
func (m model) answerDiscard(msg tea.KeyMsg) (model, tea.Cmd) {
	p := m.discard
	m.discard = discardPrompt{}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "Y":
	default:
		m.notice = "Nothing discarded"
		return m, nil
	}
	m.staging = true
	if m.commitView.open {
		m.commitView.selecting = false
		m.commitView.want = p.want
	}
	change := stageCmd(m.repoPath, m.diffStatWidth(), p.change)
	return m, func() tea.Msg {
		msg := change().(stageFinishedMsg)
		msg.discarded = p.done
		return msg
	}
}

// finishDiscard is finishStage's ending for a discard. A discard that leaves
// nothing uncommitted takes the working tree's row off the graph, and from
// the graph the row's count and diff are what changed: either way the graph
// is read again, and the commit view, left with nothing to show, is closed.
func (m model) finishDiscard(msg stageFinishedMsg) (model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	m.notice = msg.discarded
	clean := msg.summary == ""
	if !clean && m.commitView.open {
		return m, nil
	}
	if clean {
		m.commitView = commitView{}
	}
	next, cmd := m.reloadRepo()
	next.notice = msg.discarded
	return next, cmd
}

func discardFailedNotice(msg stageFinishedMsg) string {
	switch {
	case errors.Is(msg.err, git.ErrStaleDiff):
		return "That file has changed since it was read — here it is as it is now, nothing discarded"
	case errors.Is(msg.err, git.ErrNoLines):
		return "No changed lines there to discard"
	case errors.Is(msg.err, git.ErrStagedChanges):
		return keptNotice
	case errors.Is(msg.err, git.ErrInProgress):
		return inProgressNotice(msg.diff.State.Operation)
	}
	reason := firstLine(msg.detail)
	if reason == "" {
		reason = msg.err.Error()
	}
	return "Not discarded: " + reason
}
