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
	focusedBox    int // 0 = repo info, 1 = commit list, 2 = commit details
	detailsScroll int // scroll offset for the details panel
}

func initialModel(repoPath string) model {
	return model{
		repoPath:   repoPath,
		focusedBox: 1, // default focus on commit list
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
		case "0":
			m.focusedBox = 0
			return m, nil
		case "1":
			m.focusedBox = 1
			return m, nil
		case "2":
			m.focusedBox = 2
			return m, nil
		}

		// Handle scrolling within the focused box
		if m.ready && len(m.commits) > 0 {
			switch m.focusedBox {
			case 1: // commit list
				switch msg.String() {
				case "j", "down":
					if m.selected < len(m.commits)-1 {
						m.selected++
						m.detailsScroll = 0
					}
					return m, nil
				case "k", "up":
					if m.selected > 0 {
						m.selected--
						m.detailsScroll = 0
					}
					return m, nil
				case "d", "ctrl+d":
					m.selected += 10
					if m.selected >= len(m.commits) {
						m.selected = len(m.commits) - 1
					}
					m.detailsScroll = 0
					return m, nil
				case "u", "ctrl+u":
					m.selected -= 10
					if m.selected < 0 {
						m.selected = 0
					}
					m.detailsScroll = 0
					return m, nil
				case "g", "home":
					m.selected = 0
					m.detailsScroll = 0
					return m, nil
				case "G", "end":
					m.selected = len(m.commits) - 1
					m.detailsScroll = 0
					return m, nil
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

	leftContent := sb.String()

	// Title on the right
	title := titleStyle.Render("🦒 Gitraffe - Git Graph Viewer")

	// Calculate available width for content (subtract borders and padding)
	availableWidth := m.windowWidth - 2 - 2 // borders (2) + padding (2)
	leftWidth := lipgloss.Width(leftContent)
	rightWidth := lipgloss.Width(title)

	// Add spacing to push title to the right
	spacing := availableWidth - leftWidth - rightWidth
	if spacing < 1 {
		spacing = 1
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, leftContent, strings.Repeat(" ", spacing), title)
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

	// Apply scroll offset
	content := sb.String()
	if m.detailsScroll > 0 {
		allLines := strings.Split(content, "\n")
		if m.detailsScroll >= len(allLines) {
			m.detailsScroll = len(allLines) - 1
		}
		if m.detailsScroll < 0 {
			m.detailsScroll = 0
		}
		content = strings.Join(allLines[m.detailsScroll:], "\n")
	}

	return content
}

// addBoxLabel overlays a label like [0] onto the top-left corner of a rendered box border.
// It accounts for ANSI escape sequences so it only replaces visible border characters.
func addBoxLabel(rendered string, label string) string {
	lines := strings.SplitN(rendered, "\n", 2)
	if len(lines) == 0 {
		return rendered
	}
	topLine := lines[0]
	labelRunes := []rune(label)

	// Walk through the top line, skipping ANSI escape sequences,
	// and replace visible characters at positions 1..len(label) (after the corner char).
	var result strings.Builder
	visibleIdx := 0
	labelIdx := 0
	runes := []rune(topLine)
	for i := 0; i < len(runes); i++ {
		// Detect ANSI escape sequence: ESC [ params final_byte
		if runes[i] == '\033' && i+1 < len(runes) && runes[i+1] == '[' {
			// Copy ESC and [ first
			result.WriteRune(runes[i]) // \033
			i++
			result.WriteRune(runes[i]) // [
			i++
			// Now copy parameter/intermediate bytes until final byte (0x40-0x7E)
			for i < len(runes) {
				result.WriteRune(runes[i])
				if runes[i] >= 0x40 && runes[i] <= 0x7E {
					break
				}
				i++
			}
			continue
		}
		// This is a visible character
		if visibleIdx >= 1 && labelIdx < len(labelRunes) {
			result.WriteRune(labelRunes[labelIdx])
			labelIdx++
		} else {
			result.WriteRune(runes[i])
		}
		visibleIdx++
	}

	lines[0] = result.String()
	if len(lines) > 1 {
		return lines[0] + "\n" + lines[1]
	}
	return lines[0]
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

	help := helpStyle.Render("0/1/2: focus box • ↑/↓/j/k: scroll • d/u: half page • g/G: top/bottom • q/esc: quit")

	// Border colors: orange for focused, purple for unfocused
	focusedBorderColor := lipgloss.Color("#FFA500")
	unfocusedBorderColor := lipgloss.Color("#7D56F4")
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

	// Create repo info box
	repoInfoContent := m.renderRepoInfo()
	repoInfoBox := addBoxLabel(lipgloss.NewStyle().
		Width(m.windowWidth-2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(box0Border).
		Padding(0, 1).
		Render(repoInfoContent), "[0]")

	// Calculate dimensions
	headerHeight := 5 // repo info box (3 with borders) + spacing (2)
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
	leftPanel := addBoxLabel(lipgloss.NewStyle().
		Width(leftPanelWidth-2). // subtract borders (2); Width includes padding
		Height(contentHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(box1Border).
		Padding(0, 1).
		Render(leftContent), "[1]")

	// Create right panel (commit details)
	rightContent := m.renderCommitDetails()
	rightPanel := addBoxLabel(lipgloss.NewStyle().
		Width(rightPanelWidth-2). // subtract borders (2); Width includes padding
		Height(contentHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(box2Border).
		Padding(1, 2).
		Render(rightContent), "[2]")

	// Join panels horizontally
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	return fmt.Sprintf("%s\n%s\n%s", repoInfoBox, content, help)
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
