package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
)

type pullFinishedMsg struct {
	repoPath string // the repository pulled; see the handler
	result   git.PullResult
	err      error
}

// startPull catches the current branch up with its upstream. Like checking
// out, it changes your branch and your working tree, so it is kept to what
// cannot lose work or need resolving: a fast-forward, or nothing. See git.Pull.
//
// Like a fetch it is never started for you. Auto-fetch may show that there is
// something to pull; moving the branch is always a key press.
func (m model) startPull() (model, tea.Cmd) {
	if m.pulling {
		return m, nil
	}
	// A pull fetches first, and two fetches side by side fight over the same
	// refs. The one running will be done in a moment.
	if m.fetching || m.autoFetching {
		m.notice = "A fetch is running — pull again when it has finished"
		return m, nil
	}
	m.pulling = true
	m.notice = ""
	return m, pullCmd(m.repoPath)
}

func pullCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		result, err := git.Pull(repoPath)
		return pullFinishedMsg{repoPath: repoPath, result: result, err: err}
	}
}

// finishPull says what happened. Whenever the remotes were asked, the
// repository is read again even if the branch did not move: the fetch may
// have moved the remote-tracking refs, and the ahead/behind counts with them.
func (m model) finishPull(msg pullFinishedMsg) (model, tea.Cmd) {
	m.pulling = false
	notice := pullNotice(msg)
	if !msg.result.Fetched {
		m.notice = notice
		return m, nil
	}
	// The remotes answered, which is what auto-fetch was waiting to see.
	m.autoFetchStopped = false
	next, cmd := m.reloadRepo()
	next.notice = notice
	return next, cmd
}

func pullNotice(msg pullFinishedMsg) string {
	r := msg.result
	if msg.err != nil {
		reason := firstLine(r.Detail)
		if reason == "" {
			reason = msg.err.Error()
		}
		return "Pull failed: " + reason
	}
	switch r.Outcome {
	case git.PullFastForwarded:
		return fmt.Sprintf("Pulled %s from %s", plural(r.Behind, "commit"), r.Upstream)
	case git.PullDiverged:
		// Said in full, since "nothing happened" needs a reason and a way on.
		return fmt.Sprintf("Not pulled: %s and %s have diverged (↑%d ↓%d) — merge or rebase in a terminal",
			r.Branch, r.Upstream, r.Ahead, r.Behind)
	case git.PullNoUpstream:
		return "Nothing to pull — " + r.Branch + " tracks no remote branch"
	case git.PullDetached:
		return "Nothing to pull — HEAD is not on a branch"
	}
	return "Already up to date with " + r.Upstream
}
