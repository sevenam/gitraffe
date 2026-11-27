package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
)

// Conflict blocks: in the conflict view's diff box, o, t, b and B on a
// conflict block (from <<<<<<< to >>>>>>>) resolve that block alone, in the
// file on disk; n and N go from block to block, and on to the next file in
// conflict. When a file's last block is resolved the file is marked
// resolved, and the cursor goes on to the next file in conflict.
//
// Each choice is undone with u: the file's text before it is kept here, for
// as long as the merge is under way and gitraffe runs, and put back only if
// the file is still what the choice made it. Without that memory (gitraffe
// started again, or the file edited since) u can only write every conflict of
// the file back in as git first wrote them, which throws away what was
// resolved in it by hand too, so it asks first.
//
// Which block a row of the diff box is in is answered by conflictBlocks
// alone, for the drawing, the keys and the status line: the rows are the
// file's lines, in order, so a row's index is its line in the file.

// blockStep is one block choice, kept for u.
type blockStep struct {
	before, after string // the file's text before and after
	line          int    // the block's first line, where u puts the cursor
}

// blockUndo is the block choices made in the merge under way, newest last,
// by path. repo is the repository they were made in.
type blockUndo struct {
	repo  string
	steps map[string][]blockStep
}

// stepsFor is path's choices in repo, oldest first.
func (u blockUndo) stepsFor(repo, path string) []blockStep {
	if u.repo != repo {
		return nil
	}
	return u.steps[path]
}

// push records a choice.
func (u *blockUndo) push(repo, path string, s blockStep) {
	if u.repo != repo || u.steps == nil {
		*u = blockUndo{repo: repo, steps: map[string][]blockStep{}}
	}
	u.steps[path] = append(u.steps[path], s)
}

// pop forgets path's newest choice.
func (u *blockUndo) pop(path string) {
	if steps := u.steps[path]; len(steps) > 0 {
		u.steps[path] = steps[:len(steps)-1]
	}
}

// forget drops what is kept for path: a choice made since of the whole file
// leaves nothing for them to undo.
func (u *blockUndo) forget(path string) {
	delete(u.steps, path)
}

// blockDone is what a block choice, or its undo, did; carried on
// stageFinishedMsg.
type blockDone struct {
	path   string
	undo   bool
	index  int // the block resolved, of the blocks there were
	step   blockStep
	result git.BlockResult
}

// conflictBlocks is the conflict blocks of the file the diff box shows: none
// unless it is a file still in conflict, drawn as it is on disk.
func conflictBlocks(d stagingDiff) []git.ConflictBlock {
	f := d.unstaged
	if f == nil || !f.Conflicted || f.Resolved || f.Binary {
		return nil
	}
	return git.ConflictBlocks(d.texts(), f.MarkerSize)
}

// texts is the text of each line of the box.
func (d stagingDiff) texts() []string {
	out := make([]string, len(d.lines))
	for i, l := range d.lines {
		out[i] = l.text
	}
	return out
}

// blockAt is the index of the block line i is in, or -1.
func blockAt(blocks []git.ConflictBlock, i int) int {
	for k, b := range blocks {
		if b.Contains(i) {
			return k
		}
	}
	return -1
}

// cursorBlock is the block the diff box's cursor is in, when the diff box
// has the focus.
func (m model) cursorBlock(d stagingDiff) (int, []git.ConflictBlock) {
	blocks := conflictBlocks(d)
	if m.commitView.focus != commitBoxDiff {
		return -1, blocks
	}
	return blockAt(blocks, m.commitView.diffCursor), blocks
}

// blockKeys are the keys that pick for a block, and what each picks.
var blockKeys = map[string]git.Pick{
	"o": git.PickOurs,
	"t": git.PickTheirs,
	"b": git.PickBoth,
	"B": git.PickBothTheirsFirst,
}

// blockKey is o, t, b and B in the conflict view. In the diff box of a file
// with blocks they take the block under the cursor; the last result is
// false where o and t are left to take the whole file instead.
func (m model) blockKey(key string) (model, tea.Cmd, bool) {
	d := m.stagingDiff()
	k, blocks := m.cursorBlock(d)
	both := key == "b" || key == "B"
	switch {
	case k >= 0:
		next, cmd := m.pickBlock(d, k, blocks[k], blockKeys[key])
		return next, cmd, true
	case m.commitView.focus == commitBoxDiff && len(blocks) > 0:
		m.notice = "The cursor is on no conflict block — n goes to the next; o / t in the file list take the whole file"
		return m, nil, true
	case both:
		m.notice = "b / B keep both sides of a conflict block — n puts the cursor on one, in the diff"
		return m, nil, true
	}
	return m, nil, false
}

// pickName is what the status line says a pick kept.
func pickName(p git.Pick, s git.ConflictState) string {
	switch p {
	case git.PickOurs:
		return s.Ours + "'s lines kept"
	case git.PickTheirs:
		return s.Theirs + "'s lines taken"
	case git.PickBoth:
		return "both kept, " + s.Ours + "'s first"
	}
	return "both kept, " + s.Theirs + "'s first"
}

// pickBlock resolves block k of the file shown as pick.
func (m model) pickBlock(d stagingDiff, k int, b git.ConflictBlock, pick git.Pick) (model, tea.Cmd) {
	if m.staging || m.committing || m.aborting {
		return m, nil
	}
	path, s := d.unstaged.Path, m.conflict
	shown := d.texts()[b.Start : b.End+1]
	done := &blockDone{path: path, index: k, step: blockStep{line: b.Start}}
	change := func(dir string) (string, error) {
		r, err := git.ResolveBlock(dir, path, k, shown, pick)
		done.result = r
		done.step.before, done.step.after = r.Before, r.After
		return r.Output, err
	}
	return m.runBlockChange(path, done, change, func() string {
		r := done.result
		switch {
		case r.Resolved:
			return path + ": " + pickName(pick, s) + ", the last block — marked resolved; u undoes"
		case r.Left == 0:
			return path + ": " + pickName(pick, s) + ", the last block, but lines git reads as conflict markers are left — e edits them out, then a"
		}
		return path + ": " + pickName(pick, s) + " — " + plural(r.Left, "block") + " left; u undoes"
	})
}

// undoBlock is u on a file with a block choice to undo.
func (m model) undoBlock(path string, step blockStep) (model, tea.Cmd) {
	done := &blockDone{path: path, undo: true, step: step}
	change := func(dir string) (string, error) {
		return git.RestoreConflictText(dir, path, step.before, step.after)
	}
	return m.runBlockChange(path, done, change, func() string {
		return path + ": the last block choice undone"
	})
}

// runBlockChange makes a block change and reads the uncommitted changes
// again, as staging does; notice is the status line once it is done.
func (m model) runBlockChange(path string, done *blockDone, change func(dir string) (string, error), notice func() string) (model, tea.Cmd) {
	m.staging = true
	files := m.viewedFiles()
	v := &m.commitView
	v.want = []fileKey{{path, false}, {path, true}}
	v.want = append(v.want, neighbours(files, v.file)...)
	cmd := stageCmd(m.repoPath, m.diffStatWidth(), change)
	return m, func() tea.Msg {
		msg := cmd().(stageFinishedMsg)
		msg.block = done
		msg.resolved = notice()
		msg.resolveFailed = "Not resolved: "
		if done.undo {
			msg.resolveFailed = "Not undone: "
		}
		return msg
	}
}

// finishBlock remembers a block choice, or forgets the one undone, and puts
// the cursor where the work goes on: the next block, the next file in
// conflict, or the block put back.
func (m model) finishBlock(msg stageFinishedMsg) model {
	done := msg.block
	if msg.err != nil {
		if done.undo && errors.Is(msg.err, git.ErrResolvedByHand) {
			// Nothing kept can be put back now.
			m.blockUndo.forget(done.path)
			m.notice = "Not undone: " + done.path + " changed since the last block choice — u again brings back all its conflicts, after asking"
		}
		return m
	}
	v := &m.commitView
	if done.undo {
		m.blockUndo.pop(done.path)
		v.focus, v.diffCursor = commitBoxDiff, max(0, min(done.step.line, m.diffLineCount()-1))
		return m
	}
	m.blockUndo.push(m.repoPath, done.path, done.step)
	if done.result.Resolved {
		return m.nextFileInConflict()
	}
	blocks := conflictBlocks(m.stagingDiff())
	switch {
	case done.index < len(blocks):
		v.diffCursor = blocks[done.index].Start
	case len(blocks) > 0:
		v.diffCursor = blocks[len(blocks)-1].Start
	}
	return m
}

// nextFileInConflict selects the file in conflict after the one selected,
// with the cursor on its first block; with none left, the selection stays.
func (m model) nextFileInConflict() model {
	files := m.viewedFiles()
	v := &m.commitView
	for step := 1; step < len(files); step++ {
		j := (v.file + step) % len(files)
		if f := files[j]; f.Conflicted && !f.Resolved {
			v.file = j
			m.showFile()
			if blocks := conflictBlocks(m.stagingDiff()); len(blocks) > 0 {
				v.diffCursor = blocks[0].Start
			}
			return m
		}
	}
	return m
}

// jumpBlock is n (dir 1) and N (dir -1): the cursor to the next or previous
// conflict block, in the file shown and then the files after or before it,
// coming round to the first again. It leaves the focus on the diff box,
// where o, t, b and B take the block.
func (m model) jumpBlock(dir int) model {
	if _, ok := m.selectedFile(); !ok {
		return m
	}
	v := &m.commitView
	d := m.stagingDiff()
	blocks := conflictBlocks(d)
	from := v.diffCursor
	if v.focus != commitBoxDiff {
		// From the file list, the file's first or last block.
		from = -1
		if dir < 0 {
			from = len(d.lines)
		}
	}
	if k := blockAt(blocks, from); k >= 0 && dir < 0 {
		from = blocks[k].Start
	}
	v.focus = commitBoxDiff
	if dir > 0 {
		for _, b := range blocks {
			if b.Start > from {
				v.diffCursor = b.Start
				return m
			}
		}
	} else {
		for k := len(blocks) - 1; k >= 0; k-- {
			if blocks[k].Start < from {
				v.diffCursor = blocks[k].Start
				return m
			}
		}
	}

	// On to the other files, and round to this one's first or last.
	files := m.viewedFiles()
	for step := 1; step <= len(files); step++ {
		j := ((v.file+dir*step)%len(files) + len(files)) % len(files)
		other := conflictBlocks(stagingDiffOf(files, j, true))
		if len(other) == 0 {
			continue
		}
		if j != v.file {
			v.file = j
			m.showFile()
		}
		at := other[0]
		if dir < 0 {
			at = other[len(other)-1]
		}
		v.diffCursor = at.Start
		return m
	}
	m.notice = "No conflict blocks are left — files still in conflict are resolved whole: o / t, or e and a"
	return m
}

// blockHints is the status line's keys on a conflict block, with the sides
// named.
func (m model) blockHints() string {
	s := m.conflict
	return "?: help • o: keep " + s.Ours + " • t: take " + s.Theirs + " • b / B: both • n / N: next / previous block • u: undo"
}

// askReconflict asks before u writes every conflict of path back in.
func (m model) askReconflict(path string) model {
	s := m.conflict
	m.abortMerge = abortPrompt{asking: true, op: s.Operation, theirs: s.Theirs, ours: s.Ours, reconflict: path}
	return m
}

// reconflictCmd is the yes to that question.
func (m model) reconflictCmd(path string) (model, tea.Cmd) {
	m.staging = true
	m.blockUndo.forget(path)
	files := m.viewedFiles()
	v := &m.commitView
	v.want = []fileKey{{path, false}, {path, true}}
	v.want = append(v.want, neighbours(files, v.file)...)
	cmd := stageCmd(m.repoPath, m.diffStatWidth(), func(dir string) (string, error) {
		return git.RecreateConflicts(dir, path)
	})
	return m, func() tea.Msg {
		msg := cmd().(stageFinishedMsg)
		msg.resolved = path + ": every conflict is back, as git wrote it"
		msg.resolveFailed = "Not put back: "
		return msg
	}
}

// reconflictLines is the question's text.
func reconflictLines(path string) (title, what, note string) {
	return "Bring back every conflict in " + path + "?",
		"Its conflicts are written back in, as git first wrote them.",
		"What was resolved in it so far, by hand too, is lost."
}
