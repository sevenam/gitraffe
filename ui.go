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
	// Deferred so every screen View returns gets the background, including
	// the loading and error ones.
	defer func() { result = paintBackground(result, m.windowWidth) }()
	log.Printf("View: ready=%v, err=%v, commits=%d, displayRows=%d, window=%dx%d, focused=%d",
		m.ready, m.err, len(m.commits), len(m.displayRows), m.windowWidth, m.windowHeight, m.focusedBox)

	if !m.ready {
		// Named, since after a switch it is not obvious which repository is
		// loading.
		return "\n  Opening " + displayPath(m.repoPath) + "..."
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
		screen := fmt.Sprintf("\n  %s\n\n  Error: %v\n\n  Press o to open another repository, or q to quit. Check %s for details.\n",
			errorStyle.Render("❌ Error loading repository"),
			m.err, logFileName)
		// Wrapped rather than left to the terminal: the log path makes the last
		// line long, and the switcher is spliced in by column.
		screen = lipgloss.NewStyle().Width(m.windowWidth).Render(screen)
		// The switcher is the way out of a folder that isn't a repository, so it
		// has to be drawable here too.
		if m.switcher.open {
			screen = overlayCentre(screen, m.switcher.render(m.windowWidth, m.windowHeight), m.windowWidth, m.windowHeight)
		}
		return screen
	}

	// The commit view is a screen, not an overlay: it replaces the graph
	// rather than covering it, so nothing below here runs while it is open.
	if m.commitView.open {
		screen := m.renderCommitView()
		if m.showHelp {
			screen = overlayCentre(screen, renderHelpSections(commitViewHelp), m.windowWidth, m.windowHeight)
		}
		return screen
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

	layout := m.currentLayout()
	leftPanelWidth, rightPanelWidth := layout.leftWidth, layout.rightWidth

	log.Printf("View: leftPanelWidth=%d, rightPanelWidth=%d, contentHeight=%d, branchColWidth=%d, dateColWidth=%d, authorColWidth=%d",
		leftPanelWidth, rightPanelWidth, contentHeight, layout.branchCol, layout.dateCol, layout.authorCol)

	// Target height for both panels (content + 2 border lines)
	targetPanelHeight := contentHeight + 2

	// Create left panel (commit list). Content width is the panel minus its
	// borders (2) and horizontal padding (2).
	var leftPanel, rightPanel string
	if leftPanelWidth > 0 {
		leftContent := m.renderCommitList(layout, leftPanelWidth-4)
		leftPanel = addBoxLabel(lipgloss.NewStyle().
			Width(leftPanelWidth-2). // subtract borders (2); Width includes padding
			Height(contentHeight).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(box1Border).
			Padding(0, 1).
			Render(leftContent), "[1]-git-graph")
		// lipgloss Height() is a minimum, not a maximum — long lines that wrap
		// inside the panel can make it taller. Trim any excess lines so both
		// panels are exactly the same height.
		leftPanel = trimToHeight(leftPanel, targetPanelHeight)
	}

	// Create right panel (commit details)
	// Padding(1,2) → 2*2=4 horizontal padding + 2 borders = 6 overhead
	if rightPanelWidth > 0 {
		m.detailsContentWidth = rightPanelWidth - 6
		rightContent := m.renderCommitDetails()
		rightPanel = addBoxLabel(lipgloss.NewStyle().
			Width(rightPanelWidth-2). // subtract borders (2); Width includes padding
			Height(contentHeight).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(box2Border).
			Padding(1, 2).
			Render(rightContent), "[2]-commit-details")
		rightPanel = trimToHeight(rightPanel, targetPanelHeight)
	}

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

	if m.showHelp {
		output = overlayCentre(output, renderHelpBox(), m.windowWidth, m.windowHeight)
	}
	if m.picker.open {
		// 10 rows go to the box's title, footer, spacing, padding and border, so
		// the list scrolls rather than being clipped off the screen.
		box := m.picker.render(m.windowHeight - 10)
		output = overlayCentre(output, box, m.windowWidth, m.windowHeight)
	}
	if m.refs.open {
		// The same room the theme list leaves for its title, footer and border.
		output = overlayCentre(output, m.refs.render(m.windowHeight-10), m.windowWidth, m.windowHeight)
	}
	if m.switcher.open {
		output = overlayCentre(output, m.switcher.render(m.windowWidth, m.windowHeight), m.windowWidth, m.windowHeight)
	}

	return output
}

const (
	// Date only, fixed width so the column aligns and reads the same in every
	// locale. Relative dates ("3 days ago") vary in width and go stale, since
	// the view isn't redrawn on a timer. The time is in the details panel.
	dateColumnFormat = "2006-01-02"

	minRightPanelWidth = 30
	// Beyond this an author name buys little and costs graph width on every row.
	maxAuthorColWidth = 24
	// Below this a name is truncated past recognition, so the column is dropped.
	minAuthorColWidth = 6
	// Below this a subject is cut so short it says less than the space it took.
	minMessageColWidth = 12
)

// panelLayout is how the window's width is shared between the commit list and
// the details panel, and within the list between its optional columns.
type panelLayout struct {
	leftWidth, rightWidth         int
	branchCol, dateCol, authorCol int
	// messageCol is the subject line at the end of each row, and only the
	// maximised graph has one: beside the details panel the message is already
	// on screen, and the room is better spent on the graph.
	messageCol int
}

// computePanelLayout sizes the panels. The graph always comes first; the room
// left beside it goes to branch labels, then dates, then authors. Refs say where
// you are, which matters most; a date is short and fixed, so it is cheaper to
// keep than a name. dateWidth is 0 when no date column is wanted.
func computePanelLayout(windowWidth, maxGraphWidth, maxBranchWidth, dateWidth, maxAuthorWidth int) panelLayout {
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
		l.branchCol, l.dateCol, l.authorCol = allocateColumns(room, maxBranchWidth, dateWidth, maxAuthorWidth)

		l.leftWidth = graphBase
		if l.branchCol > 0 {
			l.leftWidth += l.branchCol + 1
		}
		if l.dateCol > 0 {
			l.leftWidth += l.dateCol + 1
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

// currentLayout is how the window is divided right now. View draws from it,
// and the mouse reads it to work out which panel the pointer is over — the
// division has to be the one on screen, so both ask the same question.
func (m model) currentLayout() panelLayout {
	// Only graph mode draws the date column; the fallback list has no room for it.
	dateWidth := 0
	if len(m.displayRows) > 0 {
		dateWidth = len(dateColumnFormat)
	}
	// Maximised, the focused panel takes the window and the other is not drawn
	// at all; a width of 0 is what View reads to leave it out.
	switch {
	case m.maximised && m.focusedBox == 2:
		return panelLayout{rightWidth: m.windowWidth}
	case m.maximised:
		return computeMaximisedLayout(m.windowWidth, m.maxGraphWidth, m.maxBranchWidth, dateWidth, m.maxAuthorWidth)
	}
	return computePanelLayout(m.windowWidth, m.maxGraphWidth, m.maxBranchWidth, dateWidth, m.maxAuthorWidth)
}

// allocateColumns shares the room beside the graph between the optional
// columns, in priority order.
func allocateColumns(room, maxBranchWidth, dateWidth, maxAuthorWidth int) (branchCol, dateCol, authorCol int) {
	if maxBranchWidth > 0 && room > 1 {
		branchCol = min(maxBranchWidth, room-1) // -1: the space after the labels
		room -= branchCol + 1
	}
	// A cut-off date is useless, so it fits whole (plus its leading space) or
	// not at all.
	if dateWidth > 0 && room > dateWidth {
		dateCol = dateWidth
		room -= dateWidth + 1
	}
	// Columns drop strictly in reverse priority as the window narrows. Letting
	// a short name take room a date couldn't fit would make widening the
	// window swap the author column for the date column.
	dateSettled := dateWidth == 0 || dateCol > 0
	if maxAuthorWidth > 0 && room > 1 && dateSettled {
		authorCol = min(maxAuthorWidth, maxAuthorColWidth, room-1) // -1: the space before the name
		if authorCol < minAuthorColWidth {
			authorCol = 0
		}
	}
	return branchCol, dateCol, authorCol
}

// computeMaximisedLayout sizes the graph when it has the window to itself.
// There is no details panel to leave room for, so the columns get everything
// beside the graph and none of the usual caps apply.
func computeMaximisedLayout(windowWidth, maxGraphWidth, maxBranchWidth, dateWidth, maxAuthorWidth int) panelLayout {
	l := panelLayout{leftWidth: windowWidth}
	l.branchCol, l.dateCol, l.authorCol = allocateColumns(windowWidth-(maxGraphWidth+14), maxBranchWidth, dateWidth, maxAuthorWidth)
	// Whatever is left over goes to the message, but only if enough is left to
	// read: a few characters and an ellipsis say less than the empty space did.
	if spare := windowWidth - 4 - rowWidth(l, maxGraphWidth) - 1; spare >= minMessageColWidth {
		l.messageCol = spare
	}
	return l
}

// rowWidth is how much of a row the columns before the message take:
// "> " + labels + graph + " " + 7-char hash [+ " " + date] [+ " " + author].
func rowWidth(l panelLayout, maxGraphWidth int) int {
	w := 2 + maxGraphWidth + 1 + 7
	for _, col := range []int{l.branchCol, l.dateCol, l.authorCol} {
		if col > 0 {
			w += col + 1
		}
	}
	return w
}

// renderStatusLine renders the bottom line: the update prompt or notice when one
// is pending, otherwise the usual key help. Sharing the single line keeps the
// height arithmetic in View() unchanged.
func (m *model) renderStatusLine() string {
	noticeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Tag))

	// The search prompt takes the line while it is open: it is where you are
	// typing, so nothing else on it could be read anyway.
	if m.search.active {
		return m.renderSearchPrompt()
	}

	if m.updateState == updateConfirming {
		return truncateLines(noticeStyle.Render(
			fmt.Sprintf("Update v%s → %s? This replaces the binary and quits. (y/n)", version, m.latestVersion)),
			m.windowWidth)
	}
	if m.updateMessage != "" {
		return truncateLines(noticeStyle.Render(m.updateMessage), m.windowWidth)
	}
	// Ahead of the notice: a fetch is the one thing that keeps running after
	// the key that started it, so the line has to say it is still going.
	if m.fetching {
		return truncateLines(noticeStyle.Render("Fetching from the remote..."), m.windowWidth)
	}
	if m.notice != "" {
		return truncateLines(noticeStyle.Render(m.notice), m.windowWidth)
	}

	// Only the keys needed to get around; "?" lists the rest. The line has to
	// fit a typical terminal, and "?" leads so truncation never hides the way
	// to find everything else.
	help := "?: help • enter: details • space: diff • r: reload • tab: cycle • ↑/↓/j/k: scroll • q: quit"
	if m.updateAvailable() {
		// Leads rather than trails: the line is already near a typical terminal's
		// width, so a trailing hint is the first thing truncation eats.
		help = "U: update to " + m.latestVersion + " • " + help
	}
	return truncateLines(helpStyle.Render(help), m.windowWidth)
}

// syncLabel renders how the current branch differs from its upstream, e.g.
// "↑3 ↓1", each direction in its own colour. In sync or unknown renders nothing,
// so only a difference draws the eye — the same convention as the tag sync marks.
func syncLabel(ahead, behind int) string {
	var parts []string
	if ahead > 0 {
		parts = append(parts, aheadStyle.Render(fmt.Sprintf("↑%d", ahead)))
	}
	if behind > 0 {
		parts = append(parts, behindStyle.Render(fmt.Sprintf("↓%d", behind)))
	}
	return strings.Join(parts, " ")
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
	if s := syncLabel(m.ahead, m.behind); s != "" {
		sb.WriteString(" ")
		sb.WriteString(s)
	}
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

// visibleGraphRows is how many rows of the graph panel hold commits. It has to
// match the panel height View works out, or the panel would change size as the
// list scrolls.
func (m model) visibleGraphRows() int {
	return max(1, m.windowHeight-8)
}

// graphWindow is the half-open range of rows the graph panel draws, indexing
// displayRows -- or commits, in the fallback mode that has no graph rows. The
// graph keeps no scroll offset of its own: the window follows the selection,
// moving only when the selected row would leave it, the way a text editor
// scrolls to its cursor.
//
// The renderer and the mouse both ask here rather than working it out apiece:
// a click that disagreed with the drawing by one row would quietly select the
// commit above the one pointed at.
func (m model) graphWindow() (start, end int) {
	visible := m.visibleGraphRows()
	if len(m.displayRows) == 0 {
		// The fallback keeps the selection on the last row instead.
		if m.selected >= visible {
			start = m.selected - visible + 1
		}
		return start, min(start+visible, len(m.commits))
	}
	selectedRow := 0
	for i, row := range m.displayRows {
		if row.CommitIdx == m.selected {
			selectedRow = i
			break
		}
	}
	// A third of the way down, so there is history visible either side of the
	// selection after a jump.
	start = max(0, selectedRow-visible/3)
	end = start + visible
	if end > len(m.displayRows) {
		end = len(m.displayRows)
		start = max(0, end-visible)
	}
	return start, end
}

// renderCommitList renders the left panel with the commit list/graph
// layout holds the column widths computePanelLayout allocated for this pass
// (0 hides a column); contentWidth is the width inside the panel's borders
// and padding.
func (m *model) renderCommitList(layout panelLayout, contentWidth int) string {
	log.Printf("renderCommitList: commits=%d, displayRows=%d, selected=%d, windowHeight=%d, maxGraphWidth=%d, layout.branchCol=%d",
		len(m.commits), len(m.displayRows), m.selected, m.windowHeight, m.maxGraphWidth, layout.branchCol)

	if len(m.commits) == 0 {
		return "No commits found"
	}

	var sb strings.Builder

	visibleHeight := m.visibleGraphRows()
	log.Printf("renderCommitList: visibleHeight=%d", visibleHeight)

	graphColor := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.Graph))
	lanePalette := buildLanePalette()
	selGraphColor := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.SelectedFg)).Bold(true)
	selHashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.SelectedFg)).Bold(true)
	selectedBg := lipgloss.Color(currentTheme.SelectedBg)
	plainStyle := lipgloss.NewStyle()

	if len(m.displayRows) > 0 {
		// Graph mode: use displayRows from git log --graph

		// The author column sits against the panel's right edge, so the gap in
		// front of it absorbs any slack width and names line up on every row.
		// With a message after it there is no slack to absorb: the message
		// takes it, and one space separates the two.
		authorGap := 0
		if layout.authorCol > 0 {
			withoutAuthor := layout
			withoutAuthor.authorCol = 0
			authorGap = contentWidth - layout.authorCol - rowWidth(withoutAuthor, m.maxGraphWidth)
			if layout.messageCol > 0 {
				authorGap = 1
			}
		}

		startIdx, endIdx := m.graphWindow()
		log.Printf("renderCommitList graph mode: startIdx=%d, endIdx=%d", startIdx, endIdx)

		linesWritten := 0
		for i := startIdx; i < endIdx; i++ {
			row := m.displayRows[i]
			// A note is text rather than graph: no lanes, no columns.
			if row.Note != "" {
				sb.WriteString(helpStyle.Render("  " + ansi.Truncate(row.Note, contentWidth-2, "…")))
				sb.WriteString("\n")
				linesWritten++
				continue
			}
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

			// write renders one piece of the row. On the selected row every piece,
			// spaces included, carries the highlight band's background: each styled
			// piece ends in an SGR reset, so a background wrapped around the whole
			// row would be cleared after its first piece.
			rowStart := sb.Len()
			write := func(style lipgloss.Style, s string) {
				if isSel {
					style = style.Background(selectedBg)
				}
				sb.WriteString(style.Render(s))
			}

			// writeGraph draws the glyphs, each in its lane colour when lane
			// colouring is on. Characters sharing a lane go out as one piece:
			// write() re-applies the row styling per piece, so a piece per
			// character would multiply escape sequences on every row.
			writeGraph := func(s string, lanes []int) {
				if !m.colourLanes {
					write(graphColor, s)
					return
				}
				runes := []rune(s)
				start, lane := 0, -1
				for i := range runes {
					at := 0
					if i < len(lanes) {
						at = lanes[i]
					}
					if i > 0 && at != lane {
						write(laneStyle(lane, graphColor, lanePalette), string(runes[start:i]))
						start = i
					}
					lane = at
				}
				if len(runes) > 0 {
					write(laneStyle(lane, graphColor, lanePalette), string(runes[start:]))
				}
			}

			// Helper to render the label column, truncated to the width this
			// layout pass allows and padded so the column stays aligned.
			renderBranchLabel := func() {
				if layout.branchCol <= 0 {
					return
				}
				used := 0
				for i, seg := range segments {
					if i > 0 {
						if used+2 > layout.branchCol {
							break
						}
						write(plainStyle, ", ")
						used += 2
					}
					// Truncate to runes, not bytes
					text := seg.text
					if r := []rune(text); len(r) > layout.branchCol-used {
						text = string(r[:layout.branchCol-used])
					}
					if text == "" {
						break
					}
					write(seg.style, text)
					used += utf8.RuneCountInString(text)
				}
				if pad := layout.branchCol - used; pad > 0 {
					write(plainStyle, strings.Repeat(" ", pad))
				}
				write(plainStyle, " ")
			}

			// The working tree has no hash, date or author; its counts go where
			// the hash would be, and the columns after it stay empty.
			working := isCommit && m.commits[row.CommitIdx].WorkingTree

			if isSel {
				highlighted := strings.ReplaceAll(graphPadded, "●", "◉")
				write(plainStyle, "> ")
				renderBranchLabel()
				write(selGraphColor, highlighted)
				write(plainStyle, " ")
				if working {
					write(selHashStyle, m.commits[row.CommitIdx].Message)
				} else {
					write(selHashStyle, m.commits[row.CommitIdx].Hash)
				}
			} else {
				write(plainStyle, "  ")
				renderBranchLabel()
				writeGraph(row.GraphChars, row.Lanes)
				if padLen > 0 {
					write(plainStyle, strings.Repeat(" ", padLen))
				}
				if working {
					write(plainStyle, " ")
					write(workingTreeStyle, m.commits[row.CommitIdx].Message)
				} else if isCommit {
					write(plainStyle, " ")
					write(commitHashStyle, m.commits[row.CommitIdx].Hash)
				}
			}
			if isCommit && !working && layout.dateCol > 0 {
				write(plainStyle, " ")
				write(dateStyle, m.commits[row.CommitIdx].Date.Format(dateColumnFormat))
			}
			// A gap under 1 means the row doesn't fit as computed; drawing the
			// name anyway would wrap the row and break the panel's layout.
			if isCommit && !working && layout.authorCol > 0 && authorGap >= 1 {
				write(plainStyle, strings.Repeat(" ", authorGap))
				name := ansi.Truncate(m.commits[row.CommitIdx].Author, layout.authorCol, "…")
				write(authorStyle, name)
				// Padded only when a message follows: the names are then a
				// column with an edge rather than a ragged left margin for it.
				if pad := layout.authorCol - ansi.StringWidth(name); layout.messageCol > 0 && pad > 0 {
					write(plainStyle, strings.Repeat(" ", pad))
				}
			}
			// The subject last, taking whatever is left: it is the one column
			// with no natural width, and cutting it costs least.
			if isCommit && !working && layout.messageCol > 0 {
				used := ansi.StringWidth(sb.String()[rowStart:])
				if room := min(layout.messageCol, contentWidth-used-1); room >= 2 {
					write(plainStyle, " ")
					write(messageStyle, ansi.Truncate(m.commits[row.CommitIdx].Message, room, "…"))
				}
			}
			// Carry the band to the panel edge, whichever columns are showing.
			if isSel {
				if pad := contentWidth - ansi.StringWidth(sb.String()[rowStart:]); pad > 0 {
					write(plainStyle, strings.Repeat(" ", pad))
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
		startIdx, endIdx := m.graphWindow()

		linesWritten := 0
		for i := startIdx; i < endIdx; i++ {
			c := m.commits[i]

			if i == m.selected {
				// Same band as graph mode; each piece carries the background (see
				// the note on write there).
				band := func(s lipgloss.Style) lipgloss.Style { return s.Background(selectedBg) }
				row := band(plainStyle).Render("> ") +
					band(selGraphColor).Render(c.GraphLine) +
					band(plainStyle).Render(" ") +
					band(selHashStyle).Render(c.Hash)
				if pad := contentWidth - ansi.StringWidth(row); pad > 0 {
					row += band(plainStyle).Render(strings.Repeat(" ", pad))
				}
				sb.WriteString(row)
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

	return sb.String()
}

// renderDiffSections renders the stats and the diff itself, the part of the
// details panel that reads the same for a commit and for uncommitted changes.
func (m *model) renderDiffSections(c commit) string {
	var sb strings.Builder

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

		for _, line := range strings.Split(c.DiffBody, "\n") {
			sb.WriteString(styleDiffLine(line))
			sb.WriteString("\n")
		}
	} else if !c.DiffLoaded {
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("Loading diff..."))
		sb.WriteString("\n")
	}

	return sb.String()
}

// styleDiffLine colours one line of a diff by what git meant it to be. The
// "+++"/"---" headers start with the same characters as an added and a removed
// line and are neither, so they are told apart before the colouring.
func styleDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.DiffAdd)).Render(line)
	case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.DiffDel)).Render(line)
	case strings.HasPrefix(line, "@@"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.DiffHunk)).Render(line)
	case strings.HasPrefix(line, "diff "):
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.DiffHeader)).Render(line)
	}
	return line
}

// fitDetails applies the scroll offset and clips the panel's content to the
// room it has.
//
// lipgloss Height() only pads short content, it does NOT clip overflow, so
// without this the panel grows unbounded.
func (m *model) fitDetails(content string) string {
	content = truncateLines(content, m.detailsContentWidth)
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
