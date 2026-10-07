package tui

import (
	"time"

	"github.com/sevenam/gitraffe/internal/git"
)

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
	headHash            string // full hash of the checked out commit, marked with ">" in the graph
	ahead, behind       int    // current branch vs its upstream; both 0 when in sync or unknown
	focusedBox          int    // 0 = repo info, 1 = commit list, 2 = commit details
	detailsScroll       int    // scroll offset for the details panel
	displayRows         []displayRow
	maxGraphWidth       int
	maxBranchWidth      int
	localLabelWidth     int // width the local-branch part is padded to, so remote branches start in one column
	maxAuthorWidth      int // display columns, not runes: names may be wide (CJK)
	detailsContentWidth int
	latestVersion       string          // latest version from GitHub, e.g., "v0.2.0"
	remoteTags          map[tagRef]bool // union of all remotes' tags; nil while unknown
	updateState         updateState
	updateMessage       string         // prompt, progress or error text for the status line
	updatedTo           string         // tag installed this session; read by main after Run returns
	colourLanes         bool           // tint each graph column differently; see lane_colours.go
	showHelp            bool           // key reference overlay, toggled with "?"
	copying             bool           // the copy prompt opened with "y" waits for its answer; see copy.go
	checkout            checkoutPrompt // which branch "c" asks for; see checkout.go
	switching           bool           // a checkout is running
	deletePicker        deletePicker   // the list "d" opens; see branch_delete.go
	deleting            bool           // a branch delete is running
	picker              themePicker
	refs                refPicker // the branch and tag list opened with "B"
	switcher            repoSwitcher
	repoRoot            string // absolute root of the open repository; "" until it has loaded
	reselect            string // full hash to reselect once a reload finishes; see reloadRepo
	maximised           bool   // the focused panel has the window to itself; toggled with enter
	fetching            bool   // a fetch is running; see startFetch
	pulling             bool   // a pull is running; see startPull
	commitLimit         int    // how many commits to read; grows with "m", see loadMoreCommits
	moreCommits         bool   // the log was cut off at commitLimit
	search              commitSearch
	filter              git.Filter // narrows the graph; the zero filter is all of it. See filter.go
	files               filePicker // the file list opened with "h"
	commitView          commitView // the whole-screen look at one commit; see commit_view.go
	// Staging and committing from the commit view; see staging.go and
	// commit_prompt.go.
	workingState git.WorkingState // what the repository is in the middle of, as last read
	staging      bool             // a change to the index is running
	commitPrompt commitPrompt     // the message box "c" opens
	committing   bool             // a commit is running
	committed    string           // full hash of the commit just made, until the reload finds it
	// Pushing; see push.go.
	push    pushPrompt // what "P" has to ask before it pushes
	pushing bool       // a push is running

	branchPrompt branchPrompt // the name "b" asks for; see branch_create.go

	configDir    string // gitraffe's config directory; "" means a picked theme can't be saved
	notice       string // one-off status line text, e.g. the theme just saved; cleared by the next key
	loadedNotice string // takes over from notice once the repository has loaded; see refresh
	// Unasked refreshing and fetching; see auto_refresh.go. Zero intervals mean off.
	autoRefresh      time.Duration
	autoFetch        time.Duration
	fingerprint      string // the repository as it was when last read; "" when unknown
	autoFetching     bool   // an unasked fetch is running
	autoFetchStopped bool   // one failed, so no more are tried until a fetch you ask for works
	// stale is the model a reload replaced, drawn in place of the loading page
	// until this one is ready, so reading the repository again doesn't blank
	// the screen. Nil on a first load and after a switch, which have nothing
	// to keep showing. See reloadRepo.
	stale *model
}

func initialModel(repoPath string) model {
	return model{
		repoPath:    repoPath,
		focusedBox:  1,    // default focus on commit list
		colourLanes: true, // lane colouring is the default; "L" turns it off
		maximised:   true, // one panel fills the window; enter brings the other back
		commitLimit: commitBatch,
	}
}
