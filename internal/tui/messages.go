package tui

import (
	"github.com/sevenam/gitraffe/internal/git"
)

// Message types for the Bubble Tea event system

// repoMsg and errMsg both carry the repository's fingerprint as it was just
// before the history is read; see auto_refresh.go.
type repoMsg struct {
	repo        *git.Repository
	fingerprint string
}

type errMsg struct {
	err         error
	fingerprint string
}

func (e errMsg) Error() string {
	return e.err.Error()
}

type versionCheckMsg struct {
	latestVersion string
}

type remoteTagsMsg struct {
	tags     map[tagRef]bool // nil when unknown: no remotes, or one didn't answer
	repoPath string          // the repository it answers for; see the handler
}

type updateFinishedMsg struct {
	version string
	err     error
}

type diffLoadedMsg struct {
	repoPath  string // the repository it was loaded for; see the handler
	commitIdx int
	diffStat  string
	diffBody  string
	// diffFiles is the same diff split per file. It is parsed before diffBody
	// is cut to length, so the commit view can list files the panel's text no
	// longer reaches.
	diffFiles []fileDiff
	// workingTree marks the uncommitted changes' diff, and state is what
	// came with it; see git.WorkingState.
	workingTree bool
	state       git.WorkingState
}
