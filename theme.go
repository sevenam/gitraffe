package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

// ThemeColors holds all color values for the application.
type ThemeColors struct {
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

func loadTheme() {
	currentTheme = defaultTheme()

	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("Theme: config dir unavailable: %v", err)
		return
	}

	themePath := filepath.Join(configDir, "gitraffe", "theme.yml")
	log.Printf("Theme: looking for %s", themePath)

	data, err := os.ReadFile(themePath)
	if err != nil {
		log.Printf("Theme: no file at %s, using defaults", themePath)
		return
	}

	var tf themeFile
	tf.Colors = currentTheme
	if err := yaml.Unmarshal(data, &tf); err != nil {
		log.Printf("Theme: parse error in %s: %v", themePath, err)
		return
	}

	currentTheme = tf.Colors
	log.Printf("Theme: loaded from %s", themePath)
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
