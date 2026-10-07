package tui

import (
	"log"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// renderCommitDetails renders the right panel with commit details and diff
func (m *model) renderCommitDetails() (string, scrollMarks) {
	log.Printf("renderCommitDetails: selected=%d, len(commits)=%d", m.selected, len(m.commits))
	if len(m.commits) == 0 || m.selected < 0 || m.selected >= len(m.commits) {
		log.Printf("renderCommitDetails: skipping (empty or out of bounds)")
		return "", scrollMarks{}
	}

	c := m.commits[m.selected]
	return m.fitDetails(commitIdentity(c) + m.renderDiffSections(c))
}

// commitIdentity is what a commit is, as against what it changed: hash, date,
// author, parents, refs and the message. The details panel puts the diff under
// it; the commit view gives it a box of its own. Both ask here, so a commit
// reads the same on either screen.
func commitIdentity(c commit) string {
	var sb strings.Builder

	// The working tree has none of a commit's fields — no hash, author or
	// parents — so it gets its own header and goes straight to the changes.
	if c.WorkingTree {
		sb.WriteString(workingTreeStyle.Render("Uncommitted changes"))
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render(c.Message + " — not committed yet"))
		sb.WriteString("\n")
		return sb.String()
	}

	// SHA
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Hash)).Render("SHA:     "))
	sb.WriteString(commitHashStyle.Render(c.FullHash))
	sb.WriteString("\n")

	// Date
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Date)).Render("Date:    "))
	sb.WriteString(dateStyle.Render(c.Date.Format("2006-01-02 15:04:05")))
	sb.WriteString("\n")

	// Author
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Author)).Render("Author:  "))
	sb.WriteString(authorStyle.Render(c.Author))
	sb.WriteString("\n")

	// Parents
	if len(c.Parents) > 0 {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("Parents: "))
		sb.WriteString(strings.Join(c.Parents, ", "))
		sb.WriteString("\n")
	}

	// Refs
	if segs := refSegments(c); len(segs) > 0 {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Branch)).Render("Refs:    "))
		for i, seg := range segs {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(seg.style.Render(seg.text))
		}
		sb.WriteString("\n")

		// The graph has no room for a legend, so explain any tag marks here.
		if len(c.UnpushedTags) > 0 {
			sb.WriteString(helpStyle.Render("         " + tagLocalOnlyMark + " local only — not on any remote"))
			sb.WriteString("\n")
		}
		if len(c.RemoteOnlyTags) > 0 {
			sb.WriteString(helpStyle.Render("         " + tagRemoteOnlyMark + " remote only — not fetched"))
			sb.WriteString("\n")
		}
	}

	// A branch recovered from a merge commit has no ref of its own, so it would
	// otherwise appear in the graph but nowhere in the details.
	if c.MergedBranch != "" {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Branch)).Render("Branch:  "))
		sb.WriteString(mergedBranchStyle.Render(c.MergedBranch))
		sb.WriteString(helpStyle.Render(" (merged, deleted)"))
		sb.WriteString("\n")
	}

	// Commit message
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.SectionHeader)).Render("─── Message ───────────────────────"))
	sb.WriteString("\n")
	sb.WriteString(messageStyle.Render(c.Message))
	sb.WriteString("\n")

	return sb.String()
}

// renderDiffSections renders the stats and the diff itself, the part of the
// details panel that reads the same for a commit and for uncommitted changes.
func (m *model) renderDiffSections(c commit) string {
	// A graph filtered to a file is that file's history, and the commit's
	// part in it is what it did to that file. A commit that has no such file
	// to show — a merge, whose diff is not split by file — is shown whole.
	if only := m.filteredFiles(c.DiffFiles); c.DiffLoaded && len(only) > 0 {
		return m.renderFilteredDiff(c, only)
	}

	var sb strings.Builder

	// Diff stats
	if c.DiffLoaded && c.DiffStat != "" {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.SectionHeader)).Render("─── Stats ─────────────────────────"))
		sb.WriteString("\n")
		sb.WriteString(c.DiffStat)
		sb.WriteString("\n")
	}

	// Diff content
	if c.DiffLoaded && c.DiffBody != "" {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.SectionHeader)).Render("─── Diff ──────────────────────────"))
		sb.WriteString("\n")

		gutter := newDiffGutter(git.LineNumbers(c.DiffBody))
		for i, line := range strings.Split(c.DiffBody, "\n") {
			sb.WriteString(gutter.render(i))
			sb.WriteString(styleDiffLine(line, m.showWhitespace))
			sb.WriteString("\n")
		}
	} else if !c.DiffLoaded {
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("Loading diff..."))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderFilteredDiff is renderDiffSections for a graph filtered to a path:
// only the files the filter is about, each under its own name, with a line
// for how much else the commit changed so that the rest is not taken for
// absent. The commit view still lists every file.
func (m *model) renderFilteredDiff(c commit, only []fileDiff) string {
	section := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.SectionHeader))
	name := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.DiffHeader))

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(section.Render("─── Stats ─────────────────────────"))
	sb.WriteString("\n")
	for _, f := range only {
		sb.WriteString(f.Path + "  " + fileCounts(f) + "\n")
	}
	if others := len(c.DiffFiles) - len(only); others > 0 {
		sb.WriteString(helpStyle.Render(plural(others, "other file") + " in this commit — space lists them all"))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(section.Render("─── Diff ──────────────────────────"))
	sb.WriteString("\n")
	for i, f := range only {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(name.Render(f.Path))
		sb.WriteString("\n")
		if strings.TrimSpace(f.Body) == "" {
			sb.WriteString(helpStyle.Render("No textual change"))
			sb.WriteString("\n")
			continue
		}
		gutter := newDiffGutter(f.LineNumbers())
		for j, line := range strings.Split(f.Body, "\n") {
			sb.WriteString(gutter.render(j))
			sb.WriteString(styleDiffLine(line, m.showWhitespace))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// styleDiffLine colours one line of a diff by what git meant it to be. The
// "+++"/"---" headers start with the same characters as an added and a removed
// line and are neither, so they are told apart before the colouring.
//
// whitespace spells out the spaces and tabs of a line of the file, in the
// line's own colour between them; see whitespace.go. The column git puts in
// front of the line is left as it is: on a line that did not change it is a
// space, and is no part of the file.
func styleDiffLine(line string, whitespace bool) string {
	if prefix, content, ok := diffContent(line); ok && whitespace {
		style := lipgloss.NewStyle()
		switch prefix {
		case "+":
			style = style.Foreground(lipgloss.Color(theme.Current.DiffAdd))
		case "-":
			style = style.Foreground(lipgloss.Color(theme.Current.DiffDel))
		}
		return style.Render(prefix) + renderWhitespace(content, style)
	}
	switch {
	case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.DiffAdd)).Render(line)
	case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.DiffDel)).Render(line)
	case strings.HasPrefix(line, "@@"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.DiffHunk)).Render(line)
	case strings.HasPrefix(line, "diff "):
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.DiffHeader)).Render(line)
	}
	// The styled lines above had their tabs spelled out as they were rendered;
	// this one has to be given the same, or it is measured for cutting with a
	// tab counted as no width and then drawn four columns wider.
	return expandTabs(line)
}

// expandTabs spells a tab out as the four spaces lipgloss draws it as, so a
// line's width can be counted before it is drawn.
func expandTabs(line string) string {
	return strings.ReplaceAll(line, "\t", "    ")
}

// fitDetails applies the scroll offset and clips the panel's content to the
// room it has.
//
// lipgloss Height() only pads short content, it does NOT clip overflow, so
// without this the panel grows unbounded.
func (m *model) fitDetails(content string) (string, scrollMarks) {
	content = truncateLines(content, m.detailsContentWidth)
	allLines := strings.Split(content, "\n")
	total := textLines(allLines)

	// Clamp scroll
	if m.detailsScroll >= len(allLines) {
		m.detailsScroll = len(allLines) - 1
	}
	if m.detailsScroll < 0 {
		m.detailsScroll = 0
	}
	if m.detailsScroll > 0 {
		allLines = allLines[m.detailsScroll:]
	}

	// Truncate to available height inside the panel
	// Panel uses Height(contentHeight) with Padding(1,2) → 2 vertical padding lines
	maxLines := m.windowHeight - 8 - 2 // contentHeight minus vertical padding
	if maxLines < 3 {
		maxLines = 3
	}
	if len(allLines) > maxLines {
		allLines = allLines[:maxLines]
	}

	return strings.Join(allLines, "\n"), marksFor(total, m.detailsScroll, maxLines)
}
