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
		m.autoRefreshTick(),
		m.autoFetchTick(),
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
	// Not while loading: there are no rows yet, so both windows would be
	// recorded as back at the top, and a reload would lose the scroll position
	// it carried over to come back to.
	if !nm.ready {
		return nm, cmd
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
		return m.finishLoad(nil, msg.fingerprint)

	case errMsg:
		log.Printf("Error from go-git: %v\n", msg.err)
		return m.finishLoad(msg.err, msg.fingerprint)

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
			// The commit view opened before its files were known; now they
			// are, it can open on the one the filter is about.
			if v := &m.commitView; v.open && v.commit == msg.commitIdx && v.file == 0 {
				v.file = max(0, m.filteredFile(msg.diffFiles))
			}
		}
		return m, nil

	case fetchFinishedMsg:
		// A fetch of the repository you have since left says nothing about the
		// one on screen, and must not reload it.
		// The timer's next tick is asked for either way; see auto_refresh.go.
		var tick tea.Cmd
		if msg.auto {
			tick = m.autoFetchTick()
		}
		if msg.repoPath != m.repoPath {
			return m, tick
		}
		next, cmd := m.finishFetch(msg)
		return next, tea.Batch(cmd, tick)

	case filesLoadedMsg:
		// Dropped once the list has closed, or for a repository since left.
		if msg.repoPath == m.repoPath && m.files.open {
			m.files.setFiles(msg.files, msg.err)
		}
		return m, nil

	case copyDiffMsg:
		// A diff read before a switch is of a commit no longer on screen,
		// and copying it now would surprise whoever pastes it.
		if msg.repoPath != m.repoPath {
			return m, nil
		}
		return m.finishCopyDiff(msg), nil

	case switchFinishedMsg:
		// A switch is answered by the repository it ran in; the one on
		// screen now has its own HEAD, which this says nothing about.
		if msg.repoPath != m.repoPath {
			return m, nil
		}
		return m.finishSwitch(msg)

	case deleteFinishedMsg:
		// As with a switch: the answer is about the repository it ran in.
		if msg.repoPath != m.repoPath {
			return m, nil
		}
		return m.finishDelete(msg)

	case pullFinishedMsg:
		// As for a fetch: a pull of the repository you have since left says
		// nothing about the one on screen.
		if msg.repoPath != m.repoPath {
			return m, nil
		}
		return m.finishPull(msg)

	case autoRefreshTickMsg:
		return m.onAutoRefreshTick()

	case autoFetchTickMsg:
		return m.onAutoFetchTick()

	case tea.FocusMsg:
		return m.onFocus()

	case repoStateMsg:
		return m.onRepoState(msg)

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
