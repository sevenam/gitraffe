package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// remoteTagsTimeout bounds each remote so an unreachable host can't leave the
// check hanging for the lifetime of the session.
const remoteTagsTimeout = 15 * time.Second

// TagRef identifies a tag by name and the commit it points at. Both halves
// matter: a tag moved to another commit is out of sync even though the name
// exists on both sides.
type TagRef struct {
	Name   string
	Commit string // full hash
}

// RemoteTags returns the union of tags across all remotes, or nil when the
// answer is unknown: no remotes, or any remote failing to respond.
func RemoteTags(repoPath string) (map[TagRef]bool, error) {
	cmd := exec.Command("git", "remote")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git remote: %w", err)
	}
	remotes := strings.Fields(string(out))
	if len(remotes) == 0 {
		return nil, nil
	}

	tags := make(map[TagRef]bool)
	for _, remote := range remotes {
		out, err := lsRemoteTags(repoPath, remote)
		if err != nil {
			// All or nothing: treating one unreachable remote as empty would mark
			// every tag it holds as unpushed, which is a false alarm.
			return nil, fmt.Errorf("ls-remote %s: %w", remote, err)
		}
		for _, t := range parseLsRemoteTags(out) {
			tags[t] = true
		}
	}
	return tags, nil
}

// lsRemoteTags runs `git ls-remote --tags` without ever prompting. This runs
// while the TUI owns the terminal, where a username, password or SSH passphrase
// prompt would either hang the check or scribble over the screen.
func lsRemoteTags(repoPath, remote string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteTagsTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", remote)
	cmd.Dir = repoPath
	cmd.Env = noPromptEnv(repoPath)

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// parseLsRemoteTags reads `git ls-remote --tags` output. An annotated tag is
// listed twice — as the tag object, then peeled (^{}) to its commit — and only
// the peeled hash matches the commit the graph shows, so it wins when present.
func parseLsRemoteTags(out string) []TagRef {
	commits := make(map[string]string)
	peeled := make(map[string]bool)
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		hash, ref, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		name, ok := strings.CutPrefix(ref, "refs/tags/")
		if !ok {
			continue
		}
		if base, isPeeled := strings.CutSuffix(name, "^{}"); isPeeled {
			commits[base] = hash
			peeled[base] = true
		} else if !peeled[name] {
			commits[name] = hash
		}
	}

	tags := make([]TagRef, 0, len(commits))
	for name, hash := range commits {
		tags = append(tags, TagRef{Name: name, Commit: hash})
	}
	return tags
}

// applyRemoteTags records, per commit, which of its tags no remote holds there
// and which tags a remote holds there that are missing locally. Comparing by
