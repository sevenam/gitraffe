package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// threeFileRepo has one commit touching three files, one of them deep enough
// that its path has to be cut to fit the list.
func threeFileRepo(t *testing.T) string {
	t.Helper()
	dir, git, _ := gitFixture(t)
	git("init", "-q", "-b", "main")
	write(t, dir, "parser.go", "package main\n\nfunc parse() {\n}\n")
	write(t, dir, "README.md", "# project\n")
	git("add", "-A")
	git("commit", "-qm", "first")

	write(t, dir, "parser.go", "package main\n\nfunc parse() int {\n\treturn 42\n}\n")
	write(t, dir, "README.md", "# project\n\nnow with words\n")
	writeFile(t, filepath.Join(dir, "deep", "nested", "dir", "helper.go"), "package deep\n")
	git("add", "-A")
	git("commit", "-qm", "teach the parser about nested groups")
	return dir
}

// longDiffRepo has a first file longer than any box on screen, so there is
// something to scroll through.
func longDiffRepo(t *testing.T) string {
	t.Helper()
	dir, git, _ := gitFixture(t)
	git("init", "-q", "-b", "main")
	write(t, dir, "AAA.txt", "first line\n")
	write(t, dir, "zzz.txt", "other\n")
	git("add", "-A")
	git("commit", "-qm", "first")

	var sb strings.Builder
	for i := range 200 {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	write(t, dir, "AAA.txt", sb.String())
	write(t, dir, "zzz.txt", "other, changed\n")
	git("add", "-A")
	git("commit", "-qm", "a long change")
	return dir
}

// withDiff answers the diff command the model is waiting on, which is what the
// program does between one keystroke and the next.
func withDiff(t *testing.T, m model) model {
	t.Helper()
	cmd := m.maybeLoadDiff()
	if cmd == nil {
		return m
	}
	res, _ := m.Update(cmd())
	return res.(model)
}

// openedView is the commit view on a loaded repository, with the diff in.
func openedView(t *testing.T, dir string) model {
	t.Helper()
	m := withDiff(t, loadedModel(t, dir))
	m.windowWidth, m.windowHeight = 110, 26
	m = press(m, space)
	if !m.commitView.open {
		t.Fatal("v did not open the commit view")
	}
	return m
}

// boxesIn says which of the view's boxes are drawn.
func boxesIn(screen string) (commit, files, diff bool) {
	plain := ansi.Strip(screen)
	return strings.Contains(plain, "[1]-commit"),
		strings.Contains(plain, "[2]-files"),
		strings.Contains(plain, "[3]-diff")
}

func TestVOpensAndEscCloses(t *testing.T) {
	m := openedView(t, threeFileRepo(t))
	if m.commitView.commit != m.selected {
		t.Errorf("view opened on commit %d, want the selected %d", m.commitView.commit, m.selected)
	}
	if c, f, d := boxesIn(m.View()); !c || !f || !d {
		t.Errorf("boxes drawn: commit=%v files=%v diff=%v, want all three", c, f, d)
	}
	// The graph is a different screen, not something behind the boxes.
	if strings.Contains(ansi.Strip(m.View()), "git-graph") {
		t.Error("the graph is still on screen under the commit view")
	}

	back := press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if back.commitView.open {
		t.Error("esc did not close the commit view")
	}
	if _, _, diff := boxesIn(back.View()); diff {
		t.Error("the diff box is still drawn after going back")
	}
}

// q means "back" here, as it does in the help overlay: it was pressed on a
// screen you stepped into, not on the one you started from.
func TestQClosesTheViewWithoutQuitting(t *testing.T) {
	m := openedView(t, threeFileRepo(t))
	res, cmd := m.Update(keyPress("q"))
	if res.(model).commitView.open {
		t.Error("q left the view open")
	}
	if isQuit(cmd) {
		t.Error("q quit gitraffe instead of going back to the graph")
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); !isQuit(cmd) {
		t.Error("ctrl+c did not quit from the commit view")
	}
}

func TestViewListsEveryFileWithItsCounts(t *testing.T) {
	m := openedView(t, threeFileRepo(t))
	files := m.viewedFiles()
	if len(files) != 3 {
		t.Fatalf("files = %v, want three", paths(files))
	}

	screen := ansi.Strip(m.View())
	for _, want := range []string{"README.md", "helper.go", "parser.go"} {
		if !strings.Contains(screen, want) {
			t.Errorf("%s is not in the file list:\n%s", want, screen)
		}
	}
	// The count is in the box's own label, so it is answered without counting.
	if !strings.Contains(screen, "[2]-files-(3)") {
		t.Error("the files box does not say how many files there are")
	}
	readme := findFile(t, files, "README.md")
	if !strings.Contains(screen, "+2 -0") || readme.Added != 2 {
		t.Errorf("README.md is +%d -%d; the screen should show +2 -0", readme.Added, readme.Removed)
	}
}

// The point of the view: one file's diff at a time, not the whole commit's.
func TestDiffBoxShowsOnlyTheSelectedFile(t *testing.T) {
	m := openedView(t, threeFileRepo(t))
	m.commitView.focus = commitBoxFiles

	first := diffBoxOf(t, m)
	if !strings.Contains(first, "now with words") {
		t.Errorf("the first file's diff is not in the diff box:\n%s", first)
	}
	if strings.Contains(first, "return 42") {
		t.Errorf("the diff box holds another file's changes too:\n%s", first)
	}

	// Walk to parser.go and the box follows.
	m = press(m, keyPress("j"), keyPress("j"))
	if got := m.viewedFiles()[m.commitView.file].Path; got != "parser.go" {
		t.Fatalf("selected %q after two js, want parser.go", got)
	}
	second := diffBoxOf(t, m)
	if !strings.Contains(second, "return 42") {
		t.Errorf("the diff box did not follow the file list:\n%s", second)
	}
	if strings.Contains(second, "now with words") {
		t.Errorf("the diff box still holds the file before it:\n%s", second)
	}
}

// diffBoxOf is the right-hand column of the screen, where the diff is drawn.
func diffBoxOf(t *testing.T, m model) string {
	t.Helper()
	l := m.currentCommitViewLayout()
	var sb strings.Builder
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		runes := []rune(line)
		if len(runes) > l.leftWidth {
			sb.WriteString(string(runes[l.leftWidth:]))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// A line count from one file means nothing in the next, so the diff starts at
// the top of whichever file is chosen.
func TestChangingFileRewindsTheDiff(t *testing.T) {
	m := openedView(t, longDiffRepo(t))
	m.commitView.focus = commitBoxDiff
	m = press(m, keyPress("j"), keyPress("j"))
	if m.commitView.diffScroll == 0 {
		t.Fatal("the diff did not scroll")
	}

	m.commitView.focus = commitBoxFiles
	m = press(m, keyPress("j"))
	if m.commitView.diffScroll != 0 {
		t.Errorf("diffScroll = %d after choosing another file, want the top", m.commitView.diffScroll)
	}
}

func TestFocusMovesBetweenTheThreeBoxes(t *testing.T) {
	m := openedView(t, threeFileRepo(t))
	m.commitView.focus = commitBoxDetails

	for _, want := range []int{commitBoxFiles, commitBoxDiff, commitBoxDetails} {
		m = press(m, tea.KeyMsg{Type: tea.KeyTab})
		if m.commitView.focus != want {
			t.Fatalf("tab went to box %d, want %d", m.commitView.focus, want)
		}
	}
	for _, want := range []int{commitBoxDiff, commitBoxFiles, commitBoxDetails} {
		m = press(m, tea.KeyMsg{Type: tea.KeyShiftTab})
		if m.commitView.focus != want {
			t.Fatalf("shift+tab went to box %d, want %d", m.commitView.focus, want)
		}
	}
	for key, want := range map[string]int{"1": commitBoxDetails, "2": commitBoxFiles, "3": commitBoxDiff} {
		if got := press(m, keyPress(key)).commitView.focus; got != want {
			t.Errorf("%s focused box %d, want %d", key, got, want)
		}
	}
}

// The graph is not on screen, so a key meant for it would move something the
// user cannot see.
func TestKeysDoNotLeakThroughToTheGraph(t *testing.T) {
	m := openedView(t, threeFileRepo(t))
	m.selected = 1
	m.commitView.commit = 1
	before := m

	for _, key := range []string{"j", "k", "G", "g", "c", "2"} {
		after := press(before, keyPress(key))
		if after.selected != before.selected {
			t.Errorf("%s moved the graph's selection %d→%d", key, before.selected, after.selected)
		}
		if after.colourLanes != before.colourLanes {
			t.Errorf("%s toggled lane colours behind the view", key)
		}
	}
}

func TestScrollingTheDiff(t *testing.T) {
	m := openedView(t, longDiffRepo(t))
	m.commitView.focus = commitBoxDiff

	if got := press(m, keyPress("j")).commitView.diffScroll; got != 1 {
		t.Errorf("diffScroll = %d after j, want 1", got)
	}
	if got := press(m, keyPress("k")).commitView.diffScroll; got != 0 {
		t.Errorf("diffScroll = %d at the top, want it to stay there", got)
	}
	down := press(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if down.commitView.diffScroll != 10 {
		t.Errorf("diffScroll = %d after d, want a page", down.commitView.diffScroll)
	}
	if got := press(down, keyPress("g")).commitView.diffScroll; got != 0 {
		t.Errorf("diffScroll = %d after g, want the top", got)
	}
}

// Scrolling stops where the content does, and the offset stops with it: an
// offset past the end would leave the box looking stuck on the way back up.
func TestScrollingStopsAtTheEndOfTheDiff(t *testing.T) {
	ctrlD := tea.KeyMsg{Type: tea.KeyCtrlD}
	m := openedView(t, longDiffRepo(t))
	m.commitView.focus = commitBoxDiff
	most := m.maxScroll(commitBoxDiff)
	if most <= 10 {
		t.Fatalf("maxScroll = %d, want a diff worth scrolling", most)
	}

	end := press(m, keyPress("G"))
	if end.commitView.diffScroll != most {
		t.Errorf("diffScroll = %d after G, want the last screenful at %d", end.commitView.diffScroll, most)
	}
	// The last line is on screen, and nothing past it.
	screen := diffBoxOf(t, end)
	if !strings.Contains(screen, "line 199") {
		t.Errorf("the end of the diff is not on screen:\n%s", screen)
	}
	past := press(end, ctrlD, ctrlD, ctrlD)
	if past.commitView.diffScroll != most {
		t.Errorf("diffScroll = %d after paging past the end, want %d", past.commitView.diffScroll, most)
	}
	if got := press(past, keyPress("k")).commitView.diffScroll; got != most-1 {
		t.Errorf("diffScroll = %d after one k, want %d — one press should move it", got, most-1)
	}
}

// The details box scrolls only when its content is taller than it is, which on
// a short window it is.
func TestScrollingTheDetails(t *testing.T) {
	m := openedView(t, threeFileRepo(t))
	m.windowWidth, m.windowHeight = 80, 14
	m.commitView.focus = commitBoxDetails
	if m.maxScroll(commitBoxDetails) == 0 {
		t.Fatal("the details box has nothing to scroll in a 14-row window")
	}

	if got := press(m, keyPress("j")).commitView.detailsScroll; got != 1 {
		t.Errorf("detailsScroll = %d after j, want 1", got)
	}
	most := m.maxScroll(commitBoxDetails)
	if got := press(m, keyPress("G")).commitView.detailsScroll; got != most {
		t.Errorf("detailsScroll = %d after G, want %d", got, most)
	}
}

func TestFileSelectionStopsAtTheEnds(t *testing.T) {
	m := openedView(t, threeFileRepo(t))
	m.commitView.focus = commitBoxFiles

	if got := press(m, keyPress("k")).commitView.file; got != 0 {
		t.Errorf("file = %d at the top of the list, want 0", got)
	}
	end := press(m, keyPress("G"))
	if got := end.commitView.file; got != 2 {
		t.Errorf("file = %d after G, want the last of three", got)
	}
	if got := press(end, keyPress("j")).commitView.file; got != 2 {
		t.Errorf("file = %d past the last file, want it to stay", got)
	}
}

// Uncommitted changes are a commit as far as this screen is concerned, and an
// untracked file is listed even though there is no diff to read.
func TestViewOnUncommittedChanges(t *testing.T) {
	dir, _, _ := dirtyRepo(t)
	write(t, dir, "first", "changed")
	write(t, dir, "brand-new.txt", "hello")

	m := openedView(t, dir)
	if c, _ := m.viewedCommit(); !c.WorkingTree {
		t.Fatal("the view did not open on the working tree row")
	}
	screen := ansi.Strip(m.View())
	if !strings.Contains(screen, "uncommitted changes") {
		t.Errorf("the header does not say what this is:\n%s", screen)
	}

	var untracked bool
	for i, f := range m.viewedFiles() {
		if f.Path == "brand-new.txt" {
			untracked = f.Untracked
			m.commitView.file = i
		}
	}
	if !untracked {
		t.Fatalf("brand-new.txt is not listed as untracked; files = %v", paths(m.viewedFiles()))
	}
	if !strings.Contains(screen, "untracked") {
		t.Errorf("the list does not mark the untracked file:\n%s", screen)
	}
	if body := diffBoxOf(t, m); !strings.Contains(body, "not in git yet") {
		t.Errorf("the diff box does not say why there is nothing to read:\n%s", body)
	}
}

// Opening before the diff has arrived says so rather than looking empty.
func TestViewBeforeTheDiffArrives(t *testing.T) {
	m := loadedModel(t, threeFileRepo(t))
	m.windowWidth, m.windowHeight = 110, 26
	m, _ = m.openCommitView()

	screen := ansi.Strip(m.View())
	if !strings.Contains(screen, "Loading") {
		t.Errorf("the view does not say it is still loading:\n%s", screen)
	}
}

// A reload renumbers the commits, so the view has to be told where its commit
// went or it would start describing the one next to it.
func TestViewFollowsAReload(t *testing.T) {
	dir, git, commit := gitFixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	commit("second")

	m := openedView(t, dir)
	m.selected = 1
	m.commitView.commit = 1
	was := m.commits[1].FullHash

	commit("third") // everything shifts down by one
	m = press(m, keyPress("r"))
	m, _ = m.reloadRepo()
	res, _ := m.Update(loadRepo(dir)())
	m = res.(model)

	if !m.commitView.open {
		t.Fatal("the reload closed the commit view")
	}
	if got := m.commits[m.commitView.commit].FullHash; got != was {
		t.Errorf("the view is on %s, want the commit it was opened on (%s)", got[:7], was[:7])
	}
}

func TestHelpOpensOverTheView(t *testing.T) {
	m := press(openedView(t, threeFileRepo(t)), keyPress("?"))
	if !m.showHelp {
		t.Fatal("? did not open the help over the commit view")
	}
	if !strings.Contains(ansi.Strip(m.View()), "Keyboard shortcuts") {
		t.Error("the help box is not drawn over the commit view")
	}
	if closed := press(m, keyPress("?")); closed.showHelp || !closed.commitView.open {
		t.Errorf("closing the help left showHelp=%v view=%v, want the view back",
			closed.showHelp, closed.commitView.open)
	}
}

// The boxes have to fill the window exactly, as the graph screen's panels do:
// a row too many would scroll the terminal, one too few would leave a gap.
func TestViewFillsTheWindowAtEverySize(t *testing.T) {
	base := openedView(t, threeFileRepo(t))
	for _, size := range [][2]int{{110, 26}, {80, 24}, {70, 12}, {60, 10}, {200, 50}} {
		m := base
		m.windowWidth, m.windowHeight = size[0], size[1]
		lines := strings.Split(ansi.Strip(m.View()), "\n")

		if len(lines) != m.windowHeight {
			t.Errorf("%dx%d: %d lines, want %d", size[0], size[1], len(lines), m.windowHeight)
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > m.windowWidth {
				t.Errorf("%dx%d: line %d is %d wide, want at most %d", size[0], size[1], i, w, m.windowWidth)
			}
		}
		// The way out has to stay on screen however little room there is.
		if last := lines[len(lines)-1]; !strings.Contains(last, "esc: back") {
			t.Errorf("%dx%d: last line is %q, want the status line", size[0], size[1], last)
		}
	}
}

func TestClickSelectsAFileAndTheWheelScrolls(t *testing.T) {
	m := openedView(t, threeFileRepo(t))
	l := m.currentCommitViewLayout()
	// The files box starts under the details box; its first row is one below
	// its own top border.
	firstFileRow := commitViewTopRow + l.detailsRows + 1

	res, _ := m.Update(click(2, firstFileRow+2))
	if got := res.(model).commitView.file; got != 2 {
		t.Errorf("clicking the third row selected file %d, want 2", got)
	}
	// Past the end of a three-file list nothing is pointed at.
	res, _ = m.Update(click(2, firstFileRow+5))
	if got := res.(model).commitView.file; got != m.commitView.file {
		t.Errorf("clicking an empty row selected %d, want the selection left alone", got)
	}

	// The wheel drives whatever the pointer is over, as it does on the graph.
	res, _ = m.Update(wheel(l.leftWidth+4, 8, tea.MouseButtonWheelDown))
	if got := res.(model).commitView.diffScroll; got != mouseScrollLines {
		t.Errorf("diffScroll = %d after a notch over the diff, want %d", got, mouseScrollLines)
	}
	res, _ = m.Update(wheel(2, firstFileRow, tea.MouseButtonWheelDown))
	if got := res.(model).commitView.file; got != 2 {
		t.Errorf("file = %d after a notch over the list, want the last of three", got)
	}
}

// manyFilesRepo has more files in one commit than the list can show at once.
func manyFilesRepo(t *testing.T) string {
	t.Helper()
	dir, git, _ := gitFixture(t)
	git("init", "-q", "-b", "main")
	write(t, dir, "start.txt", "one\n")
	git("add", "-A")
	git("commit", "-qm", "first")
	for i := range 30 {
		write(t, dir, fmt.Sprintf("file%02d.txt", i), fmt.Sprintf("contents %d\n", i))
	}
	git("add", "-A")
	git("commit", "-qm", "thirty files at once")
	return dir
}

// fileOnRow is the file whose path is printed on screen row y, read off the
// drawing rather than worked out, so this fails if the list's geometry ever
// drifts from the renderer's.
func fileOnRow(t *testing.T, m model, y int) int {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if y >= len(lines) {
		t.Fatalf("row %d is past the %d-line screen", y, len(lines))
	}
	for i, f := range m.viewedFiles() {
		if strings.Contains(lines[y], f.Path) {
			return i
		}
	}
	return -1
}

// A list long enough to scroll is where a click and the drawing can disagree:
// the row is only the nth file when the list happens to start at its top.
func TestClickOnAScrolledFileList(t *testing.T) {
	m := openedView(t, manyFilesRepo(t))
	m.windowWidth, m.windowHeight = 110, 20
	if len(m.viewedFiles()) < 30 {
		t.Fatalf("files = %d, want the whole commit listed", len(m.viewedFiles()))
	}

	// Walk to the end so the list has scrolled well past its first screenful.
	m.commitView.focus = commitBoxFiles
	m = press(m, keyPress("G"))
	l := m.currentCommitViewLayout()
	if m.fileTop(l.fileRows-2) == 0 {
		t.Fatal("the file list did not scroll")
	}

	firstRow := commitViewTopRow + l.detailsRows + 1
	for _, row := range []int{firstRow, firstRow + 2, firstRow + l.fileRows - 3} {
		want := fileOnRow(t, m, row)
		if want < 0 {
			t.Fatalf("row %d holds no file", row)
		}
		res, _ := m.Update(click(2, row))
		if got := res.(model).commitView.file; got != want {
			t.Errorf("clicking row %d selected file %d, want %d — the file drawn there", row, got, want)
		}
	}
}

// The selection stays on screen however it is moved, which is what makes the
// list readable without a scrollbar.
func TestTheSelectedFileIsAlwaysOnScreen(t *testing.T) {
	m := openedView(t, manyFilesRepo(t))
	m.windowWidth, m.windowHeight = 110, 20
	m.commitView.focus = commitBoxFiles
	l := m.currentCommitViewLayout()
	rows := l.fileRows - 2

	for _, keys := range [][]string{{"G"}, {"g"}, {"d"}, {"d", "d"}, {"G", "u"}} {
		moved := m
		for _, k := range keys {
			moved = press(moved, keyPress(k))
		}
		top := moved.fileTop(rows)
		if moved.commitView.file < top || moved.commitView.file >= top+rows {
			t.Errorf("after %v the selection is file %d but the list shows %d..%d",
				keys, moved.commitView.file, top, top+rows-1)
		}
	}
}
