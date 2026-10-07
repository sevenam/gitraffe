package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// Committing: "c" in the commit view of the uncommitted changes asks for a
// message and commits what is staged — only that, and only on enter. See
// git.CommitStaged for what the commit itself does and refuses.
//
// With nothing staged, "c" stages everything first and says so for as long
// as the box is open: someone who staged nothing and asks to commit means all
// of it, and being sent back to press "S" first was a step that taught
// nothing. It is still only staging, which copies changes into the index and
// loses none; nothing is committed until enter, and esc leaves a list of
// staged files that "S" takes back.
//
// The message is two fields, a subject line and a body, because that is what
// a commit message is; and because it lets enter mean "commit" where one line
// is being typed and "new line" where several are, with tab between the two.

// commitPrompt is the box "c" opens. It keeps what was typed when it is
// closed without committing, and when the commit is refused, so a message is
// never typed twice.
type commitPrompt struct {
	open    bool
	started bool // the inputs have been made
	subject textinput.Model
	body    textarea.Model
	inBody  bool
	// failure is why the last try did not commit: nothing typed, or what git
	// or a hook of the repository's said.
	failure string
	// autoStaged is how many files "c" staged itself, nothing having been
	// staged when it was pressed; 0 when the box holds what the user staged.
	autoStaged int
}

type commitFinishedMsg struct {
	repoPath string // the repository committed in; see the handler
	result   git.CommitResult
	err      error
}

// The 50/72 rule: a subject of at most fifty characters, which is what fits
// in a one-line log, and body lines of at most seventy-two, which leaves git's
// indent and as much again to spare in an eighty-column terminal. The box
// counts down to both and does not stop at either: they are conventions, and
// some subjects are worth more.
const (
	commitSubjectWidth  = 50
	commitBodyWidth     = 72
	commitPromptWidth   = commitBodyWidth
	commitBodyRows      = 6
	commitFailureRows   = 6
	commitPromptPadding = 2
)

// roomLeft draws a count of the columns left before a limit: how many more
// fit, or below zero, in the colour of an error, how many too many there are.
// Counting down says the one thing worth knowing while typing, and says it
// without the limit having to be remembered.
func roomLeft(n int) string {
	if n < 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.Error)).Render(fmt.Sprint(n))
	}
	return helpStyle.Render(fmt.Sprint(n))
}

// bodyRoom is the room left on the body line being typed. With the cursor
// elsewhere it is that of the longest line, so one that is too long is not
// forgotten for being out of sight.
func (p commitPrompt) bodyRoom() int {
	lines := strings.Split(p.body.Value(), "\n")
	if row := p.body.Line(); p.inBody && row >= 0 && row < len(lines) {
		return commitBodyWidth - ansi.StringWidth(lines[row])
	}
	room := commitBodyWidth
	for _, line := range lines {
		room = min(room, commitBodyWidth-ansi.StringWidth(line))
	}
	return room
}

// openCommitPrompt asks for the message, or says why there is nothing to
// commit yet. With changes and none of them staged it stages them all first,
// and the box opens when that is done; see finishStage.
func (m model) openCommitPrompt() (model, tea.Cmd) {
	if !m.commitView.workingTree || m.staging || m.committing {
		return m, nil
	}
	switch {
	case m.workingState.Operation != "":
		m.notice = inProgressNotice(m.workingState.Operation)
		return m, nil
	case m.workingState.Detached:
		m.notice = "HEAD is on no branch — check one out before committing, or the commit is easy to lose"
		return m, nil
	case len(m.viewedFiles()) == 0:
		m.notice = "Nothing to commit — there are no changes"
		return m, nil
	case stagedCount(m.viewedFiles()) == 0:
		m.staging = true
		m.commitView.selecting = false
		m.commitView.commitNext = true
		return m, stageCmd(m.repoPath, m.diffStatWidth(), git.StageAll)
	}
	return m.showCommitPrompt(0), nil
}

// showCommitPrompt opens the box. autoStaged is how many files were staged
// to make a commit of, when "c" did that itself.
func (m model) showCommitPrompt(autoStaged int) model {
	p := &m.commitPrompt
	if !p.started {
		p.subject = textinput.New()
		p.subject.Prompt = ""
		p.subject.Placeholder = "what this commit does"
		p.subject.PlaceholderStyle = helpStyle
		p.subject.Cursor.SetMode(cursor.CursorStatic)

		p.body = textarea.New()
		p.body.Prompt = ""
		p.body.Placeholder = "why, if it needs saying (optional)"
		p.body.ShowLineNumbers = false
		p.body.CharLimit = 0
		p.body.Cursor.SetMode(cursor.CursorStatic)
		// The textarea's own look tints the line the cursor is on, which
		// reads here as a second selection.
		plain := lipgloss.NewStyle()
		for _, s := range []*textarea.Style{&p.body.FocusedStyle, &p.body.BlurredStyle} {
			s.Base, s.CursorLine, s.Text, s.EndOfBuffer = plain, plain, plain, plain
			s.Placeholder = helpStyle
		}
		p.started = true
	}
	p.open, p.failure, p.autoStaged = true, "", autoStaged
	p.focusField(false)
	p.resize(m.windowWidth)
	return m
}

// fieldWidth is how wide the two fields are in a window this wide: as wide as
// a body line may be, so that a line which wraps on screen is a line that is
// too long, and narrower only where the window is.
func fieldWidth(windowWidth int) int {
	return max(20, min(commitPromptWidth, windowWidth-8))
}

// resize fits the fields to the window. It is done to the fields themselves
// and not just to a copy being drawn: where a long subject scrolls and where
// a body line wraps are worked out as the text is typed, from the width the
// field has then.
func (p *commitPrompt) resize(windowWidth int) {
	width := fieldWidth(windowWidth)
	if p.subject.Width == width {
		return
	}
	p.subject.Width = width
	// The width decides which part of a long subject is on show.
	p.subject.SetCursor(p.subject.Position())
	// One column more than the text: the cursor sits past the last character.
	p.body.SetWidth(width + 1)
	p.body.SetHeight(commitBodyRows)
}

// focusField puts the keyboard in the subject line or the body.
func (p *commitPrompt) focusField(body bool) {
	p.inBody = body
	if body {
		p.subject.Blur()
		p.body.Focus()
		return
	}
	p.body.Blur()
	p.subject.Focus()
}

// message is what was typed, as git wants it: the subject, a blank line, the
// body.
func (p commitPrompt) message() string {
	subject := strings.TrimSpace(p.subject.Value())
	body := strings.TrimSpace(p.body.Value())
	if body == "" {
		return subject
	}
	return subject + "\n\n" + body
}

// updateCommitPrompt is the keyboard while the box is open.
func (m model) updateCommitPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.commitPrompt
	// The window may have changed size since the box was opened.
	p.resize(m.windowWidth)
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		// Closed, not cleared: "c" again finds the message as it was left.
		p.open = false
		return m, nil
	case "tab", "shift+tab":
		p.focusField(!p.inBody)
		return m, nil
	case "enter":
		if p.inBody {
			break // a new line, which the textarea knows how to make
		}
		if strings.TrimSpace(p.subject.Value()) == "" {
			p.failure = "A commit needs a subject line."
			return m, nil
		}
		p.open, p.failure = false, ""
		m.committing = true
		return m, commitCmd(m.repoPath, p.message())
	}

	var cmd tea.Cmd
	if p.inBody {
		p.body, cmd = p.body.Update(msg)
	} else {
		p.subject, cmd = p.subject.Update(msg)
	}
	return m, cmd
}

func commitCmd(repoPath, message string) tea.Cmd {
	return func() tea.Msg {
		result, err := git.CommitStaged(repoPath, message)
		return commitFinishedMsg{repoPath: repoPath, result: result, err: err}
	}
}

// finishCommit shows the new commit, or gives the message back with the
// reason it was refused.
func (m model) finishCommit(msg commitFinishedMsg) (model, tea.Cmd) {
	m.committing = false
	if msg.err != nil {
		switch {
		case errors.Is(msg.err, git.ErrNothingStaged):
			m.notice = "Nothing staged — press s on a file or a hunk to stage it"
		case errors.Is(msg.err, git.ErrDetached):
			m.notice = "Not committed: HEAD is on no branch"
		case errors.Is(msg.err, git.ErrInProgress):
			m.notice = "Not committed: a merge or rebase is in progress — finish it in a terminal"
		default:
			// A hook said no, or git did. What it printed is the whole of
			// the explanation, so it is shown where the message can be
			// changed to suit it; nothing is retried or skipped.
			reason := msg.result.Output
			if reason == "" {
				reason = msg.err.Error()
			}
			m.commitPrompt.open = m.commitView.open && m.commitView.workingTree
			m.commitPrompt.failure = reason
			m.notice = "Not committed: " + firstLine(reason)
		}
		return m, nil
	}

	// The message has been used; the next commit starts with an empty box.
	m.commitPrompt = commitPrompt{}
	next, cmd := m.reloadRepo()
	next.committed = msg.result.Hash
	next.notice = fmt.Sprintf("Committed %s — %s", shortHash(msg.result.Hash), msg.result.Subject)
	return next, cmd
}

// applyCommitted settles where a reload after a commit leaves you. With
// changes still uncommitted the view stays on them, for the next commit to be
// put together; with none left there is nothing for it to show, so it closes
// and the graph has the new commit selected.
func (m *model) applyCommitted() {
	hash := m.committed
	m.committed = ""
	if hash == "" {
		return
	}
	if len(m.commits) > 0 && m.commits[0].WorkingTree {
		m.selected = 0
		return
	}
	m.commitView = commitView{}
	for i, c := range m.commits {
		if c.FullHash == hash {
			m.selected = i
			break
		}
	}
}

// render draws the box: where the commit is going, the two fields, the reason
// the last try failed, and the keys of the field the cursor is in.
func (p commitPrompt) render(windowWidth, staged int, branch string) string {
	// The box's padding goes round the fields; see resize for their width.
	p.resize(windowWidth)
	width := fieldWidth(windowWidth)

	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Title))
	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Commit"))
	sb.WriteString(helpStyle.Render(fmt.Sprintf("  %s to %s", plural(staged, "staged file"), branch)))
	sb.WriteString("\n\n")

	sb.WriteString(label.Render("Subject") + "  " + roomLeft(commitSubjectWidth-ansi.StringWidth(p.subject.Value())))
	sb.WriteString("\n")
	sb.WriteString(p.subject.View())
	sb.WriteString("\n\n")
	sb.WriteString(label.Render("Body") + "  " + roomLeft(p.bodyRoom()))
	sb.WriteString("\n")
	sb.WriteString(p.body.View())
	sb.WriteString("\n\n")

	if p.failure != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.Error))
		lines := strings.Split(strings.TrimSpace(p.failure), "\n")
		if len(lines) > commitFailureRows {
			lines = append(lines[:commitFailureRows-1], "…")
		}
		for _, line := range lines {
			sb.WriteString(errStyle.Render(ansi.Truncate(strings.ReplaceAll(line, "\t", "    "), width, "…")))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	footer := "enter: commit • tab: body • esc: cancel"
	if p.inBody {
		footer = "enter: new line • tab: subject, to commit • esc: cancel"
	}
	sb.WriteString(helpStyle.Render(footer))

	return lipgloss.NewStyle().
		Width(width+2*commitPromptPadding+1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, commitPromptPadding).
		Render(sb.String())
}
