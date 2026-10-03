package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
)

// Checking out: "C" on the graph switches to the selected commit's branch.
// With one place to go it switches at once: the key is a capital, the switch
// is refused while there are uncommitted changes, and it is undone by
// pressing "C" on the branch left behind, so a question first only cost a
// key press. With several branches on the commit it asks which, on the
// status line like the copy prompt.
//
// A commit with no branch is checked out detached, which the notice says
// afterwards. One with branches is never offered detached: a detached HEAD
// is where commits get lost, and anyone who wants one has a terminal.

// checkoutPrompt is the question "C" asks, while it waits for its answer,
// about a commit with more than one branch.
type checkoutPrompt struct {
	open    bool
	targets []git.SwitchTarget
}

// The number keys pick from a list, so a list longer than this cannot be
// answered; the rest are left to the terminal.
const maxCheckoutChoices = 9

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
	if len(targets) > maxCheckoutChoices {
		targets = targets[:maxCheckoutChoices]
	}
	m.checkout = checkoutPrompt{open: true, targets: targets}
	return m, nil
}

// checkoutQuestion is the status line while the prompt is open.
func (m model) checkoutQuestion() string {
	t := m.checkout.targets
	parts := make([]string, 0, len(t)+1)
	for i, target := range t {
		parts = append(parts, fmt.Sprintf("%d %s", i+1, target.Name))
	}
	parts = append(parts, "esc cancel")
	return "Check out: " + strings.Join(parts, " • ")
}

// answerCheckout switches to what the key picked. Any key it doesn't know
// closes the prompt, so a stray key press changes nothing.
func (m model) answerCheckout(msg tea.KeyMsg) (model, tea.Cmd) {
	targets := m.checkout.targets
	m.checkout = checkoutPrompt{}
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	if len(key) != 1 || key[0] < '1' || int(key[0]-'0') > len(targets) {
		return m, nil
	}
	return m.startSwitch(targets[key[0]-'1'])
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
