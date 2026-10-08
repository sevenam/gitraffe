package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWatchFocusSendsOnReturnOnly(t *testing.T) {
	type answer struct{ in, known bool }
	answers := []answer{
		{true, true},   // already in front at start: not news
		{false, true},  // left
		{false, false}, // could not tell
		{true, true},   // back: one FocusMsg
		{true, true},   // still in front
		{true, false},  // could not tell
		{true, true},   // still in front, not a return
		{false, true},
		{true, true}, // back again: a second
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	var asked, sent int
	focused := func() (bool, bool) {
		if asked == len(answers) { // a tick that raced the stop
			return false, false
		}
		a := answers[asked]
		asked++
		if asked == len(answers) {
			close(stop)
		}
		return a.in, a.known
	}
	send := func(msg tea.Msg) {
		if _, ok := msg.(tea.FocusMsg); !ok {
			t.Errorf("sent %#v, want a tea.FocusMsg", msg)
		}
		sent++
	}
	go func() {
		watchFocus(focused, time.Millisecond, send, stop)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watchFocus did not stop")
	}
	if sent != 2 {
		t.Errorf("sent %d focus messages, want 2", sent)
	}
}
