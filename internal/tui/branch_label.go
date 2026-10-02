package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/git"
)

// labelSegment is one styled entry in the branch/tag label column.
type labelSegment struct {
	text  string
	style lipgloss.Style
}

// refSegments lists a commit's refs: local branches, remote-tracking branches,
// then tags. Tag sync state is shown as a mark rather than a colour, because
// colour already says "this is a tag".
func refSegments(c commit) []labelSegment {
	local, remote, tags := git.ParseRefs(c.Refs)
	segs := make([]labelSegment, 0, len(local)+len(remote)+len(tags)+len(c.RemoteOnlyTags))
	for _, b := range local {
		segs = append(segs, labelSegment{b, localBranchStyle})
	}
	for _, b := range remote {
		segs = append(segs, labelSegment{b, remoteBranchStyle})
	}
	for _, t := range tags {
		if c.UnpushedTags[t] {
			t += tagLocalOnlyMark
		}
		segs = append(segs, labelSegment{t, tagStyle})
	}
	for _, t := range c.RemoteOnlyTags {
		segs = append(segs, labelSegment{t + tagRemoteOnlyMark, tagStyle})
	}
	return segs
}

// labelSegments is refSegments plus the name of a deleted branch this commit was
// the tip of.
func labelSegments(c commit) []labelSegment {
	segs := refSegments(c)
	if c.MergedBranch != "" {
		segs = append(segs, labelSegment{c.MergedBranch, mergedBranchStyle})
	}
	return segs
}

// labelText is the full label-column text for a commit. It is built from the
// very segments the renderer draws, so the column can never be sized for
// different text than it shows.
func labelText(c commit) string {
	segs := labelSegments(c)
	texts := make([]string, len(segs))
	for i, seg := range segs {
		texts[i] = seg.text
	}
	return strings.Join(texts, ", ")
}

// updateLabelWidth sizes the label column to the widest label in the graph.
func (m *model) updateLabelWidth() {
	m.maxBranchWidth = 0
	for _, c := range m.commits {
		if w := utf8.RuneCountInString(labelText(c)); w > m.maxBranchWidth {
			m.maxBranchWidth = w
		}
	}
}
