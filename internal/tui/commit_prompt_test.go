package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// counts reads the numbers the message box draws beside "Subject" and "Body".
func counts(t *testing.T, m model) (subject, body string) {
	t.Helper()
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if i+1 >= len(fields) {
				continue
			}
			switch f {
			case "Subject":
				subject = fields[i+1]
			case "Body":
				body = fields[i+1]
			}
		}
	}
	if subject == "" || body == "" {
		t.Fatalf("the box does not show both counts:\n%s", ansi.Strip(m.View()))
	}
	return subject, body
}

// The 50/72 rule, counted down: what is shown is the room left, and past the
// limit it goes below zero rather than stopping anyone.
func TestCommitBoxCountsDownToFiftyAndSeventyTwo(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	m := press(do(t, openedView(t, dir), keyPress("S")), keyPress("c"))

	if s, b := counts(t, m); s != "50" || b != "72" {
		t.Fatalf("an empty box counts %s and %s, want 50 and 72", s, b)
	}

	m = typeText(m, strings.Repeat("x", 21))
	if s, _ := counts(t, m); s != "29" {
		t.Errorf("subject count = %s after 21 characters, want 29", s)
	}
	m = typeText(m, strings.Repeat("x", 29))
	if s, _ := counts(t, m); s != "0" {
		t.Errorf("subject count = %s at fifty characters, want 0", s)
	}
	m = typeText(m, "xxxx")
	if s, _ := counts(t, m); s != "-4" {
		t.Errorf("subject count = %s at fifty-four characters, want -4", s)
	}
	// A convention, not a limit: the characters past it are still there.
	if got := len(m.commitPrompt.subject.Value()); got != 54 {
		t.Errorf("the subject holds %d characters, want all 54", got)
	}

	// The body counts the line being typed, each from 72 again.
	m = typeText(press(m, tab), strings.Repeat("y", 80))
	if _, b := counts(t, m); b != "-8" {
		t.Errorf("body count = %s on an 80-character line, want -8", b)
	}
	m = typeText(press(m, enter), "short")
	if _, b := counts(t, m); b != "67" {
		t.Errorf("body count = %s on a new 5-character line, want 67", b)
	}
	// Back in the subject, the body's count is of its longest line, so the
	// one that ran over is not forgotten.
	m = press(m, tab)
	if _, b := counts(t, m); b != "-8" {
		t.Errorf("body count = %s with the cursor in the subject, want the longest line's -8", b)
	}
	if got := m.commitPrompt.message(); got != strings.Repeat("x", 54)+"\n\n"+strings.Repeat("y", 80)+"\nshort" {
		t.Errorf("message = %q, want everything typed", got)
	}
}

// rowsWith counts the screen rows holding a run of this character at least
// ten long: how many rows a long line of it takes up.
func rowsWith(m model, char string) int {
	n := 0
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, strings.Repeat(char, 10)) {
			n++
		}
	}
	return n
}

// The body is drawn 72 columns wide, so a line that wraps on screen is a line
// that is too long: the count going below zero and the wrap happen together.
func TestCommitBoxWrapsTheBodyAtSeventyTwo(t *testing.T) {
	dir, _ := stagingRepo(t)
	twoHunks(t, dir)
	m := press(do(t, openedView(t, dir), keyPress("S")), keyPress("c"))

	m = typeText(press(m, tab), strings.Repeat("y", 72))
	if _, b := counts(t, m); b != "0" || rowsWith(m, "y") != 1 {
		t.Fatalf("72 characters: count %s on %d rows, want 0 on one row", b, rowsWith(m, "y"))
	}
	m = typeText(m, strings.Repeat("y", 12))
	if _, b := counts(t, m); b != "-12" || rowsWith(m, "y") != 2 {
		t.Errorf("84 characters: count %s on %d rows, want -12 and the line wrapped", b, rowsWith(m, "y"))
	}
	// Wrapped to be read, not broken: it is still one line of the message.
	if got := m.commitPrompt.message(); strings.Contains(got, "y\ny") {
		t.Errorf("the wrap put a line break in the message: %q", got)
	}

	// A subject is one line however long, in however narrow a window: it
	// scrolls, where wrapping would push the box out of shape.
	m = press(m, tab)
	m.windowWidth = 50
	m = typeText(m, strings.Repeat("x", 60))
	if rows := rowsWith(m, "x"); rows != 1 {
		t.Errorf("a 60-character subject takes %d rows in a 50-column window, want 1", rows)
	}
	if s, _ := counts(t, m); s != "-10" {
		t.Errorf("subject count = %s, want -10", s)
	}
}
