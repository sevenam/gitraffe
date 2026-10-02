package git

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Commit represents a single git commit with metadata
type Commit struct {
	Hash     string
	FullHash string
	Author   string
	Date     time.Time
	Message  string
	Parents  []string
	Refs     string
	// MergedBranch is the name of a branch whose tip this commit was, recovered
	// from the message of the merge commit that absorbed it. Set only when no
	// ref points here any more, i.e. the branch has since been deleted.
	MergedBranch string
	// Tag sync state, filled in once every remote has answered; empty until then.
	UnpushedTags   map[string]bool // tags here that no remote has at this commit
	RemoteOnlyTags []string        // tags a remote has at this commit, missing locally
	// WorkingTree marks the synthetic row for uncommitted changes, which sits
	// above the newest commit and has no hash of its own. See addWorkingTreeRow.
	WorkingTree bool
	GraphLine   string
	DiffLoaded  bool
	DiffStat    string
	DiffBody    string
	// DiffFiles is the same diff split per file, for the commit view. It is
	// parsed from the untruncated output, so a commit too long for DiffBody
	// still lists every file it touched.
	DiffFiles []FileDiff
}

// DisplayRow represents a single line in the commit graph display
type DisplayRow struct {
	GraphChars string // the row of the graph, in box-drawing characters; see layoutGraph
	CommitIdx  int    // index into commits slice, -1 for a row that is not a commit
	GraphWidth int    // visual width of the graph portion
	Lanes      []int  // lane number per character of GraphChars; see layoutGraph
	// Note is plain text drawn instead of a graph row, for the line saying the
	// history was cut short. Empty on every row git produced.
	Note string
}

// Graph is a repository's history laid out as a graph: the commits, and the
// rows of the drawing that carry them.
type Graph struct {
	Commits       []Commit
	Rows          []DisplayRow
	MaxGraphWidth int // the widest row's graph, in characters
	// More reports that the history was cut off at the limit asked for.
	More bool
}

// LoadCommits reads the history without its graph, for when LoadGraph
// fails. Each commit gets a marker in place of the drawing, and the second
// result reports whether the history was cut off at limit.
func LoadCommits(dir string, limit int) ([]Commit, bool, error) {

	log.Println("Using git CLI to load commits...")

	// Use git log with a custom format
	cmd := exec.Command("git", "log",
		fmt.Sprintf("-n%d", limit),
		"--pretty=format:%H%x00%an%x00%at%x00%s%x00%P",
		"--all")
	cmd.Dir = dir

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		log.Printf("Git CLI error: %v, stderr: %s\n", err, errOut.String())
		return nil, false, fmt.Errorf("git command failed: %v", err)
	}

	raw := strings.ReplaceAll(out.String(), "\r", "")
	lines := strings.Split(raw, "\n")
	commits := make([]Commit, 0, len(lines))

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

		commits = append(commits, Commit{
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
	generateGraph(commits)

	return commits, len(commits) >= limit, nil
}

// generateGraph gives each commit a marker by how many parents it has, in place
// of the drawing LoadGraph would have supplied.
func generateGraph(commits []Commit) {
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

// The markers a commit is drawn with. A merge gets its own, so that the commit
// a branch was merged into stands out from the commits around it.
const (
	CommitMarker = '●'
	MergeMarker  = '◆' // a commit with more than one parent
)

// IsCommitMarker reports whether r is one of the characters a commit is drawn
// with, as against the lines that join them.
func IsCommitMarker(r rune) bool {
	return r == CommitMarker || r == MergeMarker
}

// LoadGraph reads up to limit commits across every ref and lays them out as a
// graph; see layoutGraph for the drawing.
func LoadGraph(dir string, limit int) (Graph, error) {
	log.Println("Loading graph data from git CLI...")

	cmd := exec.Command("git", "log",
		// Every commit before its parents, which the layout depends on: a
		// lane is opened by a child and closed by the parent it leads to.
		"--topo-order",
		"--all",
		fmt.Sprintf("-n%d", limit),
		// Full ref paths, so refs/heads/ can be told from refs/remotes/ without
		// guessing — see ParseRefs.
		"--decorate=full",
		"--pretty=format:%H%x00%an%x00%at%x00%s%x00%P%x00%D",
	)
	cmd.Dir = dir

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		return Graph{}, fmt.Errorf("git log failed: %v (%s)", err, errOut.String())
	}

	var g Graph
	raw := strings.ReplaceAll(out.String(), "\r", "")
	for _, line := range strings.Split(raw, "\n") {
		// hash\x00author\x00timestamp\x00subject\x00parents\x00refs
		parts := strings.SplitN(line, "\x00", 6)
		if len(parts) < 4 {
			continue
		}

		var date time.Time
		if ts, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
			date = time.Unix(ts, 0)
		}
		var parents []string
		if len(parts) > 4 {
			for _, p := range strings.Fields(parts[4]) {
				parents = append(parents, shortHash(p))
			}
		}
		refs := ""
		if len(parts) > 5 {
			refs = strings.TrimSpace(parts[5])
		}

		g.Commits = append(g.Commits, Commit{
			Hash:     shortHash(parts[0]),
			FullHash: parts[0],
			Author:   parts[1],
			Date:     date,
			Message:  parts[3],
			Parents:  parents,
			Refs:     refs,
		})
	}

	g.Rows, g.MaxGraphWidth = layoutGraph(g.Commits)
	labelMergedBranches(g.Commits)

	// Exactly the limit means git stopped counting rather than ran out. A
	// repository with exactly this many commits says "more" once, and loading
	// again settles it.
	g.More = len(g.Commits) >= limit

	log.Printf("Loaded %d commits, %d display rows, max graph width: %d\n",
		len(g.Commits), len(g.Rows), g.MaxGraphWidth)
	return g, nil
}

// shortHash is the seven characters commits are known by in the graph.
func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}
