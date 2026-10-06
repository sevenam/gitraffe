package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/git"
)

// labelSegment is one styled entry in the branch/tag label column.
type labelSegment struct {
	text   string
	style  lipgloss.Style
	remote bool
	lead   string // written instead of the default ", " before this segment when set
}

// refSegments lists a commit's refs: local branches, remote-tracking branches,
// then tags. Tag sync state is shown as a mark rather than a colour, because
// colour already says "this is a tag".
func refSegments(c commit) []labelSegment {
	local, remote, tags := git.ParseRefs(c.Refs)
	segs := make([]labelSegment, 0, len(local)+len(remote)+len(tags)+len(c.RemoteOnlyTags))
	for _, b := range local {
		segs = append(segs, labelSegment{text: b, style: localBranchStyle})
	}
	for _, b := range remote {
		segs = append(segs, labelSegment{text: b, style: remoteBranchStyle, remote: true})
	}
	for _, t := range tags {
		if c.UnpushedTags[t] {
			t += tagLocalOnlyMark
		}
		segs = append(segs, labelSegment{text: t, style: tagStyle})
	}
	for _, t := range c.RemoteOnlyTags {
		segs = append(segs, labelSegment{text: t + tagRemoteOnlyMark, style: tagStyle})
	}
	return segs
}

// labelSegments is refSegments plus the name of a deleted branch this commit was
// the tip of.
func labelSegments(c commit) []labelSegment {
	segs := refSegments(c)
	if c.MergedBranch != "" {
		segs = append(segs, labelSegment{text: c.MergedBranch, style: mergedBranchStyle})
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

// localWidth is the width of the local branches before the first remote
// branch, and whether there is a remote branch at all.
func localWidth(segs []labelSegment) (width int, hasRemote bool) {
	for i, seg := range segs {
		if seg.remote {
			return width, true
		}
		if i > 0 {
			width += 2
		}
		width += utf8.RuneCountInString(seg.text)
	}
	return width, false
}

// alignRemote pads the part before the first remote branch to width columns,
// so remote branches start in the same column on every row.
func alignRemote(segs []labelSegment, width int) []labelSegment {
	first := -1
	for i, seg := range segs {
		if seg.remote {
			first = i
			break
		}
	}
	if first < 0 || width == 0 {
		return segs
	}
	own, _ := localWidth(segs[:first])
	out := append([]labelSegment(nil), segs...)
	if first == 0 {
		out[0].lead = strings.Repeat(" ", width+2)
	} else {
		out[first].lead = strings.Repeat(" ", width-own) + ", "
	}
	return out
}

// updateLabelWidth sizes the label column to the widest label in the graph.
func (m *model) updateLabelWidth() {
	m.localLabelWidth = 0
	for _, c := range m.commits {
		if w, hasRemote := localWidth(labelSegments(c)); hasRemote && w > m.localLabelWidth {
			m.localLabelWidth = w
		}
	}
	m.maxBranchWidth = 0
	for _, c := range m.commits {
		if w := segmentsWidth(alignRemote(labelSegments(c), m.localLabelWidth)); w > m.maxBranchWidth {
			m.maxBranchWidth = w
		}
	}
}

func segmentsWidth(segs []labelSegment) int {
	width := 0
	for i, seg := range segs {
		switch {
		case seg.lead != "":
			width += utf8.RuneCountInString(seg.lead)
		case i > 0:
			width += 2
		}
		width += utf8.RuneCountInString(seg.text)
	}
	return width
}
