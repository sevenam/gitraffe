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

// Cherry-picking: "C" on the graph copies the selected commit's changes onto
// the branch checked out, as a new commit there. See git.CherryPick.
//
// It asks first, in a box over the graph that names the commit and the
// branch, because it makes a commit, and the selection may have been left on
// a commit by reading the history rather than picked for this. Only enter or
// "y" answers yes. Capital, like "D" and "P": the key that makes something
// happen to the branch is never one that a slip on its lower-case neighbour
// could press.
//
// What can be told before asking is told instead of asking: a merge commit,
// a commit already on the branch, a detached HEAD, uncommitted changes. What
// only the trial in git.CherryPick can tell — a conflict, a change already
// there — is said once enter has been pressed, with nothing done.

// cherryPickPrompt is the question "C" asks.
type cherryPickPrompt struct {
	open bool
	hash string // the full hash of the commit to pick
	// What the box shows: the commit's short hash and subject, and the
	// branch it goes onto.
	short, subject, onto string
}

type cherryPickedMsg struct {
	repoPath string // the repository picked into; see the handler
	short    string // the commit picked, as the box showed it
	onto     string // the branch it went onto
	newHash  string
	detail   string
	err      error
}

// openCherryPick asks whether to pick the commit on the graph, unless it is
// already clear that the answer could only be no.
func (m model) openCherryPick() (model, tea.Cmd) {
	if !m.ready || m.err != nil {
		return m, nil
	}
	c, ok := m.commitOnScreen()
	if !ok {
		return m, nil
	}
	if notice := m.cherryPickRefusal(c); notice != "" {
		m.notice = notice
		return m, nil
	}
	subject, _, _ := strings.Cut(c.Message, "\n")
	m.cherryPick = cherryPickPrompt{
		open:    true,
		hash:    c.FullHash,
		short:   c.Hash,
		subject: subject,
		onto:    m.currentBranch,
	}
	return m, nil
}

// cherryPickRefusal is why the commit cannot be picked now, as the status
// line says it, or "" when it may be.
func (m model) cherryPickRefusal(c commit) string {
	switch {
	case c.WorkingTree:
		return "These are your uncommitted changes — pick a commit to cherry-pick"
	case m.branchBlocked() != "":
		return m.branchBlocked() + " is running — cherry-pick when it has finished"
	case m.workingState.Operation != "":
		return inProgressNotice(m.workingState.Operation)
	case m.workingState.Detached || m.currentBranch == "":
		return "Not cherry-picked: HEAD is on no branch — check out the branch to pick onto"
	case len(c.Parents) > 1:
		return "Not cherry-picked: " + c.Hash + " is a merge — pick its commits one at a time, or use a terminal"
	case c.FullHash == m.headHash:
		return "Not cherry-picked: " + c.Hash + " is the commit checked out"
	}
	// Asked here rather than once enter is pressed, so that nobody reads
	// and answers a question for nothing. git.CherryPick asks again.
	if dirty, _ := git.LocalChanges(m.repoPath); dirty {
		return cherryPickDirtyNotice
	}
	return ""
}

const cherryPickDirtyNotice = "Not cherry-picked: you have uncommitted changes — commit or stash them first"

// updateCherryPick picks on enter or "y" and on nothing else, as a discard
// does: a key pressed by mistake costs nothing.
func (m model) updateCherryPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.cherryPick
	m.cherryPick = cherryPickPrompt{}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter", "y", "Y":
	default:
		m.notice = "Nothing cherry-picked"
		return m, nil
	}
	// Something may have started while the box was open.
	if busy := m.branchBlocked(); busy != "" {
		m.notice = busy + " is running — cherry-pick when it has finished"
		return m, nil
	}
	m.picking = true
	return m, cherryPickCmd(m.repoPath, p)
}

func cherryPickCmd(repoPath string, p cherryPickPrompt) tea.Cmd {
	return func() tea.Msg {
		newHash, detail, err := git.CherryPick(repoPath, p.hash)
		return cherryPickedMsg{repoPath: repoPath, short: p.short, onto: p.onto,
			newHash: newHash, detail: detail, err: err}
	}
}

// finishCherryPick says what happened, and on success reads the repository
// again with the new commit selected: it is the one just made, and the one
// to look at to see that it is right.
func (m model) finishCherryPick(msg cherryPickedMsg) (model, tea.Cmd) {
	m.picking = false
	if msg.err != nil {
		m.notice = cherryPickFailedNotice(msg)
		return m, nil
	}
	next, cmd := m.reloadRepo()
	next.reselect = msg.newHash
	next.notice = "Cherry-picked " + msg.short + " onto " + msg.onto + " as " + shortHash(msg.newHash)
	return next, cmd
}

func cherryPickFailedNotice(msg cherryPickedMsg) string {
	switch {
	case errors.Is(msg.err, git.ErrCherryPickConflicts):
		return "Not cherry-picked: " + msg.short + " would conflict with " + msg.onto + " — resolve that in a terminal"
	case errors.Is(msg.err, git.ErrCherryPickEmpty):
		return "Not cherry-picked: " + msg.onto + " already has the changes of " + msg.short
	case errors.Is(msg.err, git.ErrCherryPickOnBranch):
		return "Not cherry-picked: " + msg.short + " is already on " + msg.onto
	case errors.Is(msg.err, git.ErrCherryPickMerge):
		return "Not cherry-picked: " + msg.short + " is a merge — pick its commits one at a time, or use a terminal"
	case errors.Is(msg.err, git.ErrCherryPickRoot):
		return "Not cherry-picked: " + msg.short + " is the first commit of its history"
	case errors.Is(msg.err, git.ErrLocalChanges):
		return cherryPickDirtyNotice
	case errors.Is(msg.err, git.ErrDetached):
		return "Not cherry-picked: HEAD is on no branch — check out the branch to pick onto"
	case errors.Is(msg.err, git.ErrNoHead):
		return "Not cherry-picked: " + msg.onto + " has no commits yet"
	case errors.Is(msg.err, git.ErrInProgress):
		return inProgressNotice(strings.TrimPrefix(msg.err.Error(), git.ErrInProgress.Error()+": "))
	}
	reason := firstLine(msg.detail)
	if reason == "" {
		reason = msg.err.Error()
	}
	return "Not cherry-picked: " + reason
}

// render draws the question as a box over the screen, made like the discard
// question: a title, the commit, where it goes, and the keys along the bottom
// with the keys themselves standing out from their words.
func (p cherryPickPrompt) render(windowWidth int) string {
	keys := [][2]string{{"enter", "cherry-pick"}, {"esc", "cancel"}}
	key := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.SelectedFg))
	var plain, footer []string
	for _, k := range keys {
		plain = append(plain, k[0]+": "+k[1])
		footer = append(footer, key.Render(k[0])+helpStyle.Render(": "+k[1]))
	}
	footerWidth := ansi.StringWidth(strings.Join(plain, " • "))

	commitLine := p.short + "  " + p.subject
	onto := "onto " + p.onto + ", as a new commit"
	// The border and the padding take six columns.
	width := max(ansi.StringWidth(commitLine), ansi.StringWidth(onto), footerWidth)
	width = max(10, min(width, windowWidth-6))

	// The hash stays whole and the subject is cut, which is the part with
	// room to spare; and the branch name is the one thing in its line that
	// matters, so it is the one kept.
	hash := commitHashStyle.Render(p.short)
	subject := ansi.Truncate(p.subject, max(0, width-ansi.StringWidth(p.short)-2), "…")
	ontoLine := helpStyle.Render("onto ") + localBranchStyle.Render(p.onto) + helpStyle.Render(", as a new commit")
	if ansi.StringWidth(onto) > width {
		ontoLine = helpStyle.Render("onto ") + localBranchStyle.Render(ansi.Truncate(p.onto, max(1, width-5), "…"))
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render(ansi.Truncate("Cherry-pick", width, "…")))
	sb.WriteString("\n\n")
	sb.WriteString(hash + "  " + subject + "\n")
	sb.WriteString(ontoLine)
	sb.WriteString("\n\n")
	sb.WriteString(ansi.Truncate(strings.Join(footer, helpStyle.Render(" • ")), width, "…"))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}
