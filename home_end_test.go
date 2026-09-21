package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	home     = tea.KeyMsg{Type: tea.KeyHome}
	end      = tea.KeyMsg{Type: tea.KeyEnd}
	ctrlHome = tea.KeyMsg{Type: tea.KeyCtrlHome}
	ctrlEnd  = tea.KeyMsg{Type: tea.KeyCtrlEnd}
)

func TestFollowWindow(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		top, selected, count, rows int
		want                       int
	}{
		{"inside stays put", 10, 15, 100, 10, 10},
		{"top row stays put", 10, 10, 100, 10, 10},
		{"bottom row stays put", 10, 19, 100, 10, 10},
		{"one past the bottom scrolls one", 10, 20, 100, 10, 11},
		{"a page past the bottom scrolls a page", 10, 29, 100, 10, 20},
		{"one above the top scrolls one", 10, 9, 100, 10, 9},
		{"a far jump lands a third down", 0, 60, 100, 9, 57},
		{"a far jump up lands a third down", 80, 30, 100, 9, 27},
		{"never past the end", 0, 99, 100, 10, 90},
		{"a short list never scrolls", 0, 4, 5, 10, 0},
		{"no rows", 5, 5, 100, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := followWindow(tc.top, tc.selected, tc.count, tc.rows); got != tc.want {
				t.Errorf("followWindow(%d, %d, %d, %d) = %d, want %d",
					tc.top, tc.selected, tc.count, tc.rows, got, tc.want)
			}
		})
	}
}

// graphModel is a 200-commit history with the graph focused. With no graph
// rows it draws one commit per row, which keeps the arithmetic readable.
func graphModel() model {
	m := positionModel(200)
	m.focusedBox = 1
	return m
}

func TestHomeAndEndStayOnTheScreen(t *testing.T) {
	m := graphModel()
	rows := m.visibleGraphRows()

	bottom := press(m, end)
	if bottom.selected != rows-1 {
		t.Errorf("end selected %d, want the bottom of the screen at %d", bottom.selected, rows-1)
	}
	if start, _ := bottom.graphWindow(); start != 0 {
		t.Errorf("end scrolled the graph to row %d; it should not move", start)
	}
	if again := press(bottom, end); again.selected != bottom.selected {
		t.Errorf("a second end moved to %d; the bottom of the screen is still %d", again.selected, bottom.selected)
	}

	// Walk down far enough to scroll, and home should go to the top of what is
	// now on screen, not the newest commit.
	scrolled := press(m, keyPress("G"), keyPress("k"), keyPress("k"))
	start, _ := scrolled.graphWindow()
	if start == 0 {
		t.Fatal("the graph did not scroll; the test needs it to")
	}
	top := press(scrolled, home)
	if top.selected != start {
		t.Errorf("home selected %d, want the top of the screen at %d", top.selected, start)
	}
	if now, _ := top.graphWindow(); now != start {
		t.Errorf("home scrolled the graph from row %d to %d; it should not move", start, now)
	}
}

func TestCtrlHomeAndCtrlEndGoToTheEnds(t *testing.T) {
	m := press(graphModel(), keyPress("j"), keyPress("j"))
	if got := press(m, ctrlEnd).selected; got != 199 {
		t.Errorf("ctrl+end selected %d, want the oldest commit at 199", got)
	}
	if got := press(m, ctrlEnd, ctrlHome).selected; got != 0 {
		t.Errorf("ctrl+home selected %d, want the newest commit at 0", got)
	}
	// g and G are vim's, and keep meaning the ends.
	if got := press(m, keyPress("G")).selected; got != 199 {
		t.Errorf("G selected %d, want 199", got)
	}
	if got := press(m, keyPress("G"), keyPress("g")).selected; got != 0 {
		t.Errorf("g selected %d, want 0", got)
	}
}

// Walking down the screen leaves the graph where it is; only stepping past
// the bottom scrolls it, and then by one row.
func TestGraphScrollsOnlyAtTheEdge(t *testing.T) {
	m := graphModel()
	rows := m.visibleGraphRows()
	for i := 0; i < rows-1; i++ {
		m = press(m, keyPress("j"))
	}
	if start, _ := m.graphWindow(); start != 0 {
		t.Fatalf("graph scrolled to %d with the selection still on screen", start)
	}
	m = press(m, keyPress("j"))
	if start, _ := m.graphWindow(); start != 1 {
		t.Errorf("one step past the bottom scrolled to %d, want 1", start)
	}
}

// Lines that only join lanes, and the note that the history was cut short,
// hold no commit to select.
func TestHomeAndEndSkipRowsWithoutACommit(t *testing.T) {
	m := graphModel()
	m.commits = m.commits[:3]
	m.displayRows = []displayRow{
		{CommitIdx: -1}, {CommitIdx: 0}, {CommitIdx: -1},
		{CommitIdx: 1}, {CommitIdx: 2}, {CommitIdx: -1, Note: "… more history — press m"},
	}
	m.selected = 1
	if got := press(m, home).selected; got != 0 {
		t.Errorf("home selected %d, want commit 0", got)
	}
	if got := press(m, end).selected; got != 2 {
		t.Errorf("end selected %d, want commit 2", got)
	}
}

func TestHomeAndEndInTheFileList(t *testing.T) {
	m := openedView(t, manyFilesRepo(t))
	m.commitView.focus = commitBoxFiles
	rows := m.fileRows()
	if rows >= 30 {
		t.Fatalf("the list shows %d rows; the test needs it to scroll", rows)
	}

	if got := press(m, end).commitView.file; got != rows-1 {
		t.Errorf("end selected file %d, want the bottom of the list at %d", got, rows-1)
	}
	scrolled := press(m, keyPress("G"), keyPress("k"))
	top := scrolled.fileTop(rows)
	if top == 0 {
		t.Fatal("the file list did not scroll; the test needs it to")
	}
	atTop := press(scrolled, home)
	if atTop.commitView.file != top {
		t.Errorf("home selected file %d, want the top of the list at %d", atTop.commitView.file, top)
	}
	if now := atTop.fileTop(rows); now != top {
		t.Errorf("home scrolled the list from %d to %d; it should not move", top, now)
	}
	if got := press(scrolled, ctrlHome).commitView.file; got != 0 {
		t.Errorf("ctrl+home selected file %d, want 0", got)
	}
	if got := press(m, ctrlEnd).commitView.file; got != 29 {
		t.Errorf("ctrl+end selected file %d, want 29", got)
	}
}

// Text has no cursor to keep on screen, so home and end go to its ends.
func TestHomeAndEndInTheDiff(t *testing.T) {
	m := openedView(t, longDiffRepo(t))
	m.commitView.focus = commitBoxDiff
	most := m.maxScroll(commitBoxDiff)
	for _, key := range []tea.KeyMsg{end, ctrlEnd} {
		if got := press(m, key).commitView.diffScroll; got != most {
			t.Errorf("%s scrolled to %d, want the end at %d", key, got, most)
		}
	}
	for _, key := range []tea.KeyMsg{home, ctrlHome} {
		if got := press(m, keyPress("G"), key).commitView.diffScroll; got != 0 {
			t.Errorf("%s scrolled to %d, want the start", key, got)
		}
	}
}
