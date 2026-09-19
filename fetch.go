package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fetchTimeout gives a slow remote room to answer while still ending a fetch
// that has stalled — on a network that swallows the connection, git would wait
// far longer than anyone watching a graph will.
const fetchTimeout = 60 * time.Second

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
	if out, err := m.git("remote"); err != nil || strings.TrimSpace(out) == "" {
		m.notice = "Nothing to fetch — this repository has no remote"
		return m, nil
	}
	m.fetching = true
	m.notice = ""
	return m, fetchCmd(m.repoPath)
}

// fetchCmd runs the fetch. --prune drops remote-tracking branches whose remote
// branch is gone, which is the point of asking: stale ones are what make the
// graph disagree with the server. Local branches and tags are left alone.
func fetchCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "fetch", "--all", "--prune", "--quiet")
		cmd.Dir = repoPath
		cmd.Env = noPromptEnv(repoPath)
		var errOut bytes.Buffer
		cmd.Stderr = &errOut

		err := cmd.Run()
		if ctx.Err() == context.DeadlineExceeded {
			err = fmt.Errorf("no answer after %s", fetchTimeout)
		}
		if err != nil {
			log.Printf("Fetch failed: %v (%s)", err, errOut.String())
		}
		return fetchFinishedMsg{repoPath: repoPath, err: err, detail: firstLine(errOut.String())}
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
