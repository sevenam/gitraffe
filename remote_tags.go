package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Marks appended to a tag whose local and remote copies disagree. A tag present
// on both carries no mark: that is the normal case, and marking every tag would
// spend label-column width to say nothing.
const (
	tagLocalOnlyMark  = "↑" // here, but on no remote — not pushed
	tagRemoteOnlyMark = "↓" // on a remote, but not here — not fetched
)

// remoteTagsTimeout bounds each remote so an unreachable host can't leave the
// check hanging for the lifetime of the session.
const remoteTagsTimeout = 15 * time.Second

// tagRef identifies a tag by name and the commit it points at. Both halves
// matter: a tag moved to another commit is out of sync even though the name
// exists on both sides.
type tagRef struct {
	name   string
	commit string // full hash
}

// loadRemoteTagsCmd asks every remote which tags it holds.
//
// Unlike branches, tags have no remote-tracking copy (there is no
// refs/remotes/origin/tags/...): fetched tags land directly in refs/tags, so
// nothing stored locally can say whether a tag was ever pushed. Asking the remote
// is the only source of truth, which makes this a network call — hence async.
func loadRemoteTagsCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		tags, err := fetchRemoteTags(repoPath)
		if err != nil {
			log.Printf("Remote tag check skipped: %v\n", err)
			return remoteTagsMsg{repoPath: repoPath}
		}
		return remoteTagsMsg{repoPath: repoPath, tags: tags}
	}
}

// fetchRemoteTags returns the union of tags across all remotes, or nil when the
// answer is unknown: no remotes, or any remote failing to respond.
func fetchRemoteTags(repoPath string) (map[tagRef]bool, error) {
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

	tags := make(map[tagRef]bool)
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

// noPromptEnv is the environment for a git command that talks to a remote
// while the TUI owns the terminal, where a username, password or SSH
// passphrase prompt would either hang or scribble over the screen.
func noPromptEnv(repoPath string) []string {
	env := append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // HTTPS: fail rather than ask for credentials
		"GCM_INTERACTIVE=never", // Git Credential Manager: no sign-in window
	)
	if usesDefaultSSH(repoPath) {
		// BatchMode makes ssh fail instead of asking for a passphrase. Only
		// injected for plain ssh: GIT_SSH_COMMAND would override a user's own
		// GIT_SSH / core.sshCommand (e.g. plink), breaking their setup.
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return env
}

func usesDefaultSSH(repoPath string) bool {
	if os.Getenv("GIT_SSH_COMMAND") != "" || os.Getenv("GIT_SSH") != "" {
		return false
	}
	cmd := exec.Command("git", "config", "--get", "core.sshCommand")
	cmd.Dir = repoPath
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)) == ""
}

// parseLsRemoteTags reads `git ls-remote --tags` output. An annotated tag is
// listed twice — as the tag object, then peeled (^{}) to its commit — and only
// the peeled hash matches the commit the graph shows, so it wins when present.
func parseLsRemoteTags(out string) []tagRef {
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

	tags := make([]tagRef, 0, len(commits))
	for name, hash := range commits {
		tags = append(tags, tagRef{name: name, commit: hash})
	}
	return tags
}

// applyRemoteTags records, per commit, which of its tags no remote holds there
// and which tags a remote holds there that are missing locally. Comparing by
// name and commit means a moved tag shows at both ends: marked local-only where
// it now points, remote-only where the remote still has it.
func (m *model) applyRemoteTags() {
	// Unknown shows no marks. The graph is also required: tag refs only come from
	// the graph loader, and the fallback loader has none, so every remote tag
	// would wrongly read as missing locally.
	if m.remoteTags == nil || len(m.displayRows) == 0 {
		return
	}

	local := make(map[tagRef]bool)
	byHash := make(map[string]int, len(m.commits))
	for i := range m.commits {
		c := &m.commits[i]
		byHash[c.FullHash] = i
		c.UnpushedTags = nil
		c.RemoteOnlyTags = nil

		_, _, tags := parseRefs(c.Refs)
		for _, name := range tags {
			ref := tagRef{name: name, commit: c.FullHash}
			local[ref] = true
			if !m.remoteTags[ref] {
				if c.UnpushedTags == nil {
					c.UnpushedTags = make(map[string]bool)
				}
				c.UnpushedTags[name] = true
			}
		}
	}

	for ref := range m.remoteTags {
		if local[ref] {
			continue
		}
		// Remote tags on commits outside the loaded history have nowhere to go.
		if i, ok := byHash[ref.commit]; ok {
			m.commits[i].RemoteOnlyTags = append(m.commits[i].RemoteOnlyTags, ref.name)
		}
	}
	for i := range m.commits {
		// Map iteration order is random; sort so labels don't reshuffle.
		sort.Strings(m.commits[i].RemoteOnlyTags)
	}
}
