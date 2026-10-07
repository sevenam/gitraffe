package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// Pushing: "P" sends the current branch to the branch it tracks. It is the
// one thing gitraffe does that leaves the machine with your work on it, so it
// is a capital, and it is kept to what cannot overwrite anything: the remote
// branch is moved forward or not at all. See git.Push.
//
// Two things are asked about first, because each puts a new name on the
// remote for everyone else to see. A tag on the selected commit that no
// remote has is offered in place of the branch, on the status line like the
// copy prompt. And a branch that tracks nothing has no remote branch to
// move, so a box asks what to call the one to create, offering its own name.

// pushPrompt is what "P" is waiting to be told, when it has to ask.
type pushPrompt struct {
	plan git.PushPlan

	// asking is the status-line question: one of the commit's unpushed tags,
	// or the branch.
	asking bool
	tags   []string
	commit string // full hash of the commit the tags are on

	// naming is the box that asks what to call a new remote branch.
	naming  bool
	name    textinput.Model
	remote  int    // index into plan.Remotes of where it would go
	failure string // why the name typed will not do
}

func (p pushPrompt) open() bool {
	return p.asking || p.naming
}

// The number keys pick a tag, so more than this cannot be offered; a commit
// with ten unpushed tags is one for a terminal.
const maxPushTags = 9

type pushFinishedMsg struct {
	repoPath string // the repository pushed from; see the handler
	result   git.PushResult
	commit   string // full hash of the commit a pushed tag is on
	err      error
}

// openPush pushes the current branch, or asks first when there is something
// to ask: see pushPrompt.
func (m model) openPush() (model, tea.Cmd) {
	if !m.ready || m.err != nil || m.pushing {
		return m, nil
	}
	if busy := m.busyWithRefs(); busy != "" {
		m.notice = busy + " is running — push again when it has finished"
		return m, nil
	}
	plan := git.ReadPushPlan(m.repoPath)
	if len(plan.Remotes) == 0 {
		m.notice = "Nothing to push to — this repository has no remote"
		return m, nil
	}

	if c, ok := m.commitOnScreen(); ok && !c.WorkingTree && len(c.UnpushedTags) > 0 {
		tags := make([]string, 0, len(c.UnpushedTags))
		for tag := range c.UnpushedTags {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
		if len(tags) > maxPushTags {
			tags = tags[:maxPushTags]
		}
		m.push = pushPrompt{plan: plan, asking: true, tags: tags, commit: c.FullHash}
		return m, nil
	}
	return m.pushBranch(plan)
}

// busyWithRefs names what is already moving refs or talking to the remotes,
// which a push would run into; "" when nothing is.
func (m model) busyWithRefs() string {
	switch {
	case m.fetching || m.autoFetching:
		return "A fetch"
	case m.pulling:
		return "A pull"
	case m.deleting:
		return "A delete"
	case m.switching:
		return "A checkout"
	case m.committing:
		return "A commit"
	}
	return ""
}

// pushQuestion is the status line while the tag-or-branch prompt is open.
// One tag is "t"; several are numbered.
func (m model) pushQuestion() string {
	p := m.push
	parts := make([]string, 0, len(p.tags)+2)
	for i, tag := range p.tags {
		key := "t"
		if len(p.tags) > 1 {
			key = fmt.Sprint(i + 1)
		}
		parts = append(parts, key+" tag "+tag)
	}
	if p.plan.Branch != "" {
		parts = append(parts, "b branch "+p.plan.Branch)
	}
	parts = append(parts, "esc cancel")
	return "Push: " + strings.Join(parts, " • ")
}

// answerPush pushes what the key picked. Any key it doesn't know closes the
// prompt, so a stray key press sends nothing.
func (m model) answerPush(msg tea.KeyMsg) (model, tea.Cmd) {
	p := m.push
	m.push = pushPrompt{}
	key := msg.String()
	switch {
	case key == "ctrl+c":
		return m, tea.Quit
	case key == "b":
		return m.pushBranch(p.plan)
	case key == "t" && len(p.tags) == 1:
		return m.startPush(pushTagCmd(m.repoPath, p.plan.NewBranchRemote, p.tags[0], p.commit))
	case len(p.tags) > 1 && len(key) == 1 && key[0] >= '1' && int(key[0]-'0') <= len(p.tags):
		return m.startPush(pushTagCmd(m.repoPath, p.plan.NewBranchRemote, p.tags[key[0]-'1'], p.commit))
	}
	return m, nil
}

// pushBranch pushes the current branch to what it tracks, or opens the box
// that asks what to create when it tracks nothing.
func (m model) pushBranch(plan git.PushPlan) (model, tea.Cmd) {
	switch {
	case plan.Branch == "":
		m.notice = "Nothing to push — HEAD is not on a branch"
		return m, nil
	case plan.Remote != "":
		return m.startPush(pushCmd(m.repoPath))
	}

	name := textinput.New()
	name.Prompt = ""
	name.CharLimit = 200
	name.Cursor.SetMode(cursor.CursorStatic)
	// The branch's own name, which is what it is called on the remote nine
	// times in ten; there to be accepted with enter or typed over.
	name.SetValue(plan.Branch)
	name.CursorEnd()
	name.Focus()

	m.push = pushPrompt{plan: plan, naming: true, name: name}
	for i, remote := range plan.Remotes {
		if remote == plan.NewBranchRemote {
			m.push.remote = i
		}
	}
	return m, nil
}

// updatePushNaming is the keyboard while the new-branch box is open.
func (m model) updatePushNaming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.push
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.push = pushPrompt{}
		return m, nil
	case "tab":
		p.remote = (p.remote + 1) % len(p.plan.Remotes)
		return m, nil
	case "shift+tab":
		p.remote = (p.remote + len(p.plan.Remotes) - 1) % len(p.plan.Remotes)
		return m, nil
	case "enter":
		name := strings.TrimSpace(p.name.Value())
		// Checked here, where it can still be changed, rather than by
		// the push once the box has gone.
		if !git.ValidBranchName(m.repoPath, name) {
			p.failure = "Git does not accept that as a branch name."
			return m, nil
		}
		remote := p.plan.Remotes[p.remote]
		m.push = pushPrompt{}
		return m.startPush(pushNewBranchCmd(m.repoPath, remote, name))
	}

	var cmd tea.Cmd
	p.failure = ""
	p.name, cmd = p.name.Update(msg)
	return m, cmd
}

func (m model) startPush(cmd tea.Cmd) (model, tea.Cmd) {
	m.pushing = true
	m.notice = ""
	return m, cmd
}

func pushCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		result, err := git.Push(repoPath)
		return pushFinishedMsg{repoPath: repoPath, result: result, err: err}
	}
}

func pushNewBranchCmd(repoPath, remote, name string) tea.Cmd {
	return func() tea.Msg {
		result, err := git.PushNewBranch(repoPath, remote, name)
		return pushFinishedMsg{repoPath: repoPath, result: result, err: err}
	}
}

func pushTagCmd(repoPath, remote, tag, commit string) tea.Cmd {
	return func() tea.Msg {
		result, err := git.PushTag(repoPath, remote, tag)
		return pushFinishedMsg{repoPath: repoPath, result: result, commit: commit, err: err}
	}
}

// finishPush says what happened and, when the remote took something, reads
// the repository again: the remote-tracking branch has moved, and the
// ahead/behind counts and the tag marks with it.
func (m model) finishPush(msg pushFinishedMsg) (model, tea.Cmd) {
	m.pushing = false
	notice := pushNotice(msg)
	r := msg.result
	if msg.err != nil || r.Outcome == git.PushBehind || r.Outcome == git.PushTagTaken {
		m.notice = notice
		return m, nil
	}
	// The remotes answered, which is what auto-fetch was waiting to see.
	m.autoFetchStopped = false
	// The remotes are asked for their tags again by the reload, but that
	// takes seconds, and until then the tag just pushed would go on being
	// marked as not pushed. This much of the answer is already known.
	if r.Tag != "" && msg.commit != "" && m.remoteTags != nil {
		tags := make(map[tagRef]bool, len(m.remoteTags)+1)
		for ref := range m.remoteTags {
			tags[ref] = true
		}
		tags[tagRef{Name: r.Tag, Commit: msg.commit}] = true
		m.remoteTags = tags
	}
	next, cmd := m.reloadRepo()
	next.notice = notice
	return next, cmd
}

func pushNotice(msg pushFinishedMsg) string {
	r := msg.result
	if msg.err != nil {
		if errors.Is(msg.err, git.ErrDetached) {
			return "Nothing to push — HEAD is not on a branch"
		}
		reason := firstLine(r.Detail)
		if reason == "" {
			reason = msg.err.Error()
		}
		return "Push failed: " + reason
	}
	if r.Tag != "" {
		switch r.Outcome {
		case git.PushTagTaken:
			return fmt.Sprintf("Not pushed: %s already has a tag %s, on another commit", r.Remote, r.Tag)
		case git.PushUpToDate:
			return fmt.Sprintf("Nothing to push — %s already has tag %s", r.Remote, r.Tag)
		}
		return fmt.Sprintf("Pushed tag %s to %s", r.Tag, r.Remote)
	}
	switch r.Outcome {
	case git.PushCreated:
		return fmt.Sprintf("Pushed %s to %s, a new branch it now tracks", r.Branch, r.Upstream())
	case git.PushUpToDate:
		return fmt.Sprintf("Nothing to push — %s already has every commit of %s", r.Upstream(), r.Branch)
	case git.PushBehind:
		// Said in full, since "nothing happened" needs a reason and a way on.
		return fmt.Sprintf("Not pushed: %s has commits %s lacks — pull first (p), or merge or rebase in a terminal",
			r.Upstream(), r.Branch)
	}
	if r.Commits == 0 {
		return fmt.Sprintf("Pushed %s to %s", r.Branch, r.Upstream())
	}
	return fmt.Sprintf("Pushed %s to %s", plural(r.Commits, "commit"), r.Upstream())
}

// pushNameWidth is the room for the name being typed: enough for a long
// branch name, and narrower only where the window is.
func pushNameWidth(windowWidth int) int {
	return max(20, min(50, windowWidth-12))
}

// render draws the new-branch box: what is being pushed, where to, the name
// it will have there, and why the last name typed would not do.
func (p pushPrompt) render(windowWidth int) string {
	width := pushNameWidth(windowWidth)
	p.name.Width = width
	// The width decides which part of a long name is on show.
	p.name.SetCursor(p.name.Position())

	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Title))
	remote := p.plan.Remotes[p.remote]
	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Push"))
	sb.WriteString(helpStyle.Render("  " + ansi.Truncate(p.plan.Branch+" has no branch on a remote yet", width, "…")))
	sb.WriteString("\n\n")

	sb.WriteString(label.Render("Remote") + "  " + remoteBranchStyle.Render(remote))
	if len(p.plan.Remotes) > 1 {
		sb.WriteString(helpStyle.Render(fmt.Sprintf("  %d of %d", p.remote+1, len(p.plan.Remotes))))
	}
	sb.WriteString("\n")
	sb.WriteString(label.Render("Branch") + "  " + p.name.View())
	sb.WriteString("\n\n")

	if p.failure != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.Error)).Render(p.failure))
		sb.WriteString("\n\n")
	}

	footer := "enter: create it and push • esc: cancel"
	if len(p.plan.Remotes) > 1 {
		footer = "enter: create it and push • tab: another remote • esc: cancel"
	}
	sb.WriteString(helpStyle.Render(footer))

	return lipgloss.NewStyle().
		Width(width+len("Branch  ")+2*commitPromptPadding+1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, commitPromptPadding).
		Render(sb.String())
}
