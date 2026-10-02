package tui

import "github.com/sevenam/gitraffe/internal/git"

// The data on screen is read by internal/git. These aliases let the interface
// code name it without the package prefix on every line; being aliases, they
// are the git package's types, not copies of them.
type (
	commit     = git.Commit
	displayRow = git.DisplayRow
	fileDiff   = git.FileDiff
	tagRef     = git.TagRef
)
