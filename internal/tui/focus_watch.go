package tui

import (
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Bubble Tea learns of focus from the terminal (tea.WithReportFocus), which
// works wherever the terminal speaks the xterm focus sequences. On Windows it
// reads the console's input records instead and drops their focus events, so
// coming back to the window there never reaches onFocus. There, a watcher
// asks the system which window is in front and sends the tea.FocusMsg itself;
// focusProbe (one per platform) says how, or is nil where nothing need be
// watched.

const focusWatchEvery = 500 * time.Millisecond

// watchFocus sends a tea.FocusMsg each time focused turns from false to true,
// until stop is closed. The state it starts in is not news: a window already
// in front was not just returned to. An answer of known=false (the window
// could not be identified this time) is skipped, so it neither counts as
// leaving nor, once identified again, as coming back.
func watchFocus(focused func() (in, known bool), every time.Duration, send func(tea.Msg), stop <-chan struct{}) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	was, started := false, false
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		in, known := focused()
		if !known {
			continue
		}
		if started && in && !was {
			log.Print("Focus: back in front")
			send(tea.FocusMsg{})
		}
		was, started = in, true
	}
}
