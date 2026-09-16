package main

import (
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m model) View() (result string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in View: %v", r)
			result = fmt.Sprintf("\n  PANIC caught: %v\n\n  Check %s for details.\n  Press q to quit.", r, logFileName)
		}
	}()
	log.Printf("View: ready=%v, err=%v, commits=%d, displayRows=%d, window=%dx%d, focused=%d",
		m.ready, m.err, len(m.commits), len(m.displayRows), m.windowWidth, m.windowHeight, m.focusedBox)

	if !m.ready {
		return "\n  Initializing..."
	}

	// Guard against zero window dimensions (WindowSizeMsg not yet received)
	if m.windowWidth < 20 || m.windowHeight < 10 {
		log.Printf("View: window too small (%dx%d), waiting for resize", m.windowWidth, m.windowHeight)
		return "\n  Waiting for terminal size..."
	}

	if m.err != nil {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(currentTheme.Error)).
			Bold(true)
		return fmt.Sprintf("\n  %s\n\n  Error: %v\n\n  Press q to quit. Check %s for details.\n",
			errorStyle.Render("❌ Error loading repository"),
			m.err, logFileName)
	}

	help := m.renderStatusLine()

	// Border colors: active for focused, inactive for unfocused
	focusedBorderColor := lipgloss.Color(currentTheme.BorderActive)
	unfocusedBorderColor := lipgloss.Color(currentTheme.BorderInactive)
	box0Border := unfocusedBorderColor
	box1Border := unfocusedBorderColor
	box2Border := unfocusedBorderColor
	switch m.focusedBox {
	case 0:
		box0Border = focusedBorderColor
	case 1:
		box1Border = focusedBorderColor
	case 2:
		box2Border = focusedBorderColor
	}

	// Create repo info box - fixed Height(1) so it never changes size
	repoInfoContent := m.renderRepoInfo()
	repoInfoBox := addBoxLabel(lipgloss.NewStyle().
		Width(m.windowWidth-2).
		Height(1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(box0Border).
		Padding(0, 1).
		Render(repoInfoContent), "")

	// Calculate dimensions based on actual rendered box 0 height
	repoInfoHeight := lipgloss.Height(repoInfoBox) // should be 3 (1 content + 2 border)
	// Layout: repoInfoBox + \n + content panels (contentHeight + 2 border) + \n + help
	// Total = repoInfoHeight + 1 + contentHeight + 2 + 1 + 1 = repoInfoHeight + contentHeight + 5
	contentHeight := m.windowHeight - repoInfoHeight - 3

	if contentHeight < 3 {
		contentHeight = 3
	}

	layout := computePanelLayout(m.windowWidth, m.maxGraphWidth, m.maxBranchWidth, m.maxAuthorWidth)
	leftPanelWidth, rightPanelWidth := layout.leftWidth, layout.rightWidth

	log.Printf("View: leftPanelWidth=%d, rightPanelWidth=%d, contentHeight=%d, branchColWidth=%d, authorColWidth=%d",
		leftPanelWidth, rightPanelWidth, contentHeight, layout.branchCol, layout.authorCol)

	// Target height for both panels (content + 2 border lines)
	targetPanelHeight := contentHeight + 2

	// Create left panel (commit list). Content width is the panel minus its
	// borders (2) and horizontal padding (2).
	leftContent := m.renderCommitList(layout.branchCol, layout.authorCol, leftPanelWidth-4)
	leftPanel := addBoxLabel(lipgloss.NewStyle().
		Width(leftPanelWidth-2). // subtract borders (2); Width includes padding
		Height(contentHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(box1Border).
		Padding(0, 1).
		Render(leftContent), "[1]-git-graph")

	// Create right panel (commit details)
	// Padding(1,2) → 2*2=4 horizontal padding + 2 borders = 6 overhead
	m.detailsContentWidth = rightPanelWidth - 6
	rightContent := m.renderCommitDetails()
	rightPanel := addBoxLabel(lipgloss.NewStyle().
		Width(rightPanelWidth-2). // subtract borders (2); Width includes padding
		Height(contentHeight).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(box2Border).
		Padding(1, 2).
		Render(rightContent), "[2]-commit-details")

	// Force both panels to exactly the same height.
	// lipgloss Height() is a minimum, not a maximum — long lines that wrap
	// inside the panel can make it taller. Trim any excess lines from either panel.
	leftPanel = trimToHeight(leftPanel, targetPanelHeight)
	rightPanel = trimToHeight(rightPanel, targetPanelHeight)

	// Join panels horizontally
	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	output := fmt.Sprintf("%s\n%s\n%s", repoInfoBox, content, help)

	// Force exact windowHeight lines. We count lines via lipgloss.Height which
	// correctly handles ANSI escape sequences, then trim or pad as needed.
	actualHeight := lipgloss.Height(output)
	log.Printf("View: actualHeight=%d, windowHeight=%d", actualHeight, m.windowHeight)

	if actualHeight > m.windowHeight {
		// Trim from the bottom
		lines := strings.Split(output, "\n")
		output = strings.Join(lines[:m.windowHeight], "\n")
	} else if actualHeight < m.windowHeight {
		// Pad bottom with empty lines
		for i := actualHeight; i < m.windowHeight; i++ {
			output += "\n"
		}
	}

	return output
}

const (
	minRightPanelWidth = 30
	// Beyond this an author name buys little and costs graph width on every row.
	maxAuthorColWidth = 24
	// Below this a name is truncated past recognition, so the column is dropped.
	minAuthorColWidth = 6
)

// panelLayout is how the window's width is shared between the commit list and
// the details panel, and within the list between its optional columns.
type panelLayout struct {
	leftWidth, rightWidth int
	branchCol, authorCol  int
}

// computePanelLayout sizes the panels. The graph always comes first; the room
// left beside it goes to branch labels, then to authors, because refs say where
// you are, which matters more than who wrote each commit.
func computePanelLayout(windowWidth, maxGraphWidth, maxBranchWidth, maxAuthorWidth int) panelLayout {
	// Base graph needs: 2 (selection "> ") + maxGraphWidth + 1 (space) +
	// 7 (hash) + borders(2) + padding(2) = maxGraphWidth + 14
	graphBase := maxGraphWidth + 14
	maxLeftWidth := windowWidth * 4 / 5

	var l panelLayout
	if graphBase > maxLeftWidth {
		// graph alone is wider than our normal cap; give it the full window
		l.leftWidth = windowWidth
	} else {
		// Room beside the graph must respect both the left panel's cap and the
		// details panel's minimum.
		room := min(windowWidth-graphBase-minRightPanelWidth, maxLeftWidth-graphBase)
		if maxBranchWidth > 0 && room > 1 {
			l.branchCol = min(maxBranchWidth, room-1) // -1: the space after the labels
			room -= l.branchCol + 1
		}
		if maxAuthorWidth > 0 && room > 1 {
			l.authorCol = min(maxAuthorWidth, maxAuthorColWidth, room-1) // -1: the space before the name
			if l.authorCol < minAuthorColWidth {
				l.authorCol = 0
			}
		}

		l.leftWidth = graphBase
		if l.branchCol > 0 {
			l.leftWidth += l.branchCol + 1
		}
		if l.authorCol > 0 {
			l.leftWidth += l.authorCol + 1
		}
		l.leftWidth = max(l.leftWidth, 25)
	}

	l.rightWidth = windowWidth - l.leftWidth // fill remaining space

	// Ensure right panel has a minimum width, but never let total exceed window
	if l.rightWidth < minRightPanelWidth {
		l.rightWidth = minRightPanelWidth
		l.leftWidth = windowWidth - l.rightWidth
		if l.leftWidth < 15 {
			l.leftWidth = 15
			l.rightWidth = windowWidth - l.leftWidth
		}
	}

	// Final safety: total must not exceed window width
	if total := l.leftWidth + l.rightWidth; total > windowWidth {
		log.Printf("View: width overflow detected: left=%d + right=%d = %d > window=%d, adjusting",
			l.leftWidth, l.rightWidth, total, windowWidth)
		l.rightWidth = windowWidth - l.leftWidth
		if l.rightWidth < 10 {
			l.rightWidth = windowWidth / 3
			l.leftWidth = windowWidth - l.rightWidth
		}
	}

	return l
}

// renderStatusLine renders the bottom line: the update prompt or notice when one
// is pending, otherwise the usual key help. Sharing the single line keeps the
// height arithmetic in View() unchanged.
func (m *model) renderStatusLine() string {
	noticeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Tag))

	if m.updateState == updateConfirming {
		return truncateLines(noticeStyle.Render(
			fmt.Sprintf("Update v%s → %s? This replaces the binary and quits. (y/n)", version, m.latestVersion)),
			m.windowWidth)
	}
	if m.updateMessage != "" {
		return truncateLines(noticeStyle.Render(m.updateMessage), m.windowWidth)
	}

	help := "1/2: focus box • tab/shift+tab: cycle boxes • ↑/↓/j/k: scroll • d/u: half page • g/G: top/bottom • q/esc: quit"
	if m.updateAvailable() {
		// Leads rather than trails: the line is already near a typical terminal's
		// width, so a trailing hint is the first thing truncation eats.
		help = "U: update to " + m.latestVersion + " • " + help
	}
	return truncateLines(helpStyle.Render(help), m.windowWidth)
}

// renderRepoInfo renders the top repository info box
func (m *model) renderRepoInfo() string {
	var sb strings.Builder

	// Repository name
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Title)).Render("Repository: "))
	sb.WriteString(m.repoName)
	sb.WriteString("  ")

	// Branch
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Branch)).Render("Branch: "))
	sb.WriteString(localBranchStyle.Render(m.currentBranch))
	sb.WriteString("  ")

	// Current commit
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Hash)).Render("Commit: "))
	sb.WriteString(commitHashStyle.Render(m.currentCommit))

	leftContent := sb.String()

	// Title on the right
	versionStr := "v" + version
	if m.updateAvailable() {
		versionStr = versionStr + " → " + m.latestVersion + " available"
	}
	title := titleStyle.Render("🦒 " + appName + " - Git Graph Viewer (" + versionStr + ")")

	// Calculate available width for content (subtract borders and padding)
	availableWidth := m.windowWidth - 2 - 2 // borders (2) + padding (2)
	leftWidth := lipgloss.Width(leftContent)
	rightWidth := lipgloss.Width(title)

	// Add spacing to push title to the right
	spacing := availableWidth - leftWidth - rightWidth
	if spacing < 1 {
		spacing = 1
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, leftContent, strings.Repeat(" ", spacing), title)
}

// renderCommitList renders the left panel with the commit list/graph
// branchColWidth and authorColWidth are the columns computePanelLayout
// allocated for this layout pass (0 hides a column); contentWidth is the width
// inside the panel's borders and padding.
func (m *model) renderCommitList(branchColWidth, authorColWidth, contentWidth int) string {
	log.Printf("renderCommitList: commits=%d, displayRows=%d, selected=%d, windowHeight=%d, maxGraphWidth=%d, branchColWidth=%d",
		len(m.commits), len(m.displayRows), m.selected, m.windowHeight, m.maxGraphWidth, branchColWidth)

	if len(m.commits) == 0 {
		return "No commits found"
	}

	var sb strings.Builder

	// Calculate visible range based on window height
	// Must match the contentHeight from View(): windowHeight - 8
	visibleHeight := m.windowHeight - 8
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	log.Printf("renderCommitList: visibleHeight=%d", visibleHeight)

	graphColor := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.Graph))
	selGraphColor := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.SelectedFg)).Bold(true)
	selHashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.SelectedFg)).Bold(true)

	if len(m.displayRows) > 0 {
		// Graph mode: use displayRows from git log --graph

		// The author column sits against the panel's right edge, so the gap in
		// front of it absorbs any slack width and names line up on every row.
		authorGap := 0
		if authorColWidth > 0 {
			labelWidth := 0
			if branchColWidth > 0 {
				labelWidth = branchColWidth + 1
			}
			// "> " + labels + graph + " " + 7-char hash
			rowWidth := 2 + labelWidth + m.maxGraphWidth + 1 + 7
			authorGap = contentWidth - authorColWidth - rowWidth
		}

		// Find the display row index of the selected commit
		selectedRowIdx := 0
		for i, row := range m.displayRows {
			if row.CommitIdx == m.selected {
				selectedRowIdx = i
				break
			}
		}
		log.Printf("renderCommitList graph mode: selectedRowIdx=%d", selectedRowIdx)

		// Scroll to keep selected row visible
		// Use a stable scroll offset that only changes when the selected row
		// would move outside the visible window (like a typical text editor).
		startIdx := selectedRowIdx - visibleHeight/3
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := startIdx + visibleHeight
		if endIdx > len(m.displayRows) {
			endIdx = len(m.displayRows)
			startIdx = endIdx - visibleHeight
			if startIdx < 0 {
				startIdx = 0
			}
		}
		log.Printf("renderCommitList graph mode: startIdx=%d, endIdx=%d", startIdx, endIdx)

		linesWritten := 0
		for i := startIdx; i < endIdx; i++ {
			row := m.displayRows[i]
			isCommit := row.CommitIdx >= 0
			isSel := isCommit && row.CommitIdx == m.selected

			// Bounds check before accessing commits slice
			if isCommit && (row.CommitIdx < 0 || row.CommitIdx >= len(m.commits)) {
				log.Printf("renderCommitList ERROR: row %d has out-of-bounds CommitIdx=%d (len(commits)=%d), skipping",
					i, row.CommitIdx, len(m.commits))
				sb.WriteString("\n")
				continue
			}

			// Pad graph to max width for alignment
			padLen := m.maxGraphWidth - row.GraphWidth
			if padLen < 0 {
				padLen = 0
			}
			graphPadded := row.GraphChars + strings.Repeat(" ", padLen)

			// Branch / tag / merged-branch label column
			var segments []labelSegment
			if isCommit {
				segments = labelSegments(m.commits[row.CommitIdx])
			}

			// Helper to render the label column, truncated to the width this
			// layout pass allows and padded so the column stays aligned.
			renderBranchLabel := func() {
				if branchColWidth <= 0 {
					return
				}
				used := 0
				for i, seg := range segments {
					if i > 0 {
						if used+2 > branchColWidth {
							break
						}
						sb.WriteString(", ")
						used += 2
					}
					// Truncate to runes, not bytes
					text := seg.text
					if r := []rune(text); len(r) > branchColWidth-used {
						text = string(r[:branchColWidth-used])
					}
					if text == "" {
						break
					}
					sb.WriteString(seg.style.Render(text))
					used += utf8.RuneCountInString(text)
				}
				if pad := branchColWidth - used; pad > 0 {
					sb.WriteString(strings.Repeat(" ", pad))
				}
				sb.WriteString(" ")
			}

			if isSel {
				highlighted := strings.ReplaceAll(graphPadded, "●", "◉")
				sb.WriteString("> ")
				renderBranchLabel()
				sb.WriteString(selGraphColor.Render(highlighted))
				sb.WriteString(" ")
				sb.WriteString(selHashStyle.Render(m.commits[row.CommitIdx].Hash))
			} else {
				sb.WriteString("  ")
				renderBranchLabel()
				sb.WriteString(graphColor.Render(graphPadded))
				if isCommit {
					sb.WriteString(" ")
					sb.WriteString(commitHashStyle.Render(m.commits[row.CommitIdx].Hash))
				}
			}
			// A gap under 1 means the row doesn't fit as computed; drawing the
			// name anyway would wrap the row and break the panel's layout.
			if isCommit && authorColWidth > 0 && authorGap >= 1 {
				sb.WriteString(strings.Repeat(" ", authorGap))
				sb.WriteString(authorStyle.Render(ansi.Truncate(m.commits[row.CommitIdx].Author, authorColWidth, "…")))
			}
			sb.WriteString("\n")
			linesWritten++
		}
		// Pad to exactly visibleHeight lines so the panel never changes size
		for linesWritten < visibleHeight {
			sb.WriteString("\n")
			linesWritten++
		}
	} else {
		// Simple mode: one row per commit with basic symbol (fallback)
		startIdx := 0
		if m.selected >= visibleHeight {
			startIdx = m.selected - visibleHeight + 1
		}
		endIdx := startIdx + visibleHeight
		if endIdx > len(m.commits) {
			endIdx = len(m.commits)
		}

		linesWritten := 0
		for i := startIdx; i < endIdx; i++ {
			c := m.commits[i]

			if i == m.selected {
				sb.WriteString("> ")
				sb.WriteString(selGraphColor.Render(c.GraphLine))
				sb.WriteString(" ")
				sb.WriteString(selHashStyle.Render(c.Hash))
			} else {
				sb.WriteString("  ")
				sb.WriteString(graphColor.Render(c.GraphLine))
				sb.WriteString(" ")
				sb.WriteString(commitHashStyle.Render(c.Hash))
			}
			sb.WriteString("\n")
			linesWritten++
		}
		for linesWritten < visibleHeight {
			sb.WriteString("\n")
			linesWritten++
		}
	}

	// Truncate to available height inside the panel.
	// lipgloss Height() does NOT clip overflow.
	// Panel uses Height(contentHeight) with Padding(0,1) → 0 vertical padding.
	result := sb.String()
	resultLines := strings.Split(result, "\n")
	maxLines := m.windowHeight - 8
	if maxLines < 3 {
		maxLines = 3
	}
	if len(resultLines) > maxLines {
		resultLines = resultLines[:maxLines]
	}
	return strings.Join(resultLines, "\n")
}

// labelSegment is one styled entry in the branch/tag label column.
type labelSegment struct {
	text  string
	style lipgloss.Style
}

// refSegments lists a commit's refs: local branches, remote-tracking branches,
// then tags. Tag sync state is shown as a mark rather than a colour, because
// colour already says "this is a tag".
func refSegments(c commit) []labelSegment {
	local, remote, tags := parseRefs(c.Refs)
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

// truncateLines truncates each line of s to maxWidth visible characters,
// correctly handling ANSI escape sequences.
func truncateLines(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if ansi.StringWidth(line) > maxWidth {
			lines[i] = ansi.Truncate(line, maxWidth, "")
		}
	}
	return strings.Join(lines, "\n")
}

// renderCommitDetails renders the right panel with commit details and diff
func (m *model) renderCommitDetails() string {
	log.Printf("renderCommitDetails: selected=%d, len(commits)=%d", m.selected, len(m.commits))
	if len(m.commits) == 0 || m.selected < 0 || m.selected >= len(m.commits) {
		log.Printf("renderCommitDetails: skipping (empty or out of bounds)")
		return ""
	}

	c := m.commits[m.selected]

	var sb strings.Builder

	// SHA
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Hash)).Render("SHA:     "))
	sb.WriteString(commitHashStyle.Render(c.FullHash))
	sb.WriteString("\n")

	// Date
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Date)).Render("Date:    "))
	sb.WriteString(dateStyle.Render(c.Date.Format("2006-01-02 15:04:05")))
	sb.WriteString("\n")

	// Author
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Author)).Render("Author:  "))
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
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Branch)).Render("Refs:    "))
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
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Branch)).Render("Branch:  "))
		sb.WriteString(mergedBranchStyle.Render(c.MergedBranch))
		sb.WriteString(helpStyle.Render(" (merged, deleted)"))
		sb.WriteString("\n")
	}

	// Commit message
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.SectionHeader)).Render("─── Message ───────────────────────"))
	sb.WriteString("\n")
	sb.WriteString(messageStyle.Render(c.Message))
	sb.WriteString("\n")

	// Diff stats
	if c.DiffLoaded && c.DiffStat != "" {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.SectionHeader)).Render("─── Stats ─────────────────────────"))
		sb.WriteString("\n")
		sb.WriteString(c.DiffStat)
		sb.WriteString("\n")
	}

	// Diff content
	if c.DiffLoaded && c.DiffBody != "" {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.SectionHeader)).Render("─── Diff ──────────────────────────"))
		sb.WriteString("\n")

		addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.DiffAdd))
		delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.DiffDel))
		hunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.DiffHunk))
		diffHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.DiffHeader))

		for _, line := range strings.Split(c.DiffBody, "\n") {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				sb.WriteString(addStyle.Render(line))
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				sb.WriteString(delStyle.Render(line))
			} else if strings.HasPrefix(line, "@@") {
				sb.WriteString(hunkStyle.Render(line))
			} else if strings.HasPrefix(line, "diff ") {
				sb.WriteString(diffHeaderStyle.Render(line))
			} else {
				sb.WriteString(line)
			}
			sb.WriteString("\n")
		}
	} else if !c.DiffLoaded {
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("Loading diff..."))
		sb.WriteString("\n")
	}

	// Apply scroll offset and truncate to fit panel height.
	// lipgloss Height() only pads short content, it does NOT clip overflow,
	// so we must truncate here to prevent the panel from growing unbounded.
	content := truncateLines(sb.String(), m.detailsContentWidth)
	allLines := strings.Split(content, "\n")

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

	return strings.Join(allLines, "\n")
}

// trimToHeight ensures a rendered string is exactly targetHeight lines.
// If taller, excess lines are removed from the bottom (preserving the bottom border).
// If shorter, empty lines are appended.
func trimToHeight(rendered string, targetHeight int) string {
	lines := strings.Split(rendered, "\n")
	// Remove trailing empty string from split if present
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > targetHeight {
		// Keep first line (top border), middle content up to targetHeight-2, and last line (bottom border)
		top := lines[0]
		bottom := lines[len(lines)-1]
		middle := lines[1 : targetHeight-1]
		result := make([]string, 0, targetHeight)
		result = append(result, top)
		result = append(result, middle...)
		result = append(result, bottom)
		return strings.Join(result, "\n")
	}
	for len(lines) < targetHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// addBoxLabel overlays a label like [0] onto the top-left corner of a rendered box border.
// It accounts for ANSI escape sequences so it only replaces visible border characters.
func addBoxLabel(rendered string, label string) string {
	lines := strings.SplitN(rendered, "\n", 2)
	if len(lines) == 0 {
		return rendered
	}
	topLine := lines[0]
	labelRunes := []rune(label)

	// Walk through the top line, skipping ANSI escape sequences,
	// and replace visible characters at positions 1..len(label) (after the corner char).
	var result strings.Builder
	visibleIdx := 0
	labelIdx := 0
	runes := []rune(topLine)
	for i := 0; i < len(runes); i++ {
		// Detect ANSI escape sequence: ESC [ params final_byte
		if runes[i] == '\033' && i+1 < len(runes) && runes[i+1] == '[' {
			// Copy ESC and [ first
			result.WriteRune(runes[i]) // \033
			i++
			result.WriteRune(runes[i]) // [
			i++
			// Now copy parameter/intermediate bytes until final byte (0x40-0x7E)
			for i < len(runes) {
				result.WriteRune(runes[i])
				if runes[i] >= 0x40 && runes[i] <= 0x7E {
					break
				}
				i++
			}
			continue
		}
		// This is a visible character
		if visibleIdx >= 1 && labelIdx < len(labelRunes) {
			result.WriteRune(labelRunes[labelIdx])
			labelIdx++
		} else {
			result.WriteRune(runes[i])
		}
		visibleIdx++
	}

	lines[0] = result.String()
	if len(lines) > 1 {
		return lines[0] + "\n" + lines[1]
	}
	return lines[0]
}
