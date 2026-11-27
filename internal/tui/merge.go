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

// Merging: "m" on the graph merges the selected commit's branch into the
// branch checked out. See git.Merge.
//
// It opens a box that says what enter will do before anything is done: move
// the branch forward to the commit, or make a merge commit. That is git's own
// answer, merge.ff included, so enter does what "git merge" would. Tab steps
// through the other choices: a merge commit even where a fast-forward would do
// (--no-ff), and a squash, which stages the branch's changes and commits
// nothing (--squash). A squash then opens the commit box on the uncommitted
// changes with a message made from the squashed commits, so enter commits
// them, and esc leaves them staged like any others. It is lower case, unlike
// "C", because the box is the question: a slip opens it and esc closes it.
//
// The branch is the one "c" would check out, and with several on the commit
// the same list asks which. A commit with none is merged as a commit.
//
// What can be told before the box opens is told instead of opening it: a
// detached HEAD, uncommitted changes, a commit already on the branch, and a
// merge that would conflict, which the plan finds by trying it out in the
// object store. git.Merge asks all of it again when enter is pressed.

// mergePrompt is the box "m" opens.
type mergePrompt struct {
	open bool
	ref  string // the ref as git.MergePlan was asked it: a full ref name or a hash
	// name is what was picked, as the box and the notices name it: a branch
	// name, or the commit's short hash.
	name  string
	short string // the commit's short hash, which a fast-forward moves to
	into  string // the branch checked out
	plan  git.PlannedMerge
	// choice is what tab has picked; see choices.
	choice mergeChoice
	// message is the merge commit's message as "e" left it; "" is the plan's.
	message string
	// editing says the message box "e" opens is over this one, holding the
	// message being written.
	editing bool
	editor  commitPrompt
}

// mergeChoice is one of the ways the box can merge.
type mergeChoice int

const (
	// mergeDefault is the plan's: git's own answer, merge.ff included.
	mergeDefault mergeChoice = iota
	// mergeNoFF is a merge commit asked for in so many words (--no-ff).
	mergeNoFF
	// mergeSquash stages the changes and commits nothing (--squash).
	mergeSquash
)

type mergePlannedMsg struct {
	repoPath string // the repository planned in; see the handler
	ref      string
	name     string
	short    string
	plan     git.PlannedMerge
	err      error
}

type mergedMsg struct {
	repoPath    string // the repository merged in; see the handler
	name, into  string
	fastForward bool
	squash      bool
	// squashMessage is the message for the commit of what a squash staged.
	squashMessage string
	newHash       string
	detail        string
	err           error
}

// choices are the ways tab steps through, in order. A merge commit is
// offered on its own only where the plan's answer is something else: a
// fast-forward, or nothing at all under merge.ff=only.
func (p mergePrompt) choices() []mergeChoice {
	if p.plan.Kind == git.MergeFastForward || p.onlyBlocks() {
		return []mergeChoice{mergeDefault, mergeNoFF, mergeSquash}
	}
	return []mergeChoice{mergeDefault, mergeSquash}
}

// step moves the choice along the list by delta, round from the end.
func (p *mergePrompt) step(delta int) {
	choices := p.choices()
	at := 0
	for i, c := range choices {
		if c == p.choice {
			at = i
		}
	}
	p.choice = choices[((at+delta)%len(choices)+len(choices))%len(choices)]
}

// onlyBlocks says merge.ff is "only" and the branch cannot be moved forward,
// so the plan's own answer is to do nothing.
func (p mergePrompt) onlyBlocks() bool {
	return !p.plan.CanFastForward && p.plan.FastForward == git.FastForwardOnly
}

// fastForward says what enter would do as the box stands now.
func (p mergePrompt) fastForward() bool {
	return p.plan.Kind == git.MergeFastForward && p.choice == mergeDefault
}

// squash says enter would stage the changes rather than commit them.
func (p mergePrompt) squash() bool {
	return p.choice == mergeSquash
}

// blocked says enter would only be refused: merge.ff is "only" and the
// branch cannot be moved forward. Tab is still offered, since a merge commit
// asked for in so many words is what --no-ff means to git as well, and a
// squash moves no branch.
func (p mergePrompt) blocked() bool {
	return p.choice == mergeDefault && p.onlyBlocks()
}

// makesCommit says enter would make a merge commit, whose message "e" edits.
func (p mergePrompt) makesCommit() bool {
	return !p.fastForward() && !p.blocked() && !p.squash()
}

// squashSubject is the subject of the commit of a squash of several commits:
// the merge commit's, with the word for what it is changed. One commit
// squashed keeps its own message; see git.SquashMessage.
func (p mergePrompt) squashSubject() string {
	subject, _, _ := strings.Cut(p.plan.Message, "\n")
	if rest, ok := strings.CutPrefix(subject, "Merge "); ok {
		return "Squash " + rest
	}
	return "Squash " + p.name
}

// commitMessage is the merge commit's message as it will be made.
func (p mergePrompt) commitMessage() string {
	if strings.TrimSpace(p.message) != "" {
		return p.message
	}
	return p.plan.Message
}

// openMerge plans the merge of the commit on the graph, unless it is already
// clear that it could not be made; the box opens when the plan comes back.
func (m model) openMerge() (model, tea.Cmd) {
	if !m.ready || m.err != nil || m.merging {
		return m, nil
	}
	c, ok := m.commitOnScreen()
	if !ok {
		return m, nil
	}
	if notice := m.mergeRefusal(c); notice != "" {
		m.notice = notice
		return m, nil
	}
	var targets []git.SwitchTarget
	for _, t := range git.SwitchTargets(c.Refs, c.FullHash) {
		if t.Kind == git.SwitchBranch && t.Name == m.currentBranch {
			continue
		}
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		// Only the branch checked out is here, so this is HEAD's commit,
		// which mergeRefusal has said; kept for a ref read since.
		m.notice = "Not merged: " + c.Hash + " is already on " + m.currentBranch
		return m, nil
	}
	if len(targets) == 1 {
		return m.planMerge(targets[0])
	}
	m.checkout = checkoutPrompt{open: true, targets: targets, current: m.currentBranch, merge: true}
	return m, nil
}

// mergeRefusal is why nothing could be merged into the branch now, as the
// status line says it, or "" when something may be.
func (m model) mergeRefusal(c commit) string {
	switch {
	case c.WorkingTree:
		return "These are your uncommitted changes — pick a commit to merge"
	case m.branchBlocked() != "":
		return m.branchBlocked() + " is running — merge when it has finished"
	}
	// Read here rather than taken from the model, which knows the working
	// state only once the uncommitted changes have been opened; and asked
	// before the box opens rather than once enter is pressed, so that nobody
	// reads the box for nothing. git.Merge asks it all again.
	state := git.ReadWorkingState(m.repoPath)
	switch {
	case state.Operation != "":
		return inProgressNotice(state.Operation)
	case state.Detached || m.currentBranch == "":
		return mergeDetachedNotice
	case c.FullHash == m.headHash:
		return "Not merged: " + c.Hash + " is the commit checked out"
	}
	if dirty, _ := git.LocalChanges(m.repoPath); dirty {
		return mergeDirtyNotice
	}
	return ""
}

const (
	mergeDirtyNotice    = "Not merged: you have uncommitted changes — commit or stash them first"
	mergeDetachedNotice = "Not merged: HEAD is on no branch — check out the branch to merge into"
)

// planMerge asks git what merging target would do; the box opens with the
// answer. The commit's short hash is kept for the box to say where a
// fast-forward goes.
func (m model) planMerge(target git.SwitchTarget) (model, tea.Cmd) {
	if busy := m.branchBlocked(); busy != "" {
		m.notice = busy + " is running — merge when it has finished"
		return m, nil
	}
	ref, name := target.Name, target.Name
	switch target.Kind {
	case git.SwitchBranch:
		// Full names, so that a tag of the same name is never the one meant.
		ref = "refs/heads/" + target.Name
	case git.SwitchRemote:
		ref = "refs/remotes/" + target.Name
	case git.SwitchDetached:
		name = shortHash(target.Name)
	}
	short := name
	if c, ok := m.commitOnScreen(); ok {
		short = c.Hash
	}
	m.merging = true
	repoPath := m.repoPath
	return m, func() tea.Msg {
		plan, err := git.MergePlan(repoPath, ref)
		return mergePlannedMsg{repoPath: repoPath, ref: ref, name: name, short: short, plan: plan, err: err}
	}
}

// finishMergePlan opens the box, or says why there is nothing to ask.
func (m model) finishMergePlan(msg mergePlannedMsg) (model, tea.Cmd) {
	m.merging = false
	into := m.currentBranch
	switch {
	case errors.Is(msg.err, git.ErrMergeUnrelated):
		m.notice = "Not merged: " + msg.name + " shares no history with " + into + " — join them in a terminal"
		return m, nil
	case errors.Is(msg.err, git.ErrNoHead):
		m.notice = "Not merged: " + into + " has no commits yet"
		return m, nil
	case msg.err != nil:
		m.notice = "Not merged: " + msg.err.Error()
		return m, nil
	case msg.plan.Kind == git.MergeUpToDate:
		m.notice = "Not merged: " + msg.name + " is already on " + into
		return m, nil
	case msg.plan.Conflicts:
		m.notice = "Not merged: " + msg.name + " would conflict with " + into + " — resolve that in a terminal"
		return m, nil
	}
	m.merge = mergePrompt{open: true, ref: msg.ref, name: msg.name, short: msg.short, into: into, plan: msg.plan}
	return m, nil
}

// updateMerge is the keyboard while the box is open. Enter or "y" merges;
// tab and "e" change what enter does; any other key closes the box, as at
// the cherry-pick question, so a key pressed by mistake costs nothing.
func (m model) updateMerge(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.merge
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "tab":
		p.step(1)
		return m, nil
	case "shift+tab":
		p.step(-1)
		return m, nil
	case "e":
		if !p.makesCommit() {
			return m, nil
		}
		return m.openMergeMessage(), nil
	case "enter", "y", "Y":
		if p.blocked() {
			// Nothing enter could do; the box says that tab is the way.
			return m, nil
		}
		return m.startMerge()
	}
	m.merge = mergePrompt{}
	m.notice = "Nothing merged"
	return m, nil
}

// startMerge merges as the box says.
func (m model) startMerge() (model, tea.Cmd) {
	p := m.merge
	m.merge = mergePrompt{}
	// Something may have started while the box was open.
	if busy := m.branchBlocked(); busy != "" {
		m.notice = busy + " is running — merge when it has finished"
		return m, nil
	}
	m.merging = true
	repoPath := m.repoPath
	opts := git.MergeOptions{NoFF: p.choice == mergeNoFF, Squash: p.squash(), Message: p.message}
	return m, func() tea.Msg {
		newHash, detail, err := git.Merge(repoPath, p.ref, opts)
		msg := mergedMsg{repoPath: repoPath, name: p.name, into: p.into,
			fastForward: p.fastForward(), squash: opts.Squash, newHash: newHash, detail: detail, err: err}
		if err == nil && opts.Squash {
			msg.squashMessage = git.SquashMessage(repoPath, p.squashSubject())
		}
		return msg
	}
}

// openMergeMessage opens the commit message box over the merge box, holding
// the merge commit's message: the plan's, or what was written last time.
func (m model) openMergeMessage() model {
	p := &m.merge
	p.editor.start()
	subject, body, _ := strings.Cut(p.commitMessage(), "\n")
	p.editor.subject.SetValue(strings.TrimSpace(subject))
	p.editor.body.SetValue(strings.TrimSpace(body))
	p.editor.open, p.editor.failure = true, ""
	p.editor.focusField(false)
	p.editor.resize(m.windowWidth)
	p.editing = true
	return m
}

// updateMergeMessage is the keyboard of the message box over the merge box.
// It is the commit box's, but enter merges with the message, and esc goes
// back to the merge box keeping what was typed.
func (m model) updateMergeMessage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.merge
	e := &p.editor
	e.resize(m.windowWidth)
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		p.editing = false
		if strings.TrimSpace(e.subject.Value()) != "" {
			p.message = e.message()
		}
		return m, nil
	case "tab", "shift+tab":
		e.focusField(!e.inBody)
		return m, nil
	case "enter":
		if e.inBody {
			break
		}
		if strings.TrimSpace(e.subject.Value()) == "" {
			e.failure = "A merge commit needs a subject line."
			return m, nil
		}
		p.message, p.editing = e.message(), false
		return m.startMerge()
	}
	var cmd tea.Cmd
	if e.inBody {
		e.body, cmd = e.body.Update(msg)
	} else {
		e.subject, cmd = e.subject.Update(msg)
	}
	return m, cmd
}

// finishMerge says what happened, and on success reads the repository again
// with the new HEAD selected: the merge commit, or the commit the branch
// moved to.
func (m model) finishMerge(msg mergedMsg) (model, tea.Cmd) {
	m.merging = false
	if msg.err != nil {
		m.notice = mergeFailedNotice(msg)
		return m, nil
	}
	next, cmd := m.reloadRepo()
	if msg.squash {
		// HEAD has not moved; the changes are the uncommitted ones, which
		// the reload puts at the top.
		next.reselect = ""
		message := msg.squashMessage
		next.squashed = &message
		next.notice = "Squashed " + msg.name + " onto " + msg.into + ", staged — enter commits it, esc leaves it staged"
		return next, cmd
	}
	next.reselect = msg.newHash
	if msg.fastForward {
		next.notice = "Fast-forwarded " + msg.into + " to " + shortHash(msg.newHash)
	} else {
		next.notice = "Merged " + msg.name + " into " + msg.into + " as " + shortHash(msg.newHash)
	}
	return next, cmd
}

// applySquashed opens the commit box on the changes a squash has just
// staged, once the reload has read them: the commit view of the uncommitted
// changes, and the box over it holding the squash's message. With a file's
// history on screen there is no row of uncommitted changes to open, and the
// changes wait, staged, for the filter to be left.
func (m *model) applySquashed() {
	message := m.squashed
	m.squashed = nil
	if message == nil || len(m.commits) == 0 || !m.commits[0].WorkingTree {
		return
	}
	m.selected = 0
	m.commitView = commitView{open: true, commit: 0, workingTree: true, focus: commitBoxFiles}
	m.showFile()
	// Whatever was typed before is not about these changes.
	m.commitPrompt = commitPrompt{}
	*m = m.showCommitPrompt(0)
	subject, body, _ := strings.Cut(*message, "\n")
	m.commitPrompt.subject.SetValue(strings.TrimSpace(subject))
	m.commitPrompt.body.SetValue(strings.TrimSpace(body))
}

func mergeFailedNotice(msg mergedMsg) string {
	switch {
	case errors.Is(msg.err, git.ErrMergeConflicts):
		return "Not merged: " + msg.name + " would conflict with " + msg.into + " — resolve that in a terminal"
	case errors.Is(msg.err, git.ErrMergeUpToDate):
		return "Not merged: " + msg.name + " is already on " + msg.into
	case errors.Is(msg.err, git.ErrMergeNotFastForward):
		return "Not merged: merge.ff is only, and " + msg.name + " is not a fast-forward of " + msg.into
	case errors.Is(msg.err, git.ErrMergeUnrelated):
		return "Not merged: " + msg.name + " shares no history with " + msg.into + " — join them in a terminal"
	case errors.Is(msg.err, git.ErrLocalChanges):
		return mergeDirtyNotice
	case errors.Is(msg.err, git.ErrDetached):
		return mergeDetachedNotice
	case errors.Is(msg.err, git.ErrNoHead):
		return "Not merged: " + msg.into + " has no commits yet"
	case errors.Is(msg.err, git.ErrInProgress):
		return inProgressNotice(strings.TrimPrefix(msg.err.Error(), git.ErrInProgress.Error()+": "))
	}
	reason := firstLine(msg.detail)
	if reason == "" {
		reason = msg.err.Error()
	}
	return "Not merged: " + reason
}

// what is the line saying what enter does.
func (p mergePrompt) what() string {
	switch {
	case p.blocked():
		return "nothing: merge.ff is only, and this is not a fast-forward"
	case p.fastForward():
		return "fast-forward " + p.into + " to " + p.short
	case p.squash():
		return "squash: its changes staged, to commit (--squash)"
	case p.choice == mergeNoFF:
		return "as a merge commit (--no-ff)"
	}
	return "as a merge commit"
}

// keys are the keys the box offers as it stands, in the order the footer
// lists them.
func (p mergePrompt) keys() [][2]string {
	var keys [][2]string
	switch {
	case p.squash():
		keys = append(keys, [2]string{"enter", "squash"})
	case !p.blocked():
		keys = append(keys, [2]string{"enter", "merge"})
	}
	next := p
	next.step(1)
	keys = append(keys, [2]string{"tab", next.choiceName()})
	if p.makesCommit() {
		keys = append(keys, [2]string{"e", "message"})
	}
	return append(keys, [2]string{"esc", "cancel"})
}

// choiceName is what the footer calls the choice, as tab would pick it.
func (p mergePrompt) choiceName() string {
	switch {
	case p.choice == mergeSquash:
		return "squash"
	case p.choice == mergeNoFF:
		return "merge commit"
	case p.fastForward():
		return "fast-forward"
	case p.blocked():
		return "no merge commit"
	}
	return "merge commit"
}

// render draws the box over the screen, made like the cherry-pick question:
// a title, what goes into what, what enter does, the merge commit's message
// when there is to be one, and the keys along the bottom.
func (p mergePrompt) render(windowWidth int) string {
	key := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.SelectedFg))
	var plain, footer []string
	for _, k := range p.keys() {
		plain = append(plain, k[0]+": "+k[1])
		footer = append(footer, key.Render(k[0])+helpStyle.Render(": "+k[1]))
	}
	footerWidth := ansi.StringWidth(strings.Join(plain, " • "))

	what := p.what()
	subject, _, _ := strings.Cut(p.commitMessage(), "\n")
	showMessage := p.makesCommit()
	into := p.name + "  into  " + p.into
	// The border and the padding take six columns.
	width := max(ansi.StringWidth(into), ansi.StringWidth(what), footerWidth)
	if showMessage {
		width = max(width, ansi.StringWidth(subject))
	}
	width = max(10, min(width, windowWidth-6))

	// The names are what matter, so each keeps half the line when both
	// will not fit.
	name, onto := p.name, p.into
	if ansi.StringWidth(into) > width {
		half := max(1, (width-8)/2)
		name, onto = ansi.Truncate(name, half, "…"), ansi.Truncate(onto, half, "…")
	}
	nameStyle := localBranchStyle
	if strings.HasPrefix(p.ref, "refs/remotes/") {
		nameStyle = remoteBranchStyle
	} else if !strings.HasPrefix(p.ref, "refs/") {
		nameStyle = commitHashStyle
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render(ansi.Truncate("Merge", width, "…")))
	sb.WriteString("\n\n")
	sb.WriteString(nameStyle.Render(name) + helpStyle.Render("  into  ") + localBranchStyle.Render(onto) + "\n")
	sb.WriteString(helpStyle.Render(ansi.Truncate(what, width, "…")))
	if showMessage {
		sb.WriteString("\n" + ansi.Truncate(subject, width, "…"))
	}
	sb.WriteString("\n\n")
	sb.WriteString(ansi.Truncate(strings.Join(footer, helpStyle.Render(" • ")), width, "…"))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}

// renderMessage draws the message box "e" opens.
func (p mergePrompt) renderMessage(windowWidth int) string {
	return p.editor.renderAs(windowWidth, "Merge commit", "  "+p.name+" into "+p.into, "merge", "back")
}
