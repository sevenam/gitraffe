package tui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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

// filePicker is the list opened with "h" on the graph: every file in the
// repository, narrowed by typing, to show one file's history. It is a box like
// the branch list on "b" rather than a line of its own, because the point of
// it is the list: nobody remembers where a file lives, only roughly what it is
// called.
type filePicker struct {
	open  bool
	input textinput.Model
	// choices are every tracked file and every directory above one, the
	// directories ending in "/". Nil until the list has been read.
	choices []string
	shown   []int // indexes into choices that match what is typed, best first
	cursor  int   // an index into shown
	err     string
}

// maxFileMatches caps the list. A short query in a large repository matches
// thousands of files, and a list longer than the screen is only scrolled
// through by someone who should be typing another letter.
const maxFileMatches = 200

// filesLoadedMsg is the repository's file list, for the picker.
type filesLoadedMsg struct {
	repoPath string // the repository it answers for; see the handler
	files    []string
	err      error
}

func loadFilesCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		files, err := git.ListFiles(repoPath)
		return filesLoadedMsg{repoPath: repoPath, files: files, err: err}
	}
}

// openFilePicker opens the list with the current filter already typed, so
// changing it is an edit rather than starting over. The files are read afresh
// each time: they change as you work, and listing them is quick.
func (m model) openFilePicker() (model, tea.Cmd) {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "part of a file or directory name"
	in.PlaceholderStyle = helpStyle
	in.Cursor.SetMode(cursor.CursorStatic)
	in.SetValue(m.filter.Path)
	in.CursorEnd()
	in.Focus()
	m.files = filePicker{open: true, input: in}
	return m, loadFilesCmd(m.repoPath)
}

// setFiles takes the list once it has been read.
func (p *filePicker) setFiles(files []string, err error) {
	if err != nil {
		p.err = err.Error()
	}
	p.choices = withDirectories(files)
	p.refresh()
}

// withDirectories adds every directory that holds a file, so a directory's
// history can be picked as well as a file's.
func withDirectories(files []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(files))
	for _, f := range files {
		for i := strings.IndexByte(f, '/'); i >= 0; {
			dir := f[:i+1]
			if !seen[dir] {
				seen[dir] = true
				out = append(out, dir)
			}
			next := strings.IndexByte(f[i+1:], '/')
			if next < 0 {
				break
			}
			i += next + 1
		}
		out = append(out, f)
	}
	return out
}

// refresh rebuilds the visible list for what is typed. Nothing is shown for
// an empty query: the whole repository in path order is not a list anyone
// picks from, and enter on an empty prompt means "no filter".
func (p *filePicker) refresh() {
	p.shown = rankFiles(p.choices, p.input.Value())
	p.cursor = max(0, min(p.cursor, len(p.shown)-1))
}

// rankFiles orders the choices that match query, best first. A query is most
// often the start of a file's name, so a name that starts with it beats one
// that only contains it, and either beats a match somewhere in the
// directories above. Last come paths that hold the query's letters in order
// with others between, so "tuikeys" still finds internal/tui/keys.go. Shorter
// paths win ties: they are the less specific, and so the likelier, guess.
func rankFiles(choices []string, query string) []int {
	q := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(query, `\`, "/")))
	if q == "" {
		return nil
	}
	type match struct{ idx, tier int }
	var matches []match
	for i, c := range choices {
		path := strings.ToLower(c)
		name := path[strings.LastIndexByte(strings.TrimSuffix(path, "/"), '/')+1:]
		name = strings.TrimSuffix(name, "/")
		tier := -1
		switch {
		case name == strings.TrimSuffix(q, "/") || path == q:
			tier = 0
		case strings.HasPrefix(name, q):
			tier = 1
		case strings.HasPrefix(path, q):
			tier = 2
		case strings.Contains(name, q):
			tier = 3
		case strings.Contains(path, q):
			tier = 4
		case isSubsequence(q, path):
			tier = 5
		}
		if tier >= 0 {
			matches = append(matches, match{i, tier})
		}
	}
	slices.SortStableFunc(matches, func(a, b match) int {
		if a.tier != b.tier {
			return a.tier - b.tier
		}
		if la, lb := len(choices[a.idx]), len(choices[b.idx]); la != lb {
			return la - lb
		}
		return strings.Compare(choices[a.idx], choices[b.idx])
	})
	out := make([]int, 0, min(len(matches), maxFileMatches))
	for _, mt := range matches[:min(len(matches), maxFileMatches)] {
		out = append(out, mt.idx)
	}
	return out
}

// isSubsequence reports whether every letter of q appears in s, in order.
func isSubsequence(q, s string) bool {
	for _, r := range q {
		i := strings.IndexRune(s, r)
		if i < 0 {
			return false
		}
		s = s[i+len(string(r)):]
	}
	return true
}

// selectedFile is the choice under the cursor.
func (p filePicker) selectedFile() (string, bool) {
	if p.cursor < 0 || p.cursor >= len(p.shown) {
		return "", false
	}
	return p.choices[p.shown[p.cursor]], true
}

// updateFilePicker handles keys while the list is open. Every printable key is
// text for the query, as in the branch list, so the arrows move and esc is the
// way out.
func (m model) updateFilePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.files
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.files = filePicker{}
		return m, nil
	case "up", "ctrl+p":
		p.cursor = max(0, p.cursor-1)
		return m, nil
	case "down", "ctrl+n":
		p.cursor = max(0, min(len(p.shown)-1, p.cursor+1))
		return m, nil
	case "pgup":
		p.cursor = max(0, p.cursor-10)
		return m, nil
	case "pgdown":
		p.cursor = max(0, min(len(p.shown)-1, p.cursor+10))
		return m, nil
	case "tab":
		// Completes rather than chooses, so a directory can be narrowed into:
		// tab on "internal/" leaves "internal/" typed, and the list shows
		// what is under it.
		if f, ok := p.selectedFile(); ok {
			p.input.SetValue(f)
			p.input.CursorEnd()
			p.cursor = 0
			p.refresh()
		}
		return m, nil
	case "enter":
		// The highlighted match when there is one. When nothing matches,
		// what was typed is still a path git can be asked about: a file
		// deleted since is not in the list, but its history is there.
		path, ok := p.selectedFile()
		if !ok {
			path = p.input.Value()
		}
		m.files = filePicker{}
		return m.setFilter(git.Filter{Path: m.filterPath(path)})
	}
	before := p.input.Value()
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	if p.input.Value() != before {
		// A new query starts at its best match, not wherever the last one
		// had been walked to.
		p.cursor = 0
		p.refresh()
	}
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

// showFileHistory is "h" in the commit view's file list: the graph, filtered to the file
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

// render draws the box, at most maxRows matches tall and no wider than the
// window allows.
func (p filePicker) render(maxRows, windowWidth int) string {
	footer := "type to narrow • ↑/↓: choose • tab: complete • enter: show history • esc: cancel"
	// The box's border and padding take six columns.
	contentWidth := max(min(windowWidth-6, 80), 20)

	selected := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Current.SelectedFg)).
		Background(lipgloss.Color(theme.Current.SelectedBg))

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("File history"))
	sb.WriteString("\n")
	in := p.input
	in.Width = contentWidth - 2
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Branch)).Render("> "))
	sb.WriteString(in.View())
	sb.WriteString("\n")
	sb.WriteString(p.statusLine(contentWidth))
	sb.WriteString("\n")

	maxRows = max(1, maxRows)
	start := 0
	if len(p.shown) > maxRows {
		start = max(0, min(p.cursor-maxRows/2, len(p.shown)-maxRows))
	}
	end := min(len(p.shown), start+maxRows)
	for i := start; i < end; i++ {
		// Cut from the left: the file's own name says more than the
		// directories above it.
		path := truncateLeft(p.choices[p.shown[i]], contentWidth-2)
		sb.WriteString("\n")
		if i == p.cursor {
			row := "> " + path
			sb.WriteString(selected.Render(row + strings.Repeat(" ", max(0, contentWidth-ansi.StringWidth(row)))))
			continue
		}
		if strings.HasSuffix(path, "/") {
			sb.WriteString("  " + helpStyle.Render(path))
		} else {
			sb.WriteString("  " + path)
		}
	}

	sb.WriteString("\n\n")
	sb.WriteString(helpStyle.Render(ansi.Truncate(footer, contentWidth, "…")))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}

// statusLine says how the list stands: still being read, how much matches, or
// that nothing does and what enter will do about it.
func (p filePicker) statusLine(width int) string {
	var line string
	switch {
	case p.err != "":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.Error)).
			Render(ansi.Truncate("Can't list the files: "+p.err, width, "…"))
	case p.choices == nil:
		line = "Reading the files…"
	case strings.TrimSpace(p.input.Value()) == "":
		line = plural(len(p.choices), "path") + " — type to narrow; enter on nothing clears the filter"
	case len(p.shown) == 0:
		line = "Nothing matches — enter asks git about the path as typed"
	case len(p.shown) == maxFileMatches:
		line = "The best " + strconv.Itoa(maxFileMatches) + " matches — type more to narrow"
	case len(p.shown) == 1:
		line = "1 match"
	default:
		line = strconv.Itoa(len(p.shown)) + " matches"
	}
	return helpStyle.Render(ansi.Truncate(line, width, "…"))
}
