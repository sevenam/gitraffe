package tui

import (
	"log"
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
)

// Marks appended to a tag whose local and remote copies disagree. A tag present
// on both carries no mark: that is the normal case, and marking every tag would
// spend label-column width to say nothing.
const (
	tagLocalOnlyMark  = "↑" // here, but on no remote — not pushed
	tagRemoteOnlyMark = "↓" // on a remote, but not here — not fetched
)

// loadRemoteTagsCmd asks every remote which tags it holds.
//
// Unlike branches, tags have no remote-tracking copy (there is no
// refs/remotes/origin/tags/...): fetched tags land directly in refs/tags, so
// nothing stored locally can say whether a tag was ever pushed. Asking the remote
// is the only source of truth, which makes this a network call — hence async.
func loadRemoteTagsCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		tags, err := git.RemoteTags(repoPath)
		if err != nil {
			log.Printf("Remote tag check skipped: %v\n", err)
			return remoteTagsMsg{repoPath: repoPath}
		}
		return remoteTagsMsg{repoPath: repoPath, tags: tags}
	}
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

		_, _, tags := git.ParseRefs(c.Refs)
		for _, name := range tags {
			ref := tagRef{Name: name, Commit: c.FullHash}
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
		if i, ok := byHash[ref.Commit]; ok {
			m.commits[i].RemoteOnlyTags = append(m.commits[i].RemoteOnlyTags, ref.Name)
		}
	}
	for i := range m.commits {
		// Map iteration order is random; sort so labels don't reshuffle.
		sort.Strings(m.commits[i].RemoteOnlyTags)
	}
}
