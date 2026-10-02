package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/theme"
)

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
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Title)).Render("Repository: "))
	sb.WriteString(m.repoName)
	sb.WriteString("  ")

	// Branch
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Branch)).Render("Branch: "))
	sb.WriteString(localBranchStyle.Render(m.currentBranch))
	if s := syncLabel(m.ahead, m.behind); s != "" {
		sb.WriteString(" ")
		sb.WriteString(s)
	}
	sb.WriteString("  ")

	// Current commit
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Hash)).Render("Commit: "))
	sb.WriteString(commitHashStyle.Render(m.currentCommit))

	leftContent := sb.String()

	// Title on the right
	versionStr := "v" + Version
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
