package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
)

type fetchFinishedMsg struct {
	repoPath string // the repository fetched; see the handler
	err      error
	detail   string // git's own first line of complaint, when it has one
}

// startFetch begins a fetch, or explains why there is nothing to do. Fetching
// is never automatic: it is the one thing gitraffe does that reaches the
// network and writes to the repository, so it happens when you ask and not
// before.
func (m model) startFetch() (model, tea.Cmd) {
	if m.fetching {
		return m, nil
	}
	if len(git.Remotes(m.repoPath)) == 0 {
		m.notice = "Nothing to fetch — this repository has no remote"
		return m, nil
	}
	m.fetching = true
	m.notice = ""
	return m, fetchCmd(m.repoPath)
}

// fetchCmd runs the fetch in the background; git.Fetch says what it asks for.
func fetchCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		stderr, err := git.Fetch(repoPath)
		return fetchFinishedMsg{repoPath: repoPath, err: err, detail: firstLine(stderr)}
	}
}

// firstLine is git's most useful line of an error: the rest is usually advice
// for a terminal the TUI is sitting on top of.
func firstLine(s string) string {
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// finishFetch reports the result and, when it worked, reads the repository
// again: a fetch only moves refs, and every count and tag mark on screen was
// worked out from the refs as they were.
func (m model) finishFetch(msg fetchFinishedMsg) (model, tea.Cmd) {
	m.fetching = false
	if msg.err != nil {
		m.notice = "Fetch failed: " + firstLine(msg.detail)
		if m.notice == "Fetch failed: " {
			m.notice = "Fetch failed: " + msg.err.Error()
		}
		return m, nil
	}
	next, cmd := m.reloadRepo()
	next.notice = "Fetched — ahead/behind and tags are up to date"
	return next, cmd
}
