package tui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// A filter narrows the graph to the commits that answer one question, of which
// so far there is one: what happened to this file. It is applied by reading
// the history again with the filter, not by hiding rows of the full graph:
// git rewrites each commit's parents to the nearest ones that pass, so the
// lanes join commits that are both on screen rather than running off to ones
// that are not.
//
// It lasts until esc, and a reload — "r", a fetch, "m", auto-refresh — keeps
// it, since each of those means "show me this again", not "show me something
// else". Opening another repository drops it: its path is this one's.

// filterPrompt is the "F" prompt. Like the search prompt it takes the status
// line rather than a box, so the graph being filtered stays in view.
type filterPrompt struct {
	active bool
	input  textinput.Model
}

func (m model) openFilterPrompt() model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "path of a file or directory, from the top of the repository"
	in.PlaceholderStyle = helpStyle
	in.Cursor.SetMode(cursor.CursorStatic)
	in.SetValue(m.filter.Path)
	in.CursorEnd()
	in.Focus()
	m.filterPrompt = filterPrompt{active: true, input: in}
	return m
}

// updateFilterPrompt handles keys while the prompt is open. Nothing is read
// until enter: each letter would otherwise be a whole git log, and a path half
// typed is a filter nobody wanted.
func (m model) updateFilterPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.filterPrompt = filterPrompt{}
		return m, nil
	case "enter":
		path := m.filterPath(m.filterPrompt.input.Value())
		m.filterPrompt = filterPrompt{}
		return m.setFilter(git.Filter{Path: path})
	}
	var cmd tea.Cmd
	m.filterPrompt.input, cmd = m.filterPrompt.input.Update(msg)
	return m, cmd
}

// filterPath turns what was typed into the path git is given: relative to the
// top of the repository, with forward slashes, as the commit view lists files.
// Windows users type backslashes, and a path pasted from a file manager is
// absolute; both name the same file as the forward-slashed relative one.
func (m model) filterPath(typed string) string {
	p := strings.TrimSpace(typed)
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) && m.repoRoot != "" {
		if rel, err := filepath.Rel(m.repoRoot, p); err == nil && !strings.HasPrefix(rel, "..") {
			p = rel
		}
	}
	p = strings.ReplaceAll(p, `\`, "/")
	for strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	p = strings.TrimRight(p, "/")
	if p == "." {
		return ""
	}
	return p
}

// setFilter reads the history again with f, keeping the selected commit if it
// passes. Setting the filter already on is still a reload, as "r" would be.
func (m model) setFilter(f git.Filter) (model, tea.Cmd) {
	if !m.ready {
		return m, nil
	}
	m.filter = f
	next, cmd := m.reloadRepo()
	// A different question deserves a fresh batch: the commits it reads are
	// not the ones "m" was asked to read more of.
	next.commitLimit = commitBatch
	if f.IsZero() {
		next.notice = "Reading every commit..."
		next.loadedNotice = "Showing every commit"
	} else {
		next.notice = "Reading the history of " + f.Path + "..."
		next.loadedNotice = "History of " + f.Path + " — esc shows every commit"
	}
	return next, cmd
}

// clearFilter is esc on the graph.
func (m model) clearFilter() (model, tea.Cmd) {
	if m.filter.IsZero() {
		return m, nil
	}
	return m.setFilter(git.Filter{})
}

// showFileHistory is "h" in the commit view: the graph, filtered to the file
// selected there. The commit view closes, since the question has moved from
// this commit to that file; the commit stays selected, and as it changed the
// file it is still in the graph.
func (m model) showFileHistory() (model, tea.Cmd) {
	files := m.viewedFiles()
	if m.commitView.file < 0 || m.commitView.file >= len(files) {
		return m, nil
	}
	f := files[m.commitView.file]
	if f.Untracked {
		m.notice = f.Path + " is untracked, so it has no history yet"
		return m, nil
	}
	m.commitView = commitView{}
	return m.setFilter(git.Filter{Path: currentPath(f.Path)})
}

// currentPath is the path a file has after the commit: a rename is listed as
// "old → new", and the history wanted is the file's under the name it has now.
func currentPath(listed string) string {
	if _, after, ok := strings.Cut(listed, " → "); ok {
		return after
	}
	return listed
}

// filteredFile is the file of files the filter is about, so the commit view
// opens on it rather than on whatever sorts first. A directory filter picks
// the first file under it. -1 when none is, or there is no filter.
func (m model) filteredFile(files []fileDiff) int {
	p := m.filter.Path
	if p == "" {
		return -1
	}
	for i, f := range files {
		path := currentPath(f.Path)
		if path == p || strings.HasPrefix(path, p+"/") {
			return i
		}
	}
	return -1
}

// noteEmptyFilter says why the graph is empty, when the filter is the reason.
// An empty screen with no word of explanation looks like a broken load.
func (m *model) noteEmptyFilter() {
	if m.filter.IsZero() || len(m.commits) > 0 {
		return
	}
	m.notice = "No commit changed " + m.filter.Path + " — esc shows every commit"
}

// renderFilterPrompt draws the prompt in place of the status line.
func (m *model) renderFilterPrompt() string {
	in := m.filterPrompt.input
	in.Width = max(10, m.windowWidth-20)
	prompt := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Branch)).Render("filter: ")
	return truncateLines(prompt+in.View(), m.windowWidth)
}
