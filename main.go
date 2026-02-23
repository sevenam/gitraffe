package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
)

const (
	appName     = "Gitraffe"
	version     = "0.2.3"
	logFileName = "gitraffe.log"
)

var (
	// Styles — initialized by initStyles() after theme is loaded.
	titleStyle      lipgloss.Style
	commitHashStyle lipgloss.Style
	authorStyle     lipgloss.Style
	dateStyle       lipgloss.Style
	messageStyle    lipgloss.Style
	branchStyle     lipgloss.Style
	helpStyle       lipgloss.Style
)

type model struct {
	repo                *git.Repository
	commits             []commit
	ready               bool
	repoPath            string
	err                 error
	selected            int
	windowHeight        int
	windowWidth         int
	repoName            string
	currentBranch       string
	currentCommit       string
	focusedBox          int // 0 = repo info, 1 = commit list, 2 = commit details
	detailsScroll       int // scroll offset for the details panel
	displayRows         []displayRow
	maxGraphWidth       int
	maxBranchWidth      int
	detailsContentWidth int
	latestVersion       string // latest version from GitHub, e.g., "v0.2.0"
}

func initialModel(repoPath string) model {
	return model{
		repoPath:   repoPath,
		focusedBox: 1, // default focus on commit list
	}
}

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

func (m model) View() (result string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in View: %v", r)
			result = fmt.Sprintf("\n  PANIC caught: %v\n\n  Check %s for details.\n  Press q to quit.", r, logFileName)
		}
	}()
	log.Printf("View: ready=%v, err=%v, commits=%d, displayRows=%d, window=%dx%d, focused=%d",
		m.ready, m.err, len(m.commits), len(m.displayRows), m.windowWidth, m.windowHeight, m.focusedBox)

	if !m.ready {
		return "\n  Initializing..."
	}

	// Guard against zero window dimensions (WindowSizeMsg not yet received)
	if m.windowWidth < 20 || m.windowHeight < 10 {
		log.Printf("View: window too small (%dx%d), waiting for resize", m.windowWidth, m.windowHeight)
		return "\n  Waiting for terminal size..."
	}

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(currentTheme.Error)).
			Bold(true)
		return fmt.Sprintf("\n  %s\n\n  Error: %v\n\n  Press q to quit. Check %s for details.\n",
			errorStyle.Render("❌ Error loading repository"),
			m.err, logFileName)
	}

	help := helpStyle.Render("1/2: focus box • tab/shift+tab: cycle boxes • ↑/↓/j/k: scroll • d/u: half page • g/G: top/bottom • q/esc: quit")

	// Border colors: active for focused, inactive for unfocused
	focusedBorderColor := lipgloss.Color(currentTheme.BorderActive)
	unfocusedBorderColor := lipgloss.Color(currentTheme.BorderInactive)
	box0Border := unfocusedBorderColor
	box1Border := unfocusedBorderColor
	box2Border := unfocusedBorderColor
	switch m.focusedBox {
	case 0:
		box0Border = focusedBorderColor
	case 1:
		box1Border = focusedBorderColor
	case 2:
		box2Border = focusedBorderColor
	}

	// Create repo info box - fixed Height(1) so it never changes size
	repoInfoContent := m.renderRepoInfo()
	repoInfoBox := addBoxLabel(lipgloss.NewStyle().
		Width(m.windowWidth-2).
		Height(1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(box0Border).
		Padding(0, 1).
		Render(repoInfoContent), "")

	// Calculate dimensions based on actual rendered box 0 height
	repoInfoHeight := lipgloss.Height(repoInfoBox) // should be 3 (1 content + 2 border)
	// Layout: repoInfoBox + \n + content panels (contentHeight + 2 border) + \n + help
	// Total = repoInfoHeight + 1 + contentHeight + 2 + 1 + 1 = repoInfoHeight + contentHeight + 5
	contentHeight := m.windowHeight - repoInfoHeight - 3

	if contentHeight < 3 {
		contentHeight = 3
	}

	// Panel widths - dynamic based on graph width
	// graph needs: 2 (selection "> ") + maxGraphWidth + 1 (space) + 7 (hash) + borders(2) + padding(2) = maxGraphWidth + 14
	leftPanelWidth := m.maxGraphWidth + 14
	if m.maxBranchWidth > 0 {
		leftPanelWidth += m.maxBranchWidth + 1
	}
	if leftPanelWidth < 25 {
		leftPanelWidth = 25
	}
	maxLeftWidth := m.windowWidth * 3 / 5
	if leftPanelWidth > maxLeftWidth {
		leftPanelWidth = maxLeftWidth
	}
	rightPanelWidth := m.windowWidth - leftPanelWidth // fill remaining space

	// Ensure right panel has a minimum width, but never let total exceed window
	minRightWidth := 30
	if rightPanelWidth < minRightWidth {
		rightPanelWidth = minRightWidth
		leftPanelWidth = m.windowWidth - rightPanelWidth
		if leftPanelWidth < 15 {
			leftPanelWidth = 15
			rightPanelWidth = m.windowWidth - leftPanelWidth
		}
	}

	// Final safety: total must not exceed window width
	totalWidth := leftPanelWidth + rightPanelWidth
	if totalWidth > m.windowWidth {
		log.Printf("View: width overflow detected: left=%d + right=%d = %d > window=%d, adjusting",
			leftPanelWidth, rightPanelWidth, totalWidth, m.windowWidth)
		rightPanelWidth = m.windowWidth - leftPanelWidth
		if rightPanelWidth < 10 {
			rightPanelWidth = m.windowWidth / 3
			leftPanelWidth = m.windowWidth - rightPanelWidth
		}
	}

	log.Printf("View: leftPanelWidth=%d, rightPanelWidth=%d, contentHeight=%d", leftPanelWidth, rightPanelWidth, contentHeight)

	// Target height for both panels (content + 2 border lines)
	targetPanelHeight := contentHeight + 2

	// Create left panel (commit list)
	leftContent := m.renderCommitList()
	leftPanel := addBoxLabel(lipgloss.NewStyle().
		Width(leftPanelWidth-2). // subtract borders (2); Width includes padding
		Height(contentHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(box1Border).
		Padding(0, 1).
		Render(leftContent), "[1]-git-graph")

	// Create right panel (commit details)
	// Padding(1,2) → 2*2=4 horizontal padding + 2 borders = 6 overhead
	m.detailsContentWidth = rightPanelWidth - 6
	rightContent := m.renderCommitDetails()
	rightPanel := addBoxLabel(lipgloss.NewStyle().
		Width(rightPanelWidth-2). // subtract borders (2); Width includes padding
		Height(contentHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(box2Border).
		Padding(1, 2).
		Render(rightContent), "[2]-commit-details")

	// Force both panels to exactly the same height.
	// lipgloss Height() is a minimum, not a maximum — long lines that wrap
	// inside the panel can make it taller. Trim any excess lines from either panel.
	leftPanel = trimToHeight(leftPanel, targetPanelHeight)
	rightPanel = trimToHeight(rightPanel, targetPanelHeight)

	// Join panels horizontally
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	output := fmt.Sprintf("%s\n%s\n%s", repoInfoBox, content, help)

	// Force exact windowHeight lines. We count lines via lipgloss.Height which
	// correctly handles ANSI escape sequences, then trim or pad as needed.
	actualHeight := lipgloss.Height(output)
	log.Printf("View: actualHeight=%d, windowHeight=%d", actualHeight, m.windowHeight)

	if actualHeight > m.windowHeight {
		// Trim from the bottom
		lines := strings.Split(output, "\n")
		output = strings.Join(lines[:m.windowHeight], "\n")
	} else if actualHeight < m.windowHeight {
		// Pad bottom with empty lines
		for i := actualHeight; i < m.windowHeight; i++ {
			output += "\n"
		}
	}

	return output
}

func main() {
	// Set up logging to file for debugging
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	log.Println("Starting " + appName + "...")

	// Handle update subcommand
	if len(os.Args) > 1 && os.Args[1] == "update" {
		if err := checkUpdate(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	loadTheme()
	initStyles()

	repoPath := "."
	if len(os.Args) > 1 {
		repoPath = os.Args[1]
	}

	log.Printf("Opening repository: %s\n", repoPath)

	// Set terminal title (works on Windows 10+, macOS, Linux)
	setTerminalTitle(appName)
	defer resetTerminalTitle()

	p := tea.NewProgram(
		initialModel(repoPath),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		log.Printf("Program error: %v\n", err)
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	log.Println(appName + " exited normally")
}
