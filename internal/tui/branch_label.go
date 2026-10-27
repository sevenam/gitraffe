package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/git"
)

// labelKind is what a label names. Labels are listed a kind at a time, and
// each kind has a colour of its own.
type labelKind int

const (
	labelLocal labelKind = iota
	labelRemote
	labelTag
	labelCheckpoint
	labelStash
	labelMerged
)

// labelSegment is one styled entry in the branch/tag label column.
type labelSegment struct {
	text  string
	style lipgloss.Style
	kind  labelKind
	lead  string // written before this segment, in place of its usual separator
}

// labelGap is what stands between two labels of different kinds. It is as
// wide as the comma and space it stands in for, so the column is the width it
// would be with a comma everywhere.
const labelGap = "  "

// labelSeparator is what is written before segment i. A comma joins two
// labels of one kind, where nothing else says one name has ended and the next
// begun. Where the kind changes the colour changes with it, and a gap is
// enough: a comma there only adds a mark between things already told apart.
//
// The renderer and the width of the column both ask here, so the column is
// never sized for other text than it is given.
func labelSeparator(segs []labelSegment, i int) string {
	switch {
	case segs[i].lead != "":
		return segs[i].lead
	case i == 0:
		return ""
	case segs[i].kind != segs[i-1].kind:
		return labelGap
	}
	return ", "
}

// refSegments lists a commit's refs: local branches, remote-tracking branches,
// tags, checkpoints, then the stash. Tag sync state is shown as a mark rather than a
// colour, because colour already says "this is a tag".
//
// Checkpoints, local or remote, go last and in a colour of their own: they are
// an agent's snapshots, not anyone's work, and in the branch colours they
// would be taken for branches.
//
// refs/stash comes out of ParseRefs as a local branch called "stash", and is
// no branch: it is labelled with the name it was stashed under, in a colour of
// its own, so it is not taken for a branch somebody could check out.
func refSegments(c commit) []labelSegment {
	local, remote, tags := git.ParseRefs(c.Refs)
	segs := make([]labelSegment, 0, len(local)+len(remote)+len(tags)+len(c.RemoteOnlyTags))
	var checkpoints, stash []labelSegment
	for _, b := range local {
		if b == "stash" {
			stash = append(stash, labelSegment{text: stashRefLabel(c.Message), style: stashStyle, kind: labelStash})
			continue
		}
		if git.IsCheckpoint(b) {
			checkpoints = append(checkpoints, labelSegment{text: b, style: checkpointStyle, kind: labelCheckpoint})
			continue
		}
		segs = append(segs, labelSegment{text: b, style: localBranchStyle, kind: labelLocal})
	}
	for _, b := range remote {
		if git.IsCheckpoint(b) {
			checkpoints = append(checkpoints, labelSegment{text: b, style: checkpointStyle, kind: labelCheckpoint})
			continue
		}
		segs = append(segs, labelSegment{text: b, style: remoteBranchStyle, kind: labelRemote})
	}
	for _, t := range tags {
		if c.UnpushedTags[t] {
			t += tagLocalOnlyMark
		}
		segs = append(segs, labelSegment{text: t, style: tagStyle, kind: labelTag})
	}
	for _, t := range c.RemoteOnlyTags {
		segs = append(segs, labelSegment{text: t + tagRemoteOnlyMark, style: tagStyle, kind: labelTag})
	}
	return append(append(segs, checkpoints...), stash...)
}

// stashRefLabel is "stash/<name>", or "stash" for one stashed without a name.
func stashRefLabel(subject string) string {
	if name := git.StashName(subject); name != "" {
		return "stash/" + name
	}
	return "stash"
}

// labelSegments is refSegments plus the name of a deleted branch this commit was
// the tip of.
func labelSegments(c commit) []labelSegment {
	segs := refSegments(c)
	if c.MergedBranch != "" {
		segs = append(segs, labelSegment{text: c.MergedBranch, style: mergedBranchStyle, kind: labelMerged})
	}
	return segs
}

// labelText is a commit's label as the column would show it with nothing
// lined up.
func labelText(c commit) string {
	return segmentsText(labelSegments(c))
}

// segmentsText is the text of a label: each segment after its separator.
func segmentsText(segs []labelSegment) string {
	var sb strings.Builder
	for i, seg := range segs {
		sb.WriteString(labelSeparator(segs, i))
		sb.WriteString(seg.text)
	}
	return sb.String()
}

// localWidth is the width of the local branches before the first remote
// branch, and whether there is a remote branch at all.
func localWidth(segs []labelSegment) (width int, hasRemote bool) {
	for i, seg := range segs {
		if seg.kind == labelRemote {
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
//
// The padding is added to the gap labelSeparator would have put there, so the
// names are still apart on the row with the longest local name.
func alignRemote(segs []labelSegment, width int) []labelSegment {
	first := -1
	for i, seg := range segs {
		if seg.kind == labelRemote {
			first = i
			break
		}
	}
	if first < 0 || width == 0 {
		return segs
	}
	own, _ := localWidth(segs[:first])
	out := append([]labelSegment(nil), segs...)
	out[first].lead = strings.Repeat(" ", width-own) + labelGap
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
		width += utf8.RuneCountInString(labelSeparator(segs, i)) + utf8.RuneCountInString(seg.text)
	}
	return width
}
