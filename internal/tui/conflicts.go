package tui

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/git"
	"github.com/sevenam/gitraffe/internal/theme"
)

// The conflict view: the commit view of the uncommitted changes while a
// merge or cherry-pick is stopped in conflicts, started in a terminal (or,
// once #245 allows it, here). Where it would otherwise refuse to stage, it
// says where the merge stands: the files that conflicted first, each marked
// ✗ until resolved and ✓ after, with what kind of conflict it is; then what
// merged cleanly, already staged. A file in conflict is shown as it is on
// disk, markers and all, each side in a colour of its own.
//
// From here each file is resolved, and the merge ended. o takes the branch
// checked out's version of a file whole, t the incoming one's, and a marks
// it resolved as it is on disk, once edited; u puts a side taken back in
// conflict. A aborts the merge, after asking, and c commits it once nothing
// is left unresolved. Editing a file is a terminal's work for now;
// auto-refresh or r shows it as it is afterwards.

// abortPrompt is the question A asks before the merge is thrown away.
type abortPrompt struct {
	asking bool
	// What is being aborted, as the conflict view's header names it.
	op, theirs, ours string
}

type abortedMsg struct {
	repoPath string // the repository aborted in; see the handler
	op       string
	ours     string
	detail   string
	err      error
}

// mergeMessageMsg is git's message for the merge under way, read for the
// box c opens.
type mergeMessageMsg struct {
	repoPath string
	message  string
}

// inConflict reports whether the commit view is the conflict view: on the
// uncommitted changes, with a merge or cherry-pick under way.
func (m model) inConflict() bool {
	return m.commitView.open && m.commitView.workingTree && m.conflict.Operation != ""
}

// conflictCounts is how many of the files that conflicted are still
// unresolved, and how many conflicted in all.
func conflictCounts(files []fileDiff) (left, total int) {
	for _, f := range files {
		if f.Conflicted {
			total++
			if !f.Resolved {
				left++
			}
		}
	}
	return left, total
}

// conflictWords is what the header calls the operation, and the word between
// the two names: "Merging x into main", "Cherry-picking x onto main".
func conflictWords(op string) (verb, join string) {
	if op == "cherry-pick" {
		return "Cherry-picking", "onto"
	}
	return "Merging", "into"
}

// conflictProgress is the part of the header after the names: how far the
// resolving has got, and what to do once it is done.
func conflictProgress(left, total int) string {
	switch {
	case total == 0:
		return "no conflicts — c commits"
	case left == 0:
		return "all " + plural(total, "conflict") + " resolved — c commits"
	}
	files := "files"
	if total == 1 {
		files = "file"
	}
	return fmt.Sprintf("%d of %d %s still in conflict", left, total, files)
}

// conflictHeader is the header box's line in the conflict view:
// "Merging feature/login into main — 2 of 5 files still in conflict".
func (m model) conflictHeader() string {
	s := m.conflict
	verb, join := conflictWords(s.Operation)
	left, total := conflictCounts(m.viewedFiles())
	bold := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Title))
	progress := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.Tag))
	if left == 0 {
		progress = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.DiffAdd))
	}
	// The names in their sides' colours, which are the colours of the
	// sides in the diff box below.
	side := func(r conflictRegion) lipgloss.Style {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(conflictColour(r)))
	}
	return bold.Render(verb+" ") + side(regionTheirs).Render(s.Theirs) + bold.Render(" "+join+" ") +
		side(regionOurs).Render(s.Ours) + helpStyle.Render(" — ") + progress.Render(conflictProgress(left, total))
}

// conflictKindLabel says what kind of conflict a file is, with the sides
// called by their names: "deleted by feature/login" says which side did it,
// where git's "deleted by them" leaves you to work out who "them" is.
func conflictKindLabel(f fileDiff, s git.ConflictState) string {
	switch f.Kind {
	case git.DeletedByUs:
		return "deleted by " + s.Ours
	case git.DeletedByThem:
		return "deleted by " + s.Theirs
	case git.AddedByUs:
		return "added by " + s.Ours
	case git.AddedByThem:
		return "added by " + s.Theirs
	}
	return f.Kind.String()
}

// The marks a conflicted file's row carries in place of the staged marks.
const (
	unresolvedMarker = "✗"
	resolvedMarker   = "✓"
)

// conflictMark is the coloured mark for a conflicted file's row.
func conflictMark(f fileDiff) string {
	if f.Resolved {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.DiffAdd)).Render(resolvedMarker + " ")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Current.Error)).Bold(true).Render(unresolvedMarker + " ")
}

// conflictNotice is the status line for a key that stages, discards or
// stashes, pressed in the conflict view: during a merge staging a file means
// "this conflict is resolved", which o, t and a say on purpose.
func (m model) conflictNotice() string {
	if left, _ := conflictCounts(m.viewedFiles()); left == 0 {
		return "Everything is resolved — c commits the " + m.conflict.Operation + ", A aborts it"
	}
	return "On a file in conflict: o / t takes a side, a marks it resolved as edited — A aborts the " + m.conflict.Operation
}

// conflictKey handles the keys the conflict view gives a meaning of its own.
// The last result is false for the keys it leaves to the commit view.
func (m model) conflictKey(msg tea.KeyMsg) (model, tea.Cmd, bool) {
	switch msg.String() {
	case "A":
		return m.askAbort(), nil, true
	case "c":
		next, cmd := m.openContinue()
		return next, cmd, true
	case "o", "t", "a", "u":
		next, cmd := m.resolveSelected(msg.String())
		return next, cmd, true
	case "d", "D", "s", "S", "v":
		m.notice = m.conflictNotice()
		return m, nil, true
	}
	return m, nil, false
}

// sideHas reports whether side's version of a conflicted file is there to
// take: a side that deleted the file, or never had it, has none, and taking
// it deletes the file.
func sideHas(kind git.ConflictKind, side git.Side) bool {
	switch kind {
	case git.BothDeleted:
		return false
	case git.DeletedByUs, git.AddedByThem:
		return side == git.Theirs
	case git.DeletedByThem, git.AddedByUs:
		return side == git.Ours
	}
	return true
}

// sideHint is what o or t does to a file, with the side named: "ours" and
// "theirs" swap meaning between a merge and a rebase, and a branch's name
// does not.
func sideHint(f fileDiff, s git.ConflictState, side git.Side) string {
	name, verb := s.Ours, "keep "
	if side == git.Theirs {
		name, verb = s.Theirs, "take "
	}
	if !sideHas(f.Kind, side) {
		return "delete, as " + name
	}
	return verb + name
}

// conflictHints is the status line's keys in the conflict view, for the file
// selected: how to resolve it, how to take that back, or how to end.
func (m model) conflictHints() string {
	op := m.conflict.Operation
	if f, ok := m.selectedFile(); ok && f.Conflicted {
		if !f.Resolved {
			return "?: help • o: " + sideHint(f, m.conflict, git.Ours) + " • t: " + sideHint(f, m.conflict, git.Theirs) +
				" • a: mark resolved • A: abort"
		}
		return "?: help • u: back in conflict • c: commit the " + op + " • A: abort it"
	}
	return "?: help • space: back • c: commit the " + op + " • A: abort it • w: whitespace"
}

// resolveSelected is o, t, a and u on the file selected, from either box:
// each takes a file whole.
func (m model) resolveSelected(key string) (model, tea.Cmd) {
	if m.staging || m.committing || m.aborting {
		return m, nil
	}
	f, ok := m.selectedFile()
	if !ok {
		return m, nil
	}
	if !f.Conflicted {
		m.notice = f.Path + " merged without a conflict — there is nothing to resolve in it"
		return m, nil
	}
	path, s := f.Path, m.conflict
	var change func(dir string) (string, error)
	var done string
	switch {
	case key == "u" && !f.Resolved:
		m.notice = path + " is still in conflict — u puts back a file resolved with o, t or a"
		return m, nil
	case key == "u":
		change = func(dir string) (string, error) { return git.Unresolve(dir, path) }
		done = path + " is in conflict again"
	case f.Resolved:
		m.notice = path + " is resolved — u puts it back in conflict first"
		return m, nil
	case key == "a":
		change = func(dir string) (string, error) { return git.MarkResolved(dir, path) }
		done = path + " marked resolved"
	default:
		side, name := git.Ours, s.Ours
		if key == "t" {
			side, name = git.Theirs, s.Theirs
		}
		change = func(dir string) (string, error) { return git.TakeSide(dir, path, side) }
		done = path + ": " + name + "'s version taken — u puts the conflict back"
		if !sideHas(f.Kind, side) {
			done = path + " deleted, as on " + name + " — u puts the conflict back"
		}
	}

	m.staging = true
	files := m.viewedFiles()
	v := &m.commitView
	// The same file, resolved or not: its mark changes, and its row stays.
	v.want = []fileKey{{path, true}, {path, false}}
	v.want = append(v.want, neighbours(files, v.file)...)
	cmd := stageCmd(m.repoPath, m.diffStatWidth(), change)
	return m, func() tea.Msg {
		msg := cmd().(stageFinishedMsg)
		msg.resolved = done
		msg.resolveFailed = "Not resolved: "
		if key == "u" {
			msg.resolveFailed = "Not put back: "
		}
		return msg
	}
}

// resolveFailedNotice says why o, t, a or u changed nothing.
func resolveFailedNotice(msg stageFinishedMsg) string {
	switch {
	case errors.Is(msg.err, git.ErrConflictMarkers):
		return msg.resolveFailed + "conflict markers are left in it — edit them out, or take a side with o / t"
	case errors.Is(msg.err, git.ErrResolvedByHand):
		return msg.resolveFailed + "it was resolved by hand or changed since, and u would write over that — git checkout -m in a terminal does it"
	case errors.Is(msg.err, git.ErrNotResolved), errors.Is(msg.err, git.ErrNotUnmerged):
		return msg.resolveFailed + "it changed since it was read — here it is as it is now"
	}
	reason := firstLine(msg.detail)
	if reason == "" {
		reason = msg.err.Error()
	}
	return msg.resolveFailed + reason
}

// askAbort opens the question A asks.
func (m model) askAbort() model {
	if m.staging || m.committing || m.aborting {
		return m
	}
	s := m.conflict
	m.abortMerge = abortPrompt{asking: true, op: s.Operation, theirs: s.Theirs, ours: s.Ours}
	return m
}

// answerAbort aborts on enter or "y" and on nothing else, as a discard is
// answered: what is lost is the resolving done so far.
func (m model) answerAbort(msg tea.KeyMsg) (model, tea.Cmd) {
	p := m.abortMerge
	m.abortMerge = abortPrompt{}
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter", "y", "Y":
	default:
		m.notice = "Nothing aborted — the " + p.op + " is still in progress"
		return m, nil
	}
	m.aborting = true
	repoPath := m.repoPath
	return m, func() tea.Msg {
		detail, err := git.AbortMerge(repoPath)
		return abortedMsg{repoPath: repoPath, op: p.op, ours: p.ours, detail: detail, err: err}
	}
}

// finishAbort goes back to the graph, read again, with the branch where it
// was before the merge started.
func (m model) finishAbort(msg abortedMsg) (model, tea.Cmd) {
	m.aborting = false
	if msg.err != nil {
		reason := firstLine(msg.detail)
		if errors.Is(msg.err, git.ErrNoMerge) || reason == "" {
			reason = msg.err.Error()
		}
		m.notice = "Not aborted: " + reason
		return m, nil
	}
	m.commitView = commitView{}
	if m.commitPrompt.merge {
		m.commitPrompt = commitPrompt{}
	}
	next, cmd := m.reloadRepo()
	op := strings.ToUpper(msg.op[:1]) + msg.op[1:]
	next.notice = op + " aborted — " + msg.ours + " is back as it was"
	return next, cmd
}

// render draws the question, made like the discard question.
func (p abortPrompt) render(windowWidth int) string {
	keys := [][2]string{{"enter", "abort"}, {"esc", "keep going"}}
	key := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.Current.SelectedFg))
	var plain, footer []string
	for _, k := range keys {
		plain = append(plain, k[0]+": "+k[1])
		footer = append(footer, key.Render(k[0])+helpStyle.Render(": "+k[1]))
	}
	verb, join := conflictWords(p.op)
	title := "Abort the " + p.op + "?"
	what := verb + " " + p.theirs + " " + join + " " + p.ours + " stops."
	note := "Everything goes back to how it was before, resolved files too."
	width := max(ansi.StringWidth(what), ansi.StringWidth(note), ansi.StringWidth(strings.Join(plain, " • ")))
	// The border and the padding take six columns.
	width = max(10, min(width, windowWidth-6))

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render(ansi.Truncate(title, width, "…")))
	sb.WriteString("\n\n")
	sb.WriteString(ansi.Truncate(what, width, "…") + "\n")
	sb.WriteString(ansi.Truncate(note, width, "…"))
	sb.WriteString("\n\n")
	sb.WriteString(ansi.Truncate(strings.Join(footer, helpStyle.Render(" • ")), width, "…"))
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(theme.Current.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}

// openContinue is c in the conflict view: the message box, once nothing is
// left unresolved, holding git's message for the merge. A message typed
// for this merge before and left with esc is kept.
func (m model) openContinue() (model, tea.Cmd) {
	if m.staging || m.committing || m.aborting {
		return m, nil
	}
	if left, _ := conflictCounts(m.viewedFiles()); left > 0 {
		m.notice = plural(left, "file") + " still in conflict — o / t takes a side, a marks a file resolved as edited"
		return m, nil
	}
	if m.workingState.Detached {
		m.notice = "HEAD is on no branch — finish this " + m.conflict.Operation + " in a terminal"
		return m, nil
	}
	if m.commitPrompt.merge && m.commitPrompt.started {
		return m.showCommitPrompt(0), nil
	}
	repoPath := m.repoPath
	return m, func() tea.Msg {
		return mergeMessageMsg{repoPath: repoPath, message: git.MergeMessage(repoPath)}
	}
}

// finishMergeMessage opens the box on git's message, if the view is still
// where c was pressed.
func (m model) finishMergeMessage(msg mergeMessageMsg) model {
	if !m.inConflict() {
		return m
	}
	m.commitPrompt = commitPrompt{merge: true}
	m = m.showCommitPrompt(0)
	subject, body, _ := strings.Cut(msg.message, "\n")
	m.commitPrompt.subject.SetValue(strings.TrimSpace(subject))
	m.commitPrompt.body.SetValue(strings.TrimSpace(body))
	return m
}

func continueCmd(repoPath, message string) tea.Cmd {
	return func() tea.Msg {
		result, err := git.ContinueMerge(repoPath, message)
		return commitFinishedMsg{repoPath: repoPath, result: result, err: err}
	}
}

// renderContinuePrompt is the message box over the conflict view.
func (m model) renderContinuePrompt() string {
	s := m.conflict
	title, join := "Merge commit", " into "
	if s.Operation == "cherry-pick" {
		title, join = "Cherry-pick", " onto "
	}
	return m.commitPrompt.renderAs(m.windowWidth, title, "  "+s.Theirs+join+s.Ours, "commit", "cancel")
}

// conflictSummary is the graph's top row while a merge or cherry-pick is
// under way, so that it is never missed: "merge in progress — 2 conflicts".
func conflictSummary(op string, left int) string {
	switch left {
	case 0:
		return op + " in progress — conflicts resolved"
	}
	return op + " in progress — " + plural(left, "conflict")
}

// A file in conflict is drawn in regions: what is outside any conflict, and
// inside one the branch checked out's side, the base (diff3 and zdiff3 write
// it), and the side coming in.
type conflictRegion int

const (
	regionOutside conflictRegion = iota
	regionOurs
	regionBase
	regionTheirs
)

// conflictLine is one line of a file in conflict, with the region it is in;
// marker is set on the lines git wrote to divide them.
type conflictLine struct {
	region conflictRegion
	marker bool
}

// conflictOpen is the line a conflict starts with: seven or more "<", then a
// space or nothing. The rest of the conflict's markers are the same length,
// which is how a line of "=" in the file itself is told from one git wrote.
var conflictOpen = regexp.MustCompile(`^(<{7,})( |$)`)

// conflictRegions reads where each line of a file is. A conflict that is
// never closed runs to the end, as git would read it.
func conflictRegions(lines []string) []conflictLine {
	out := make([]conflictLine, len(lines))
	region, size := regionOutside, 0
	opens := func(line string, c byte) bool {
		return len(line) >= size && strings.Count(line[:size], string(c)) == size &&
			(len(line) == size || line[size] == ' ')
	}
	for i, line := range lines {
		switch {
		case region == regionOutside:
			if m := conflictOpen.FindStringSubmatch(line); m != nil {
				region, size = regionOurs, len(m[1])
				out[i] = conflictLine{region: regionOurs, marker: true}
				continue
			}
		case region == regionOurs && opens(line, '|'):
			region = regionBase
			out[i] = conflictLine{region: regionBase, marker: true}
			continue
		case (region == regionOurs || region == regionBase) && line == strings.Repeat("=", size):
			region = regionTheirs
			out[i] = conflictLine{region: regionTheirs, marker: true}
			continue
		case region == regionTheirs && opens(line, '>'):
			out[i] = conflictLine{region: regionTheirs, marker: true}
			region = regionOutside
			continue
		}
		out[i] = conflictLine{region: region}
	}
	return out
}

// conflictColour is the colour of a region, from the theme's conflict keys
// or, where it leaves them out, colours it already has that are told apart
// in every bundled theme; "" for what is outside a conflict.
func conflictColour(r conflictRegion) string {
	c := theme.Current
	switch r {
	case regionOurs:
		return firstColour(c.ConflictOurs, c.Branch)
	case regionBase:
		return firstColour(c.ConflictBase, c.Help)
	case regionTheirs:
		return firstColour(c.ConflictTheirs, c.Hash)
	}
	return ""
}

// renderConflictFile draws a file in conflict, as it is on disk: its lines
// in their region's colour, the markers in bold, and the line under the
// cursor as a band.
func (m model) renderConflictFile(sb *strings.Builder, d stagingDiff, width int) {
	texts := make([]string, len(d.lines))
	for i, l := range d.lines {
		texts[i] = l.text
	}
	regions := conflictRegions(texts)
	gutter := newDiffGutter(d.numbers(), false)
	for i, text := range texts {
		r := regions[i]
		colour := conflictColour(r.region)
		if m.linePicked(d, i) {
			if colour == "" {
				colour = theme.Current.SelectedFg
			}
			sb.WriteString(gutter.renderPicked(i))
			sb.WriteString(bandLine(text, colour, width-gutter.width(), m.showWhitespace))
			sb.WriteString("\n")
			continue
		}
		sb.WriteString(gutter.render(i))
		style := lipgloss.NewStyle()
		if colour != "" {
			style = style.Foreground(lipgloss.Color(colour))
		}
		if r.marker {
			style = style.Bold(true)
		}
		if m.showWhitespace {
			sb.WriteString(renderWhitespace(text, style))
		} else {
			sb.WriteString(style.Render(expandTabs(text)))
		}
		sb.WriteString("\n")
	}
}

// bandLine is a line drawn as the cursor's band, the width of the box, in
// one colour.
func bandLine(text, colour string, width int, whitespace bool) string {
	shown := expandTabs(text)
	if whitespace {
		shown = markWhitespace(text)
	}
	shown = ansi.Truncate(shown, width, "")
	if pad := width - ansi.StringWidth(shown); pad > 0 {
		shown += strings.Repeat(" ", pad)
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(theme.Current.SelectedBg)).
		Foreground(lipgloss.Color(colour)).
		Bold(true).
		Render(shown)
}

// conflictFileRow is fileRow for a file that conflicted: "✗ path  both
// modified", the kind where the counts go. The kind is cut before the path
// is squeezed below half the row: the path says which file, the kind only
// what happened to it.
func conflictFileRow(f fileDiff, width int, selected bool, kind string) string {
	marker := "  "
	if selected {
		marker = "> "
	}
	width -= 2 // the mark
	room := max(width-2, 1)
	// What the path leaves, and never less than half.
	kindRoom := min(ansi.StringWidth(kind), max(room-ansi.StringWidth(f.Path)-1, room/2))
	kind = ansi.Truncate(kind, kindRoom, "…")
	pathRoom := max(room-ansi.StringWidth(kind)-1, 1)
	path := truncateLeft(f.Path, pathRoom)
	gap := strings.Repeat(" ", max(room-ansi.StringWidth(path)-ansi.StringWidth(kind), 1))

	if !selected {
		return marker + conflictMark(f) + path + gap + helpStyle.Render(kind)
	}
	band := lipgloss.NewStyle().Background(lipgloss.Color(theme.Current.SelectedBg))
	mark := unresolvedMarker + " "
	if f.Resolved {
		mark = resolvedMarker + " "
	}
	return band.Foreground(lipgloss.Color(theme.Current.SelectedFg)).Bold(true).Render(marker+mark+path) +
		band.Render(gap+kind)
}
