package tui

import (
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
)

// loadRepo opens the repository. Whether that works decides which of repoMsg
// and errMsg comes back, and both go on to read the history: go-git failing to
// open a repository does not mean the git command line can't read it.
//
// The fingerprint is taken here, before the history is read, so a change made
// while it is being read makes the next check differ rather than being missed.
func loadRepo(path string) tea.Cmd {
	return loadRepoKnowing(path, "")
}

// loadRepoKnowing is loadRepo for a caller that has just taken the fingerprint
// itself, and so need not pay for it twice.
func loadRepoKnowing(path, fingerprint string) tea.Cmd {
	return func() tea.Msg {
		if fingerprint == "" {
			// Unreadable leaves it empty, which the checks take to mean "unknown".
			fingerprint, _ = git.Fingerprint(path)
		}
		repo, err := git.Open(path)
		if err != nil {
			return errMsg{err, fingerprint}
		}
		return repoMsg{repo, fingerprint}
	}
}

func (m *model) loadRepoInfo() {
	m.setRepoInfo(git.ReadInfo(m.repoPath, m.repo))
}

func (m *model) loadRepoInfoFromCLI() {
	m.setRepoInfo(git.ReadInfoCLI(m.repoPath))
}

func (m *model) setRepoInfo(info git.Info) {
	m.repoName, m.currentBranch, m.currentCommit = info.Name, info.Branch, info.Commit
}

// loadUpstreamSync counts the commits the current branch and the remote don't
// share. The counts are as of the last fetch — gitraffe never fetches unasked —
// and both stay zero where there is no meaningful answer, which renders as
// nothing.
func (m *model) loadUpstreamSync() {
	m.ahead, m.behind = git.UpstreamSync(m.repoPath)
}

// loadGraphData reads the history with its graph, then works out everything
// the screen derives from it: tag marks, and the widths of the label and
// author columns.
func (m *model) loadGraphData() error {
	g, err := git.LoadGraph(m.repoPath, m.commitCount(), m.filter)
	if err != nil {
		return err
	}
	m.commits = g.Commits
	m.displayRows = g.Rows
	m.maxGraphWidth = g.MaxGraphWidth
	m.moreCommits = g.More

	m.applyRemoteTags()
	m.updateLabelWidth()

	m.maxAuthorWidth = 0
	for _, c := range m.commits {
		m.maxAuthorWidth = max(m.maxAuthorWidth, ansi.StringWidth(c.Author))
	}
	return nil
}

// loadCommitsFromGitCLI reads the history without a graph, for when the graph
// could not be read.
func (m *model) loadCommitsFromGitCLI() ([]commit, error) {
	commits, more, err := git.LoadCommits(m.repoPath, m.commitCount(), m.filter)
	if err != nil {
		return nil, err
	}
	m.moreCommits = more
	return commits, nil
}

// finishLoad reads the open repository into the model once loadRepo has
// answered. openErr is why go-git could not open it, or nil; the history is
// read from the command line either way.
func (m model) finishLoad(openErr error, fingerprint string) (model, tea.Cmd) {
	m.fingerprint = fingerprint
	// The screen a reload kept up has done its job, whichever way this goes.
	prev := m.stale
	m.stale = nil
	if m.loadedNotice != "" {
		m.notice, m.loadedNotice = m.loadedNotice, ""
	}

	if openErr == nil {
		m.loadRepoInfo()
	} else {
		m.loadRepoInfoFromCLI()
	}
	m.loadUpstreamSync()

	if err := m.loadGraphData(); err != nil {
		log.Printf("Graph loading failed: %v, trying simple load...\n", err)
		commits, err2 := m.loadCommitsFromGitCLI()
		if err2 != nil {
			if openErr != nil {
				m.err = fmt.Errorf("%v (graph: %v, fallback: %v)", openErr, err, err2)
			} else {
				m.err = fmt.Errorf("graph: %v, fallback: %v", err, err2)
			}
			m.ready = true
			return m, nil
		}
		m.commits = commits
	}
	m.ready = true
	m.selected = 0
	m.addWorkingTreeRow()
	m.addMoreCommitsRow()
	m.applyReselect()
	m.noteEmptyFilter()
	m.rememberCurrentRepo()
	m.followSelectionInCommitView()
	if cmd := m.keepLoadedDiffs(prev); cmd != nil {
		return m, cmd
	}
	return m, m.maybeLoadDiff()
}
