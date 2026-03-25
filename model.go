package main

import (
	"time"

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
