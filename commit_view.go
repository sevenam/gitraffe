package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// commitView is the whole-screen look at one commit: what it is, which files
// it touched, and one of those files' diff.
//
// It is a screen of its own rather than more boxes on the main one because the
// graph is what gitraffe is for. Splitting the details panel three ways would
// have taken room from the graph on every screen to serve the question you ask
// on some of them; here the commit has the window, and Esc gives the graph
// back untouched.
type commitView struct {
	open bool
	// commit is the index the view is open on, with the hash it had when it
	// was opened. Indexes are renumbered by every reload — a new commit at the
	// top moves them all — so the hash is what says whether the commit under
	// the file list is still the same one. See followSelectionInCommitView.
	commit      int
	hash        string
	workingTree bool
	// focus is which box the keyboard drives: see the box constants.
	focus         int
	file          int // selected file, an index into the commit's DiffFiles
	detailsScroll int
	diffScroll    int
}

// The commit view's boxes, numbered as they are labelled on screen.
const (
	commitBoxDetails = 1
	commitBoxFiles   = 2
	commitBoxDiff    = 3
)

// openCommitView opens the view on the selected commit. Focus starts on the
// file list: it is the one box you steer, and the diff follows it.
func (m model) openCommitView() (model, tea.Cmd) {
	if !m.ready || m.err != nil || m.selected < 0 || m.selected >= len(m.commits) {
		return m, nil
	}
	c := m.commits[m.selected]
	m.commitView = commitView{
		open:        true,
		commit:      m.selected,
		hash:        c.FullHash,
		workingTree: c.WorkingTree,
		focus:       commitBoxFiles,
	}
	// Usually already loaded, since selecting a commit asks for its diff; this
	// covers opening the view before the answer arrived.
	return m, m.maybeLoadDiff()
}

// viewedCommit is the commit the view is open on, and whether there is one.
func (m model) viewedCommit() (commit, bool) {
	if m.commitView.commit < 0 || m.commitView.commit >= len(m.commits) {
		return commit{}, false
	}
	return m.commits[m.commitView.commit], true
}

// viewedFiles are the files of the commit on screen.
func (m model) viewedFiles() []fileDiff {
	c, ok := m.viewedCommit()
	if !ok {
		return nil
	}
	return c.DiffFiles
}

// updateCommitView is the keyboard while the view is open. It owns every key
// it is given: the graph behind it is not on screen, so a key meant for it
// would move something the user cannot see.
func (m model) updateCommitView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", " ":
		// q closes rather than quits, as it does in the help overlay: pressed
		// on a screen you stepped into, it means "back". Space closes too, so
		// the key that opened the view also shuts it.
		m.commitView = commitView{}
		return m, nil
	case "?":
		m.showHelp = true
		return m, nil
	case "1":
		m.commitView.focus = commitBoxDetails
		return m, nil
	case "2":
		m.commitView.focus = commitBoxFiles
		return m, nil
	case "3":
		m.commitView.focus = commitBoxDiff
		return m, nil
	case "tab":
		m.commitView.focus = m.commitView.focus%commitBoxDiff + 1
		return m, nil
	case "shift+tab":
		m.commitView.focus = (m.commitView.focus+1)%commitBoxDiff + 1
		return m, nil
	}

	switch m.commitView.focus {
	case commitBoxDetails:
		m.commitView.detailsScroll = scrollBy(m.commitView.detailsScroll, msg, m.maxScroll(commitBoxDetails))
	case commitBoxFiles:
		m = m.moveFileSelection(msg)
	case commitBoxDiff:
		m.commitView.diffScroll = scrollBy(m.commitView.diffScroll, msg, m.maxScroll(commitBoxDiff))
	}
	return m, nil
}

// scrollBy applies the shared scrolling keys to an offset, never past the last
// screenful. Clamping here rather than only where the box is drawn is what
// keeps a page-down at the end of a short diff from storing an offset the eye
// cannot see: scrolling back up would then do nothing for ten presses.
func scrollBy(offset int, msg tea.KeyMsg, most int) int {
	switch msg.String() {
	case "j", "down":
		offset++
	case "k", "up":
		offset--
	case "ctrl+d", "pgdown":
		offset += 10
	case "ctrl+u", "pgup":
		offset -= 10
	case "g", "home":
		offset = 0
	case "G", "end":
		offset = most
	}
	return max(0, min(offset, most))
}

// maxScroll is how far a box can be scrolled: far enough to bring its last
// line into the box, and no further, so the end of the content is the end of
// the scrolling.
func (m model) maxScroll(box int) int {
	c, ok := m.viewedCommit()
	if !ok {
		return 0
	}
	l := m.currentCommitViewLayout()
	switch box {
	case commitBoxDetails:
		return max(0, lineCount(commitIdentity(c))-(l.detailsRows-2))
	case commitBoxDiff:
		files := c.DiffFiles
		if len(files) == 0 {
			return 0
		}
		// The path and the blank line under it are drawn above the hunks.
		body := files[max(0, min(m.commitView.file, len(files)-1))].Body
		return max(0, lineCount(body)+2-(l.rows-2))
	}
	return 0
}

func lineCount(s string) int {
	return strings.Count(s, "\n") + 1
}

// moveFileSelection walks the file list. Changing file puts the diff back at
// its top: it is a different file, so a line count carried over from the last
// one would mean nothing.
func (m model) moveFileSelection(msg tea.KeyMsg) model {
	files := m.viewedFiles()
	if len(files) == 0 {
		return m
	}
	was := m.commitView.file
	switch msg.String() {
	case "j", "down":
		m.commitView.file++
	case "k", "up":
		m.commitView.file--
	case "ctrl+d", "pgdown":
		m.commitView.file += 10
	case "ctrl+u", "pgup":
		m.commitView.file -= 10
	case "g", "home":
		m.commitView.file = 0
	case "G", "end":
		m.commitView.file = len(files) - 1
	default:
		return m
	}
	m.commitView.file = max(0, min(m.commitView.file, len(files)-1))
	if m.commitView.file != was {
		m.commitView.diffScroll = 0
	}
	return m
}

// Sizes for the commit view. The left column carries names and short counts,
// the right one carries code, so the split is not an even one.
const (
	minCommitLeftWidth = 22
	maxCommitLeftWidth = 56
	// A diff box narrower than this says nothing useful, so the columns are
	// evened out rather than leaving a sliver.
	minCommitDiffWidth = 24
	minCommitBoxRows   = 3
)

// commitViewLayout is how the window is divided in the commit view. The
// renderer draws from it and the mouse reads it, so both ask the same
// question — the same bargain panelLayout makes on the main screen.
type commitViewLayout struct {
	leftWidth, rightWidth int
	// rows is the height of both columns, borders included: the details and
	// files boxes share it and the diff box takes it whole.
	rows                  int
	detailsRows, fileRows int
}

// commitViewLayoutFor divides a window of this size, given how many lines the
// commit's own details come to. Details take what they need up to half the
// column; the file list is the part that grows without limit, so it gets the
// rest.
func commitViewLayoutFor(width, height, detailLines int) commitViewLayout {
	var l commitViewLayout
	l.leftWidth = min(max(width/3, minCommitLeftWidth), maxCommitLeftWidth)
	l.rightWidth = width - l.leftWidth
	if l.rightWidth < minCommitDiffWidth {
		l.leftWidth = width / 2
		l.rightWidth = width - l.leftWidth
	}

	// The header box takes three rows and the status line one. What is left is
	// shared, and a window too short for comfortable boxes gets cramped ones
	// rather than boxes that would push the status line off the screen.
	l.rows = max(height-4, 4)
	room := l.rows - 4 // both left-hand boxes' borders
	// Details take the lines they need, but never so many that the file list
	// has nowhere to draw: the list is the half of the column you steer.
	l.detailsRows = min(max(detailLines, 1), max(room-minCommitBoxRows, 1)) + 2
	l.fileRows = max(l.rows-l.detailsRows, 2)
	l.detailsRows = l.rows - l.fileRows
	return l
}

// currentCommitViewLayout is the division on screen right now.
func (m model) currentCommitViewLayout() commitViewLayout {
	lines := 0
	if c, ok := m.viewedCommit(); ok {
		lines = strings.Count(commitIdentity(c), "\n") + 1
	}
	return commitViewLayoutFor(m.windowWidth, m.windowHeight, lines)
}

// renderCommitView draws the whole screen: the box it shares with the graph,
// the three boxes, and a status line of its own.
func (m model) renderCommitView() string {
	c, ok := m.viewedCommit()
	if !ok {
		return "\n  That commit is no longer here. Press esc to go back."
	}
	l := m.currentCommitViewLayout()

	header := addBoxLabel(lipgloss.NewStyle().
		Width(m.windowWidth-2).
		Height(1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(currentTheme.BorderInactive)).
		Padding(0, 1).
		Render(m.commitViewHeader(c)), "")

	details := commitBox(
		scrollLines(commitIdentity(c), m.commitView.detailsScroll, l.detailsRows-2, l.leftWidth-4),
		l.leftWidth, l.detailsRows, "[1]-commit", m.commitView.focus == commitBoxDetails)
	files := commitBox(
		m.renderFileList(c, l.leftWidth-4, l.fileRows-2),
		l.leftWidth, l.fileRows, fileBoxLabel(c), m.commitView.focus == commitBoxFiles)
	diff := commitBox(
		m.renderFileDiff(c, l.rightWidth-4, l.rows-2),
		l.rightWidth, l.rows, "[3]-diff", m.commitView.focus == commitBoxDiff)

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.JoinVertical(lipgloss.Left, details, files), diff)
	return fitScreen(fmt.Sprintf("%s\n%s\n%s", header, body, m.commitViewStatusLine()), m.windowHeight)
}

// commitViewHeader names the commit in the row the graph screen gives the
// repository, so stepping in and out reads as the same app rather than two.
//
// The subject is cut to what is left of the row rather than wrapped: the box
// is one line tall, and a second line would push every box below it down.
func (m model) commitViewHeader(c commit) string {
	if c.WorkingTree {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Title)).Render("Commit: ") +
			workingTreeStyle.Render("uncommitted changes")
	}
	lead := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.Title)).Render("Commit: ") +
		commitHashStyle.Render(c.Hash) + "  "
	// The box's borders take two columns and its padding two more.
	room := m.windowWidth - 4 - ansi.StringWidth(ansi.Strip(lead))
	subject, _, _ := strings.Cut(c.Message, "\n")
	return lead + ansi.Truncate(subject, max(room, 1), "…")
}

// fileBoxLabel puts the count in the box's own label, so "how many files did
// this touch" is answered without reading the list.
func fileBoxLabel(c commit) string {
	if !c.DiffLoaded {
		return "[2]-files"
	}
	return fmt.Sprintf("[2]-files-(%d)", len(c.DiffFiles))
}

// commitBox draws one bordered box at exactly the size asked for. lipgloss
// Height is a minimum rather than a maximum, so the result is trimmed the way
// the main screen's panels are.
func commitBox(content string, width, height int, label string, focused bool) string {
	border := lipgloss.Color(currentTheme.BorderInactive)
	if focused {
		border = lipgloss.Color(currentTheme.BorderActive)
	}
	box := lipgloss.NewStyle().
		Width(width-2).
		Height(height-2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Render(content)
	return trimToHeight(addBoxLabel(box, label), height)
}

// renderFileList is one row per file: what it is called, and how much of it
// changed.
func (m model) renderFileList(c commit, width, rows int) string {
	if !c.DiffLoaded {
		return helpStyle.Render("Loading...")
	}
	if len(c.DiffFiles) == 0 {
		return helpStyle.Render("No files changed")
	}

	top := m.fileTop(rows)

	var lines []string
	for i := top; i < min(top+rows, len(c.DiffFiles)); i++ {
		lines = append(lines, fileRow(c.DiffFiles[i], width, i == m.commitView.file))
	}
	return strings.Join(lines, "\n")
}

// fileRow is "> path        +12 -3", the counts against the right edge so they
// line up however long the paths are. A path too long is cut from the left:
// the file's own name says more than the directories above it.
func fileRow(f fileDiff, width int, selected bool) string {
	counts := fileCounts(f)
	countWidth := ansi.StringWidth(counts)
	marker := "  "
	if selected {
		marker = "> "
	}
	room := max(width-2-countWidth-1, 1)
	path := truncateLeft(f.Path, room)
	gap := max(room-ansi.StringWidth(path), 0) + 1

	if !selected {
		return marker + path + strings.Repeat(" ", gap) + counts
	}
	// The selected row is a band across the box, as the graph's row is: every
	// piece carries the background, since a reset inside one would end it.
	band := lipgloss.NewStyle().Background(lipgloss.Color(currentTheme.SelectedBg))
	row := band.Foreground(lipgloss.Color(currentTheme.SelectedFg)).Bold(true).Render(marker+path) +
		band.Render(strings.Repeat(" ", gap)) +
		band.Render(ansi.Strip(counts))
	if pad := width - 2 - ansi.StringWidth(path) - gap - countWidth; pad > 0 {
		row += band.Render(strings.Repeat(" ", pad))
	}
	return row
}

// fileCounts is how much of a file changed, or why there is no count to give.
func fileCounts(f fileDiff) string {
	switch {
	case f.Untracked:
		return helpStyle.Render("untracked")
	case f.Binary:
		return helpStyle.Render("binary")
	}
	add := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.DiffAdd)).Render(fmt.Sprintf("+%d", f.Added))
	del := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.DiffDel)).Render(fmt.Sprintf("-%d", f.Removed))
	return add + " " + del
}

// renderFileDiff is the selected file's hunks, coloured as the details panel
// colours them.
func (m model) renderFileDiff(c commit, width, rows int) string {
	if !c.DiffLoaded {
		return helpStyle.Render("Loading...")
	}
	files := c.DiffFiles
	if len(files) == 0 {
		return helpStyle.Render("Nothing to show")
	}
	f := files[max(0, min(m.commitView.file, len(files)-1))]

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.DiffHeader)).Render(f.Path))
	sb.WriteString("\n\n")
	if strings.TrimSpace(f.Body) == "" {
		sb.WriteString(helpStyle.Render("No textual change"))
	} else {
		for _, line := range strings.Split(f.Body, "\n") {
			sb.WriteString(styleDiffLine(line))
			sb.WriteString("\n")
		}
	}
	return scrollLines(sb.String(), m.commitView.diffScroll, rows, width)
}

// commitViewStatusLine is the view's own bottom line: the keys this screen
// answers to, with the way out first.
func (m model) commitViewStatusLine() string {
	return truncateLines(helpStyle.Render(
		"esc: back • 1/2/3: focus box • tab: cycle • ↑/↓/j/k: move • ?: help"), m.windowWidth)
}

// fileTop is the first file drawn in a list of this many rows. Like the
// graph's window it is worked out from the selection rather than kept as an
// offset, so the renderer and the mouse cannot disagree about which file a row
// holds — and there is no offset to go stale when the commit changes.
//
// The list scrolls only when the selection would fall off the bottom, which
// leaves it still while you walk down the first screenful.
func (m model) fileTop(rows int) int {
	return topShowing(m.commitView.file, len(m.viewedFiles()), rows)
}

// topShowing is the first row to draw so that selected is among the rows on
// screen, moving no further than it has to.
func topShowing(selected, count, rows int) int {
	if rows < 1 || selected < rows {
		return 0
	}
	return max(0, min(selected-rows+1, max(count-rows, 0)))
}

// scrollLines applies a scroll offset and cuts content to the box it is drawn
// in, clamping the offset so scrolling past the end stops at the last line
// rather than emptying the box.
func scrollLines(content string, offset, rows, width int) string {
	lines := strings.Split(truncateLines(content, width), "\n")
	offset = max(0, min(offset, len(lines)-1))
	lines = lines[offset:]
	if len(lines) > rows && rows > 0 {
		lines = lines[:rows]
	}
	return strings.Join(lines, "\n")
}

// truncateLeft cuts a path from the front, since its last part identifies it
// and the directories above it are context.
func truncateLeft(s string, width int) string {
	if width < 1 {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return s
	}
	runes := []rune(s)
	for i := range runes {
		tail := string(runes[i:])
		if ansi.StringWidth(tail)+1 <= width {
			return "…" + tail
		}
	}
	return "…"
}

// fitScreen forces a screen to exactly the window's height, as View does for
// the main one: short screens are padded so the background reaches the bottom,
// long ones cut so nothing scrolls the terminal.
func fitScreen(screen string, height int) string {
	lines := strings.Split(screen, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}

// followSelectionInCommitView puts an open view back on its commit after a
// reload. The graph has already found it — applyReselect selects the same
// commit again — so the view follows the selection, and only has to work out
// whether that is still the commit it was opened on.
//
// It usually is, and then the file you were reading and how far down it you
// were are kept: a reload is "show me this again". When it isn't — an amend or
// a rebase replaces a commit rather than moving it — the view starts again,
// since a file list from the old commit would describe the new one wrongly.
func (m *model) followSelectionInCommitView() {
	if !m.commitView.open {
		return
	}
	m.commitView.commit = m.selected
	c, ok := m.viewedCommit()
	if ok && c.WorkingTree == m.commitView.workingTree &&
		(c.WorkingTree || c.FullHash == m.commitView.hash) {
		return
	}
	m.commitView.hash, m.commitView.workingTree = "", false
	if ok {
		m.commitView.hash, m.commitView.workingTree = c.FullHash, c.WorkingTree
	}
	m.commitView.file = 0
	m.commitView.detailsScroll, m.commitView.diffScroll = 0, 0
}

// commitViewMouse is the wheel and the click while the view is open. It works
// the way the graph screen's does: the wheel drives whatever the pointer is
// over without taking focus, and a click on a file selects it.
func (m model) commitViewMouse(msg tea.MouseMsg) (model, tea.Cmd) {
	box := m.commitViewBoxAt(msg.X, msg.Y)
	if box == 0 {
		return m, nil
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if box == commitBoxFiles {
			if i := m.fileAt(msg.Y); i >= 0 && i != m.commitView.file {
				m.commitView.file = i
				m.commitView.diffScroll = 0
			}
		}
		return m, nil
	}

	var delta int
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		delta = -mouseScrollLines
	case tea.MouseButtonWheelDown:
		delta = mouseScrollLines
	default:
		return m, nil
	}

	switch box {
	case commitBoxDetails:
		m.commitView.detailsScroll = max(0, m.commitView.detailsScroll+delta)
	case commitBoxFiles:
		// The list has a selection, so the wheel moves it rather than sliding
		// the rows underneath it, which is what the graph does too.
		if files := m.viewedFiles(); len(files) > 0 {
			was := m.commitView.file
			m.commitView.file = max(0, min(m.commitView.file+delta, len(files)-1))
			if m.commitView.file != was {
				m.commitView.diffScroll = 0
			}
		}
	case commitBoxDiff:
		m.commitView.diffScroll = max(0, m.commitView.diffScroll+delta)
	}
	return m, nil
}

// commitViewBoxAt reports which box covers a point, or 0 for the header row,
// the status line and anything outside the window.
func (m model) commitViewBoxAt(x, y int) int {
	if y < commitViewTopRow || y >= m.windowHeight-1 || x < 0 || x >= m.windowWidth {
		return 0
	}
	l := m.currentCommitViewLayout()
	if x >= l.leftWidth {
		return commitBoxDiff
	}
	if y < commitViewTopRow+l.detailsRows {
		return commitBoxDetails
	}
	return commitBoxFiles
}

// fileAt is the file drawn on screen row y, or -1 for the rows past the end of
// a short list. It reads the same scroll offset the list was drawn with, so a
// click lands on the row it pointed at.
func (m model) fileAt(y int) int {
	l := m.currentCommitViewLayout()
	// The files box starts under the details box, and its own border takes the
	// first row.
	row := y - (commitViewTopRow + l.detailsRows) - 1
	if row < 0 || row >= l.fileRows-2 {
		return -1
	}
	i := m.fileTop(l.fileRows-2) + row
	if i >= len(m.viewedFiles()) {
		return -1
	}
	return i
}

// commitViewTopRow is the first row the boxes are drawn on: the header box
// above them takes three, as the repository box does on the graph screen.
const commitViewTopRow = 3
