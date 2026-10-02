package tui

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Init() tea.Cmd {
	return tea.Batch(
		loadRepo(m.repoPath),
		checkVersionCmd(),
		loadRemoteTagsCmd(m.repoPath),
	)
}

// Update handles a message, then records where the graph's and the file list's
// windows ended up. The selection is moved from a dozen places — keys, the
// mouse, search, the branch jumper, a reload finding its commit again — and
// recording the window here, once, is what keeps it from drifting after any
// of them.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	nm, ok := next.(model)
	if !ok {
		return next, cmd
	}
	nm.graphTop, _ = nm.graphWindow()
	if nm.commitView.open {
		nm.commitView.filesTop = nm.fileTop(nm.fileRows())
	}
	return nm, cmd
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height

	case repoMsg:
		m.repo = msg.repo
		if m.repo != nil {
			log.Println("Repository opened successfully with go-git")
		}
		return m.finishLoad(nil)

	case errMsg:
		log.Printf("Error from go-git: %v\n", msg.err)
		return m.finishLoad(msg.err)

	case diffLoadedMsg:
		// A diff requested before switching repositories would otherwise land
		// on whichever commit of the new one has the same index.
		if msg.repoPath != m.repoPath {
			return m, nil
		}
		if msg.commitIdx >= 0 && msg.commitIdx < len(m.commits) {
			m.commits[msg.commitIdx].DiffLoaded = true
			m.commits[msg.commitIdx].DiffStat = msg.diffStat
			m.commits[msg.commitIdx].DiffBody = msg.diffBody
			m.commits[msg.commitIdx].DiffFiles = msg.diffFiles
		}
		return m, nil

	case fetchFinishedMsg:
		// A fetch of the repository you have since left says nothing about the
		// one on screen, and must not reload it.
		if msg.repoPath != m.repoPath {
			return m, nil
		}
		return m.finishFetch(msg)

	case versionCheckMsg:
		m.latestVersion = msg.latestVersion
		return m, nil

	case remoteTagsMsg:
		// Asking the remotes takes seconds, so the answer for a repository
		// switched away from can still arrive; its tags would mark the new one's.
		if msg.repoPath != m.repoPath {
			return m, nil
		}
		// May arrive before or after the graph loads; loadGraphData applies it
		// in the other order.
		m.remoteTags = msg.tags
		m.applyRemoteTags()
		m.updateLabelWidth()
		return m, nil

	case updateFinishedMsg:
		if msg.err != nil {
			log.Printf("Update failed: %v\n", msg.err)
			m.updateState = updateIdle
			m.updateMessage = "Update failed: " + msg.err.Error()
			return m, nil
		}
		// Quitting is part of installing, not just politeness: on Windows the
		// helper can't replace the binary until this process releases it.
		log.Printf("Update downloaded: %s\n", msg.version)
		m.updateState = updateDone
		m.updatedTo = msg.version
		return m, tea.Quit
	}

	return m, nil
}
