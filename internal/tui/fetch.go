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
	auto     bool   // started by the timer, not by f; see auto_refresh.go
	skipped  bool   // there was no remote to fetch from
}

// startFetch begins a fetch, or explains why there is nothing to do. Unless
// settings.yml asks for auto-fetch, a fetch only ever starts from a key — this
// one, or a pull: it reaches the network and writes to the repository, so it
// happens when you ask and not before.
func (m model) startFetch() (model, tea.Cmd) {
	// A pull is fetching already, and reloads when it is done.
	if m.fetching || m.pulling {
		return m, nil
	}
	// One the timer started is already on its way. Adopting it, rather than
	// running a second beside it, makes its result the answer to this key.
	if m.autoFetching {
		m.fetching = true
		m.notice = ""
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

// fetchFailure is why a fetch failed, in git's words when it had any.
func fetchFailure(msg fetchFinishedMsg) string {
	if line := firstLine(msg.detail); line != "" {
		return line
	}
	return msg.err.Error()
}

// finishFetch reports the result and, when it worked, reads the repository
// again: a fetch only moves refs, and every count and tag mark on screen was
// worked out from the refs as they were.
func (m model) finishFetch(msg fetchFinishedMsg) (model, tea.Cmd) {
	// A fetch the timer started stays quiet unless f was pressed while it ran.
	if msg.auto && !m.fetching {
		return m.finishAutoFetch(msg)
	}
	m.fetching, m.autoFetching = false, false
	if msg.skipped {
		m.notice = "Nothing to fetch — this repository has no remote"
		return m, nil
	}
	if msg.err != nil {
		m.notice = "Fetch failed: " + fetchFailure(msg)
		return m, nil
	}
	m.autoFetchStopped = false
	next, cmd := m.reloadRepo()
	next.notice = "Fetched — ahead/behind and tags are up to date"
	return next, cmd
}
