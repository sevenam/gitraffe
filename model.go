package main

import (
	"time"

	"github.com/go-git/go-git/v5"
)

// commit represents a single git commit with metadata
type commit struct {
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
	DiffFiles []fileDiff
}

// displayRow represents a single line in the commit graph display
type displayRow struct {
	GraphChars string // transliterated Unicode graph characters
	CommitIdx  int    // index into commits slice, -1 for graph-only lines
	GraphWidth int    // visual width of the graph portion
	Lanes      []int  // lane number per character of GraphChars; see graphLanes
	// Note is plain text drawn instead of a graph row, for the line saying the
	// history was cut short. Empty on every row git produced.
	Note string
}

// updateState tracks a self-update started from within the TUI.
type updateState int

const (
	updateIdle updateState = iota
	updateConfirming
	updateDownloading
	updateDone
)

type model struct {
	repo                *git.Repository
	commits             []commit
	ready               bool
	repoPath            string
	err                 error
	selected            int
	graphTop            int // first graph row on screen; see graphWindow
	windowHeight        int
	windowWidth         int
	repoName            string
	currentBranch       string
	currentCommit       string
	ahead, behind       int // current branch vs its upstream; both 0 when in sync or unknown
	focusedBox          int // 0 = repo info, 1 = commit list, 2 = commit details
	detailsScroll       int // scroll offset for the details panel
	displayRows         []displayRow
	maxGraphWidth       int
	maxBranchWidth      int
	maxAuthorWidth      int // display columns, not runes: names may be wide (CJK)
	detailsContentWidth int
	latestVersion       string          // latest version from GitHub, e.g., "v0.2.0"
	remoteTags          map[tagRef]bool // union of all remotes' tags; nil while unknown
	updateState         updateState
	updateMessage       string // prompt, progress or error text for the status line
	updatedTo           string // tag installed this session; read by main after Run returns
	colourLanes         bool   // tint each graph column differently; see lanes.go
	showHelp            bool   // key reference overlay, toggled with "?"
	picker              themePicker
	refs                refPicker // the branch and tag list opened with "b"
	switcher            repoSwitcher
	repoRoot            string // absolute root of the open repository; "" until it has loaded
	reselect            string // full hash to reselect once a reload finishes; see reloadRepo
	maximised           bool   // the focused panel has the window to itself; toggled with enter
	fetching            bool   // a fetch is running; see startFetch
	commitLimit         int    // how many commits to read; grows with "m", see loadMoreCommits
	moreCommits         bool   // the log was cut off at commitLimit
	search              commitSearch
	commitView          commitView // the whole-screen look at one commit; see commit_view.go
	configDir           string // gitraffe's config directory; "" means a picked theme can't be saved
	notice              string // one-off status line text, e.g. the theme just saved; cleared by the next key
}

// updateAvailable reports whether GitHub advertises a release newer than this build.
func (m *model) updateAvailable() bool {
	return isNewerVersion(m.latestVersion, version)
}

func initialModel(repoPath string) model {
	return model{
		repoPath:    repoPath,
		focusedBox:  1,    // default focus on commit list
		colourLanes: true, // lane colouring is the default; "c" turns it off
		maximised:   true, // one panel fills the window; enter brings the other back
		commitLimit: commitBatch,
	}
}
