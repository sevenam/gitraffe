package main

import (
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
		return
	}

	data, err := os.ReadFile(filepath.Join(configDir, "gitraffe", "theme.yml"))
	if err != nil {
		// File missing is expected; use defaults.
		return
	}

	var tf themeFile
	if err := yaml.Unmarshal(data, &tf); err != nil {
		return
	}

	// Merge non-empty fields over defaults.
	c := tf.Colors
	if c.Title != "" {
		currentTheme.Title = c.Title
	}
	if c.Hash != "" {
		currentTheme.Hash = c.Hash
	}
	if c.Author != "" {
		currentTheme.Author = c.Author
	}
	if c.Date != "" {
		currentTheme.Date = c.Date
	}
	if c.Message != "" {
		currentTheme.Message = c.Message
	}
	if c.Branch != "" {
		currentTheme.Branch = c.Branch
	}
	if c.Help != "" {
		currentTheme.Help = c.Help
	}
	if c.Error != "" {
		currentTheme.Error = c.Error
	}
	if c.SectionHeader != "" {
		currentTheme.SectionHeader = c.SectionHeader
	}
	if c.Graph != "" {
		currentTheme.Graph = c.Graph
	}
	if c.BorderActive != "" {
		currentTheme.BorderActive = c.BorderActive
	}
	if c.BorderInactive != "" {
		currentTheme.BorderInactive = c.BorderInactive
	}
	if c.SelectedFg != "" {
		currentTheme.SelectedFg = c.SelectedFg
	}
	if c.SelectedBg != "" {
		currentTheme.SelectedBg = c.SelectedBg
	}
	if c.DiffAdd != "" {
		currentTheme.DiffAdd = c.DiffAdd
	}
	if c.DiffDel != "" {
		currentTheme.DiffDel = c.DiffDel
	}
	if c.DiffHunk != "" {
		currentTheme.DiffHunk = c.DiffHunk
	}
	if c.DiffHeader != "" {
		currentTheme.DiffHeader = c.DiffHeader
	}
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

	branchStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(currentTheme.Branch)).
		Bold(true)

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(currentTheme.Help))
}
