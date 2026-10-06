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

// Deleting: "d" on the graph opens a list of what can be deleted from the
// selected commit — each branch on it, locally, on the remote, or both — and
// enter deletes the one picked. The list is the question: nothing is deleted
// by "d" alone, and esc leaves everything as it was.
//
// It stays within what cannot lose work. A branch whose commits are on no
// other branch is refused (see git.DeleteBranch), and the branch checked out
// is not offered locally, since there would be nowhere left to stand.

// deleteChoice is one row of the list.
type deleteChoice struct {
	target git.DeleteTarget
	what   git.DeleteWhat
}

// label is the row as shown: what kind of delete first, so the rows read
// down as a choice of three, then the names in brackets.
func (c deleteChoice) label() string {
	switch c.what {
	case git.DeleteLocal:
		return "local (" + c.target.Local + ")"
	case git.DeleteRemote:
		return "remote (" + c.target.Remote + ")"
	}
	return "local & remote (" + c.target.Local + " + " + c.target.Remote + ")"
}

// deleted is the same thing as a sentence, for the notice afterwards.
func (c deleteChoice) deleted() string {
	switch c.what {
	case git.DeleteLocal:
		return "local " + c.target.Local
	case git.DeleteRemote:
		return "remote " + c.target.Remote
	}
	return "local " + c.target.Local + " and remote " + c.target.Remote
}

// deletePicker is the state of the list opened with "d".
type deletePicker struct {
	open    bool
	choices []deleteChoice
	cursor  int
}

type deleteFinishedMsg struct {
	repoPath string // the repository deleted from; see the handler
	choice   deleteChoice
	result   git.DeleteResult
	err      error
}

// deleteChoices lists what can be deleted for the branches on a commit. The
// branch checked out keeps only its remote row.
func deleteChoices(targets []git.DeleteTarget, currentBranch string) []deleteChoice {
	var choices []deleteChoice
	for _, t := range targets {
		local := t.Local != "" && t.Local != currentBranch
		if local {
			choices = append(choices, deleteChoice{t, git.DeleteLocal})
		}
		if t.Remote != "" {
			choices = append(choices, deleteChoice{t, git.DeleteRemote})
		}
		if local && t.Remote != "" {
			choices = append(choices, deleteChoice{t, git.DeleteBoth})
		}
	}
	return choices
}

// openDelete lists what can be deleted from the commit on the graph.
func (m model) openDelete() model {
	if !m.ready || m.err != nil || m.deleting {
		return m
	}
	c, ok := m.commitOnScreen()
	if !ok {
		return m
	}
	if c.WorkingTree {
		m.notice = "These are your uncommitted changes — pick a commit to delete its branch"
		return m
	}
	targets := git.DeleteTargets(c.Refs)
	if len(targets) == 0 {
		m.notice = "No branch on this commit to delete"
		return m
	}
	choices := deleteChoices(targets, m.currentBranch)
	if len(choices) == 0 {
		m.notice = m.currentBranch + " is checked out — switch to another branch before deleting it"
		return m
	}
	m.deletePicker = deletePicker{open: true, choices: choices}
	return m
}

// updateDeletePicker handles keys while the list is open. Like the other
// lists it owns the keyboard: it covers the graph, so the graph's keys would
// act unseen.
func (m model) updateDeletePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.deletePicker
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "d":
		m.deletePicker = deletePicker{}
	case "j", "down":
		p.cursor = min(p.cursor+1, len(p.choices)-1)
	case "k", "up":
		p.cursor = max(p.cursor-1, 0)
	case "g", "home":
		p.cursor = 0
	case "G", "end":
		p.cursor = len(p.choices) - 1
	case "enter":
		choice := p.choices[p.cursor]
		m.deletePicker = deletePicker{}
		return m.startDelete(choice)
	}
	return m, nil
}

func (m model) startDelete(choice deleteChoice) (model, tea.Cmd) {
	// Each of these moves refs, and a remote delete is a push: side by side
	// with a fetch, a pull or a push it would race them for the same ones.
	if m.fetching || m.autoFetching || m.pulling || m.pushing || m.switching {
		m.notice = "Something else is running — delete again when it has finished"
		return m, nil
	}
	m.deleting = true
	return m, deleteCmd(m.repoPath, choice)
}

func deleteCmd(repoPath string, choice deleteChoice) tea.Cmd {
	return func() tea.Msg {
		result, err := git.DeleteBranch(repoPath, choice.target, choice.what)
		return deleteFinishedMsg{repoPath: repoPath, choice: choice, result: result, err: err}
	}
}

// finishDelete says what happened, and reads the repository again whenever a
// branch went: its label has to leave the graph.
func (m model) finishDelete(msg deleteFinishedMsg) (model, tea.Cmd) {
	m.deleting = false
	notice := deleteNotice(msg)
	if !msg.result.Local && !msg.result.Remote {
		m.notice = notice
		return m, nil
	}
	next, cmd := m.reloadRepo()
	next.notice = notice
	return next, cmd
}

func deleteNotice(msg deleteFinishedMsg) string {
	t := msg.choice.target
	if msg.err == nil {
		return "Deleted " + msg.choice.deleted()
	}
	switch {
	case errors.Is(msg.err, git.ErrOnlyCopy):
		return "Not deleted: its commits are on no other branch — merge it first, or delete it in a terminal"
	case errors.Is(msg.err, git.ErrCurrentBranch):
		return "Not deleted: " + t.Local + " is checked out"
	case errors.Is(msg.err, git.ErrRemoteMoved):
		return "Not deleted: " + t.Remote + " has moved on the remote — fetch (f) to see what is there now"
	}
	reason := firstLine(msg.result.Detail)
	if reason == "" {
		reason = msg.err.Error()
	}
	// The remote goes first, so a failure after it leaves the local branch.
	if msg.result.Remote {
		return "Deleted remote " + t.Remote + ", but not local " + t.Local + ": " + reason
	}
	return "Delete failed: " + reason
}

// render draws the list as a bordered box, like the theme list.
func (p deletePicker) render(maxRows int) string {
	footer := "↑/↓: choose • enter: delete • esc: cancel"
	contentWidth := ansi.StringWidth(footer)
	for _, c := range p.choices {
		contentWidth = max(contentWidth, ansi.StringWidth(c.label())+2)
	}

	selected := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Current.SelectedFg)).
		Background(lipgloss.Color(theme.Current.SelectedBg))

	maxRows = max(1, maxRows)
	start := 0
	if len(p.choices) > maxRows {
		start = max(0, min(p.cursor-maxRows/2, len(p.choices)-maxRows))
	}
	end := min(len(p.choices), start+maxRows)

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Delete branch"))
	sb.WriteString("\n")
	for i := start; i < end; i++ {
		sb.WriteString("\n")
		if i == p.cursor {
			row := "> " + p.choices[i].label()
			sb.WriteString(selected.Render(row + strings.Repeat(" ", contentWidth-ansi.StringWidth(row))))
		} else {
			sb.WriteString("  " + p.choices[i].label())
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
