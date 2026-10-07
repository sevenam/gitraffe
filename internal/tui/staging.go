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
// file; in the diff box it is the hunk whose header the cursor is on, the one
// line it is on further down, or the lines picked with "v". "S" is the same
// key a size up: every file from the file list, the whole of the file shown
// from the diff box. "c" commits what is staged (see commit_prompt.go).
//
// A file with staged and unstaged changes is listed twice, once for each, so
// "s" on a row of the list moves exactly that half. The diff box shows the
// file whole beside either row, each hunk marked for its side, and "s" there
// moves what the cursor is on whichever way its mark says; see
// staging_diff.go.
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
		// As far as the box it is pressed in reaches: the list is every
		// file, the diff is one.
		if v.focus == commitBoxDiff {
			next, cmd := m.stageShownFile()
			return next, cmd, true
		}
		next, cmd := m.stageEverything()
		return next, cmd, true
	case "c":
		next, cmd := m.openCommitPrompt()
		return next, cmd, true
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
	// then the same file's other half, which is where what was just moved
	// went and, the list being in path order, the row the selection is
	// already on. Staying there means "s" again takes it back, as a slip
	// should be undone, and the file being looked at does not change under
	// the key. Another file is only for when this one has left the list.
	v.want = []fileKey{keyOf(f), {f.Path, !f.Staged}}
	v.want = append(v.want, neighbours(files, v.file)...)

	if v.focus != commitBoxDiff {
		path, staged := f.Path, f.Staged
		return m, stageCmd(m.repoPath, m.diffStatWidth(), func(dir string) (string, error) {
			if staged {
				return git.UnstageFile(dir, path)
			}
			return git.StageFile(dir, path)
		})
	}

	// The entry the lines under the cursor came from, which may be the
	// file's other row: the box shows both.
	d := m.stagingDiff()
	entry, first, last := m.pickedLines(d, f)
	v.follow = m.followAfter(d, entry, first, last)
	v.selecting = false
	return m, stageCmd(m.repoPath, m.diffStatWidth(), func(dir string) (string, error) {
		return git.StageLines(dir, entry, first, last)
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

// stageShownFile stages all of the file the diff box shows, or, when none of
// it is left to stage, takes all of it back: what "S" does to every file,
// done to one.
func (m model) stageShownFile() (model, tea.Cmd) {
	if !m.canStage() {
		return m, nil
	}
	f, ok := m.selectedFile()
	if !ok {
		return m, nil
	}
	// The file may be two rows, and the one selected may be its staged half
	// while the other still has something to stage.
	_, unstaged := m.stagingDiff().entry(false)
	m.staging = true
	v := &m.commitView
	v.selecting = false
	// Whichever row of the file is left is the one to be on.
	v.want = []fileKey{{f.Path, unstaged}, {f.Path, !unstaged}}
	v.want = append(v.want, neighbours(m.viewedFiles(), v.file)...)
	path := f.Path
	return m, stageCmd(m.repoPath, m.diffStatWidth(), func(dir string) (string, error) {
		if unstaged {
			return git.StageFile(dir, path)
		}
		return git.UnstageFile(dir, path)
	})
}

// neighbours are the entries to fall back on when the file at i has left the
// list altogether: the nearest below it on its side of the index, then the
// nearest above. The list is in path order with the two sides mixed, so the
// nearest may be several rows away.
func neighbours(files []fileDiff, i int) []fileKey {
	var keys []fileKey
	for j := i + 1; j < len(files); j++ {
		if files[j].Staged == files[i].Staged {
			keys = append(keys, keyOf(files[j]))
			break
		}
	}
	for j := i - 1; j >= 0; j-- {
		if files[j].Staged == files[i].Staged {
			keys = append(keys, keyOf(files[j]))
			break
		}
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
		// Nothing moved, so there is nothing for the cursor to follow.
		m.commitView.follow = nil
	}
	if len(m.commits) > 0 && m.commits[0].WorkingTree && msg.summary != "" {
		m.commits[0].Message = msg.summary
	}
	m.setWorkingDiff(msg.diff)

	// "c" with nothing staged staged everything so as to commit it, and
	// this is that staging done: on to the message.
	if v := &m.commitView; v.commitNext {
		v.commitNext = false
		if n := stagedCount(m.viewedFiles()); msg.err == nil && n > 0 {
			m = m.showCommitPrompt(n)
		}
	}
	return m
}

// autoStagedNotice is the status line while the message box is open on files
// "c" staged itself.
func autoStagedNotice(files int) string {
	return "Nothing was staged, so all changes were: " + plural(files, "file") + " in this commit — esc leaves them staged"
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
	var before stagingDiff
	if m.commitView.open {
		before = m.stagingDiff()
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
	key := keyOf(d.Files[v.file])
	if key.path != v.shown.path {
		// A different file: the line the cursor was on is another file's.
		v.diffCursor, v.diffScroll = 0, 0
		v.selecting = false
	} else if !m.stagingDiff().same(before) {
		// The same file with other lines in it: the ones picked were picked
		// by number, and the numbers have moved. A reload that found nothing
		// new here — it may have been for a fetch — leaves them picked.
		//
		// The cursor is left where it is, on the same file's other row too:
		// the box shows both sides, so what was just staged is still under
		// it, with another mark.
		v.selecting = false
	}
	v.shown = key
	v.diffCursor = max(0, min(v.diffCursor, m.diffLineCount()-1))
	m.followCursor()
}

// cursorFollow is where the diff's cursor belongs after "s" has moved what
// it was on, the diff having been read again in between.
//
// A hunk moved from its header changes its mark where it stands, and the
// cursor stays on that header: across is true, and line is the header as it
// will be on the other side. Lines moved from inside a hunk leave it for a
// hunk of their own some rows away, and following them there would take the
// cursor out of what is still being worked through. So it stays in the hunk
// they left, on the line that came after them: pressing "s" again takes the
// next line, and a hunk is staged a line at a time without moving a finger.
type cursorFollow struct {
	line   stagingLine
	across bool
}

// followAfter says where the cursor should be after "s" on first..last of
// entry's diff, given the box as it is now.
func (m model) followAfter(d stagingDiff, entry fileDiff, first, last int) *cursorFollow {
	l, ok := m.cursorLine(d)
	if !ok || l.staged != entry.Staged {
		return nil
	}
	if !m.commitView.selecting && m.cursorTakesHunk(d) {
		l.staged = !l.staged
		return &cursorFollow{line: l, across: true}
	}
	// The line after the last one moved, in the same entry's diff. With none
	// there, the last one moved is the nearest thing to aim for.
	var after *stagingLine
	for i := range d.lines {
		c := &d.lines[i]
		if c.staged != entry.Staged {
			continue
		}
		if c.at == last+1 {
			return &cursorFollow{line: *c}
		}
		if c.at == last {
			after = c
		}
	}
	if after == nil {
		return nil
	}
	return &cursorFollow{line: *after}
}

// followCursor puts the cursor where cursorFollow says, in the diff as it is
// now.
//
// A line on the side it was on is found by what staging cannot have changed:
// its number in the file that side is counted against which staging does not
// touch — the working tree for an unstaged line, HEAD for a staged one — and
// failing that its text, nearest to where it was in its diff. A hunk's header
// on the other side is found by its text, nearest to where the cursor is.
func (m *model) followCursor() {
	v := &m.commitView
	follow := v.follow
	v.follow = nil
	if follow == nil {
		return
	}
	want := follow.line
	lines := m.stagingDiff().lines

	best, bestScore := -1, 0
	consider := func(i, score int) {
		if best < 0 || score < bestScore {
			best, bestScore = i, score
		}
	}
	for i, l := range lines {
		if l.staged != want.staged {
			continue
		}
		switch {
		case follow.across:
			if l.text == want.text {
				consider(i, abs(i-v.diffCursor))
			}
		case stableNumber(l) != 0 && stableNumber(l) == stableNumber(want) && l.text == want.text:
			// The very line: nothing else can score this low.
			consider(i, -1)
		case l.text == want.text:
			consider(i, abs(l.at-want.at))
		}
	}
	if best < 0 && !follow.across {
		// It reads differently now, as a header does once the lines under
		// it are counted again: whatever is where it was.
		for i, l := range lines {
			if l.staged == want.staged {
				consider(i, abs(l.at-want.at))
			}
		}
	}
	if best >= 0 {
		v.diffCursor = best
	}
}

// stableNumber is a line's number in the file staging leaves alone, or 0 when
// it has none there: an unstaged line's place in the working tree, a staged
// line's place in HEAD.
func stableNumber(l stagingLine) int {
	if l.staged {
		return l.num.Old
	}
	return l.num.New
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
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

// pickedLines is what "s" in the diff box acts on: the entry of the file list
// the lines belong to, and the range of that entry's diff. That is the lines
// picked with "v"; or else, by where the cursor is, a whole hunk from its
// header or the one line under it. See cursorTakesHunk.
//
// The box may show two entries' hunks, and a range picked across both cannot
// be staged and unstaged at once; it means the side it was started on, and
// the other side's lines in between are left alone. selected is the entry to
// fall back on when the box has no lines.
func (m model) pickedLines(d stagingDiff, selected fileDiff) (entry fileDiff, first, last int) {
	if len(d.lines) == 0 {
		return selected, 0, 0
	}
	v := m.commitView
	clamp := func(i int) int { return max(0, min(i, len(d.lines)-1)) }
	if v.selecting {
		lo, hi := clamp(min(v.diffAnchor, v.diffCursor)), clamp(max(v.diffAnchor, v.diffCursor))
		side := d.lines[clamp(v.diffAnchor)].staged
		first, last = -1, -1
		for _, l := range d.lines[lo : hi+1] {
			if l.staged != side {
				continue
			}
			if first < 0 {
				first = l.at
			}
			last = l.at
		}
		entry, _ = d.entry(side)
		return entry, first, last
	}

	cursor := d.lines[clamp(v.diffCursor)]
	entry, _ = d.entry(cursor.staged)
	if !m.cursorTakesHunk(d) {
		return entry, cursor.at, cursor.at
	}
	lines := strings.Split(entry.Body, "\n")
	first, last = min(cursor.at, len(lines)-1), len(lines)-1
	for first > 0 && !strings.HasPrefix(lines[first], "@@") {
		first--
	}
	for i := cursor.at + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "@@") {
			last = i - 1
			break
		}
	}
	return entry, first, last
}

// cursorTakesHunk reports whether "s", with nothing picked, would move a
// whole hunk and not just the line under the cursor. A hunk's header stands
// for the hunk: on it the key takes all of it, and on a line below it that
// line alone, so one key reaches both sizes without a mode to enter.
//
// Lines that are under no header — an untracked file is drawn as its
// contents, a binary one as a note — go together as before: there is no
// header to press for all of them.
func (m model) cursorTakesHunk(d stagingDiff) bool {
	l, ok := m.cursorLine(d)
	if !ok || strings.HasPrefix(l.text, "@@") {
		return true
	}
	entry, _ := d.entry(l.staged)
	lines := strings.Split(entry.Body, "\n")
	for i := min(l.at, len(lines)-1); i >= 0; i-- {
		if strings.HasPrefix(lines[i], "@@") {
			return false
		}
	}
	return true
}

// linePicked reports whether line i of the diff box is under the cursor or
// in the selection, and so drawn as a band. A selection only takes the lines
// of the side it was started on, which are the ones "s" would move.
func (m model) linePicked(d stagingDiff, i int) bool {
	v := m.commitView
	if !v.workingTree || v.focus != commitBoxDiff || i >= len(d.lines) {
		return false
	}
	if v.selecting {
		anchor := max(0, min(v.diffAnchor, len(d.lines)-1))
		return i >= min(v.diffAnchor, v.diffCursor) && i <= max(v.diffAnchor, v.diffCursor) &&
			d.lines[i].staged == d.lines[anchor].staged
	}
	return i == v.diffCursor
}

// moveDiffCursor walks the cursor through the diff with the keys that scroll
// it on a commit. The window follows the cursor; see diffTop.
func (m model) moveDiffCursor(msg tea.KeyMsg) model {
	if _, ok := m.selectedFile(); !ok {
		return m
	}
	v := &m.commitView
	last := m.diffLineCount() - 1
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
	if _, ok := m.selectedFile(); !v.workingTree || !ok {
		return v.diffScroll
	}
	return followWindow(v.diffScroll, v.diffCursor+diffHeaderLines, m.diffLineCount()+diffHeaderLines, rows)
}

// diffLineAt is the line of the diff drawn on screen row y, or -1 when the
// row holds the path, the blank under it, or nothing.
func (m model) diffLineAt(y int) int {
	if _, ok := m.selectedFile(); !ok {
		return -1
	}
	l := m.currentCommitViewLayout()
	row := y - commitViewTopRow - 1 // the box's border takes the first row
	if row < 0 || row >= l.rows-2 {
		return -1
	}
	i := m.diffTop(l.rows-2) + row - diffHeaderLines
	if i < 0 || i >= m.diffLineCount() {
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
	text := ansi.Truncate(expandTabs(line), width, "")
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
