package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/theme"

	"github.com/sevenam/gitraffe/internal/git"
)

// refPicker is the list opened with "b": every branch and tag, narrowed by
// typing, to move the selection to a ref's commit.
//
// Scrolling to a commit works while the history is short enough to scroll.
// Past that the graph is a haystack, and a ref is the name of the needle.
type refPicker struct {
	open    bool
	choices []git.Ref // every ref, in the order they were read
	shown   []int     // indexes into choices that match the filter
	cursor  int       // an index into shown
	filter  string
	err     string // why the refs could not be read; "" when they could
}

func (m model) openRefPicker() model {
	choices, err := git.ListRefs(m.repoPath)
	p := refPicker{open: true, choices: choices}
	if err != nil {
		p.err = err.Error()
	}
	p.refresh()
	// Start on the branch you are on: it is the likeliest thing to come back
	// to, and it says where the list begins.
	for i, c := range p.shown {
		if choices[c].Current {
			p.cursor = i
			break
		}
	}
	m.refs = p
	return m
}

// refresh rebuilds the visible list for the current filter, keeping the cursor
// in range. Matching ignores case and looks anywhere in the name, so "fix"
// finds "bugfix" as well as "fix-the-parser".
func (p *refPicker) refresh() {
	want := strings.ToLower(strings.TrimSpace(p.filter))
	p.shown = p.shown[:0]
	for i, c := range p.choices {
		if want == "" || strings.Contains(strings.ToLower(c.Name), want) {
			p.shown = append(p.shown, i)
		}
	}
	p.cursor = max(0, min(p.cursor, len(p.shown)-1))
}

// selected is the ref under the cursor.
func (p refPicker) selected() (git.Ref, bool) {
	if p.cursor < 0 || p.cursor >= len(p.shown) {
		return git.Ref{}, false
	}
	return p.choices[p.shown[p.cursor]], true
}

// updateRefPicker handles keys while the list is open. As in the repository
// box, every printable key is text for the filter, so the arrows move and esc
// is the way out.
func (m model) updateRefPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.refs
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.refs = refPicker{}
		return m, nil
	case "up", "ctrl+p":
		p.cursor = max(0, p.cursor-1)
		return m, nil
	case "down", "ctrl+n":
		p.cursor = min(len(p.shown)-1, p.cursor+1)
		return m, nil
	case "pgup":
		p.cursor = max(0, p.cursor-10)
		return m, nil
	case "pgdown":
		p.cursor = min(len(p.shown)-1, p.cursor+10)
		return m, nil
	case "enter":
		c, ok := p.selected()
		if !ok {
			return m, nil
		}
		m.refs = refPicker{}
		return m.jumpToRef(c)
	case "backspace":
		if runes := []rune(p.filter); len(runes) > 0 {
			p.filter = string(runes[:len(runes)-1])
			p.refresh()
		}
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		p.filter += string(msg.Runes)
		p.refresh()
	}
	return m, nil
}

// jumpToRef moves the selection to the ref's commit, reading more history
// first when the commit is older than everything loaded so far.
func (m model) jumpToRef(c git.Ref) (model, tea.Cmd) {
	for i, commit := range m.commits {
		if commit.FullHash == c.Commit {
			m.selected = i
			m.detailsScroll = 0
			m.notice = c.Kind.String() + " " + c.Name + " — " + shortHash(c.Commit)
			return m, m.maybeLoadDiff()
		}
	}

	// Not on screen: the ref is older than the batch read so far. Its place in
	// the log says how deep to read to reach it.
	depth := git.CommitDepth(m.repoPath, c.Commit)
	if depth < 0 {
		m.notice = c.Name + " is not in this repository's history"
		return m, nil
	}
	m.commitLimit = depth + 1
	next, cmd := m.reloadRepo()
	// reloadRepo keeps your place; here the point is to go somewhere else.
	next.reselect = c.Commit
	next.detailsScroll = 0
	next.notice = "Reading " + thousands(m.commitLimit) + " commits to reach " + c.Name + "..."
	return next, cmd
}

// shortHash is the seven characters git and the graph both show.
func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// render draws the list as a bordered box, scrolling to keep the cursor in
// view. maxRows caps how many refs are listed at once.
func (p refPicker) render(maxRows int) string {
	nameWidth := 0
	for _, i := range p.shown {
		nameWidth = max(nameWidth, ansi.StringWidth(p.choices[i].Name))
	}
	nameWidth = min(max(nameWidth, 12), 48)

	footer := "type to filter • ↑/↓: choose • enter: jump • esc: cancel"
	contentWidth := max(nameWidth+2+7+2+len("branch"), ansi.StringWidth(footer))

	selected := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Current.SelectedFg)).
		Background(lipgloss.Color(theme.Current.SelectedBg))

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Branches and tags"))
	sb.WriteString("\n")
	sb.WriteString(p.filterLine(contentWidth))
	sb.WriteString("\n")

	maxRows = max(1, maxRows)
	start := 0
	if len(p.shown) > maxRows {
		start = max(0, min(p.cursor-maxRows/2, len(p.shown)-maxRows))
	}
	end := min(len(p.shown), start+maxRows)

	for i := start; i < end; i++ {
		c := p.choices[p.shown[i]]
		name := ansi.Truncate(c.Name, nameWidth, "…")
		pad := strings.Repeat(" ", max(0, nameWidth-ansi.StringWidth(name)))
		sb.WriteString("\n")
		if i == p.cursor {
			row := "> " + name + pad + "  " + shortHash(c.Commit) + "  " + c.Kind.String()
			sb.WriteString(selected.Render(row + strings.Repeat(" ", max(0, contentWidth-ansi.StringWidth(row)))))
			continue
		}
		sb.WriteString("  " + refNameStyle(c).Render(name) + pad + "  " +
			commitHashStyle.Render(shortHash(c.Commit)) + "  " + helpStyle.Render(c.Kind.String()))
	}

	sb.WriteString("\n\n")
	if p.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.Error))
		sb.WriteString(errStyle.Render(ansi.Truncate("Can't read the refs: "+p.err, contentWidth, "…")))
		sb.WriteString("\n")
	}
	sb.WriteString(helpStyle.Render(footer))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}

// filterLine is what has been typed, or what to do when nothing has.
func (p refPicker) filterLine(width int) string {
	if p.filter == "" {
		if len(p.choices) == 0 {
			return helpStyle.Render("No branches or tags")
		}
		return helpStyle.Render(plural(len(p.choices), "ref") + " — type to narrow")
	}
	line := "/" + p.filter
	if len(p.shown) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.Error)).
			Render(ansi.Truncate(line+" — nothing matches", width, "…"))
	}
	return ansi.Truncate(line+helpStyle.Render("  "+plural(len(p.shown), "match")), width, "…")
}

// plural counts a thing in words: "1 ref", "4 refs".
func plural(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return strconv.Itoa(n) + " " + thing + "s"
}

// refNameStyle colours a ref the way the graph colours it, so a branch looks
// like a branch in both places.
func refNameStyle(c git.Ref) lipgloss.Style {
	switch c.Kind {
	case git.RefTag:
		return tagStyle
	case git.RefRemote:
		return remoteBranchStyle
	}
	return localBranchStyle
}
