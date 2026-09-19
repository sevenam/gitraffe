package main

import (
	"fmt"
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// The confirmation prompt owns the keyboard until it is answered, so a
		// stray "j" can't scroll the graph behind a pending yes/no.
		if m.updateState == updateConfirming {
			switch msg.String() {
			case "y", "Y", "enter":
				m.updateState = updateDownloading
				m.updateMessage = "Downloading " + m.latestVersion + "..."
				return m, performUpdateCmd()
			default:
				m.updateState = updateIdle
				m.updateMessage = ""
				return m, nil
			}
		}

		// Mid-download the binary is being swapped underneath us; only quitting
		// is meaningful, and it stays available in case the download hangs.
		if m.updateState == updateDownloading {
			switch msg.String() {
			case "q", "ctrl+c", "esc":
				return m, tea.Quit
			}
			return m, nil
		}

		// Any keystroke dismisses a lingering notice.
		m.updateMessage = ""
		m.notice = ""

		if m.picker.open {
			return m.updateThemePicker(msg)
		}

		// The help overlay covers the panels, so keys acting on them would change
		// things the user can't see. Esc and q close it rather than quit: pressed
		// while reading help they mean "back", and quitting would lose the place.
		if m.showHelp {
			switch msg.String() {
			case "?", "esc", "q":
				m.showHelp = false
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "?":
			m.showHelp = true
			return m, nil
		case "t":
			return m.openThemePicker(), nil
		case "U":
			return m.startUpdate(), nil
		case "c":
			// Global rather than per-box: the graph stays visible whichever
			// box has focus, so the colours should be reachable from both.
			m.colourLanes = !m.colourLanes
			return m, nil
		case "1":
			m.focusedBox = 1
			return m, nil
		case "2":
			m.focusedBox = 2
			return m, nil
		case "tab":
			if m.focusedBox == 1 {
				m.focusedBox = 2
			} else {
				m.focusedBox = 1
			}
			return m, nil
		case "shift+tab":
			if m.focusedBox == 2 {
				m.focusedBox = 1
			} else {
				m.focusedBox = 2
			}
			return m, nil
		}

		// Handle scrolling within the focused box
		if m.ready && len(m.commits) > 0 {
			switch m.focusedBox {
			case 1: // commit list / graph
				switch msg.String() {
				case "j", "down":
					if m.selected < len(m.commits)-1 {
						m.selected++
						m.detailsScroll = 0
					}
					return m, m.maybeLoadDiff()
				case "k", "up":
					if m.selected > 0 {
						m.selected--
						m.detailsScroll = 0
					}
					return m, m.maybeLoadDiff()
				case "d", "ctrl+d", "pgdown":
					m.selected += 10
					if m.selected >= len(m.commits) {
						m.selected = len(m.commits) - 1
					}
					m.detailsScroll = 0
					return m, m.maybeLoadDiff()
				case "u", "ctrl+u", "pgup":
					m.selected -= 10
					if m.selected < 0 {
						m.selected = 0
					}
					m.detailsScroll = 0
					return m, m.maybeLoadDiff()
				case "g", "home":
					m.selected = 0
					m.detailsScroll = 0
					return m, m.maybeLoadDiff()
				case "G", "end":
					m.selected = len(m.commits) - 1
					m.detailsScroll = 0
					return m, m.maybeLoadDiff()
				}
			case 2: // commit details
				switch msg.String() {
				case "j", "down":
					m.detailsScroll++
					return m, nil
				case "k", "up":
					if m.detailsScroll > 0 {
						m.detailsScroll--
					}
					return m, nil
				case "d", "ctrl+d", "pgdown":
					m.detailsScroll += 10
					return m, nil
				case "u", "ctrl+u", "pgup":
					m.detailsScroll -= 10
					if m.detailsScroll < 0 {
						m.detailsScroll = 0
					}
					return m, nil
				case "g", "home":
					m.detailsScroll = 0
					return m, nil
				}
			}
		}

	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height

	case repoMsg:
		m.repo = msg.repo
		if m.repo != nil {
			log.Println("Repository opened successfully with go-git")
		}
		m.loadRepoInfo()
		m.loadUpstreamSync()

		if err := m.loadGraphData(); err != nil {
			log.Printf("Graph loading failed: %v, trying simple load...\n", err)
			commits, err2 := m.loadCommitsFromGitCLI()
			if err2 != nil {
				m.err = fmt.Errorf("graph: %v, fallback: %v", err, err2)
				m.ready = true
				return m, nil
			}
			m.commits = commits
		}
		m.ready = true
		m.selected = 0
		return m, m.maybeLoadDiff()

	case errMsg:
		log.Printf("Error from go-git: %v\n", msg.err)
		m.loadRepoInfoFromCLI()
		m.loadUpstreamSync()

		if err := m.loadGraphData(); err != nil {
			log.Printf("Graph loading failed: %v, trying simple load...\n", err)
			commits, err2 := m.loadCommitsFromGitCLI()
			if err2 != nil {
				m.err = fmt.Errorf("%v (graph: %v, fallback: %v)", msg.err, err, err2)
				m.ready = true
				return m, nil
			}
			m.commits = commits
		}
		m.ready = true
		m.selected = 0
		return m, m.maybeLoadDiff()

	case diffLoadedMsg:
		if msg.commitIdx >= 0 && msg.commitIdx < len(m.commits) {
			m.commits[msg.commitIdx].DiffLoaded = true
			m.commits[msg.commitIdx].DiffStat = msg.diffStat
			m.commits[msg.commitIdx].DiffBody = msg.diffBody
		}
		return m, nil

	case versionCheckMsg:
		m.latestVersion = msg.latestVersion
		return m, nil

	case remoteTagsMsg:
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

// startUpdate arms the confirmation prompt, or explains why there is nothing to do.
func (m model) startUpdate() model {
	switch {
	case m.latestVersion == "":
		m.updateMessage = "Update check unavailable — could not reach GitHub"
	case !m.updateAvailable():
		m.updateMessage = "Already on the latest version (v" + version + ")"
	default:
		m.updateState = updateConfirming
	}
	return m
}
