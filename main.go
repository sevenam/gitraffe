package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var (
	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			Background(lipgloss.Color("#1C1C1C")).
			Padding(0, 1)

	commitHashStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFA500")).
			Bold(true)

	authorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7DD3FC"))

	dateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A3BE8C"))

	messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E5E9F0"))

	branchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#88C0D0")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))
)

type commit struct {
	Hash      string
	FullHash  string
	Author    string
	Date      time.Time
	Message   string
	Parents   []string
	GraphLine string
}

type model struct {
	repo          *git.Repository
	commits       []commit
	ready         bool
	repoPath      string
	err           error
	selected      int
	windowHeight  int
	windowWidth   int
	repoName      string
	currentBranch string
	currentCommit string
}

func initialModel(repoPath string) model {
	return model{
		repoPath: repoPath,
	}
}

func (m model) Init() tea.Cmd {
	return loadRepo(m.repoPath)
}

func loadRepo(path string) tea.Cmd {
	return func() tea.Msg {
		repo, err := git.PlainOpen(path)
		if err != nil {
			return errMsg{err}
		}
		return repoMsg{repo}
	}
}

type repoMsg struct {
	repo *git.Repository
}

type errMsg struct {
	err error
}

func (e errMsg) Error() string {
	return e.err.Error()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}

		// Only handle navigation keys if ready and have commits
		if m.ready && len(m.commits) > 0 {
			switch msg.String() {
			case "j", "down":
				if m.selected < len(m.commits)-1 {
					m.selected++
				}
				return m, nil
			case "k", "up":
				if m.selected > 0 {
					m.selected--
				}
				return m, nil
			case "d", "ctrl+d":
				m.selected += 10
				if m.selected >= len(m.commits) {
					m.selected = len(m.commits) - 1
				}
				return m, nil
			case "u", "ctrl+u":
				m.selected -= 10
				if m.selected < 0 {
					m.selected = 0
				}
				return m, nil
			case "g", "home":
				m.selected = 0
				return m, nil
			case "G", "end":
				m.selected = len(m.commits) - 1
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height

	case repoMsg:
		m.repo = msg.repo
		log.Println("Repository opened successfully with go-git")

		// Load repository info
		m.loadRepoInfo()

		commits, err := m.loadCommits()
		if err != nil {
			log.Printf("Failed to load commits with go-git: %v\n", err)
			// Try CLI fallback
			log.Println("Trying CLI fallback for commit loading...")
			commits, cliErr := m.loadCommitsFromGitCLI()
			if cliErr != nil {
				log.Printf("CLI fallback also failed: %v\n", cliErr)
				m.err = fmt.Errorf("%v (CLI fallback: %v)", err, cliErr)
				m.ready = true
				return m, nil
			}
			log.Println("CLI fallback succeeded!")
			m.commits = commits
			m.ready = true
			m.selected = 0
			return m, nil
		}
		m.commits = commits
		m.ready = true
		m.selected = 0
		return m, nil

	case errMsg:
		log.Printf("Error from go-git: %v\n", msg.err)
		// Always try CLI fallback when go-git fails
		log.Println("go-git failed, trying git CLI fallback...")
		m.loadRepoInfoFromCLI()
		commits, err := m.loadCommitsFromGitCLI()
		if err != nil {
			log.Printf("CLI fallback also failed: %v\n", err)
			m.err = fmt.Errorf("%v (CLI fallback: %v)", msg.err, err)
			m.ready = true
			return m, nil
		}
		log.Println("CLI fallback succeeded!")
		m.commits = commits
		m.ready = true
		m.selected = 0
		return m, nil
	}

	return m, nil
}

func (m *model) loadRepoInfo() {
	// Get repository name from path
	m.repoName = m.repoPath
	if m.repoPath == "." {
		if wd, err := os.Getwd(); err == nil {
			m.repoName = wd[strings.LastIndex(wd, string(os.PathSeparator))+1:]
		}
	} else {
		m.repoName = m.repoPath[strings.LastIndex(m.repoPath, string(os.PathSeparator))+1:]
	}

	// Get current branch and commit
	if m.repo != nil {
		if ref, err := m.repo.Head(); err == nil {
			// Get branch name
			if ref.Name().IsBranch() {
				m.currentBranch = ref.Name().Short()
			} else {
				m.currentBranch = "HEAD (detached)"
			}
			// Get commit hash
			m.currentCommit = ref.Hash().String()[:7]
		}
	} else {
		// Use CLI to get branch and commit info
		m.loadRepoInfoFromCLI()
	}
}

func (m *model) loadRepoInfoFromCLI() {
	// Get repository name from path
	m.repoName = m.repoPath
	if m.repoPath == "." {
		if wd, err := os.Getwd(); err == nil {
			m.repoName = wd[strings.LastIndex(wd, string(os.PathSeparator))+1:]
		}
	} else {
		m.repoName = m.repoPath[strings.LastIndex(m.repoPath, string(os.PathSeparator))+1:]
	}

	// Get current branch
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = m.repoPath
	if out, err := cmd.Output(); err == nil {
		m.currentBranch = strings.TrimSpace(string(out))
	} else {
		m.currentBranch = "unknown"
	}

	// Get current commit
	cmd = exec.Command("git", "rev-parse", "--short=7", "HEAD")
	cmd.Dir = m.repoPath
	if out, err := cmd.Output(); err == nil {
		m.currentCommit = strings.TrimSpace(string(out))
	} else {
		m.currentCommit = "unknown"
	}
}

func (m *model) loadCommits() ([]commit, error) {
	const maxCommits = 5000 // Limit for large repos

	log.Println("Loading commits...")
	ref, err := m.repo.Head()
	if err != nil {
		log.Printf("Error getting HEAD: %v\n", err)
		return nil, err
	}

	commitIter, err := m.repo.Log(&git.LogOptions{
		From: ref.Hash(),
	})
	if err != nil {
		log.Printf("Error getting commit log: %v\n", err)
		return nil, err
	}

	var commits []commit
	commitMap := make(map[string]*commit)
	count := 0

	err = commitIter.ForEach(func(c *object.Commit) error {
		count++
		if count > maxCommits {
			log.Printf("Reached maximum commit limit (%d), stopping...\n", maxCommits)
			return fmt.Errorf("stopped at %d commits", maxCommits)
		}

		parents := make([]string, len(c.ParentHashes))
		for i, p := range c.ParentHashes {
			parents[i] = p.String()[:7]
		}

		fullHash := c.Hash.String()
		commit := commit{
			Hash:     fullHash[:7],
			FullHash: fullHash,
			Author:   c.Author.Name,
			Date:     c.Author.When,
			Message:  strings.Split(c.Message, "\n")[0],
			Parents:  parents,
		}
		commits = append(commits, commit)
		commitMap[commit.Hash] = &commits[len(commits)-1]

		if count%1000 == 0 {
			log.Printf("Loaded %d commits...\n", count)
		}

		return nil
	})

	// Don't treat maxCommits error as fatal
	if err != nil && count < maxCommits {
		log.Printf("Error iterating commits: %v\n", err)
		// Check if it's a packfile error - if so, try CLI fallback
		if strings.Contains(err.Error(), "packfile") || strings.Contains(err.Error(), "object not found") {
			log.Println("Detected packfile error, falling back to git CLI...")
			return m.loadCommitsFromGitCLI()
		}
		return nil, err
	}

	log.Printf("Successfully loaded %d commits\n", len(commits))

	// Generate graph lines
	m.generateGraph(commits)

	return commits, nil
}

func (m *model) loadCommitsFromGitCLI() ([]commit, error) {
	const maxCommits = 5000

	log.Println("Using git CLI to load commits...")

	// Use git log with a custom format
	cmd := exec.Command("git", "log",
		fmt.Sprintf("-n%d", maxCommits),
		"--pretty=format:%H|%an|%at|%s|%P",
		"--all")
	cmd.Dir = m.repoPath

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		log.Printf("Git CLI error: %v, stderr: %s\n", err, errOut.String())
		return nil, fmt.Errorf("git command failed: %v", err)
	}

	lines := strings.Split(out.String(), "\n")
	commits := make([]commit, 0, len(lines))

	for i, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		fullHash := parts[0]
		shortHash := fullHash
		if len(shortHash) > 7 {
			shortHash = shortHash[:7]
		}

		author := parts[1]

		timestamp := parts[2]
		var date time.Time
		if ts, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
			date = time.Unix(ts, 0)
		} else {
			log.Printf("Warning: failed to parse timestamp '%s': %v\n", timestamp, err)
			date = time.Now()
		}

		message := parts[3]

		var parents []string
		if len(parts) > 4 && parts[4] != "" {
			parentHashes := strings.Fields(parts[4])
			parents = make([]string, len(parentHashes))
			for j, p := range parentHashes {
				if len(p) > 7 {
					parents[j] = p[:7]
				} else {
					parents[j] = p
				}
			}
		}

		commits = append(commits, commit{
			Hash:     shortHash,
			FullHash: fullHash,
			Author:   author,
			Date:     date,
			Message:  message,
			Parents:  parents,
		})

		if (i+1)%1000 == 0 {
			log.Printf("Loaded %d commits from git CLI...\n", i+1)
		}
	}

	log.Printf("Successfully loaded %d commits from git CLI\n", len(commits))

	// Generate graph lines
	m.generateGraph(commits)

	return commits, nil
}

func (m *model) generateGraph(commits []commit) {
	// Enhanced graph generation with better visual representation
	for i := range commits {
		if len(commits[i].Parents) == 0 {
			// Initial commit
			commits[i].GraphLine = "◉ "
		} else if len(commits[i].Parents) == 1 {
			// Regular commit
			commits[i].GraphLine = "● "
		} else {
			// Merge commit - multiple parents
			commits[i].GraphLine = "◆ "
		}
	}
}

func (m *model) renderRepoInfo() string {
	var sb strings.Builder

	// Repository name
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render("Repository: "))
	sb.WriteString(m.repoName)
	sb.WriteString("  ")

	// Branch
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#88C0D0")).Render("Branch: "))
	sb.WriteString(branchStyle.Render(m.currentBranch))
	sb.WriteString("  ")

	// Current commit
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFA500")).Render("Commit: "))
	sb.WriteString(commitHashStyle.Render(m.currentCommit))

	return sb.String()
}

func (m *model) renderCommitList() string {
	if len(m.commits) == 0 {
		return "No commits found"
	}

	var sb strings.Builder

	// Calculate visible range based on window height
	headerHeight := 6 // title (1) + repo info box (3 with borders) + spacing (2)
	footerHeight := 2 // help + spacing
	visibleHeight := m.windowHeight - headerHeight - footerHeight

	if visibleHeight < 1 {
		visibleHeight = 1
	}

	// Calculate scroll offset to keep selected item visible
	startIdx := 0
	if m.selected >= visibleHeight {
		startIdx = m.selected - visibleHeight + 1
	}
	endIdx := startIdx + visibleHeight
	if endIdx > len(m.commits) {
		endIdx = len(m.commits)
	}

	for i := startIdx; i < endIdx; i++ {
		c := m.commits[i]

		// Selection indicator
		if i == m.selected {
			sb.WriteString("> ")
		} else {
			sb.WriteString("  ")
		}

		// Graph line
		graphStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500"))
		sb.WriteString(graphStyle.Render(c.GraphLine))
		sb.WriteString(" ")

		// Commit hash
		if i == m.selected {
			sb.WriteString(commitHashStyle.Copy().Background(lipgloss.Color("#3C3C3C")).Render(c.Hash))
		} else {
			sb.WriteString(commitHashStyle.Render(c.Hash))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m *model) renderCommitDetails() string {
	if len(m.commits) == 0 || m.selected >= len(m.commits) {
		return ""
	}

	c := m.commits[m.selected]

	var sb strings.Builder

	// Commit hash
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFA500")).Render("Commit: "))
	sb.WriteString(commitHashStyle.Render(c.FullHash))
	sb.WriteString("\n\n")

	// Date
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A3BE8C")).Render("Date: "))
	dateStr := c.Date.Format("2006-01-02 15:04:05")
	sb.WriteString(dateStyle.Render(dateStr))
	sb.WriteString("\n\n")

	// Author
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7DD3FC")).Render("Author: "))
	sb.WriteString(authorStyle.Render(c.Author))
	sb.WriteString("\n\n")

	// Parents
	if len(c.Parents) > 0 {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("Parents: "))
		sb.WriteString(strings.Join(c.Parents, ", "))
		sb.WriteString("\n\n")
	}

	// Message
	sb.WriteString(lipgloss.NewStyle().Bold(true).Render("Message: "))
	sb.WriteString(messageStyle.Render(c.Message))
	sb.WriteString("\n")

	return sb.String()
}

func (m model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true)
		return fmt.Sprintf("\n  %s\n\n  Error: %v\n\n  Press q to quit. Check gitraffe.log for details.\n",
			errorStyle.Render("❌ Error loading repository"),
			m.err)
	}

	title := titleStyle.Render("🦒 Gitraffe - Git Graph Viewer")
	help := helpStyle.Render("↑/↓/j/k: scroll • d/u: half page • g/G: top/bottom • q/esc: quit")

	// Create repo info box
	repoInfoContent := m.renderRepoInfo()
	repoInfoBox := lipgloss.NewStyle().
		Width(m.windowWidth-2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Render(repoInfoContent)

	// Calculate dimensions
	headerHeight := 6 // title (1) + repo info box (3 with borders) + spacing (2)
	footerHeight := 2 // help + spacing
	contentHeight := m.windowHeight - headerHeight - footerHeight

	if contentHeight < 5 {
		contentHeight = 5
	}

	// Panel widths - let lipgloss handle borders and padding
	leftPanelWidth := 19                              // total width including borders and padding
	rightPanelWidth := m.windowWidth - leftPanelWidth // fill remaining space

	if rightPanelWidth < 30 {
		rightPanelWidth = 30
	}

	// Create left panel (commit list)
	leftContent := m.renderCommitList()
	leftPanel := lipgloss.NewStyle().
		Width(leftPanelWidth-4). // subtract borders (2) and padding (2)
		Height(contentHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Render(leftContent)

	// Create right panel (commit details)
	rightContent := m.renderCommitDetails()
	rightPanel := lipgloss.NewStyle().
		Width(rightPanelWidth-6). // subtract borders (2) and padding (4)
		Height(contentHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(1, 2).
		Render(rightContent)

	// Join panels horizontally
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	return fmt.Sprintf("%s\n%s\n%s\n%s", title, repoInfoBox, content, help)
}

func main() {
	// Set up logging to file for debugging
	logFile, err := os.OpenFile("gitraffe.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	log.Println("Starting Gitraffe...")

	repoPath := "."
	if len(os.Args) > 1 {
		repoPath = os.Args[1]
	}

	log.Printf("Opening repository: %s\n", repoPath)

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

	log.Println("Gitraffe exited normally")
}
