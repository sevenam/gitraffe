package git

import (
	"bytes"
	"os/exec"
	"strings"
)

// PullOutcome is how a pull ended, when nothing went wrong.
type PullOutcome int

const (
	// PullFastForwarded: the branch was moved forward to its upstream.
	PullFastForwarded PullOutcome = iota
	// PullUpToDate: the upstream had nothing the branch lacks.
	PullUpToDate
	// PullDiverged: both sides have commits the other lacks, so catching up
	// would take a merge or a rebase. Nothing was changed.
	PullDiverged
	// PullNoUpstream: the branch tracks nothing, so there is nowhere to pull from.
	PullNoUpstream
	// PullDetached: HEAD is on no branch, so there is nothing to move.
	PullDetached
)

// PullResult is what a pull did.
type PullResult struct {
	Outcome  PullOutcome
	Branch   string // the branch HEAD is on; "" when detached
	Upstream string // what it tracks, e.g. "origin/main"; "" when it tracks nothing
	// Ahead and Behind are the counts after fetching and before moving anything:
	// Behind is how many commits a fast-forward brought, and both are what a
	// diverged branch is short of on each side.
	Ahead, Behind int
	// Fetched reports that the remotes were asked, whatever happened after: the
	// remote-tracking refs may have moved even if the branch did not.
	Fetched bool
	// Detail is what git wrote to stderr when the fetch or the fast-forward
	// failed.
	Detail string
}

// Pull catches the current branch up with its upstream, and only ever by
// fast-forwarding: it fetches, then moves the branch forward if the upstream
// is strictly ahead of it.
//
// That is narrower than "git pull" on purpose. A fast-forward makes no commit
// and cannot conflict, and git refuses it, changing nothing, when a local edit
// is in the way. Anything more — a merge, a rebase — can stop halfway in a
// state that has to be resolved by hand, which is work for a terminal. So a
// diverged branch is reported, not reconciled.
//
// It is "fetch" then "merge --ff-only" rather than "git pull --ff-only" so
// that pull.rebase and the like in someone's config cannot turn it into
// something else.
func Pull(dir string) (PullResult, error) {
	var r PullResult

	branch, err := Run(dir, "symbolic-ref", "-q", "--short", "HEAD")
	if err != nil || branch == "" {
		r.Outcome = PullDetached
		return r, nil
	}
	r.Branch = branch

	upstream, err := Run(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil || upstream == "" {
		r.Outcome = PullNoUpstream
		return r, nil
	}
	r.Upstream = upstream

	stderr, err := Fetch(dir)
	if err != nil {
		r.Detail = stderr
		return r, err
	}
	r.Fetched = true

	r.Ahead, r.Behind = UpstreamSync(dir)
	switch {
	case r.Behind == 0:
		r.Outcome = PullUpToDate
		return r, nil
	case r.Ahead > 0:
		r.Outcome = PullDiverged
		return r, nil
	}

	cmd := exec.Command("git", "merge", "--ff-only", "--quiet", "@{upstream}")
	cmd.Dir = dir
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		r.Detail = strings.TrimSpace(errOut.String())
		return r, err
	}
	r.Outcome = PullFastForwarded
	return r, nil
}
