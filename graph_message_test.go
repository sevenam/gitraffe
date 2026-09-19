package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// messageRepo has one commit with a subject far longer than any window, to
// show what happens when the column runs out.
func messageRepo(t *testing.T) string {
	t.Helper()
	dir, git, commit := gitFixture(t)
	git("init", "-q", "-b", "main")
	commit("teach the parser about nested groups")
	// Committed by hand: commit() names the file after the message, and this
	// one is far too long for a file name.
	write(t, dir, "long.txt", "x")
	git("add", "-A")
	git("commit", "-qm", strings.Repeat("a very long subject line ", 10))
	return dir
}

func graphPanel(m model) string {
	var out []string
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, "commit-details") {
			break
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func TestMaximisedGraphShowsMessages(t *testing.T) {
	m := loadedModel(t, messageRepo(t))
	m.windowWidth, m.windowHeight = 140, 20

	if strings.Contains(graphPanel(m), "teach the parser") {
		t.Error("the split view shows messages, though the details panel already does")
	}

	m = press(m, enter)
	if !strings.Contains(graphPanel(m), "teach the parser about nested groups") {
		t.Errorf("the maximised graph does not show the message:\n%s", graphPanel(m))
	}
}

// Every column before the message has a width, so the messages line up
// however long the authors' names are.
func TestMessagesLineUp(t *testing.T) {
	dir := searchRepo(t) // two authors, "ada" and "grace"
	m := loadedModel(t, dir)
	m.windowWidth, m.windowHeight = 140, 20
	m = press(m, enter)

	var columns []int
	for _, line := range strings.Split(graphPanel(m), "\n") {
		for _, subject := range []string{"fix the lexer", "tidy up", "add the parser"} {
			if i := strings.Index(line, subject); i >= 0 {
				columns = append(columns, i)
			}
		}
	}
	if len(columns) < 3 {
		t.Fatalf("found %d messages, want at least 3", len(columns))
	}
	for _, c := range columns[1:] {
		if c != columns[0] {
			t.Errorf("messages start at columns %v, want one column", columns)
			break
		}
	}
}

func TestLongMessagesAreCutToFit(t *testing.T) {
	m := loadedModel(t, messageRepo(t))
	m.windowWidth, m.windowHeight = 140, 20
	m = press(m, enter)

	for i, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w > m.windowWidth {
			t.Fatalf("line %d is %d wide, want at most %d", i, w, m.windowWidth)
		}
	}
	if !strings.Contains(graphPanel(m), "…") {
		t.Error("a subject longer than the window was not cut with an ellipsis")
	}
}

// A narrow window spends what it has on the graph itself.
func TestNoMessageColumnWhenThereIsNoRoom(t *testing.T) {
	wide := computeMaximisedLayout(160, 6, 20, len(dateColumnFormat), 24)
	narrow := computeMaximisedLayout(60, 6, 20, len(dateColumnFormat), 24)

	if wide.messageCol < minMessageColWidth {
		t.Errorf("a 160-column window gave the message %d, want at least %d", wide.messageCol, minMessageColWidth)
	}
	if narrow.messageCol != 0 {
		t.Errorf("a 60-column window gave the message %d, want none", narrow.messageCol)
	}
	// The split layout never has one: the details panel shows the message.
	if split := computePanelLayout(160, 6, 20, len(dateColumnFormat), 24); split.messageCol != 0 {
		t.Errorf("the split layout gave the message %d, want none", split.messageCol)
	}
}

// The working tree row already says what it is where the message would go.
func TestWorkingTreeRowHasNoMessageColumn(t *testing.T) {
	dir, _, _ := dirtyRepo(t)
	write(t, dir, "first", "changed")
	m := loadedModel(t, dir)
	m.windowWidth, m.windowHeight = 140, 20
	m = press(m, enter)

	for _, line := range strings.Split(graphPanel(m), "\n") {
		if strings.Contains(line, "1 changed") && strings.Count(line, "1 changed") != 1 {
			t.Errorf("the working tree row says it twice: %q", line)
		}
	}
}
