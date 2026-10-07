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

// A new branch: "b" asks for a name, makes the branch at the selected commit
// and switches to it. See git.CreateBranch.
//
// It starts at the selection, which is what the cursor is for, and the box
// names the commit so that a selection left somewhere by reading the history
// is seen before the branch is made. On the commit HEAD is at, or on the
// uncommitted changes above it, no file changes and those changes come
// along. Anywhere else it is a checkout as well, and is turned down as "c"
// is while there are uncommitted changes — before the box opens, so that
// nobody types a name for nothing.

// branchPrompt is the box "b" asks for the name in.
type branchPrompt struct {
	open    bool
	name    textinput.Model
	failure string // why the name typed will not do
	// start is the full hash of the commit the branch starts at, or "" for
	// where HEAD is.
	start string
	// from is what HEAD is on, as the top box shows it, when the branch starts
	// there. A branch started anywhere else has only its commit to show.
	from string
	// The commit the branch starts at: its short hash and its subject.
	commit, subject string
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

// branchElsewhereNotice turns down a branch at a commit other than HEAD's
// while there are uncommitted changes, and says what would work.
const branchElsewhereNotice = "No branch made here: you have uncommitted changes — commit or stash them, or branch from the commit you are on"

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
	p := branchPrompt{open: true, name: name, from: m.currentBranch, commit: shortHash(m.headHash)}
	if c, ok := m.commitOnScreen(); ok && !c.WorkingTree && c.FullHash != m.headHash {
		// Asked here rather than found out by the branch once it is named.
		// git.CreateBranch asks again, the box having been open a while.
		if dirty, _ := git.LocalChanges(m.repoPath); dirty {
			m.notice = branchElsewhereNotice
			return m, nil
		}
		p.start, p.from, p.commit = c.FullHash, "", c.Hash
		p.subject, _, _ = strings.Cut(c.Message, "\n")
	} else {
		for _, c := range m.commits {
			// Not there when HEAD is further back than the history loaded.
			if c.FullHash == m.headHash && !c.WorkingTree {
				p.subject, _, _ = strings.Cut(c.Message, "\n")
				break
			}
		}
	}
	m.branchPrompt = p
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
		start := p.start
		m.branchPrompt = branchPrompt{}
		// Something may have started while the box was open.
		if busy := m.branchBlocked(); busy != "" {
			m.notice = busy + " is running — make the branch when it has finished"
			return m, nil
		}
		// A switch as far as everything else is concerned: HEAD is moving.
		m.switching = true
		return m, createBranchCmd(m.repoPath, name, start)
	}

	var cmd tea.Cmd
	p.failure = ""
	p.name, cmd = p.name.Update(msg)
	return m, cmd
}

func createBranchCmd(repoPath, name, start string) tea.Cmd {
	return func() tea.Msg {
		detail, err := git.CreateBranch(repoPath, name, start)
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
	case errors.Is(msg.err, git.ErrLocalChanges):
		return branchElsewhereNotice
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

// The box is at least branchBoxMinWidth columns of content where the window
// allows, so it looks the same from one branch to the next, and no more than
// branchBoxMaxWidth, past which a line is too long to read at a glance.
const (
	branchBoxMinWidth = 50
	branchBoxMaxWidth = 100
	// What the box adds around its content: the "Name  " label, the cursor's
	// column, the padding and the border, and a column of the graph left
	// showing on each side.
	branchBoxChrome = len("Name  ") + 1 + 2*commitPromptPadding + 2 + 2
)

// width is the room for the name and for the line saying where the branch
// starts. It grows to show that line whole — the commit's subject is what
// tells one commit from another — and to hold the name as it is typed, and
// gives way to the window, so a narrow one cuts the subject short and never
// the box.
func (p branchPrompt) width(windowWidth int) int {
	want := ansi.StringWidth(p.name.Value())
	if p.commit != "" {
		from := len(p.commit)
		if p.from != "" {
			from += ansi.StringWidth(p.from) + 2
		}
		if p.subject != "" {
			from += 1 + ansi.StringWidth(p.subject)
		}
		want = max(want, from)
	}
	want = min(max(want, branchBoxMinWidth), branchBoxMaxWidth)
	return max(20, min(want, windowWidth-branchBoxChrome))
}

// render draws the box: where the branch will start, the name it is to have,
// and why the last name typed would not do.
func (p branchPrompt) render(windowWidth int) string {
	width := p.width(windowWidth)
	p.name.Width = width
	// The width decides which part of a long name is on show.
	p.name.SetCursor(p.name.Position())

	// What a line can hold before it wraps and makes the box a row taller.
	inner := width + len("Name  ") + 1

	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Title))
	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("New branch"))
	where := "starts where you are"
	if p.start != "" {
		where = "starts at the selected commit"
	}
	sb.WriteString(helpStyle.Render("  " + ansi.Truncate(where, inner-len("New branch  "), "…")))
	sb.WriteString("\n\n")

	sb.WriteString(label.Render("From") + "  ")
	room := width
	if p.from != "" {
		// The branch comes first and is kept whole; the commit takes what
		// is left.
		from := ansi.Truncate(p.from, width, "…")
		sb.WriteString(localBranchStyle.Render(from))
		room -= ansi.StringWidth(from) + 2
		if p.commit != "" && room >= len(p.commit) {
			sb.WriteString("  ")
		}
	}
	if p.commit != "" && room >= len(p.commit) {
		sb.WriteString(commitHashStyle.Render(p.commit))
		if room -= len(p.commit) + 1; p.subject != "" && room > 1 {
			sb.WriteString(" " + helpStyle.Render(ansi.Truncate(p.subject, room, "…")))
		}
	}
	sb.WriteString("\n")
	sb.WriteString(label.Render("Name") + "  " + p.name.View())
	sb.WriteString("\n\n")

	if p.failure != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.Error)).Render(
			ansi.Truncate(p.failure, inner, "…")))
		sb.WriteString("\n\n")
	}
	sb.WriteString(helpStyle.Render(ansi.Truncate("enter: create it and switch to it • esc: cancel", inner, "…")))

	return lipgloss.NewStyle().
		Width(width+len("Name  ")+2*commitPromptPadding+1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, commitPromptPadding).
		Render(sb.String())
}
