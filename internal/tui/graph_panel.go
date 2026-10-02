package tui

import (
	"log"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// visibleGraphRows is how many rows of the graph panel hold commits. It has to
// match the panel height View works out, or the panel would change size as the
// list scrolls.
func (m model) visibleGraphRows() int {
	return max(1, m.windowHeight-8)
}

// graphWindow is the half-open range of rows the graph panel draws, indexing
// displayRows -- or commits, in the fallback mode that has no graph rows. It
// starts from where the window was left (graphTop) and moves only when the
// selected row would leave it, the way a text editor scrolls to its cursor.
// That stillness is what gives home and end a "top and bottom of the screen"
// to go to: a window re-centred on every move would slide away from under them.
//
// The renderer and the mouse both ask here rather than working it out apiece:
// a click that disagreed with the drawing by one row would quietly select the
// commit above the one pointed at.
func (m model) graphWindow() (start, end int) {
	visible := m.visibleGraphRows()
	selectedRow, total := m.selected, len(m.commits)
	if len(m.displayRows) > 0 {
		selectedRow, total = 0, len(m.displayRows)
		for i, row := range m.displayRows {
			if row.CommitIdx == m.selected {
				selectedRow = i
				break
			}
		}
	}
	start = followWindow(m.graphTop, selectedRow, total, visible)
	return start, min(start+visible, total)
}

// followWindow is the first row of a window of rows rows over count, kept at
// top while selected is inside it. A selection that stepped just past an edge
// scrolls the window by as little as brings it back; one that landed further
// away — a search, a jump to a branch — is put a third of the way down, so
// there is context on both sides of where it arrived.
func followWindow(top, selected, count, rows int) int {
	if rows < 1 {
		return 0
	}
	switch {
	case selected >= top && selected < top+rows:
	case selected < top && top-selected <= rows:
		top = selected
	case selected >= top+rows && selected-(top+rows) < rows:
		top = selected - rows + 1
	default:
		top = selected - rows/3
	}
	// Never scrolled past the end: a short tail would leave the bottom of the
	// box empty while there is history above that could fill it.
	return max(0, min(top, count-rows))
}

// graphTopAndBottom are the first and last commits on screen in the graph,
// which home and end go to. Rows without a commit — the lines joining lanes,
// the note that the history was cut short — are passed over.
func (m model) graphTopAndBottom() (first, last int) {
	start, end := m.graphWindow()
	if len(m.displayRows) == 0 {
		return start, end - 1
	}
	first, last = -1, -1
	for i := start; i < end; i++ {
		if idx := m.displayRows[i].CommitIdx; idx >= 0 && idx < len(m.commits) {
			if first < 0 {
				first = idx
			}
			last = idx
		}
	}
	if first < 0 {
		return m.selected, m.selected
	}
	return first, last
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

	graphColor := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.Graph))
	lanePalette := buildLanePalette()
	selGraphColor := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.SelectedFg)).Bold(true)
	selHashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.SelectedFg)).Bold(true)
	selectedBg := lipgloss.Color(theme.Current.SelectedBg)
	plainStyle := lipgloss.NewStyle()

	if len(m.displayRows) > 0 {
		// Graph mode: the rows the layout drew

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
				highlighted := selectedMarkers.Replace(graphPadded)
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

// selectedMarkers turns each commit marker into its ringed form, which is how
// the selected row's commit is told from the others. A merge keeps its shape
// when selected, so selecting one doesn't make it look like a plain commit.
var selectedMarkers = strings.NewReplacer(
	string(git.CommitMarker), "◉",
	string(git.MergeMarker), "◈",
)
