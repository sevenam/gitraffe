package tui

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// A new branch: "b" asks for a name, makes the branch where HEAD is and
// switches to it. See git.CreateBranch.
//
// It starts at HEAD whatever commit is selected. The selection is wherever
// reading the history last left it, and a branch started there would start
// somewhere nobody chose; where you stand is the one place that is never a
// surprise, and the box names it. A branch from anywhere else is "c" on that
// commit and then "b".

// branchPrompt is the box "b" asks for the name in.
type branchPrompt struct {
	open    bool
	name    textinput.Model
	from    string // what HEAD is on, as the top box shows it
	failure string // why the name typed will not do
}

type branchCreatedMsg struct {
	repoPath string // the repository the branch was made in; see the handler
	name     string
	detail   string
	err      error
}

// branchBlocked names what is running that a new branch would get under the
// feet of, each of which lands on whatever branch HEAD is on when it ends;
// "" when nothing is.
func (m model) branchBlocked() string {
	switch {
	case m.pulling:
		return "A pull"
	case m.pushing:
		return "A push"
	case m.deleting:
		return "A delete"
	case m.switching:
		return "A checkout"
	case m.committing:
		return "A commit"
	}
	return ""
}

// openBranchPrompt opens the box, unless a branch could not be made now.
func (m model) openBranchPrompt() (model, tea.Cmd) {
	if !m.ready || m.err != nil {
		return m, nil
	}
	if busy := m.branchBlocked(); busy != "" {
		m.notice = busy + " is running — make the branch when it has finished"
		return m, nil
	}

	name := textinput.New()
	name.Prompt = ""
	name.CharLimit = 200
	name.Cursor.SetMode(cursor.CursorStatic)
	name.Focus()
	m.branchPrompt = branchPrompt{open: true, name: name, from: m.currentBranch}
	return m, nil
}

// updateBranchPrompt is the keyboard while the box is open.
func (m model) updateBranchPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.branchPrompt
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.branchPrompt = branchPrompt{}
		return m, nil
	case "enter":
		name := strings.TrimSpace(p.name.Value())
		// Checked here, where the name can still be changed, rather than
		// reported once the box has gone.
		switch {
		case name == "":
			p.failure = "Type a name for the branch."
			return m, nil
		case !git.ValidBranchName(m.repoPath, name):
			p.failure = "Git does not accept that as a branch name."
			return m, nil
		case git.BranchExists(m.repoPath, name):
			p.failure = "There is already a branch called " + name + "."
			return m, nil
		}
		m.branchPrompt = branchPrompt{}
		// Something may have started while the box was open.
		if busy := m.branchBlocked(); busy != "" {
			m.notice = busy + " is running — make the branch when it has finished"
			return m, nil
		}
		// A switch as far as everything else is concerned: HEAD is moving.
		m.switching = true
		return m, createBranchCmd(m.repoPath, name)
	}

	var cmd tea.Cmd
	p.failure = ""
	p.name, cmd = p.name.Update(msg)
	return m, cmd
}

func createBranchCmd(repoPath, name string) tea.Cmd {
	return func() tea.Msg {
		detail, err := git.CreateBranch(repoPath, name)
		return branchCreatedMsg{repoPath: repoPath, name: name, detail: detail, err: err}
	}
}

// finishBranch says what happened, and reads the repository again when the
// branch was made: HEAD is on it now, and it has a label to draw.
func (m model) finishBranch(msg branchCreatedMsg) (model, tea.Cmd) {
	m.switching = false
	if msg.err != nil {
		m.notice = branchFailedNotice(msg)
		return m, nil
	}
	next, cmd := m.reloadRepo()
	next.notice = "Created " + msg.name + " and switched to it"
	return next, cmd
}

func branchFailedNotice(msg branchCreatedMsg) string {
	switch {
	case errors.Is(msg.err, git.ErrBranchExists):
		return "No branch made: there is already a branch called " + msg.name
	case errors.Is(msg.err, git.ErrBadBranchName):
		return "No branch made: git does not accept " + quoted(msg.name) + " as a branch name"
	case errors.Is(msg.err, git.ErrInProgress):
		return inProgressNotice(strings.TrimPrefix(msg.err.Error(), git.ErrInProgress.Error()+": "))
	}
	reason := firstLine(msg.detail)
	if reason == "" {
		reason = msg.err.Error()
	}
	return "No branch made: " + reason
}

// render draws the box: where the branch will start, the name it is to have,
// and why the last name typed would not do.
func (p branchPrompt) render(windowWidth int) string {
	width := pushNameWidth(windowWidth)
	p.name.Width = width
	// The width decides which part of a long name is on show.
	p.name.SetCursor(p.name.Position())

	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Title))
	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("New branch"))
	sb.WriteString(helpStyle.Render("  " + ansi.Truncate("starts where you are, not at the selection", width, "…")))
	sb.WriteString("\n\n")

	sb.WriteString(label.Render("From") + "  " + localBranchStyle.Render(ansi.Truncate(p.from, width, "…")))
	sb.WriteString("\n")
	sb.WriteString(label.Render("Name") + "  " + p.name.View())
	sb.WriteString("\n\n")

	if p.failure != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.Error)).Render(
			ansi.Truncate(p.failure, width+6, "…")))
		sb.WriteString("\n\n")
	}
	sb.WriteString(helpStyle.Render("enter: create it and switch to it • esc: cancel"))

	return lipgloss.NewStyle().
		Width(width+len("Name  ")+2*commitPromptPadding+1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, commitPromptPadding).
		Render(sb.String())
}
