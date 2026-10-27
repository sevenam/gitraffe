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
	checkpointStyle   lipgloss.Style
	stashStyle        lipgloss.Style
	workingTreeStyle  lipgloss.Style
	helpStyle         lipgloss.Style
)

// The checkpoint and stash colours when the theme sets none; see initStyles.
const (
	checkpointDefault        = "#ff79c6"
	checkpointDefaultOnLight = "#b0175f"
	stashDefault             = "#ff9e64"
	stashDefaultOnLight      = "#a34a00"
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

	// Checkpoints need a colour of their own, and every hue the theme keys
	// give the other labels (green, blue, yellow, grey, cyan, red) is taken.
	// The fallback is a pink that is none of them, nor one of the lane
	// colours, darkened like those on a theme that paints a light background.
	checkpointColour := checkpointDefault
	if theme.IsLight(theme.Current.Background) {
		checkpointColour = checkpointDefaultOnLight
	}
	checkpointStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(firstColour(theme.Current.Checkpoint, checkpointColour))).
		Bold(true)

	// The stash is no branch either, and is told from the checkpoints too: an
	// orange, between the tag yellow and the lane red and like neither.
	stashColour := stashDefault
	if theme.IsLight(theme.Current.Background) {
		stashColour = stashDefaultOnLight
	}
	stashStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(firstColour(theme.Current.Stash, stashColour))).
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
