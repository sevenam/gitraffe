package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/theme"
)

// renderStatusLine renders the bottom line: the update prompt or notice when one
// is pending, otherwise the usual key help. Sharing the single line keeps the
// height arithmetic in View() unchanged.
func (m *model) renderStatusLine() string {
	noticeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Tag))

	// The search prompt takes the line while it is open: it is where you are
	// typing, so nothing else on it could be read anyway.
	if m.search.active {
		return m.renderSearchPrompt()
	}

	if m.copying {
		return truncateLines(noticeStyle.Render(copyPrompt), m.windowWidth)
	}
	if m.checkout.open {
		return truncateLines(noticeStyle.Render(m.checkoutQuestion()), m.windowWidth)
	}
	if m.updateState == updateConfirming {
		return truncateLines(noticeStyle.Render(
			fmt.Sprintf("Update v%s → %s? This replaces the binary and quits. (y/n)", Version, m.latestVersion)),
			m.windowWidth)
	}
	if m.updateMessage != "" {
		return truncateLines(noticeStyle.Render(m.updateMessage), m.windowWidth)
	}
	// Ahead of the notice: a fetch or a pull keeps running after
	// the key that started it, so the line has to say it is still going.
	if m.pulling {
		return truncateLines(noticeStyle.Render("Pulling from the remote..."), m.windowWidth)
	}
	if m.deleting {
		return truncateLines(noticeStyle.Render("Deleting..."), m.windowWidth)
	}
	if m.fetching {
		return truncateLines(noticeStyle.Render("Fetching from the remote..."), m.windowWidth)
	}
	if m.notice != "" {
		return truncateLines(noticeStyle.Render(m.notice), m.windowWidth)
	}

	// Only the keys needed to get around; "?" lists the rest. The line has to
	// fit a typical terminal, and "?" leads so truncation never hides the way
	// to find everything else.
	help := "?: help • enter: details • space: diff • r: reload • tab: cycle • ↑/↓/j/k: scroll • q: quit"
	if m.updateAvailable() {
		// Leads rather than trails: the line is already near a typical terminal's
		// width, so a trailing hint is the first thing truncation eats.
		help = "U: update to " + m.latestVersion + " • " + help
	}
	if !m.filter.IsZero() {
		// Leads for the same reason: a filtered graph that gave no hint of
		// the way out would look like a repository with three commits.
		help = "esc: clear filter • " + help
	}

	// The position is pinned to the right edge and the key help gives way to
	// it: help is there once, while the position changes with every keypress.
	pos := m.graphPosition()
	if pos == "" || m.windowWidth <= 0 || ansi.StringWidth(pos)+2 > m.windowWidth {
		return truncateLines(helpStyle.Render(help), m.windowWidth)
	}
	room := m.windowWidth - ansi.StringWidth(pos) - 2
	help = ansi.Truncate(help, room, "")
	gap := m.windowWidth - ansi.StringWidth(help) - ansi.StringWidth(pos)
	return helpStyle.Render(help + strings.Repeat(" ", gap) + pos)
}

// graphPosition says where the selection is in the history, e.g.
// "1,234/5,000 · 24%". Only real commits are counted: the uncommitted-changes
// row sits above them as position 0, so the newest commit is always 1.
//
// A history cut short at commitLimit shows "5,000+" and no percentage, because
// the percentage would be of what happens to be loaded — 100% at a bottom that
// isn't one.
func (m *model) graphPosition() string {
	if !m.ready || m.err != nil || m.selected < 0 || m.selected >= len(m.commits) {
		return ""
	}
	pos, total := m.selected+1, len(m.commits)
	if m.commits[0].WorkingTree {
		pos, total = pos-1, total-1
	}
	if total <= 0 {
		return ""
	}
	if m.moreCommits {
		return thousands(pos) + "/" + thousands(total) + "+"
	}
	return fmt.Sprintf("%s/%s · %d%%", thousands(pos), thousands(total), pos*100/total)
}
