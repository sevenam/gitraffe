package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"gopkg.in/yaml.v3"
)

// ThemeColors holds all color values for the application.
type ThemeColors struct {
	// Background paints the whole screen; empty leaves the terminal's own
	// background showing, which is what every theme did before it existed.
	Background string `yaml:"background"`
	// Foreground is the colour of text that has none of its own, used only with
	// a Background; empty means the Message colour. See paintBackground.
	Foreground     string `yaml:"foreground"`
	Title          string `yaml:"title"`
	Hash           string `yaml:"hash"`
	Author         string `yaml:"author"`
	Date           string `yaml:"date"`
	Message        string `yaml:"message"`
	Branch         string `yaml:"branch"`
	LocalBranch    string `yaml:"local_branch"`
	Ahead          string `yaml:"ahead"`
	Behind         string `yaml:"behind"`
	MergedBranch   string `yaml:"merged_branch"`
	Tag            string `yaml:"tag"`
	Help           string `yaml:"help"`
	Error          string `yaml:"error"`
	SectionHeader  string `yaml:"section_header"`
	Graph          string `yaml:"graph"`
	BorderActive   string `yaml:"border_active"`
	BorderInactive string `yaml:"border_inactive"`
	SelectedFg     string `yaml:"selected_fg"`
	SelectedBg     string `yaml:"selected_bg"`
	DiffAdd        string `yaml:"diff_add"`
	DiffDel        string `yaml:"diff_del"`
	DiffHunk       string `yaml:"diff_hunk"`
	DiffHeader     string `yaml:"diff_header"`
}

type themeFile struct {
	Colors ThemeColors `yaml:"colors"`
}

var currentTheme ThemeColors

func defaultTheme() ThemeColors {
	return ThemeColors{
		Title:          "#7D56F4",
		Hash:           "#FFA500",
		Author:         "#7DD3FC",
		Date:           "#A3BE8C",
		Message:        "#E5E9F0",
		Branch:         "#88C0D0",
		MergedBranch:   "#707880",
		Tag:            "#EBCB8B",
		Help:           "#626262",
		Error:          "#FF0000",
		SectionHeader:  "#7D56F4",
		Graph:          "#FFA500",
		BorderActive:   "#FFA500",
		BorderInactive: "#7D56F4",
		SelectedFg:     "#FFFFFF",
		SelectedBg:     "#3C3C3C",
		DiffAdd:        "#A3BE8C",
		DiffDel:        "#BF616A",
		DiffHunk:       "#5E81AC",
		DiffHeader:     "#E5E9F0",
	}
}

// loadTheme applies a theme over the default colours.
//
// With a path (the -theme flag) every problem is returned, and unknown keys
// count as problems: the user named that file, so quietly showing defaults
// instead would look as if the flag had been ignored. Without one it looks for
// the theme in configDir, where having no file is the normal case and problems
// are only logged: first the one picked in the app (settings.yml), then
// theme.yml. A picked theme wins because picking is the more recent, explicit
// choice; theme.yml stays in the picker for switching back.
func loadTheme(path, configDir string) error {
	currentTheme = defaultTheme()
	currentThemeName = defaultThemeName

	if path != "" {
		currentThemeName = ""
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("theme: %w", err)
		}
		colors, err := parseTheme(data, true)
		if err != nil {
			return fmt.Errorf("theme %s: %w", path, err)
		}
		currentTheme = colors
		log.Printf("Theme: loaded from %s (-theme)", path)
		return nil
	}

	if configDir == "" {
		return nil
	}

	if name := loadSettings(configDir).Theme; name != "" {
		choice, ok := findTheme(availableThemes(configDir), name)
		if !ok {
			log.Printf("Theme: picked theme %q no longer exists, falling back", name)
		} else if colors, err := choice.load(); err != nil {
			log.Printf("Theme: picked theme %q: %v, falling back", name, err)
		} else {
			currentTheme = colors
			currentThemeName = name
			log.Printf("Theme: %s (picked in the app)", name)
			return nil
		}
	}

	themePath := filepath.Join(configDir, "theme.yml")
	log.Printf("Theme: looking for %s", themePath)

	data, err := os.ReadFile(themePath)
	if err != nil {
		log.Printf("Theme: no file at %s, using defaults", themePath)
		return nil
	}

	colors, err := parseTheme(data, false)
	if err != nil {
		log.Printf("Theme: parse error in %s: %v", themePath, err)
		return nil
	}

	currentTheme = colors
	currentThemeName = customThemeName
	log.Printf("Theme: loaded from %s", themePath)
	return nil
}

// parseTheme reads a theme file's colours over the defaults, so a file only has
// to name the colours it changes. strict rejects unknown keys, which is how a
// misspelt key gets caught instead of silently doing nothing.
func parseTheme(data []byte, strict bool) (ThemeColors, error) {
	tf := themeFile{Colors: defaultTheme()}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(strict)
	// An empty file decodes to io.EOF: nothing to override, not an error.
	if err := dec.Decode(&tf); err != nil && !errors.Is(err, io.EOF) {
		return ThemeColors{}, err
	}
	return tf.Colors, nil
}

// firstColour returns the theme's own setting if it has one, else the fallback.
func firstColour(own, fallback string) string {
	if own != "" {
		return own
	}
	return fallback
}

func initStyles() {
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(currentTheme.Title)).
		Padding(0, 1)

	commitHashStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(currentTheme.Hash)).
		Bold(true)

	authorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(currentTheme.Author))

	dateStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(currentTheme.Date))

	messageStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(currentTheme.Message))

	// Remote-tracking branches keep the established "branch" colour, so themes
	// that already set it are unaffected.
	remoteBranchStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(currentTheme.Branch)).
		Bold(true)

	// Local branches default to the date colour rather than a fixed green, so
	// every existing theme gets a coherent pair without declaring a new key.
	localColour := currentTheme.LocalBranch
	if localColour == "" {
		localColour = currentTheme.Date
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
		Foreground(lipgloss.Color(firstColour(currentTheme.Ahead, currentTheme.Author))).
		Bold(true)
	behindStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(firstColour(currentTheme.Behind, currentTheme.DiffDel))).
		Bold(true)

	// Deliberately not bold: a deleted branch is history, and should read as
	// quieter than the refs that still exist.
	mergedBranchStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(currentTheme.MergedBranch))

	tagStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(currentTheme.Tag)).
		Bold(true)

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(currentTheme.Help))
}

// paintBackground fills the screen with the theme's background colour, if it
// has one.
//
// Setting it on each style instead would take a background on every piece of
// text in the app, and would still miss the gaps between them: every styled
// piece ends in a full SGR reset, which clears the background along with the
// colour. So it is applied once, to the finished screen: set at the start of
// each line, set again after every reset, and each line padded to the full
// width so it reaches the right edge.
//
// The text colour is set alongside it. Plenty of text is drawn with no colour
// of its own (the repository name, diff context lines, the stats), which means
// the terminal's default, chosen to suit the terminal's background rather than
// the theme's: near-white on a dark terminal, and unreadable on a light theme.
func paintBackground(screen string, width int) string {
	base := baseSequence()
	if base == "" {
		return screen
	}
	lines := strings.Split(screen, "\n")
	for i, l := range lines {
		l = strings.ReplaceAll(l, "\x1b[0m", "\x1b[0m"+base)
		l = strings.ReplaceAll(l, "\x1b[m", "\x1b[m"+base)
		if pad := width - ansi.StringWidth(l); pad > 0 {
			l += strings.Repeat(" ", pad)
		}
		lines[i] = base + l + "\x1b[0m"
	}
	return strings.Join(lines, "\n")
}

// baseSequence is the escape code setting the theme's background and plain text
// colour in the terminal's colour profile. Empty when the theme sets no
// background, or the terminal shows no colour, in which case nothing is
// painted.
func baseSequence() string {
	profile := lipgloss.ColorProfile()
	bg := profile.Color(currentTheme.Background)
	if bg == nil || bg.Sequence(true) == "" {
		return ""
	}
	seq := termenv.CSI + bg.Sequence(true) + "m"
	if fg := profile.Color(textColour()); fg != nil && fg.Sequence(false) != "" {
		seq += termenv.CSI + fg.Sequence(false) + "m"
	}
	return seq
}

// textColour is the colour of text that has none of its own. It only applies
// with a painted background; defaults to the commit message colour, which is
// already the theme's main text colour.
func textColour() string {
	return firstColour(currentTheme.Foreground, currentTheme.Message)
}

// isLightColour reports whether a "#rrggbb" colour is light enough that
// colours picked for a dark background would wash out on it. Anything else,
// including no colour at all, counts as dark: that is what the bundled
// palettes assume.
func isLightColour(hex string) bool {
	if len(hex) != 7 || hex[0] != '#' {
		return false
	}
	v, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return false
	}
	r, g, b := float64(v>>16&0xff), float64(v>>8&0xff), float64(v&0xff)
	return (0.299*r+0.587*g+0.114*b)/255 > 0.5
}
