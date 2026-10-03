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
}

type commitFinishedMsg struct {
	repoPath string // the repository committed in; see the handler
	result   git.CommitResult
	err      error
}

const (
	commitPromptWidth    = 72 // the width a commit message is conventionally kept to
	commitBodyRows       = 6
	commitFailureRows    = 6
	commitSubjectAdvised = 50
)

// openCommitPrompt asks for the message, or says why there is nothing to
// commit yet.
func (m model) openCommitPrompt() model {
	if !m.commitView.workingTree || m.staging || m.committing {
		return m
	}
	switch {
	case m.workingState.Operation != "":
		m.notice = inProgressNotice(m.workingState.Operation)
		return m
	case m.workingState.Detached:
		m.notice = "HEAD is on no branch — check one out before committing, or the commit is easy to lose"
		return m
	case stagedCount(m.viewedFiles()) == 0:
		// Not "commit everything": what goes in is what was put there.
		m.notice = "Nothing staged — press s on a file or a hunk to stage it"
		return m
	}

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
	p.open, p.failure = true, ""
	p.focusField(false)
	return m
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
	width := max(20, min(commitPromptWidth, windowWidth-8))
	p.subject.Width = width - 1 // the cursor takes a column past the text
	p.body.SetWidth(width)
	p.body.SetHeight(commitBodyRows)

	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Title))
	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Commit"))
	sb.WriteString(helpStyle.Render(fmt.Sprintf("  %s to %s", plural(staged, "staged file"), branch)))
	sb.WriteString("\n\n")

	// Counted, not capped: fifty is a convention for what fits in a log, and
	// some subjects are worth more.
	count := fmt.Sprintf("%d", ansi.StringWidth(p.subject.Value()))
	if ansi.StringWidth(p.subject.Value()) > commitSubjectAdvised {
		count += fmt.Sprintf(" — over %d", commitSubjectAdvised)
	}
	sb.WriteString(label.Render("Subject") + helpStyle.Render("  "+count))
	sb.WriteString("\n")
	sb.WriteString(p.subject.View())
	sb.WriteString("\n\n")
	sb.WriteString(label.Render("Body"))
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
		Width(width).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}
