package tui

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// Stashing (issue #152): "s" in the commit view of the uncommitted changes
// and "S" there or on the graph open one box, with a line for the stash's
// name and the files ticked to go in it: "s" ticks the selected file, "S"
// every file. Enter stashes what is ticked, both halves of each file. The
// box is not a question — nothing is lost — and "s" on the graph lists the
// stashes, where enter pops one back.
//
// A pop is the one part that could leave work needing to be resolved, so it
// goes ahead only when git.PopStash finds it would apply cleanly; otherwise
// the status line says why, and the stash stays in the list for a terminal.
//
// Stashing and popping change the index and the working tree as staging
// does, so they run as staging runs (stageCmd) and wait on it the same way.

// stashPicker is the state of the list opened with "s" on the graph.
type stashPicker struct {
	open    bool
	stashes []git.Stash
	cursor  int
}

type stashListMsg struct {
	repoPath string // the repository listed; see the handler
	stashes  []git.Stash
	err      error
	// selected is the full hash of the commit selected on the graph when
	// "s" was pressed: on a stash's row, the list opens on that stash.
	selected string
}

// canStash is canDiscard, and nothing moving HEAD under the change.
func (m *model) canStash() bool {
	if m.switching || m.pulling {
		m.notice = "Something else is running — try again when it has finished"
		return false
	}
	return m.canDiscard()
}

// stashBox is the box "s" and "S" open: a name for the stash, and the files
// to put in it, ticked. "s" ticks the file selected and "S" every file; the
// box is the same, so either can become the other with a few presses.
type stashBox struct {
	open    bool
	name    textinput.Model
	onList  bool // the file list has the keyboard rather than the name
	files   []stashFile
	cursor  int
	failure string // why enter did nothing
	// waiting is the box opened from the graph before the uncommitted
	// changes were read: it stands for every file until they are.
	waiting bool
	want    []fileKey // where the commit view's selection goes afterwards
}

// stashFile is one row of the box. A file listed twice in the commit view,
// once staged and once not, is one row here: a stash takes a path whole.
type stashFile struct {
	path      string
	untracked bool
	staged    bool // some of it is staged, and will come back unstaged
	ticked    bool
}

// stashFiles turns the uncommitted changes into the box's rows, in the same
// path order, with tick saying which start ticked.
func stashFiles(files []fileDiff, tick func(path string) bool) []stashFile {
	var rows []stashFile
	at := map[string]int{}
	for _, f := range files {
		i, ok := at[f.Path]
		if !ok {
			i = len(rows)
			at[f.Path] = i
			rows = append(rows, stashFile{path: f.Path, ticked: tick(f.Path)})
		}
		rows[i].untracked = rows[i].untracked || f.Untracked
		rows[i].staged = rows[i].staged || f.Staged
	}
	return rows
}

func (b stashBox) ticked() []string {
	var paths []string
	for _, f := range b.files {
		if f.ticked {
			paths = append(paths, f.path)
		}
	}
	return paths
}

// everything is whether the box would stash every uncommitted change, which
// it then does by naming no file at all; see git.StashAll.
func (b stashBox) everything() bool {
	return b.waiting || len(b.ticked()) == len(b.files)
}

// defaultStashName is what a stash of some files is called when the name is
// left empty: the files, as far as they fit in a line of the stash list.
// Every file has git's own name instead, which says where it was made.
func defaultStashName(paths []string) string {
	const room = 60
	if name := strings.Join(paths, ", "); len(paths) == 1 || len(name) <= room {
		return name
	}
	return paths[0] + " and " + plural(len(paths)-1, "more file")
}

// openStashBox opens the box on the uncommitted changes, with everything
// ticked or only the commit view's selected file.
func (m model) openStashBox(everything bool) (model, tea.Cmd) {
	if len(m.commits) == 0 || !m.commits[0].WorkingTree {
		m.notice = "Nothing to stash — there are no uncommitted changes"
		return m, nil
	}
	if !m.canStash() {
		return m, nil
	}
	name := textinput.New()
	name.Prompt = ""
	name.Placeholder = "optional"
	name.CharLimit = 200
	name.Cursor.SetMode(cursor.CursorStatic)
	name.Focus()
	b := stashBox{open: true, name: name}

	selected := ""
	if m.commitView.open && m.commitView.workingTree {
		m.commitView.selecting = false
		if f, ok := m.selectedFile(); ok && !everything {
			selected = f.Path
			// The file is leaving the list, both its rows; if it stays
			// (unticked in the box), the selection stays on it.
			b.want = append([]fileKey{keyOf(f)}, neighbours(m.viewedFiles(), m.commitView.file)...)
		}
	}
	c := m.commits[0]
	if !c.DiffLoaded {
		b.waiting = true
		m.stashBox = b
		return m, loadWorkingDiffCmd(m.repoPath, m.diffStatWidth())
	}
	b.files = stashFiles(c.DiffFiles, func(path string) bool { return everything || path == selected })
	for i, f := range b.files {
		if f.path == selected {
			b.cursor = i
		}
	}
	m.stashBox = b
	return m, nil
}

// fillStashBox gives a box opened before the uncommitted changes were read
// its files, all ticked, as it stood for them.
func (m *model) fillStashBox() {
	if b := &m.stashBox; b.open && b.waiting && len(m.commits) > 0 && m.commits[0].DiffLoaded {
		b.files = stashFiles(m.commits[0].DiffFiles, func(string) bool { return true })
		b.waiting = false
	}
}

// updateStashBox is the keyboard while the box is open. Tab moves between
// the name and the files; space ticks only on the files, since a name can
// have spaces in it.
func (m model) updateStashBox(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := &m.stashBox
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.stashBox = stashBox{}
		m.notice = "Nothing stashed"
		return m, nil
	case "enter":
		return m.submitStashBox()
	case "tab", "shift+tab":
		if len(b.files) > 0 {
			b.onList = !b.onList
			if b.onList {
				b.name.Blur()
			} else {
				b.name.Focus()
			}
		}
		return m, nil
	}
	if !b.onList {
		var cmd tea.Cmd
		b.failure = ""
		b.name, cmd = b.name.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "j", "down":
		b.cursor = min(b.cursor+1, len(b.files)-1)
	case "k", "up":
		b.cursor = max(b.cursor-1, 0)
	case "g", "home":
		b.cursor = 0
	case "G", "end":
		b.cursor = len(b.files) - 1
	case " ":
		b.failure = ""
		b.files[b.cursor].ticked = !b.files[b.cursor].ticked
	}
	return m, nil
}

// submitStashBox stashes what the box has ticked, under the name typed.
func (m model) submitStashBox() (model, tea.Cmd) {
	b := m.stashBox
	paths := b.ticked()
	if !b.waiting && len(paths) == 0 {
		m.stashBox.failure = "Tick a file to stash, with tab then space."
		return m, nil
	}
	m.stashBox = stashBox{}
	// Something may have started while the box was open.
	if !m.canStash() {
		return m, nil
	}
	if m.commitView.open {
		m.commitView.want = b.want
	}
	name := strings.TrimSpace(b.name.Value())
	if b.everything() {
		done := "Stashed all uncommitted changes"
		if name != "" {
			done += " as “" + name + "”"
		}
		return m.runStash(func(dir string) (string, error) {
			return git.StashAll(dir, name)
		}, done+" — s on the graph lists stashes", "Not stashed")
	}
	if name == "" {
		name = defaultStashName(paths)
	}
	return m.runStash(func(dir string) (string, error) {
		return git.StashPaths(dir, name, paths)
	}, "Stashed "+plural(len(paths), "file")+" as “"+name+"” — s on the graph lists stashes", "Not stashed")
}

// render draws the box: the name, the files with their ticks, and the keys.
func (b stashBox) render(maxRows, windowWidth int) string {
	// The border and the padding take six columns.
	width := max(20, min(72, windowWidth-6))
	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Title))
	b.name.Width = width - len("Name  ") - 1

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Stash"))
	sb.WriteString("\n\n")
	sb.WriteString(label.Render("Name") + "  " + b.name.View())
	sb.WriteString("\n\n")

	staged := false
	if b.waiting {
		sb.WriteString(ansi.Truncate("Every uncommitted change (reading the files…)", width, "…"))
		sb.WriteString("\n")
	} else {
		selected := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(theme.Current.SelectedFg)).
			Background(lipgloss.Color(theme.Current.SelectedBg))
		maxRows = max(1, maxRows)
		start := 0
		if len(b.files) > maxRows {
			start = max(0, min(b.cursor-maxRows/2, len(b.files)-maxRows))
		}
		end := min(len(b.files), start+maxRows)
		for i, f := range b.files {
			if f.ticked && f.staged {
				staged = true
			}
			if i < start || i >= end {
				continue
			}
			tick, note := "[ ] ", ""
			if f.ticked {
				tick = "[x] "
			}
			if f.untracked {
				note = "  (untracked)"
			}
			// The cursor is marked as in the other lists, and only while the
			// files have the keyboard: a mark there while typing a name
			// would say space ticks.
			lead := "  "
			if b.onList && i == b.cursor {
				lead = "> "
			}
			// A long path is cut at the front: its end is the file's name.
			room := width - len(lead) - len(tick) - len(note)
			path := f.path
			if ansi.StringWidth(path) > room {
				path = "…" + ansi.TruncateLeft(path, ansi.StringWidth(path)-room+1, "")
			}
			row := lead + tick + path + note
			if b.onList && i == b.cursor {
				sb.WriteString(selected.Render(row + strings.Repeat(" ", max(0, width-ansi.StringWidth(row)))))
			} else {
				sb.WriteString(row)
			}
			sb.WriteString("\n")
		}
	}
	if staged {
		sb.WriteString("\n" + helpStyle.Render(ansi.Truncate("Staged changes come back unstaged when popped.", width, "…")) + "\n")
	}
	if b.failure != "" {
		sb.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.Error)).Render(
			ansi.Truncate(b.failure, width, "…")) + "\n")
	}

	what := "everything"
	if !b.everything() {
		what = plural(len(b.ticked()), "file")
	}
	footer := "enter: stash " + what + " • tab: name/files • space: tick • esc: cancel"
	sb.WriteString("\n" + helpStyle.Render(ansi.Truncate(footer, width, "…")))

	return lipgloss.NewStyle().
		Width(width+2*commitPromptPadding).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, commitPromptPadding).
		Render(sb.String())
}

// runStash runs a stash or a pop as staging is run, and has finishStage hand
// over to finishStash once it is done.
func (m model) runStash(change func(dir string) (string, error), done, failed string) (model, tea.Cmd) {
	m.staging = true
	cmd := stageCmd(m.repoPath, m.diffStatWidth(), change)
	return m, func() tea.Msg {
		msg := cmd().(stageFinishedMsg)
		msg.stashed, msg.stashFailed = done, failed
		return msg
	}
}

// finishStash is finishStage's ending for a stash or a pop. Either moves
// refs/stash and changes the working tree's row, so the graph is always read
// again; the commit view stays open while there is anything left in it.
func (m model) finishStash(msg stageFinishedMsg) (model, tea.Cmd) {
	if msg.err != nil {
		return m, nil
	}
	if msg.summary == "" {
		m.commitView = commitView{}
	}
	next, cmd := m.reloadRepo()
	next.notice = msg.stashed
	return next, cmd
}

func stashFailedNotice(msg stageFinishedMsg) string {
	switch {
	case errors.Is(msg.err, git.ErrStashConflicts):
		return "Not popped: it would conflict with what has changed since — pop it in a terminal to resolve that"
	case errors.Is(msg.err, git.ErrStashUntrackedExists):
		return "Not popped: a file it holds is already in the working tree — move that file away first"
	case errors.Is(msg.err, git.ErrStashMoved):
		return "Not popped: the stash list has changed — s shows it as it is now"
	case errors.Is(msg.err, git.ErrInProgress):
		return inProgressNotice(msg.diff.State.Operation)
	}
	reason := firstLine(msg.detail)
	if reason == "" {
		reason = msg.err.Error()
	}
	return msg.stashFailed + ": " + reason
}

// openStashList reads the stashes; the list opens when they have been read.
//
// On a stash's own row — the newest stash is drawn, labelled "stash" — the
// list opens with that stash picked, so popping what is selected is "s" then
// enter, as acting on the selection is with every lower-case key. Popping
// stays in the list rather than on a key of its own: the list shows what is
// about to come back, and enter is the one key that brings it.
func (m model) openStashList() (model, tea.Cmd) {
	if !m.ready || m.err != nil {
		return m, nil
	}
	repoPath, selected := m.repoPath, ""
	if c, ok := m.commitOnScreen(); ok && !c.WorkingTree {
		selected = c.FullHash
	}
	return m, func() tea.Msg {
		stashes, err := git.ListStashes(repoPath)
		return stashListMsg{repoPath: repoPath, stashes: stashes, err: err, selected: selected}
	}
}

func (m model) showStashList(msg stashListMsg) model {
	switch {
	case msg.err != nil:
		m.notice = "Could not read the stashes: " + msg.err.Error()
	case len(msg.stashes) == 0:
		m.notice = "No stashes — S puts all uncommitted changes in one"
	default:
		m.stashPicker = stashPicker{open: true, stashes: msg.stashes}
		for i, s := range msg.stashes {
			if s.Hash == msg.selected {
				m.stashPicker.cursor = i
			}
		}
	}
	return m
}

// updateStashPicker handles keys while the list is open. Like the other lists
// it owns the keyboard, since it covers the graph.
func (m model) updateStashPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.stashPicker
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "s":
		m.stashPicker = stashPicker{}
	case "j", "down":
		p.cursor = min(p.cursor+1, len(p.stashes)-1)
	case "k", "up":
		p.cursor = max(p.cursor-1, 0)
	case "g", "home":
		p.cursor = 0
	case "G", "end":
		p.cursor = len(p.stashes) - 1
	case "enter":
		s := p.stashes[p.cursor]
		m.stashPicker = stashPicker{}
		return m.startPop(s)
	}
	return m, nil
}

func (m model) startPop(s git.Stash) (model, tea.Cmd) {
	if !m.canStash() {
		return m, nil
	}
	return m.runStash(func(dir string) (string, error) {
		return git.PopStash(dir, s)
	}, "Popped "+s.Ref+": "+s.Subject, "Not popped")
}

// label is a row of the list: where it is, what it was, and when.
func stashLabel(s git.Stash) string {
	return s.Ref + "  " + s.Subject + "  (" + s.When + ")"
}

// render draws the list as a bordered box, like the delete list.
func (p stashPicker) render(maxRows, windowWidth int) string {
	footer := "↑/↓: choose • enter: pop • esc: close"
	// The border and the padding take six columns.
	limit := max(ansi.StringWidth(footer), windowWidth-6)
	contentWidth := ansi.StringWidth(footer)
	for _, s := range p.stashes {
		contentWidth = max(contentWidth, ansi.StringWidth(stashLabel(s))+2)
	}
	contentWidth = min(contentWidth, limit)

	selected := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.Current.SelectedFg)).
		Background(lipgloss.Color(theme.Current.SelectedBg))

	maxRows = max(1, maxRows)
	start := 0
	if len(p.stashes) > maxRows {
		start = max(0, min(p.cursor-maxRows/2, len(p.stashes)-maxRows))
	}
	end := min(len(p.stashes), start+maxRows)

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Pop stash"))
	sb.WriteString("\n")
	for i := start; i < end; i++ {
		sb.WriteString("\n")
		if i == p.cursor {
			row := ansi.Truncate("> "+stashLabel(p.stashes[i]), contentWidth, "…")
			sb.WriteString(selected.Render(row + strings.Repeat(" ", contentWidth-ansi.StringWidth(row))))
		} else {
			sb.WriteString(ansi.Truncate("  "+stashLabel(p.stashes[i]), contentWidth, "…"))
		}
	}
	sb.WriteString("\n\n")
	sb.WriteString(helpStyle.Render(footer))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}
