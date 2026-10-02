package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/theme"
)

var (
	// Styles — initialized by initStyles() after theme is loaded.
	titleStyle        lipgloss.Style
	commitHashStyle   lipgloss.Style
	authorStyle       lipgloss.Style
	dateStyle         lipgloss.Style
	messageStyle      lipgloss.Style
	localBranchStyle  lipgloss.Style
	aheadStyle        lipgloss.Style
	behindStyle       lipgloss.Style
	remoteBranchStyle lipgloss.Style
	mergedBranchStyle lipgloss.Style
	tagStyle          lipgloss.Style
	workingTreeStyle  lipgloss.Style
	helpStyle         lipgloss.Style
)

func initStyles() {
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Current.Title)).
		Padding(0, 1)

	commitHashStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Current.Hash)).
		Bold(true)

	authorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Current.Author))

	dateStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Current.Date))

	messageStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Current.Message))

	// Remote-tracking branches keep the established "branch" colour, so themes
	// that already set it are unaffected.
	remoteBranchStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Current.Branch)).
		Bold(true)

	// Local branches default to the date colour rather than a fixed green, so
	// every existing theme gets a coherent pair without declaring a new key.
	localColour := theme.Current.LocalBranch
	if localColour == "" {
		localColour = theme.Current.Date
	}
	localBranchStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(localColour)).
		Bold(true)

	// The ahead/behind counts sit right after the branch name, so they must not
	// share its colour, nor the orange "Commit:" that follows. They borrow
	// colours every theme already defines — behind is the diff-deletion red
	// (commits you're missing), ahead the author cyan (your unpushed work);
	// green is taken by the branch name, and the tag yellow is undefined in the
	// bundled themes and unreadable on light ones.
	aheadStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(firstColour(theme.Current.Ahead, theme.Current.Author))).
		Bold(true)
	behindStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(firstColour(theme.Current.Behind, theme.Current.DiffDel))).
		Bold(true)

	// Deliberately not bold: a deleted branch is history, and should read as
	// quieter than the refs that still exist.
	mergedBranchStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Current.MergedBranch))

	tagStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Current.Tag)).
		Bold(true)

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Current.Help))

	// Uncommitted changes borrow the "ahead" colour: like unpushed commits,
	// they are work of yours that nowhere else has yet, and no bundled theme
	// has to define anything new for the row to stand out.
	workingTreeStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(firstColour(theme.Current.Ahead, theme.Current.Author))).
		Bold(true)
}

// firstColour returns the theme's own setting if it has one, else the fallback.
func firstColour(own, fallback string) string {
	if own != "" {
		return own
	}
	return fallback
}

// setTheme makes c the colours in use. The styles are package-level and built
// from theme.Current, so they have to be rebuilt too.
func setTheme(name string, c theme.Colors) {
	theme.Current = c
	theme.CurrentName = name
	initStyles()
}
