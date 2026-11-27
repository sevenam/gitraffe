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
	// offer is the merge box for the upstream, planned when the branch has
	// diverged and a merge is what git pull would do; nil when there is
	// none to offer. dirty says one was not planned over uncommitted
	// changes, which a merge is refused over.
	offer *mergePrompt
	dirty bool
}

// startPull catches the current branch up with its upstream. Like checking
// out, it changes your branch and your working tree, so it is kept to what
// cannot lose work or need resolving: a fast-forward, or nothing. See git.Pull.
//
// A branch that has diverged from its upstream is not merged, but the merge
// box is opened for the upstream, as "m" on it would: what a merge would do
// is said, and nothing happens without enter. That is offered only where
// git pull would merge too; under pull.ff=only, or a pull.rebase, it says
// what it always said, since a merge is not what that repository pulls with.
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
	if m.deleting {
		m.notice = "A delete is running — pull again when it has finished"
		return m, nil
	}
	// A push that lands while the branch is being moved would be answered
	// about a branch that is no longer where it was.
	if m.pushing {
		m.notice = "A push is running — pull again when it has finished"
		return m, nil
	}
	m.pulling = true
	m.notice = ""
	return m, pullCmd(m.repoPath)
}

func pullCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		result, err := git.Pull(repoPath)
		msg := pullFinishedMsg{repoPath: repoPath, result: result, err: err}
		if err == nil && result.Outcome == git.PullDiverged && result.Reconcile == git.PullMerges {
			msg.offer, msg.dirty = planPullMerge(repoPath, result)
		}
		return msg
	}
}

// planPullMerge plans the merge of the upstream that a diverged pull offers,
// asked as mergeRefusal asks before "m" opens its box: nothing is offered
// with a merge or cherry-pick under way, or over uncommitted changes, nor
// when the plan finds nothing a merge could do. It runs with the pull, so the
// box opens once the reload after it has read the repository.
func planPullMerge(repoPath string, r git.PullResult) (offer *mergePrompt, dirty bool) {
	if r.UpstreamRef == "" {
		return nil, false
	}
	if state := git.ReadWorkingState(repoPath); state.Operation != "" || state.Detached {
		return nil, false
	}
	if changed, _ := git.LocalChanges(repoPath); changed {
		return nil, true
	}
	plan, err := git.MergePlan(repoPath, r.UpstreamRef)
	if err != nil || plan.Kind == git.MergeUpToDate {
		return nil, false
	}
	short := r.Upstream
	if r.UpstreamCommit != "" {
		short = shortHash(r.UpstreamCommit)
	}
	return &mergePrompt{open: true, ref: r.UpstreamRef, name: r.Upstream, short: short,
		into: r.Branch, plan: plan}, false
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
	next.pullMerge = msg.offer
	return next, cmd
}

// applyPullMerge opens the merge box a diverged pull offers, once the reload
// after the pull has read the repository: so that the box is over the graph
// as it now is, upstream's new commits included. The branch checked out is
// asked again, in case it has changed since the pull began.
func (m *model) applyPullMerge() {
	offer := m.pullMerge
	m.pullMerge = nil
	if offer == nil || offer.into != m.currentBranch {
		return
	}
	m.merge = *offer
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
		switch {
		case msg.offer != nil:
			return fmt.Sprintf("%s and %s have diverged (↑%d ↓%d) — enter merges %s in, esc leaves it",
				r.Branch, r.Upstream, r.Ahead, r.Behind, r.Upstream)
		case msg.dirty:
			return fmt.Sprintf("Not pulled: %s and %s have diverged (↑%d ↓%d) — commit or stash your changes, and p offers a merge",
				r.Branch, r.Upstream, r.Ahead, r.Behind)
		}
		return fmt.Sprintf("Not pulled: %s and %s have diverged (↑%d ↓%d) — merge or rebase in a terminal",
			r.Branch, r.Upstream, r.Ahead, r.Behind)
	case git.PullNoUpstream:
		return "Nothing to pull — " + r.Branch + " tracks no remote branch"
	case git.PullDetached:
		return "Nothing to pull — HEAD is not on a branch"
	}
	return "Already up to date with " + r.Upstream
}
