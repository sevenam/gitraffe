package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/go-git/go-git/v5"
)

const (
	appName     = "Gitraffe"
	version     = "0.2.4"
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

type commit struct {
	Hash       string
	FullHash   string
	Author     string
	Date       time.Time
	Message    string
	Parents    []string
	Refs       string
	GraphLine  string
	DiffLoaded bool
	DiffStat   string
	DiffBody   string
}

type displayRow struct {
	GraphChars string // transliterated Unicode graph characters
	CommitIdx  int    // index into commits slice, -1 for graph-only lines
	GraphWidth int    // visual width of the graph portion
}

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

func checkVersionCmd() tea.Cmd {
	return func() tea.Msg {
		latestVersion := fetchLatestVersion()
		return versionCheckMsg{latestVersion: latestVersion}
	}
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

type versionCheckMsg struct {
	latestVersion string
}

type diffLoadedMsg struct {
	commitIdx int
	diffStat  string
	diffBody  string
}

func loadDiffCmd(repoPath string, fullHash string, idx int, statWidth int) tea.Cmd {
	return func() tea.Msg {
		var stat, body string

		cmd := exec.Command("git", "show", "--format=", fmt.Sprintf("--stat=%d", statWidth), "--no-color", fullHash)
		cmd.Dir = repoPath
		if out, err := cmd.Output(); err == nil {
			stat = strings.TrimSpace(strings.ReplaceAll(string(out), "\r", ""))
		}

		cmd = exec.Command("git", "show", "--format=", "--no-color", "-p", fullHash)
		cmd.Dir = repoPath
		if out, err := cmd.Output(); err == nil {
			diff := strings.ReplaceAll(string(out), "\r", "")
			diffLines := strings.Split(diff, "\n")
			if len(diffLines) > 300 {
				diffLines = diffLines[:300]
				diffLines = append(diffLines, "... (truncated)")
			}
			body = strings.TrimSpace(strings.Join(diffLines, "\n"))
		}

		return diffLoadedMsg{commitIdx: idx, diffStat: stat, diffBody: body}
	}
}

func (m *model) maybeLoadDiff() tea.Cmd {
	if m.selected >= 0 && m.selected < len(m.commits) && !m.commits[m.selected].DiffLoaded {
		statWidth := m.detailsContentWidth
		if statWidth <= 0 {
			statWidth = 80
		}
		return loadDiffCmd(m.repoPath, m.commits[m.selected].FullHash, m.selected, statWidth)
	}
	return nil
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
		log.Println("Repository opened successfully with go-git")
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

func (m *model) loadCommitsFromGitCLI() ([]commit, error) {
	const maxCommits = 5000

	log.Println("Using git CLI to load commits...")

	// Use git log with a custom format
	cmd := exec.Command("git", "log",
		fmt.Sprintf("-n%d", maxCommits),
		"--pretty=format:%H%x00%an%x00%at%x00%s%x00%P",
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

	raw := strings.ReplaceAll(out.String(), "\r", "")
	lines := strings.Split(raw, "\n")
	commits := make([]commit, 0, len(lines))

	for i, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "\x00", 5)
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
	// Basic graph generation (fallback when git log --graph is not available)
	for i := range commits {
		if len(commits[i].Parents) == 0 {
			commits[i].GraphLine = "◉ "
		} else if len(commits[i].Parents) == 1 {
			commits[i].GraphLine = "● "
		} else {
			commits[i].GraphLine = "◆ "
		}
	}
}

func transliterateGraph(s string) string {
	r := strings.NewReplacer(
		"*", "●",
		"|", "│",
	)
	return r.Replace(s)
}

func (m *model) loadGraphData() error {
	const maxCommits = 5000
	log.Println("Loading graph data from git CLI...")

	cmd := exec.Command("git", "log",
		"--graph",
		"--all",
		fmt.Sprintf("-n%d", maxCommits),
		"--pretty=format:%H%x00%an%x00%at%x00%s%x00%P%x00%D",
	)
	cmd.Dir = m.repoPath

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git log --graph failed: %v (%s)", err, errOut.String())
	}

	raw := strings.ReplaceAll(out.String(), "\r", "")
	lines := strings.Split(raw, "\n")
	hashPattern := regexp.MustCompile(`[0-9a-f]{40}`)

	m.commits = nil
	m.displayRows = nil
	m.maxGraphWidth = 0

	for _, line := range lines {
		if line == "" {
			continue
		}

		loc := hashPattern.FindStringIndex(line)
		if loc != nil {
			// This is a commit line
			graphPart := line[:loc[0]]
			dataPart := line[loc[0]:]

			// Parse commit data: hash\x00author\x00timestamp\x00subject\x00parents\x00refs
			parts := strings.SplitN(dataPart, "\x00", 6)
			if len(parts) < 4 {
				continue
			}

			fullHash := parts[0]
			shortHash := fullHash
			if len(shortHash) > 7 {
				shortHash = shortHash[:7]
			}

			author := parts[1]
			var date time.Time
			if ts, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
				date = time.Unix(ts, 0)
			}

			message := parts[3]

			var parents []string
			if len(parts) > 4 && parts[4] != "" {
				for _, p := range strings.Fields(parts[4]) {
					if len(p) > 7 {
						parents = append(parents, p[:7])
					} else {
						parents = append(parents, p)
					}
				}
			}

			refs := ""
			if len(parts) > 5 {
				refs = strings.TrimSpace(parts[5])
			}

			commitIdx := len(m.commits)
			m.commits = append(m.commits, commit{
				Hash:     shortHash,
				FullHash: fullHash,
				Author:   author,
				Date:     date,
				Message:  message,
				Parents:  parents,
				Refs:     refs,
			})

			graphStr := transliterateGraph(graphPart)
			gw := len(graphPart) // ASCII width
			if gw > m.maxGraphWidth {
				m.maxGraphWidth = gw
			}

			m.displayRows = append(m.displayRows, displayRow{
				GraphChars: graphStr,
				CommitIdx:  commitIdx,
				GraphWidth: gw,
			})
		} else {
			// Graph-only line (branch/merge connectors)
			graphStr := transliterateGraph(line)
			gw := len(line)
			if gw > m.maxGraphWidth {
				m.maxGraphWidth = gw
			}

			m.displayRows = append(m.displayRows, displayRow{
				GraphChars: graphStr,
				CommitIdx:  -1,
				GraphWidth: gw,
			})
		}
	}

	log.Printf("Loaded %d commits, %d display rows, max graph width: %d\n",
		len(m.commits), len(m.displayRows), m.maxGraphWidth)
	return nil
}

func (m *model) renderRepoInfo() string {
	var sb strings.Builder

	// Repository name
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Title)).Render("Repository: "))
	sb.WriteString(m.repoName)
	sb.WriteString("  ")

	// Branch
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Branch)).Render("Branch: "))
	sb.WriteString(branchStyle.Render(m.currentBranch))
	sb.WriteString("  ")

	// Current commit
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Hash)).Render("Commit: "))
	sb.WriteString(commitHashStyle.Render(m.currentCommit))

	leftContent := sb.String()

	// Title on the right
	versionStr := "v" + version
	if m.latestVersion != "" && m.latestVersion != versionStr {
		// New version available
		versionStr = versionStr + " → " + m.latestVersion + " available"
	}
	title := titleStyle.Render("🦒 " + appName + " - Git Graph Viewer (" + versionStr + ")")

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
	log.Printf("renderCommitList: commits=%d, displayRows=%d, selected=%d, windowHeight=%d, maxGraphWidth=%d",
		len(m.commits), len(m.displayRows), m.selected, m.windowHeight, m.maxGraphWidth)

	if len(m.commits) == 0 {
		return "No commits found"
	}

	var sb strings.Builder

	// Calculate visible range based on window height
	// Must match the contentHeight from View(): windowHeight - 8
	visibleHeight := m.windowHeight - 8
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	log.Printf("renderCommitList: visibleHeight=%d", visibleHeight)

	graphColor := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.Graph))
	selGraphColor := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.SelectedFg)).Bold(true)
	selHashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.SelectedFg)).Bold(true)

	if len(m.displayRows) > 0 {
		// Graph mode: use displayRows from git log --graph

		// Find the display row index of the selected commit
		selectedRowIdx := 0
		for i, row := range m.displayRows {
			if row.CommitIdx == m.selected {
				selectedRowIdx = i
				break
			}
		}
		log.Printf("renderCommitList graph mode: selectedRowIdx=%d", selectedRowIdx)

		// Scroll to keep selected row visible
		// Use a stable scroll offset that only changes when the selected row
		// would move outside the visible window (like a typical text editor).
		startIdx := selectedRowIdx - visibleHeight/3
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := startIdx + visibleHeight
		if endIdx > len(m.displayRows) {
			endIdx = len(m.displayRows)
			startIdx = endIdx - visibleHeight
			if startIdx < 0 {
				startIdx = 0
			}
		}
		log.Printf("renderCommitList graph mode: startIdx=%d, endIdx=%d", startIdx, endIdx)

		linesWritten := 0
		for i := startIdx; i < endIdx; i++ {
			row := m.displayRows[i]
			isCommit := row.CommitIdx >= 0
			isSel := isCommit && row.CommitIdx == m.selected

			// Bounds check before accessing commits slice
			if isCommit && (row.CommitIdx < 0 || row.CommitIdx >= len(m.commits)) {
				log.Printf("renderCommitList ERROR: row %d has out-of-bounds CommitIdx=%d (len(commits)=%d), skipping",
					i, row.CommitIdx, len(m.commits))
				sb.WriteString("\n")
				continue
			}

			// Pad graph to max width for alignment
			padLen := m.maxGraphWidth - row.GraphWidth
			if padLen < 0 {
				padLen = 0
			}
			graphPadded := row.GraphChars + strings.Repeat(" ", padLen)

			if isSel {
				highlighted := strings.ReplaceAll(graphPadded, "●", "◉")
				sb.WriteString("> ")
				sb.WriteString(selGraphColor.Render(highlighted))
				sb.WriteString(" ")
				sb.WriteString(selHashStyle.Render(m.commits[row.CommitIdx].Hash))
			} else {
				sb.WriteString("  ")
				sb.WriteString(graphColor.Render(graphPadded))
				if isCommit {
					sb.WriteString(" ")
					sb.WriteString(commitHashStyle.Render(m.commits[row.CommitIdx].Hash))
				}
			}
			sb.WriteString("\n")
			linesWritten++
		}
		// Pad to exactly visibleHeight lines so the panel never changes size
		for linesWritten < visibleHeight {
			sb.WriteString("\n")
			linesWritten++
		}
	} else {
		// Simple mode: one row per commit with basic symbol (fallback)
		startIdx := 0
		if m.selected >= visibleHeight {
			startIdx = m.selected - visibleHeight + 1
		}
		endIdx := startIdx + visibleHeight
		if endIdx > len(m.commits) {
			endIdx = len(m.commits)
		}

		linesWritten := 0
		for i := startIdx; i < endIdx; i++ {
			c := m.commits[i]

			if i == m.selected {
				sb.WriteString("> ")
				sb.WriteString(selGraphColor.Render(c.GraphLine))
				sb.WriteString(" ")
				sb.WriteString(selHashStyle.Render(c.Hash))
			} else {
				sb.WriteString("  ")
				sb.WriteString(graphColor.Render(c.GraphLine))
				sb.WriteString(" ")
				sb.WriteString(commitHashStyle.Render(c.Hash))
			}
			sb.WriteString("\n")
			linesWritten++
		}
		for linesWritten < visibleHeight {
			sb.WriteString("\n")
			linesWritten++
		}
	}

	// Truncate to available height inside the panel.
	// lipgloss Height() does NOT clip overflow.
	// Panel uses Height(contentHeight) with Padding(0,1) → 0 vertical padding.
	result := sb.String()
	resultLines := strings.Split(result, "\n")
	maxLines := m.windowHeight - 8
	if maxLines < 3 {
		maxLines = 3
	}
	if len(resultLines) > maxLines {
		resultLines = resultLines[:maxLines]
	}
	return strings.Join(resultLines, "\n")
}

// truncateLines truncates each line of s to maxWidth visible characters,
// correctly handling ANSI escape sequences.
func truncateLines(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if ansi.StringWidth(line) > maxWidth {
			lines[i] = ansi.Truncate(line, maxWidth, "")
		}
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderCommitDetails() string {
	log.Printf("renderCommitDetails: selected=%d, len(commits)=%d", m.selected, len(m.commits))
	if len(m.commits) == 0 || m.selected < 0 || m.selected >= len(m.commits) {
		log.Printf("renderCommitDetails: skipping (empty or out of bounds)")
		return ""
	}

	c := m.commits[m.selected]

	var sb strings.Builder

	// SHA
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Hash)).Render("SHA:     "))
	sb.WriteString(commitHashStyle.Render(c.FullHash))
	sb.WriteString("\n")

	// Date
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Date)).Render("Date:    "))
	sb.WriteString(dateStyle.Render(c.Date.Format("2006-01-02 15:04:05")))
	sb.WriteString("\n")

	// Author
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Author)).Render("Author:  "))
	sb.WriteString(authorStyle.Render(c.Author))
	sb.WriteString("\n")

	// Parents
	if len(c.Parents) > 0 {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("Parents: "))
		sb.WriteString(strings.Join(c.Parents, ", "))
		sb.WriteString("\n")
	}

	// Refs
	if c.Refs != "" {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Branch)).Render("Refs:    "))
		sb.WriteString(branchStyle.Render(c.Refs))
		sb.WriteString("\n")
	}

	// Commit message
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.SectionHeader)).Render("─── Message ───────────────────────"))
	sb.WriteString("\n")
	sb.WriteString(messageStyle.Render(c.Message))
	sb.WriteString("\n")

	// Diff stats
	if c.DiffLoaded && c.DiffStat != "" {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.SectionHeader)).Render("─── Stats ─────────────────────────"))
		sb.WriteString("\n")
		sb.WriteString(c.DiffStat)
		sb.WriteString("\n")
	}

	// Diff content
	if c.DiffLoaded && c.DiffBody != "" {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.SectionHeader)).Render("─── Diff ──────────────────────────"))
		sb.WriteString("\n")

		addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.DiffAdd))
		delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.DiffDel))
		hunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.DiffHunk))
		diffHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.DiffHeader))

		for _, line := range strings.Split(c.DiffBody, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				sb.WriteString(addStyle.Render(line))
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				sb.WriteString(delStyle.Render(line))
			} else if strings.HasPrefix(line, "@@") {
				sb.WriteString(hunkStyle.Render(line))
			} else if strings.HasPrefix(line, "diff ") {
				sb.WriteString(diffHeaderStyle.Render(line))
			} else {
				sb.WriteString(line)
			}
			sb.WriteString("\n")
		}
	} else if !c.DiffLoaded {
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("Loading diff..."))
		sb.WriteString("\n")
	}

	// Apply scroll offset and truncate to fit panel height.
	// lipgloss Height() only pads short content, it does NOT clip overflow,
	// so we must truncate here to prevent the panel from growing unbounded.
	content := truncateLines(sb.String(), m.detailsContentWidth)
	allLines := strings.Split(content, "\n")

	// Clamp scroll
	if m.detailsScroll >= len(allLines) {
		m.detailsScroll = len(allLines) - 1
	}
	if m.detailsScroll < 0 {
		m.detailsScroll = 0
	}
	if m.detailsScroll > 0 {
		allLines = allLines[m.detailsScroll:]
	}

	// Truncate to available height inside the panel
	// Panel uses Height(contentHeight) with Padding(1,2) → 2 vertical padding lines
	maxLines := m.windowHeight - 8 - 2 // contentHeight minus vertical padding
	if maxLines < 3 {
		maxLines = 3
	}
	if len(allLines) > maxLines {
		allLines = allLines[:maxLines]
	}

	return strings.Join(allLines, "\n")
}

// addBoxLabel overlays a label like [0] onto the top-left corner of a rendered box border.
// It accounts for ANSI escape sequences so it only replaces visible border characters.
// trimToHeight ensures a rendered string is exactly targetHeight lines.
// If taller, excess lines are removed from the bottom (preserving the bottom border).
// If shorter, empty lines are appended.
func trimToHeight(rendered string, targetHeight int) string {
	lines := strings.Split(rendered, "\n")
	// Remove trailing empty string from split if present
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > targetHeight {
		// Keep first line (top border), middle content up to targetHeight-2, and last line (bottom border)
		top := lines[0]
		bottom := lines[len(lines)-1]
		middle := lines[1 : targetHeight-1]
		result := make([]string, 0, targetHeight)
		result = append(result, top)
		result = append(result, middle...)
		result = append(result, bottom)
		return strings.Join(result, "\n")
	}
	for len(lines) < targetHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

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

// setTerminalTitle sets the terminal window title using ANSI escape sequence
// Works on Windows 10+, macOS, and Linux
func setTerminalTitle(title string) {
	fmt.Printf("\033]0;%s\007", title)
}

// resetTerminalTitle clears the terminal title back to default
func resetTerminalTitle() {
	fmt.Printf("\033]0;\007")
}
