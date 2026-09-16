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
	"github.com/charmbracelet/x/ansi"
	"github.com/go-git/go-git/v5"
)

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

// parseRefs splits the raw --pretty=%D string into local branches,
// remote-tracking branches and tags, each stripped of its ref prefix for display.
//
// It expects the full ref paths produced by --decorate=full. Git's short names
// are ambiguous: "feature/foo" is both a plausible local branch and a plausible
// branch "foo" on a remote named "feature", and only the full path
// (refs/heads/... versus refs/remotes/...) settles it.
func parseRefs(refs string) (local, remote, tags []string) {
	if refs == "" {
		return
	}
	for _, ref := range strings.Split(refs, ", ") {
		ref = strings.TrimSpace(ref)
		ref = strings.TrimPrefix(ref, "HEAD -> ")
		if ref == "" || ref == "HEAD" {
			continue
		}
		ref = strings.TrimPrefix(ref, "tag: ")

		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			local = append(local, strings.TrimPrefix(ref, "refs/heads/"))

		case strings.HasPrefix(ref, "refs/remotes/"):
			name := strings.TrimPrefix(ref, "refs/remotes/")
			// origin/HEAD is a symbolic pointer to the remote's default branch,
			// so it always duplicates a branch already listed beside it.
			if strings.HasSuffix(name, "/HEAD") {
				continue
			}
			remote = append(remote, name)

		case strings.HasPrefix(ref, "refs/tags/"):
			tags = append(tags, strings.TrimPrefix(ref, "refs/tags/"))

		default:
			// Anything else under refs/ (notes, stash, fetched PR refs). Show it
			// rather than dropping it silently.
			local = append(local, strings.TrimPrefix(ref, "refs/"))
		}
	}
	return
}

// mergedBranchName recovers the branch name recorded in a merge commit's subject.
//
// Git stores no branch name on a commit — a branch is just a ref, and deleting it
// erases the only pointer. The auto-generated merge subject is therefore the one
// place the name of a merged-then-deleted branch survives in the repository, and
// it exists only for real merge commits: squash and rebase merges leave nothing
// behind to recover.
func mergedBranchName(subject string) string {
	// GitHub: "Merge pull request #33 from owner/branch-name"
	if strings.HasPrefix(subject, "Merge pull request ") {
		_, after, ok := strings.Cut(subject, " from ")
		if !ok {
			return ""
		}
		fields := strings.Fields(after)
		if len(fields) == 0 {
			return ""
		}
		// Strip the owner (or fork owner) prefix; the remainder is the branch,
		// which may itself contain slashes.
		if _, branch, ok := strings.Cut(fields[0], "/"); ok {
			return branch
		}
		return fields[0]
	}

	// git: "Merge branch 'foo'", "Merge branch 'foo' into bar",
	// "Merge remote-tracking branch 'origin/foo'"
	if strings.HasPrefix(subject, "Merge branch ") || strings.HasPrefix(subject, "Merge remote-tracking branch ") {
		if _, after, ok := strings.Cut(subject, "'"); ok {
			if name, _, ok := strings.Cut(after, "'"); ok {
				return name
			}
		}
	}

	return ""
}

// labelMergedBranches tags each merge commit's second parent — the tip of the
// branch that was merged — with that branch's name.
func (m *model) labelMergedBranches() {
	byHash := make(map[string]int, len(m.commits))
	for i, c := range m.commits {
		byHash[c.Hash] = i
	}

	for _, c := range m.commits {
		// The second parent is the merged branch's tip; the first is the branch
		// that absorbed it.
		if len(c.Parents) < 2 {
			continue
		}
		name := mergedBranchName(c.Message)
		if name == "" {
			continue
		}
		idx, ok := byHash[c.Parents[1]]
		if !ok {
			continue
		}
		// A branch that still exists — locally or on a remote — already labels
		// itself via its ref.
		if local, remote, _ := parseRefs(m.commits[idx].Refs); len(local) > 0 || len(remote) > 0 {
			continue
		}
		m.commits[idx].MergedBranch = name
	}
}

// labelText is the full label-column text for a commit. It is built from the
// very segments the renderer draws, so the column can never be sized for
// different text than it shows.
func (c commit) labelText() string {
	segs := labelSegments(c)
	texts := make([]string, len(segs))
	for i, seg := range segs {
		texts[i] = seg.text
	}
	return strings.Join(texts, ", ")
}

// updateLabelWidth sizes the label column to the widest label in the graph.
func (m *model) updateLabelWidth() {
	m.maxBranchWidth = 0
	for _, c := range m.commits {
		if w := utf8.RuneCountInString(c.labelText()); w > m.maxBranchWidth {
			m.maxBranchWidth = w
		}
	}
}

func (m *model) loadGraphData() error {
	const maxCommits = 5000
	log.Println("Loading graph data from git CLI...")

	cmd := exec.Command("git", "log",
		"--graph",
		"--all",
		fmt.Sprintf("-n%d", maxCommits),
		// Full ref paths, so refs/heads/ can be told from refs/remotes/ without
		// guessing — see parseRefs.
		"--decorate=full",
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

	m.labelMergedBranches()
	m.applyRemoteTags()
	m.updateLabelWidth()

	m.maxAuthorWidth = 0
	for _, c := range m.commits {
		m.maxAuthorWidth = max(m.maxAuthorWidth, ansi.StringWidth(c.Author))
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
