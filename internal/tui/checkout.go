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

// Checking out: "c" on the graph switches to the selected commit's branch.
// With one place to go it switches at once: the switch is refused while there
// are uncommitted changes, and it is undone by pressing "c" on the branch left
// behind, so a question first only cost a key press. With several branches on
// the commit it asks which, in a list over the graph like the one "d" opens:
// the names are the whole question, and a status line cut short at the
// window's edge could hide the one that was wanted.
//
// A commit with no branch is checked out detached, which the notice says
// afterwards. One with branches is never offered detached: a detached HEAD
// is where commits get lost, and anyone who wants one has a terminal.

// checkoutPrompt is the list "c" opens, while it waits for its answer, on a
// commit with more than one branch.
type checkoutPrompt struct {
	open    bool
	targets []git.SwitchTarget
	cursor  int
	// current is the branch checked out, which the list marks: picking it
	// would only be told "Already on".
	current string
}

type switchFinishedMsg struct {
	repoPath string // the repository switched; see the handler
	target   git.SwitchTarget
	detail   string
	err      error
}

// openCheckout switches to the branch of the commit on the graph, or asks
// which when the commit has several.
func (m model) openCheckout() (model, tea.Cmd) {
	if !m.ready || m.err != nil || m.switching {
		return m, nil
	}
	c, ok := m.commitOnScreen()
	if !ok {
		return m, nil
	}
	if c.WorkingTree {
		m.notice = "These are your uncommitted changes — pick a commit to check out"
		return m, nil
	}
	targets := git.SwitchTargets(c.Refs, c.FullHash)
	if len(targets) == 1 {
		return m.startSwitch(targets[0])
	}
	m.checkout = checkoutPrompt{open: true, targets: targets, current: m.currentBranch}
	// Start on the first branch a switch would go to: on the commit checked
	// out, the top row is often the branch already on.
	for i, t := range targets {
		if t.Kind != git.SwitchBranch || t.Name != m.currentBranch {
			m.checkout.cursor = i
			break
		}
	}
	return m, nil
}

// updateCheckout handles keys while the list is open. Like the other lists
// it owns the keyboard: it covers the graph, so the graph's keys would act
// unseen. Only enter switches, so a stray key changes nothing.
func (m model) updateCheckout(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.checkout
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "c":
		m.checkout = checkoutPrompt{}
	case "j", "down":
		p.cursor = min(p.cursor+1, len(p.targets)-1)
	case "k", "up":
		p.cursor = max(p.cursor-1, 0)
	case "g", "home":
		p.cursor = 0
	case "G", "end":
		p.cursor = len(p.targets) - 1
	case "enter":
		target := p.targets[p.cursor]
		m.checkout = checkoutPrompt{}
		return m.startSwitch(target)
	}
	return m, nil
}

// label is a row of the list as shown, and what follows the name in a
// quieter voice. A remote branch says what picking it makes, since the name
// that ends up checked out is not the one on the row.
func (p checkoutPrompt) label(t git.SwitchTarget) (name, note string) {
	switch {
	case t.Kind == git.SwitchRemote:
		_, branch, _ := strings.Cut(t.Name, "/")
		return t.Name, "new local branch " + branch
	case t.Kind == git.SwitchBranch && t.Name == p.current:
		return t.Name, "checked out"
	}
	return t.Name, ""
}

// render draws the list as a bordered box, like the delete list. maxRows caps
// how many branches are listed at once, and the box is kept inside a window
// windowWidth wide by cutting the names short.
func (p checkoutPrompt) render(maxRows, windowWidth int) string {
	footer := "↑/↓: choose • enter: check out • esc: cancel"
	contentWidth := ansi.StringWidth(footer)
	for _, t := range p.targets {
		name, note := p.label(t)
		w := 2 + ansi.StringWidth(name)
		if note != "" {
			w += 2 + ansi.StringWidth(note)
		}
		contentWidth = max(contentWidth, w)
	}
	// The border and the padding take six columns.
	contentWidth = max(10, min(contentWidth, windowWidth-6))

	selected := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Current.SelectedFg)).
		Background(lipgloss.Color(theme.Current.SelectedBg))

	maxRows = max(1, maxRows)
	start := 0
	if len(p.targets) > maxRows {
		start = max(0, min(p.cursor-maxRows/2, len(p.targets)-maxRows))
	}
	end := min(len(p.targets), start+maxRows)

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render(ansi.Truncate("Check out", contentWidth, "…")))
	sb.WriteString("\n")
	for i := start; i < end; i++ {
		sb.WriteString("\n")
		name, note := p.label(p.targets[i])
		name = ansi.Truncate(name, contentWidth-2, "…")
		if room := contentWidth - 2 - ansi.StringWidth(name) - 2; note != "" && room > 1 {
			note = "  " + ansi.Truncate(note, room, "…")
		} else {
			note = ""
		}
		if i == p.cursor {
			row := "> " + name + note
			sb.WriteString(selected.Render(row + strings.Repeat(" ", contentWidth-ansi.StringWidth(row))))
			continue
		}
		style := localBranchStyle
		if p.targets[i].Kind == git.SwitchRemote {
			style = remoteBranchStyle
		}
		sb.WriteString("  " + style.Render(name) + helpStyle.Render(note))
	}
	sb.WriteString("\n\n")
	sb.WriteString(helpStyle.Render(ansi.Truncate(footer, contentWidth, "…")))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}

// startSwitch begins the switch to target, unless there is nothing to do or
// something is in the way.
func (m model) startSwitch(target git.SwitchTarget) (model, tea.Cmd) {
	if target.Kind == git.SwitchBranch && target.Name == m.currentBranch {
		m.notice = "Already on " + m.currentBranch
		return m, nil
	}
	// A pull is moving the branch and the working tree right now; switching
	// underneath it would leave its fast-forward landing on the wrong one.
	if m.pulling {
		m.notice = "A pull is running — check out again when it has finished"
		return m, nil
	}
	// Nor under a delete: the branch being switched to may be the one going.
	if m.deleting {
		m.notice = "A delete is running — check out again when it has finished"
		return m, nil
	}
	m.switching = true
	return m, switchCmd(m.repoPath, target)
}

func switchCmd(repoPath string, target git.SwitchTarget) tea.Cmd {
	return func() tea.Msg {
		detail, err := git.Switch(repoPath, target)
		return switchFinishedMsg{repoPath: repoPath, target: target, detail: detail, err: err}
	}
}

// finishSwitch says what happened, and reads the repository again when HEAD
// moved: the current branch, the ahead/behind counts and the uncommitted
// changes row all belong to HEAD.
func (m model) finishSwitch(msg switchFinishedMsg) (model, tea.Cmd) {
	m.switching = false
	if msg.err != nil {
		m.notice = switchFailedNotice(msg)
		return m, nil
	}
	next, cmd := m.reloadRepo()
	next.notice = "Switched to " + switchedTo(msg.target)
	return next, cmd
}

func switchFailedNotice(msg switchFinishedMsg) string {
	if errors.Is(msg.err, git.ErrLocalChanges) {
		return "Not checked out: you have uncommitted changes — commit or stash them first"
	}
	reason := firstLine(msg.detail)
	if reason == "" {
		reason = msg.err.Error()
	}
	return "Checkout failed: " + reason
}

// switchedTo names where HEAD went. A remote branch is switched to through
// a new local branch, which is the name the user will see from now on.
func switchedTo(t git.SwitchTarget) string {
	switch t.Kind {
	case git.SwitchRemote:
		_, branch, _ := strings.Cut(t.Name, "/")
		return branch + ", tracking " + t.Name
	case git.SwitchDetached:
		return shortHash(t.Name) + " (detached)"
	}
	return t.Name
}
