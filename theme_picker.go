package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// themePicker is the state of the theme list opened with "t".
type themePicker struct {
	open    bool
	choices []themeChoice
	cursor  int
	// loadErr says why the highlighted theme can't be shown; "" when it can.
	loadErr string
	// The colours in use when the picker opened, restored on cancel. The
	// preview changes the real colours, since that is the only way to show a
	// theme across the whole screen.
	before     ThemeColors
	beforeName string
}

func (m model) openThemePicker() model {
	choices := availableThemes(m.configDir)
	cursor := 0
	for i, c := range choices {
		if c.name == currentThemeName {
			cursor = i
		}
	}
	m.picker = themePicker{
		open:       true,
		choices:    choices,
		cursor:     cursor,
		before:     currentTheme,
		beforeName: currentThemeName,
	}
	return m
}

// updateThemePicker handles keys while the picker is open. Like the help, it
// owns the keyboard: it covers the panels, so their keys would act unseen.
func (m model) updateThemePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.picker
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "t":
		setTheme(p.beforeName, p.before)
		m.picker = themePicker{}
	case "enter":
		c := p.choices[p.cursor]
		// Loaded again rather than trusting the preview: when the picker opens
		// on a -theme file, which isn't in the list, nothing has been previewed.
		colors, err := c.load()
		if err != nil {
			p.loadErr = err.Error()
			return m, nil
		}
		setTheme(c.name, colors)
		m.picker = themePicker{}
		if err := saveThemeChoice(m.configDir, c.name); err != nil {
			m.notice = "Using theme " + c.name + " for now — could not save it: " + err.Error()
		} else {
			m.notice = "Theme " + c.name + " saved"
		}
	case "j", "down":
		m.previewTheme(p.cursor + 1)
	case "k", "up":
		m.previewTheme(p.cursor - 1)
	case "g", "home":
		m.previewTheme(0)
	case "G", "end":
		m.previewTheme(len(p.choices) - 1)
	}
	return m, nil
}

// previewTheme moves the cursor to i and shows that theme. One that fails to
// load shows the colours from before instead, so the screen never keeps the
// previous entry's colours while the cursor says something else.
func (m *model) previewTheme(i int) {
	p := &m.picker
	i = max(0, min(i, len(p.choices)-1))
	if i == p.cursor && p.loadErr == "" {
		return
	}
	p.cursor = i
	c := p.choices[i]
	colors, err := c.load()
	if err != nil {
		p.loadErr = err.Error()
		setTheme(p.beforeName, p.before)
		return
	}
	p.loadErr = ""
	setTheme(c.name, colors)
}

// render draws the theme list as a bordered box. maxRows caps
// how many themes are listed at once; the list scrolls to keep the cursor in
// view.
func (p themePicker) render(maxRows int) string {
	nameWidth := 0
	for _, c := range p.choices {
		nameWidth = max(nameWidth, ansi.StringWidth(c.name))
	}
	footer := "↑/↓: preview • enter: keep • esc: cancel"
	contentWidth := max(nameWidth+2+len("built in")+2, ansi.StringWidth(footer))

	selected := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(currentTheme.SelectedFg)).
		Background(lipgloss.Color(currentTheme.SelectedBg))

	maxRows = max(1, maxRows)
	start := 0
	if len(p.choices) > maxRows {
		start = max(0, min(p.cursor-maxRows/2, len(p.choices)-maxRows))
	}
	end := min(len(p.choices), start+maxRows)

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Themes"))
	sb.WriteString("\n")
	for i := start; i < end; i++ {
		c := p.choices[i]
		sb.WriteString("\n")
		name := c.name + strings.Repeat(" ", nameWidth-ansi.StringWidth(c.name))
		if i == p.cursor {
			row := "> " + name + "  " + c.source
			sb.WriteString(selected.Render(row + strings.Repeat(" ", contentWidth-ansi.StringWidth(row))))
		} else {
			sb.WriteString("  " + name + "  " + helpStyle.Render(c.source))
		}
	}
	sb.WriteString("\n\n")
	if p.loadErr != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.Error))
		sb.WriteString(errStyle.Render(ansi.Truncate("Can't load: "+p.loadErr, contentWidth, "…")))
		sb.WriteString("\n")
	}
	sb.WriteString(helpStyle.Render(footer))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(currentTheme.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}
