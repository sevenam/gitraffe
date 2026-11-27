package git

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// UpstreamSync counts the commits the current branch and the remote don't
// share. It reads only local refs, so the counts are as of the last fetch.
// Where there is no meaningful answer (detached HEAD, no remote) both counts
// are zero.
func UpstreamSync(dir string) (ahead, behind int) {
	if out, err := Run(dir, "rev-list", "--left-right", "--count", "@{upstream}...HEAD"); err == nil {
		// Left of "..." is the upstream, right is HEAD: "<behind>\t<ahead>".
		fields := strings.Fields(out)
		if len(fields) != 2 {
			return 0, 0
		}
		behind, err1 := strconv.Atoi(fields[0])
		ahead, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			return 0, 0
		}
		return ahead, behind
	}

	// No upstream: never pushed with -u, or its remote branch was deleted. Such
	// a branch is still ahead by any sensible reading — it has commits the remote
	// doesn't — so count commits no remote-tracking branch contains. "Behind" has
	// nothing to measure against, so it stays zero. Showing nothing here would
	// make an unpushed branch look identical to one that is in sync.
	if _, err := Run(dir, "symbolic-ref", "-q", "HEAD"); err != nil {
		return 0, 0 // detached HEAD: no branch to push
	}
	if refs, err := Run(dir, "for-each-ref", "--count=1", "refs/remotes"); err != nil || refs == "" {
		return 0, 0 // no remote: nowhere to push, and counting all history would be noise
	}
	if out, err := Run(dir, "rev-list", "--count", "HEAD", "--not", "--remotes"); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(out)); err == nil {
			ahead = n
		}
	}
	return ahead, 0
}

// fetchTimeout gives a slow remote room to answer while still ending a fetch
// that has stalled — on a network that swallows the connection, git would wait
// far longer than anyone watching a graph will.
const fetchTimeout = 60 * time.Second

// Fetch fetches every branch of every remote. --prune drops remote-tracking
// branches whose remote branch is gone, which is the point of asking: stale
// ones are what make the graph disagree with the server. Local branches and
// tags are left alone. The first result is what git wrote to stderr.
//
// Each remote is given an explicit refspec (issue #258). Without one git uses
// the remote's configured refspec, and a --single-branch clone or a shallow
// submodule is configured to fetch one branch only, so branches made on the
// server never appeared however often you fetched. "fetch --all" cannot take
// a refspec, so the remotes are fetched one at a time; one that fails does
// not stop the others, and the first failure is the one reported. With the
// refspec on the command line --prune prunes by it too, so a branch deleted on
// the server goes away even where the config names only one.
//
// Tags still come along for the branches fetched, as before. A remote with
// remote.<name>.skipFetchAll set is skipped, as --all skipped it.
func Fetch(dir string) (string, error) {
	// One timeout for all the remotes together, as it was for --all.
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	var errOut bytes.Buffer
	var firstErr error
	for _, remote := range Remotes(dir) {
		if skip, _ := Run(dir, "config", "--type=bool", "remote."+remote+".skipFetchAll"); strings.TrimSpace(skip) == "true" {
			continue
		}
		refspec := "+refs/heads/*:refs/remotes/" + remote + "/*"
		cmd := exec.CommandContext(ctx, "git", "fetch", "--prune", "--quiet", remote, refspec)
		cmd.Dir = dir
		cmd.Env = noPromptEnv(dir)
		cmd.Stderr = &errOut
		if err := cmd.Run(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	err := firstErr
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("no answer after %s", fetchTimeout)
	}
	if err != nil {
		log.Printf("Fetch failed: %v (%s)", err, errOut.String())
	}
	return errOut.String(), err
}
