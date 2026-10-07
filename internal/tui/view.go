package tui

import (
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/theme"
)

func (m model) View() (result string) {
	// A reload keeps the old screen up until the new one can be drawn. The
	// window size and the bottom line are the live ones: the terminal may have
	// been resized since, and the notice is where the reload says it is running.
	if !m.ready && m.stale != nil {
		s := *m.stale
		s.windowWidth, s.windowHeight = m.windowWidth, m.windowHeight
		s.notice = m.notice
		return s.View()
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in View: %v", r)
			result = fmt.Sprintf("\n  PANIC caught: %v\n\n  Check %s for details.\n  Press q to quit.", r, LogPath)
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
			Foreground(lipgloss.Color(theme.Current.Error)).
			Bold(true)
		screen := fmt.Sprintf("\n  %s\n\n  Error: %v\n\n  Press O to open another repository, or q to quit. Check %s for details.\n",
			errorStyle.Render("❌ Error loading repository"),
			m.err, LogPath)
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
			help := commitViewHelp
			if m.commitView.workingTree {
				help = stagingViewHelp
			}
			screen = overlayCentre(screen, renderHelpSections(help), m.windowWidth, m.windowHeight)
		}
		if m.commitPrompt.open {
			box := m.commitPrompt.render(m.windowWidth, stagedCount(m.viewedFiles()), m.currentBranch)
			screen = overlayCentre(screen, box, m.windowWidth, m.windowHeight)
		}
		return screen
	}

	help := m.renderStatusLine()

	// Border colors: active for focused, inactive for unfocused
	focusedBorderColor := lipgloss.Color(theme.Current.BorderActive)
	unfocusedBorderColor := lipgloss.Color(theme.Current.BorderInactive)
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
		rightContent, rightMarks := m.renderCommitDetails()
		rightPanel = addBoxLabel(lipgloss.NewStyle().
			Width(rightPanelWidth-2). // subtract borders (2); Width includes padding
			Height(contentHeight).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(box2Border).
			Padding(1, 2).
			Render(rightContent), "[2]-commit-details")
		rightPanel = markScroll(trimToHeight(rightPanel, targetPanelHeight), rightMarks, box2Border)
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
	if m.push.naming {
		output = overlayCentre(output, m.push.render(m.windowWidth), m.windowWidth, m.windowHeight)
	}
	if m.branchPrompt.open {
		output = overlayCentre(output, m.branchPrompt.render(m.windowWidth), m.windowWidth, m.windowHeight)
	}
	if m.checkout.open {
		output = overlayCentre(output, m.checkout.render(m.windowHeight-10, m.windowWidth), m.windowWidth, m.windowHeight)
	}
	if m.deletePicker.open {
		output = overlayCentre(output, m.deletePicker.render(m.windowHeight-10), m.windowWidth, m.windowHeight)
	}
	if m.refs.open {
		// The same room the theme list leaves for its title, footer and border.
		output = overlayCentre(output, m.refs.render(m.windowHeight-10), m.windowWidth, m.windowHeight)
	}
	if m.files.open {
		// The same room the branch list leaves, and two rows more for the
		// line under the query.
		output = overlayCentre(output, m.files.render(m.windowHeight-12, m.windowWidth), m.windowWidth, m.windowHeight)
	}
	if m.switcher.open {
		output = overlayCentre(output, m.switcher.render(m.windowWidth, m.windowHeight), m.windowWidth, m.windowHeight)
	}

	return output
}
