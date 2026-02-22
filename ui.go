package main

import (
	"log"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// renderRepoInfo renders the top repository info box
func (m *model) renderRepoInfo() string {
	var sb strings.Builder

	// Repository name
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Title)).Render("Repository: "))
	sb.WriteString(m.repoName)
	sb.WriteString("  ")

	// Branch
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Branch)).Render("Branch: "))
	sb.WriteString(branchStyle.Render(m.currentBranch))
	sb.WriteString("  ")

	// Current commit
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Hash)).Render("Commit: "))
	sb.WriteString(commitHashStyle.Render(m.currentCommit))

	leftContent := sb.String()

	// Title on the right
	versionStr := "v" + version
	if m.latestVersion != "" && m.latestVersion != versionStr {
		// New version available
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
func (m *model) renderCommitList() string {
	log.Printf("renderCommitList: commits=%d, displayRows=%d, selected=%d, windowHeight=%d, maxGraphWidth=%d",
		len(m.commits), len(m.displayRows), m.selected, m.windowHeight, m.maxGraphWidth)

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

			if isSel {
				highlighted := strings.ReplaceAll(graphPadded, "●", "◉")
				sb.WriteString("> ")
				sb.WriteString(selGraphColor.Render(highlighted))
				sb.WriteString(" ")
				sb.WriteString(selHashStyle.Render(m.commits[row.CommitIdx].Hash))
			} else {
				sb.WriteString("  ")
				sb.WriteString(graphColor.Render(graphPadded))
				if isCommit {
					sb.WriteString(" ")
					sb.WriteString(commitHashStyle.Render(m.commits[row.CommitIdx].Hash))
				}
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
	if c.Refs != "" {
		sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Branch)).Render("Refs:    "))
		sb.WriteString(branchStyle.Render(c.Refs))
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
