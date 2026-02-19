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

	"github.com/charmbracelet/bubbles/viewport"
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
	Author    string
	Date      time.Time
	Message   string
	Parents   []string
	GraphLine string
}

type model struct {
	repo     *git.Repository
	commits  []commit
	viewport viewport.Model
	ready    bool
	repoPath string
	err      error
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
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}

		// Only handle navigation keys if viewport is ready
		if m.ready {
			switch msg.String() {
			case "j", "down":
				m.viewport.ScrollDown(1)
				return m, nil
			case "k", "up":
				m.viewport.ScrollUp(1)
				return m, nil
			case "d", "ctrl+d":
				m.viewport.HalfPageDown()
				return m, nil
			case "u", "ctrl+u":
				m.viewport.HalfPageUp()
				return m, nil
			case "g", "home":
				m.viewport.GotoTop()
				return m, nil
			case "G", "end":
				m.viewport.GotoBottom()
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		headerHeight := 3
		footerHeight := 2
		verticalMarginHeight := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = headerHeight
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}

		if m.repo != nil {
			m.viewport.SetContent(m.renderCommits())
		}

	case repoMsg:
		m.repo = msg.repo
		log.Println("Repository opened successfully with go-git")
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
			m.viewport.SetContent(m.renderCommits())
			m.ready = true
			return m, nil
		}
		m.commits = commits
		m.viewport.SetContent(m.renderCommits())
		return m, nil

	case errMsg:
		log.Printf("Error from go-git: %v\n", msg.err)
		// Always try CLI fallback when go-git fails
		log.Println("go-git failed, trying git CLI fallback...")
		commits, err := m.loadCommitsFromGitCLI()
		if err != nil {
			log.Printf("CLI fallback also failed: %v\n", err)
			m.err = fmt.Errorf("%v (CLI fallback: %v)", msg.err, err)
			m.ready = true
			return m, nil
		}
		log.Println("CLI fallback succeeded!")
		m.commits = commits
		m.viewport.SetContent(m.renderCommits())
		m.ready = true
		return m, nil
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
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

		commit := commit{
			Hash:    c.Hash.String()[:7],
			Author:  c.Author.Name,
			Date:    c.Author.When,
			Message: strings.Split(c.Message, "\n")[0],
			Parents: parents,
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

		hash := parts[0]
		if len(hash) > 7 {
			hash = hash[:7]
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
			Hash:    hash,
			Author:  author,
			Date:    date,
			Message: message,
			Parents: parents,
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

func (m *model) renderCommits() string {
	if len(m.commits) == 0 {
		return "No commits found"
	}

	var sb strings.Builder

	for _, c := range m.commits {
		// Graph line
		graphStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500"))
		sb.WriteString(graphStyle.Render(c.GraphLine))
		sb.WriteString(" ")

		// Commit hash
		sb.WriteString(commitHashStyle.Render(c.Hash))
		sb.WriteString(" ")

		// Author
		sb.WriteString(authorStyle.Render(fmt.Sprintf("<%s>", c.Author)))
		sb.WriteString(" ")

		// Date
		dateStr := c.Date.Format("2006-01-02 15:04")
		sb.WriteString(dateStyle.Render(dateStr))
		sb.WriteString(" ")

		// Message
		sb.WriteString(messageStyle.Render(c.Message))
		sb.WriteString("\n")
	}

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
	help := helpStyle.Render("\n  ↑/↓/j/k: scroll • d/u: half page • g/G: top/bottom • q/esc: quit")

	return fmt.Sprintf("%s\n%s%s", title, m.viewport.View(), help)
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
