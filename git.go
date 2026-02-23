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
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-git/go-git/v5"
)

// commit represents a single git commit with metadata
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

// displayRow represents a single line in the commit graph display
type displayRow struct {
	GraphChars string // transliterated Unicode graph characters
	CommitIdx  int    // index into commits slice, -1 for graph-only lines
	GraphWidth int    // visual width of the graph portion
}

// Async commands for loading repo and diff data

func loadRepo(path string) tea.Cmd {
	return func() tea.Msg {
		repo, err := git.PlainOpen(path)
		if err != nil {
			return errMsg{err}
		}
		return repoMsg{repo}
	}
}

// Repo info loaders

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

// Commit loaders

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

// Graph generation and loading

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

func extractBranchLabel(refs string) string {
	if refs == "" {
		return ""
	}
	var branches []string
	for _, ref := range strings.Split(refs, ", ") {
		ref = strings.TrimSpace(ref)
		if strings.HasPrefix(ref, "tag: ") {
			continue
		}
		if strings.HasPrefix(ref, "HEAD -> ") {
			ref = strings.TrimPrefix(ref, "HEAD -> ")
		}
		if ref == "HEAD" {
			continue
		}
		branches = append(branches, ref)
	}
	if len(branches) == 0 {
		return ""
	}
	return strings.Join(branches, ", ")
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

	// Calculate max branch label width for column alignment
	m.maxBranchWidth = 0
	for _, c := range m.commits {
		label := extractBranchLabel(c.Refs)
		labelWidth := utf8.RuneCountInString(label)
		if labelWidth > m.maxBranchWidth {
			m.maxBranchWidth = labelWidth
		}
	}
	if m.maxBranchWidth > 25 {
		// Truncate to runes, not bytes
		m.maxBranchWidth = 25
	}

	log.Printf("Loaded %d commits, %d display rows, max graph width: %d, max branch width: %d\n",
		len(m.commits), len(m.displayRows), m.maxGraphWidth, m.maxBranchWidth)
	return nil
}

// Diff loading

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
