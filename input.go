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
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
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
				case "d", "ctrl+d":
					m.selected += 10
					if m.selected >= len(m.commits) {
						m.selected = len(m.commits) - 1
					}
					m.detailsScroll = 0
					return m, m.maybeLoadDiff()
				case "u", "ctrl+u":
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
				case "d", "ctrl+d":
					m.detailsScroll += 10
					return m, nil
				case "u", "ctrl+u":
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
	}

	return m, nil
}
