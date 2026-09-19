package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// commitSearch is the "/" prompt and the query it leaves behind for n and N.
// It sits on the status line rather than in a box like the other prompts: a
// box would cover the graph, which is the thing you are searching.
type commitSearch struct {
	active bool
	input  textinput.Model
	// query is kept after the prompt closes, so n and N have something to
	// continue; cleared only by searching for something else.
	query string
	// origin is where the selection was when the prompt opened. Esc puts it
	// back, so an abandoned search costs you nothing.
	origin int
	// found says whether the query matched anything, for the prompt to show
	// while typing.
	found bool
}

func (m model) openSearch() model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "message, author or hash"
	in.PlaceholderStyle = helpStyle
	in.Cursor.SetMode(cursor.CursorStatic)
	in.Focus()
	m.search = commitSearch{active: true, input: in, origin: m.selected, found: true}
	return m
}

// updateSearch handles keys while the prompt is open. Typing searches as you
// go, so the graph moves with each letter and a wrong turn is visible
// immediately rather than after enter.
func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.search
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.selected = s.origin
		m.detailsScroll = 0
		m.search = commitSearch{query: s.query}
		return m, m.maybeLoadDiff()
	case "enter":
		s.query = strings.TrimSpace(s.input.Value())
		s.active = false
		if s.query == "" {
			return m, nil
		}
		m.notice = m.matchSummary(s.query)
		return m, nil
	case "up", "ctrl+p":
		return m.stepSearch(strings.TrimSpace(s.input.Value()), -1)
	case "down", "ctrl+n":
		return m.stepSearch(strings.TrimSpace(s.input.Value()), 1)
	}

	before := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	if s.input.Value() == before {
		return m, cmd
	}

	// Each keystroke searches again from where the prompt opened, so deleting
	// a letter widens the search rather than leaving you wherever the longer
	// query had led.
	query := strings.TrimSpace(s.input.Value())
	s.query = query
	if query == "" {
		s.found = true
		m.selected = s.origin
		return m, tea.Batch(cmd, m.maybeLoadDiff())
	}
	if i, ok := m.matchFrom(query, s.origin, 1); ok {
		s.found = true
		m.selected = i
		m.detailsScroll = 0
		return m, tea.Batch(cmd, m.maybeLoadDiff())
	}
	s.found = false
	return m, cmd
}

// stepSearch moves to the next or previous match of query, wrapping round the
// ends: a search that stopped at the last match would hide the ones above it.
func (m model) stepSearch(query string, dir int) (tea.Model, tea.Cmd) {
	if query == "" {
		return m, nil
	}
	i, ok := m.matchFrom(query, m.selected+dir, dir)
	if !ok {
		m.notice = "No commit matches " + quoted(query)
		return m, nil
	}
	m.selected = i
	m.detailsScroll = 0
	m.search.found = true
	if !m.search.active {
		m.notice = m.matchSummary(query)
	}
	return m, m.maybeLoadDiff()
}

// searchNext is n and N once the prompt has closed.
func (m model) searchNext(dir int) (tea.Model, tea.Cmd) {
	if m.search.query == "" {
		return m, nil
	}
	return m.stepSearch(m.search.query, dir)
}

// matchFrom finds the next commit matching query, starting at from and moving
// in dir, wrapping once round the whole list.
func (m model) matchFrom(query string, from, dir int) (int, bool) {
	n := len(m.commits)
	if n == 0 || query == "" {
		return 0, false
	}
	q := strings.ToLower(query)
	for step := range n {
		i := ((from+dir*step)%n + n) % n
		if matchesCommit(m.commits[i], q) {
			return i, true
		}
	}
	return 0, false
}

// matchesCommit is a plain case-insensitive substring over the things you
// would search a graph for. The working tree row is not a commit and its
// "3 changed" text would match searches meant for messages.
func matchesCommit(c commit, lowerQuery string) bool {
	if c.WorkingTree {
		return false
	}
	for _, field := range []string{c.Message, c.Author, c.FullHash} {
		if strings.Contains(strings.ToLower(field), lowerQuery) {
			return true
		}
	}
	return false
}

// matchSummary is what the status line says once a search settles: which match
// you are on, and how many there are to move between with n and N.
func (m model) matchSummary(query string) string {
	q := strings.ToLower(query)
	total, at := 0, 0
	for i, c := range m.commits {
		if !matchesCommit(c, q) {
			continue
		}
		total++
		if i == m.selected {
			at = total
		}
	}
	if total == 0 {
		return "No commit matches " + quoted(query)
	}
	return fmt.Sprintf("Match %d of %d for %s — n / N for next and previous", at, total, quoted(query))
}

func quoted(s string) string { return "\"" + s + "\"" }

// renderSearchPrompt draws the prompt in place of the status line.
func (m *model) renderSearchPrompt() string {
	in := m.search.input
	in.Width = max(10, m.windowWidth-20)
	prompt := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Branch)).Render("/")
	line := prompt + in.View()
	if !m.search.found {
		line += lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.Error)).Render("  no match")
	}
	return truncateLines(line, m.windowWidth)
}
