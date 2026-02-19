package main

import (
	"fmt"
	"log"
	"os"
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
		case "j", "down":
			m.viewport.LineDown(1)
		case "k", "up":
			m.viewport.LineUp(1)
		case "d", "ctrl+d":
			m.viewport.HalfViewDown()
		case "u", "ctrl+u":
			m.viewport.HalfViewUp()
		case "g", "home":
			m.viewport.GotoTop()
		case "G", "end":
			m.viewport.GotoBottom()
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
		commits, err := m.loadCommits()
		if err != nil {
			return m, tea.Quit
		}
		m.commits = commits
		m.viewport.SetContent(m.renderCommits())
		return m, nil

	case errMsg:
		log.Fatal(msg.err)
		return m, tea.Quit
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) loadCommits() ([]commit, error) {
	ref, err := m.repo.Head()
	if err != nil {
		return nil, err
	}

	commitIter, err := m.repo.Log(&git.LogOptions{
		From: ref.Hash(),
	})
	if err != nil {
		return nil, err
	}

	var commits []commit
	commitMap := make(map[string]*commit)
	
	err = commitIter.ForEach(func(c *object.Commit) error {
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
		return nil
	})

	if err != nil {
		return nil, err
	}

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

	title := titleStyle.Render("🦒 Gitraffe - Git Graph Viewer")
	help := helpStyle.Render("\n  ↑/↓/j/k: scroll • d/u: half page • g/G: top/bottom • q/esc: quit")
	
	return fmt.Sprintf("%s\n%s%s", title, m.viewport.View(), help)
}

func main() {
	repoPath := "."
	if len(os.Args) > 1 {
		repoPath = os.Args[1]
	}

	p := tea.NewProgram(
		initialModel(repoPath),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
