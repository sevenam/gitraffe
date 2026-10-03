package tui

import (
	"errors"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// Staging: in the commit view of the uncommitted changes, "s" moves what is
// selected into the index or back out of it. On the file list that is a whole
// file; in the diff box it is the hunk under the cursor, or the lines picked
// with "v". "S" does every file at once, and "c" commits what is staged (see
// commit_prompt.go).
//
// A file with staged and unstaged changes is listed twice, once for each, so
// the diff beside an entry is exactly what "s" on it would move, and which
// way is never a question.
//
// None of it can lose work: staging copies changes into the index and
// unstaging takes them out again, and the files themselves are not touched.
// It is refused during a merge or a rebase, where staging a file means "this
// conflict is resolved" (see git.WorkingState).

// fileKey names an entry of the file list across a reload: the list is read
// again after every change, and an index into the old one means nothing.
type fileKey struct {
	path   string
	staged bool
}

func keyOf(f fileDiff) fileKey { return fileKey{f.Path, f.Staged} }

type stageFinishedMsg struct {
	repoPath string // the repository staged in; see the handler
	err      error
	detail   string
	// diff is the uncommitted changes as they are now, read in the same
	// breath so the list never shows what was true before the key.
	diff        git.Diff
	fingerprint string
	// summary is the row's "3 changed, 1 untracked", which unstaging a new
	// file changes; "" when it could not be counted.
	summary string
}

// canStage says whether the view is one staging works in, and puts the
// reason on the status line when the repository is in no state for it.
func (m *model) canStage() bool {
	if !m.commitView.workingTree || m.staging || m.committing {
		return false
	}
	if op := m.workingState.Operation; op != "" {
		m.notice = inProgressNotice(op)
		return false
	}
	return true
}

func inProgressNotice(op string) string {
	if op == "conflict" {
		return "There are unresolved conflicts — settle them in a terminal first"
	}
	return "A " + op + " is in progress — finish it in a terminal first"
}

// stagingKey handles the keys that only mean something on the uncommitted
// changes. The last result is false for every other key, and on a commit,
// where there is nothing to stage.
func (m model) stagingKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	if !m.commitView.workingTree {
		return m, nil, false
	}
	v := &m.commitView
	switch msg.String() {
	case "esc":
		// Out of the selection before out of the view: esc undoes the last
		// thing entered, and that was "v".
		if !v.selecting {
			return m, nil, false
		}
		v.selecting = false
		return m, nil, true
	case "v":
		if v.focus != commitBoxDiff {
			m.notice = "v picks lines in the diff — press 3 to go there"
			return m, nil, true
		}
		v.selecting = !v.selecting
		v.diffAnchor = v.diffCursor
		return m, nil, true
	case "s":
		next, cmd := m.stageSelected()
		return next, cmd, true
	case "S":
		next, cmd := m.stageEverything()
		return next, cmd, true
	case "c":
		return m.openCommitPrompt(), nil, true
	}
	return m, nil, false
}

// stageSelected stages or unstages what the focused box has selected.
func (m model) stageSelected() (model, tea.Cmd) {
	if !m.canStage() {
		return m, nil
	}
	files := m.viewedFiles()
	v := &m.commitView
	if v.file < 0 || v.file >= len(files) {
		return m, nil
	}
	f := files[v.file]
	m.staging = true

	// Where the selection goes once the list is read again: the entry itself
	// while any of it is left, so the next hunk moves up under the cursor;
	// then the one after it, so "s" again takes the next file; and last the
	// same file's other half, which is where what was just moved went.
	v.want = append([]fileKey{keyOf(f)}, neighbours(files, v.file)...)
	v.want = append(v.want, fileKey{f.Path, !f.Staged})

	if v.focus != commitBoxDiff {
		path, staged := f.Path, f.Staged
		return m, stageCmd(m.repoPath, m.diffStatWidth(), func(dir string) (string, error) {
			if staged {
				return git.UnstageFile(dir, path)
			}
			return git.StageFile(dir, path)
		})
	}

	first, last := m.pickedLines(f)
	v.selecting = false
	return m, stageCmd(m.repoPath, m.diffStatWidth(), func(dir string) (string, error) {
		return git.StageLines(dir, f, first, last)
	})
}

// stageEverything stages all there is, or, when nothing is left to stage,
// takes it all back: the key has one meaning per state, which the file list
// shows.
func (m model) stageEverything() (model, tea.Cmd) {
	if !m.canStage() {
		return m, nil
	}
	unstaged := false
	for _, f := range m.viewedFiles() {
		if !f.Staged {
			unstaged = true
		}
	}
	m.staging = true
	m.commitView.selecting = false
	return m, stageCmd(m.repoPath, m.diffStatWidth(), func(dir string) (string, error) {
		if unstaged {
			return git.StageAll(dir)
		}
		return git.UnstageAll(dir)
	})
}

// neighbours are the entries to fall back on when the one at i is gone: the
// next in its half of the list, then the one before.
func neighbours(files []fileDiff, i int) []fileKey {
	var keys []fileKey
	if i+1 < len(files) && files[i+1].Staged == files[i].Staged {
		keys = append(keys, keyOf(files[i+1]))
	}
	if i > 0 && files[i-1].Staged == files[i].Staged {
		keys = append(keys, keyOf(files[i-1]))
	}
	return keys
}

// stageCmd makes the change and reads the uncommitted changes again.
func stageCmd(repoPath string, statWidth int, change func(dir string) (string, error)) tea.Cmd {
	return func() tea.Msg {
		msg := stageFinishedMsg{repoPath: repoPath}
		msg.detail, msg.err = change(repoPath)

		// All read after the change, and side by side: this is the wait
		// between pressing "s" and seeing it happen.
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			// So auto-refresh does not take our own staging for something
			// that happened behind its back.
			msg.fingerprint, _ = git.Fingerprint(repoPath)
		}()
		go func() {
			defer wg.Done()
			msg.summary, _ = summariseWorkingTree(repoPath)
		}()
		msg.diff = git.WorkingTree(repoPath, statWidth)
		wg.Wait()
		return msg
	}
}

// finishStage shows the list as it is now, and says why when nothing moved.
func (m model) finishStage(msg stageFinishedMsg) model {
	m.staging = false
	if msg.fingerprint != "" {
		m.fingerprint = msg.fingerprint
	}
	if msg.err != nil {
		m.notice = stageFailedNotice(msg)
	}
	if len(m.commits) > 0 && m.commits[0].WorkingTree && msg.summary != "" {
		m.commits[0].Message = msg.summary
	}
	m.setWorkingDiff(msg.diff)
	return m
}

func stageFailedNotice(msg stageFinishedMsg) string {
	switch {
	case errors.Is(msg.err, git.ErrStaleDiff):
		return "That file has changed since it was read — here it is as it is now"
	case errors.Is(msg.err, git.ErrNoLines):
		return "No changed lines there to stage"
	case errors.Is(msg.err, git.ErrInProgress):
		return inProgressNotice(msg.diff.State.Operation)
	}
	reason := firstLine(msg.detail)
	if reason == "" {
		reason = msg.err.Error()
	}
	return "Not staged: " + reason
}

// setWorkingDiff takes in a fresh reading of the uncommitted changes and puts
// an open view's selection back on what it was on.
func (m *model) setWorkingDiff(d git.Diff) {
	if len(m.commits) == 0 || !m.commits[0].WorkingTree {
		return
	}
	c := &m.commits[0]
	// What the diff box was showing, to tell a reading that changed it from
	// one that did not.
	before := ""
	if f, ok := m.selectedFile(); ok && m.commitView.open {
		before = f.Body
	}
	c.DiffLoaded = true
	c.DiffStat, c.DiffBody, c.DiffFiles = d.Stat, d.Body, d.Files
	m.workingState = d.State

	v := &m.commitView
	if !v.open || !v.workingTree {
		return
	}
	if len(v.want) == 0 && v.shown != (fileKey{}) {
		// Nothing was asked for, so this is a reload: stay on the same entry
		// if it is still there.
		v.want = []fileKey{v.shown}
	}
	found := false
	for _, key := range v.want {
		for i, f := range d.Files {
			if keyOf(f) == key {
				v.file, found = i, true
				break
			}
		}
		if found {
			break
		}
	}
	v.want = nil
	v.file = max(0, min(v.file, len(d.Files)-1))
	if len(d.Files) == 0 {
		v.shown, v.diffCursor, v.selecting = fileKey{}, 0, false
		return
	}
	if key := keyOf(d.Files[v.file]); key != v.shown {
		// A different entry: the line the cursor was on is another file's.
		v.shown, v.diffCursor, v.diffScroll = key, 0, 0
		v.selecting = false
	} else if d.Files[v.file].Body != before {
		// The same entry with other lines in it: the ones picked were picked
		// by number, and the numbers have moved. A reload that found nothing
		// new here — it may have been for a fetch — leaves them picked.
		v.selecting = false
	}
	v.diffCursor = max(0, min(v.diffCursor, lineCount(d.Files[v.file].Body)-1))
}

// selectedFile is the entry the file list has selected, if any.
func (m model) selectedFile() (fileDiff, bool) {
	files := m.viewedFiles()
	if m.commitView.file < 0 || m.commitView.file >= len(files) {
		return fileDiff{}, false
	}
	return files[m.commitView.file], true
}

// showFile makes the selection in the file list the entry the diff box is
// about: its diff starts from the top, with nothing picked. Every way the
// selection moves ends here.
func (m *model) showFile() {
	v := &m.commitView
	v.diffScroll, v.diffCursor, v.selecting = 0, 0, false
	v.shown = fileKey{}
	if f, ok := m.selectedFile(); ok {
		v.shown = keyOf(f)
	}
}

// pickedLines is the range of the diff "s" acts on: the lines picked with
// "v", or else the hunk the cursor is in, header to last line.
func (m model) pickedLines(f fileDiff) (first, last int) {
	v := m.commitView
	if v.selecting {
		return min(v.diffAnchor, v.diffCursor), max(v.diffAnchor, v.diffCursor)
	}
	lines := strings.Split(f.Body, "\n")
	cursor := max(0, min(v.diffCursor, len(lines)-1))
	first, last = cursor, len(lines)-1
	for first > 0 && !strings.HasPrefix(lines[first], "@@") {
		first--
	}
	for i := cursor + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "@@") {
			last = i - 1
			break
		}
	}
	return first, last
}

// linePicked reports whether a line of the diff is under the cursor or in
// the selection, and so drawn as a band.
func (m model) linePicked(i int) bool {
	v := m.commitView
	if !v.workingTree || v.focus != commitBoxDiff {
		return false
	}
	if v.selecting {
		return i >= min(v.diffAnchor, v.diffCursor) && i <= max(v.diffAnchor, v.diffCursor)
	}
	return i == v.diffCursor
}

// moveDiffCursor walks the cursor through the diff with the keys that scroll
// it on a commit. The window follows the cursor; see diffTop.
func (m model) moveDiffCursor(msg tea.KeyMsg) model {
	f, ok := m.selectedFile()
	if !ok {
		return m
	}
	v := &m.commitView
	last := lineCount(f.Body) - 1
	switch msg.String() {
	case "j", "down":
		v.diffCursor++
	case "k", "up":
		v.diffCursor--
	case "ctrl+d", "pgdown":
		v.diffCursor += 10
	case "ctrl+u", "pgup":
		v.diffCursor -= 10
	case "g", "home", "ctrl+home":
		v.diffCursor = 0
	case "G", "end", "ctrl+end":
		v.diffCursor = last
	}
	v.diffCursor = max(0, min(v.diffCursor, last))
	return m
}

// diffHeaderLines is what the diff box draws above the hunks: the path and a
// blank line.
const diffHeaderLines = 2

// diffTop is the first line of the diff box's content on screen. On a commit
// that is how far it was scrolled; on the uncommitted changes the box follows
// the cursor, the way the file list follows its selection. The renderer and
// the mouse both ask here.
func (m model) diffTop(rows int) int {
	v := m.commitView
	f, ok := m.selectedFile()
	if !v.workingTree || !ok {
		return v.diffScroll
	}
	return followWindow(v.diffScroll, v.diffCursor+diffHeaderLines, lineCount(f.Body)+diffHeaderLines, rows)
}

// diffLineAt is the line of the diff drawn on screen row y, or -1 when the
// row holds the path, the blank under it, or nothing.
func (m model) diffLineAt(y int) int {
	f, ok := m.selectedFile()
	if !ok {
		return -1
	}
	l := m.currentCommitViewLayout()
	row := y - commitViewTopRow - 1 // the box's border takes the first row
	if row < 0 || row >= l.rows-2 {
		return -1
	}
	i := m.diffTop(l.rows-2) + row - diffHeaderLines
	if i < 0 || i >= lineCount(f.Body) {
		return -1
	}
	return i
}

// pickedDiffLine draws a line of the diff under the cursor or in the
// selection: a band the width of the box, in the line's own colour.
func pickedDiffLine(line string, width int) string {
	colour := theme.Current.SelectedFg
	switch {
	case strings.HasPrefix(line, "+"):
		colour = theme.Current.DiffAdd
	case strings.HasPrefix(line, "-"):
		colour = theme.Current.DiffDel
	case strings.HasPrefix(line, "@@"):
		colour = theme.Current.DiffHunk
	}
	// Tabs are spelled out so the band's width can be counted.
	text := ansi.Truncate(strings.ReplaceAll(line, "\t", "    "), width, "")
	if pad := width - ansi.StringWidth(text); pad > 0 {
		text += strings.Repeat(" ", pad)
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(theme.Current.SelectedBg)).
		Foreground(lipgloss.Color(colour)).
		Bold(true).
		Render(text)
}

// stagedCount is how many of the listed entries are staged.
func stagedCount(files []fileDiff) int {
	n := 0
	for _, f := range files {
		if f.Staged {
			n++
		}
	}
	return n
}
