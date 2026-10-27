package tui

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// Stashing (issue #152): "s" in the commit view of the uncommitted changes
// puts the selected file aside, staged and unstaged halves together, and "S"
// there or on the graph puts everything aside. Neither asks: nothing is lost,
// and "s" on the graph lists the stashes, where enter pops one back.
//
// A pop is the one part that could leave work needing to be resolved, so it
// goes ahead only when git.PopStash finds it would apply cleanly; otherwise
// the status line says why, and the stash stays in the list for a terminal.
//
// Stashing and popping change the index and the working tree as staging
// does, so they run as staging runs (stageCmd) and wait on it the same way.

// stashPicker is the state of the list opened with "s" on the graph.
type stashPicker struct {
	open    bool
	stashes []git.Stash
	cursor  int
}

type stashListMsg struct {
	repoPath string // the repository listed; see the handler
	stashes  []git.Stash
	err      error
}

// canStash is canDiscard, and nothing moving HEAD under the change.
func (m *model) canStash() bool {
	if m.switching || m.pulling {
		m.notice = "Something else is running — try again when it has finished"
		return false
	}
	return m.canDiscard()
}

// stashSelected puts aside the file the commit view has selected: both its
// rows, since a stash takes a path whole.
func (m model) stashSelected() (model, tea.Cmd) {
	if !m.canStage() || !m.canStash() {
		return m, nil
	}
	f, ok := m.selectedFile()
	if !ok {
		return m, nil
	}
	v := &m.commitView
	v.selecting = false
	v.want = neighbours(m.viewedFiles(), v.file)
	path := f.Path
	return m.runStash(func(dir string) (string, error) {
		return git.StashFile(dir, path)
	}, "Stashed "+path+" — s on the graph lists stashes", "Not stashed")
}

// stashEverything puts every uncommitted change aside: "S" in the commit view
// and on the graph alike.
func (m model) stashEverything() (model, tea.Cmd) {
	if len(m.commits) == 0 || !m.commits[0].WorkingTree {
		m.notice = "Nothing to stash — there are no uncommitted changes"
		return m, nil
	}
	if !m.canStash() {
		return m, nil
	}
	m.commitView.selecting = false
	return m.runStash(git.StashAll, "Stashed all uncommitted changes — s on the graph lists stashes", "Not stashed")
}

// runStash runs a stash or a pop as staging is run, and has finishStage hand
// over to finishStash once it is done.
func (m model) runStash(change func(dir string) (string, error), done, failed string) (model, tea.Cmd) {
	m.staging = true
	cmd := stageCmd(m.repoPath, m.diffStatWidth(), change)
	return m, func() tea.Msg {
		msg := cmd().(stageFinishedMsg)
		msg.stashed, msg.stashFailed = done, failed
		return msg
	}
}

// finishStash is finishStage's ending for a stash or a pop. Either moves
// refs/stash and changes the working tree's row, so the graph is always read
// again; the commit view stays open while there is anything left in it.
func (m model) finishStash(msg stageFinishedMsg) (model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	if msg.summary == "" {
		m.commitView = commitView{}
	}
	next, cmd := m.reloadRepo()
	next.notice = msg.stashed
	return next, cmd
}

func stashFailedNotice(msg stageFinishedMsg) string {
	switch {
	case errors.Is(msg.err, git.ErrStashConflicts):
		return "Not popped: it would conflict with what has changed since — pop it in a terminal to resolve that"
	case errors.Is(msg.err, git.ErrStashUntrackedExists):
		return "Not popped: a file it holds is already in the working tree — move that file away first"
	case errors.Is(msg.err, git.ErrStashMoved):
		return "Not popped: the stash list has changed — s shows it as it is now"
	case errors.Is(msg.err, git.ErrInProgress):
		return inProgressNotice(msg.diff.State.Operation)
	}
	reason := firstLine(msg.detail)
	if reason == "" {
		reason = msg.err.Error()
	}
	return msg.stashFailed + ": " + reason
}

// openStashList reads the stashes; the list opens when they have been read.
func (m model) openStashList() (model, tea.Cmd) {
	if !m.ready || m.err != nil {
		return m, nil
	}
	repoPath := m.repoPath
	return m, func() tea.Msg {
		stashes, err := git.ListStashes(repoPath)
		return stashListMsg{repoPath: repoPath, stashes: stashes, err: err}
	}
}

func (m model) showStashList(msg stashListMsg) model {
	switch {
	case msg.err != nil:
		m.notice = "Could not read the stashes: " + msg.err.Error()
	case len(msg.stashes) == 0:
		m.notice = "No stashes — S puts all uncommitted changes in one"
	default:
		m.stashPicker = stashPicker{open: true, stashes: msg.stashes}
	}
	return m
}

// updateStashPicker handles keys while the list is open. Like the other lists
// it owns the keyboard, since it covers the graph.
func (m model) updateStashPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.stashPicker
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "s":
		m.stashPicker = stashPicker{}
	case "j", "down":
		p.cursor = min(p.cursor+1, len(p.stashes)-1)
	case "k", "up":
		p.cursor = max(p.cursor-1, 0)
	case "g", "home":
		p.cursor = 0
	case "G", "end":
		p.cursor = len(p.stashes) - 1
	case "enter":
		s := p.stashes[p.cursor]
		m.stashPicker = stashPicker{}
		return m.startPop(s)
	}
	return m, nil
}

func (m model) startPop(s git.Stash) (model, tea.Cmd) {
	if !m.canStash() {
		return m, nil
	}
	return m.runStash(func(dir string) (string, error) {
		return git.PopStash(dir, s)
	}, "Popped "+s.Ref+": "+s.Subject, "Not popped")
}

// label is a row of the list: where it is, what it was, and when.
func stashLabel(s git.Stash) string {
	return s.Ref + "  " + s.Subject + "  (" + s.When + ")"
}

// render draws the list as a bordered box, like the delete list.
func (p stashPicker) render(maxRows, windowWidth int) string {
	footer := "↑/↓: choose • enter: pop • esc: close"
	// The border and the padding take six columns.
	limit := max(ansi.StringWidth(footer), windowWidth-6)
	contentWidth := ansi.StringWidth(footer)
	for _, s := range p.stashes {
		contentWidth = max(contentWidth, ansi.StringWidth(stashLabel(s))+2)
	}
	contentWidth = min(contentWidth, limit)

	selected := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Current.SelectedFg)).
		Background(lipgloss.Color(theme.Current.SelectedBg))

	maxRows = max(1, maxRows)
	start := 0
	if len(p.stashes) > maxRows {
		start = max(0, min(p.cursor-maxRows/2, len(p.stashes)-maxRows))
	}
	end := min(len(p.stashes), start+maxRows)

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Pop stash"))
	sb.WriteString("\n")
	for i := start; i < end; i++ {
		sb.WriteString("\n")
		if i == p.cursor {
			row := ansi.Truncate("> "+stashLabel(p.stashes[i]), contentWidth, "…")
			sb.WriteString(selected.Render(row + strings.Repeat(" ", contentWidth-ansi.StringWidth(row))))
		} else {
			sb.WriteString(ansi.Truncate("  "+stashLabel(p.stashes[i]), contentWidth, "…"))
		}
	}
	sb.WriteString("\n\n")
	sb.WriteString(helpStyle.Render(footer))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}
