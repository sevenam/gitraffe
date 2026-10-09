//go:build windows

package tui

import (
	"log"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/erikgeiser/coninput"
	"golang.org/x/sys/windows"
)

// On Windows, Bubble Tea reads the console's input records and drops the
// focus events among them, so a terminal reporting that gitraffe's pane got
// the focus back, as Windows Terminal does when moving between split panes,
// never reaches onFocus. The window watcher in focus_windows.go cannot see
// that either, since the window in front stays the same. So here gitraffe
// reads the console itself: Bubble Tea's own reader, copied below, with the
// focus events passed on.
//
// The key and mouse translation is Bubble Tea v1.3.10's key_windows.go
// (MIT licence, Copyright (c) 2020-2025 Charmbracelet, Inc), unchanged but for
// the package; keep it in step when Bubble Tea is upgraded. Bubble Tea is
// given no input of its own (tea.WithInput(nil)), which also means nothing
// may turn the mouse on or off while running: on Windows that restarts Bubble
// Tea's reader, and it would start reading nothing.

type consoleInput struct {
	conin    windows.Handle
	original uint32
	stop     chan struct{}
	done     chan struct{}
}

// openConsoleInput takes over the console's input, or returns nil when
// standard input is not a console (mintty, a pipe), where Bubble Tea's own
// reader, which parses the terminal's focus sequences, is left to it.
func openConsoleInput() *consoleInput {
	conin, err := coninput.NewStdinHandle()
	if err != nil {
		return nil
	}
	var original uint32
	if windows.GetConsoleMode(conin, &original) != nil {
		return nil
	}
	// The modes Bubble Tea's reader sets, mouse included: no line editing,
	// no echo, Ctrl+C as a key; resizes and focus as records.
	mode := coninput.AddInputModes(0,
		windows.ENABLE_WINDOW_INPUT, windows.ENABLE_EXTENDED_FLAGS, windows.ENABLE_MOUSE_INPUT)
	if err := windows.SetConsoleMode(conin, mode); err != nil {
		log.Printf("Console input: could not set its mode: %v", err)
		return nil
	}
	return &consoleInput{conin: conin, original: original, stop: make(chan struct{}), done: make(chan struct{})}
}

// close stops the reading and gives the console back as it was found.
func (c *consoleInput) close() {
	close(c.stop)
	<-c.done
	_ = windows.SetConsoleMode(c.conin, c.original)
}

func (c *consoleInput) run(send func(tea.Msg)) {
	defer close(c.done)
	var ps coninput.ButtonState                 // keep track of previous mouse state
	var ws coninput.WindowBufferSizeEventRecord // keep track of the last window size event
	for {
		events, ok := c.next()
		if !ok {
			return
		}
		for _, event := range events {
			switch e := event.Unwrap().(type) {
			case coninput.KeyEventRecord:
				if !e.KeyDown || e.VirtualKeyCode == coninput.VK_SHIFT {
					continue
				}
				for i := 0; i < int(e.RepeatCount); i++ {
					eventKeyType := keyType(e)
					var runes []rune
					// Add the character only if the key type is an actual character and not a control sequence.
					if eventKeyType == tea.KeyRunes {
						runes = []rune{e.Char}
					}
					send(tea.KeyMsg{
						Type:  eventKeyType,
						Runes: runes,
						Alt:   e.ControlKeyState.Contains(coninput.LEFT_ALT_PRESSED | coninput.RIGHT_ALT_PRESSED),
					})
				}
			case coninput.WindowBufferSizeEventRecord:
				if e != ws {
					ws = e
					send(tea.WindowSizeMsg{Width: int(e.Size.X), Height: int(e.Size.Y)})
				}
			case coninput.MouseEventRecord:
				event := mouseEvent(ps, e)
				if event.Type != tea.MouseUnknown {
					send(event)
				}
				ps = e.ButtonState
			case coninput.FocusEventRecord:
				// The one change from Bubble Tea's reader.
				if e.SetFocus {
					log.Print("Focus: the console says it has the focus")
					send(tea.FocusMsg{})
				} else {
					send(tea.BlurMsg{})
				}
			}
		}
	}
}

// next waits for input records and reads them, or reports false once close
// has been called. It peeks before reading, as Bubble Tea does, so that a
// stop is noticed without a read blocking on a console nobody is typing in.
func (c *consoleInput) next() ([]coninput.InputRecord, bool) {
	for {
		select {
		case <-c.stop:
			return nil, false
		default:
		}
		events, err := coninput.PeekNConsoleInputs(c.conin, 16)
		if err != nil {
			log.Printf("Console input: %v", err)
			return nil, false
		}
		if len(events) == 0 {
			time.Sleep(16 * time.Millisecond)
			continue
		}
		events, err = coninput.ReadNConsoleInputs(c.conin, uint32(len(events)))
		if err != nil {
			log.Printf("Console input: %v", err)
			return nil, false
		}
		return events, true
	}
}

// Below: Bubble Tea v1.3.10's key_windows.go from mouseEventButton on.

func mouseEventButton(p, s coninput.ButtonState) (button tea.MouseButton, action tea.MouseAction) {
	btn := p ^ s
	action = tea.MouseActionPress
	if btn&s == 0 {
		action = tea.MouseActionRelease
	}

	if btn == 0 {
		switch {
		case s&coninput.FROM_LEFT_1ST_BUTTON_PRESSED > 0:
			button = tea.MouseButtonLeft
		case s&coninput.FROM_LEFT_2ND_BUTTON_PRESSED > 0:
			button = tea.MouseButtonMiddle
		case s&coninput.RIGHTMOST_BUTTON_PRESSED > 0:
			button = tea.MouseButtonRight
		case s&coninput.FROM_LEFT_3RD_BUTTON_PRESSED > 0:
			button = tea.MouseButtonBackward
		case s&coninput.FROM_LEFT_4TH_BUTTON_PRESSED > 0:
			button = tea.MouseButtonForward
		}
		return button, action
	}

	switch btn {
	case coninput.FROM_LEFT_1ST_BUTTON_PRESSED: // left button
		button = tea.MouseButtonLeft
	case coninput.RIGHTMOST_BUTTON_PRESSED: // right button
		button = tea.MouseButtonRight
	case coninput.FROM_LEFT_2ND_BUTTON_PRESSED: // middle button
		button = tea.MouseButtonMiddle
	case coninput.FROM_LEFT_3RD_BUTTON_PRESSED: // unknown (possibly mouse backward)
		button = tea.MouseButtonBackward
	case coninput.FROM_LEFT_4TH_BUTTON_PRESSED: // unknown (possibly mouse forward)
		button = tea.MouseButtonForward
	}

	return button, action
}

func mouseEvent(p coninput.ButtonState, e coninput.MouseEventRecord) tea.MouseMsg {
	ev := tea.MouseMsg{
		X:     int(e.MousePositon.X),
		Y:     int(e.MousePositon.Y),
		Alt:   e.ControlKeyState.Contains(coninput.LEFT_ALT_PRESSED | coninput.RIGHT_ALT_PRESSED),
		Ctrl:  e.ControlKeyState.Contains(coninput.LEFT_CTRL_PRESSED | coninput.RIGHT_CTRL_PRESSED),
		Shift: e.ControlKeyState.Contains(coninput.SHIFT_PRESSED),
	}
	switch e.EventFlags {
	case coninput.CLICK, coninput.DOUBLE_CLICK:
		ev.Button, ev.Action = mouseEventButton(p, e.ButtonState)
		if ev.Action == tea.MouseActionRelease {
			ev.Type = tea.MouseRelease
		}
		switch ev.Button { //nolint:exhaustive
		case tea.MouseButtonLeft:
			ev.Type = tea.MouseLeft
		case tea.MouseButtonMiddle:
			ev.Type = tea.MouseMiddle
		case tea.MouseButtonRight:
			ev.Type = tea.MouseRight
		case tea.MouseButtonBackward:
			ev.Type = tea.MouseBackward
		case tea.MouseButtonForward:
			ev.Type = tea.MouseForward
		}
	case coninput.MOUSE_WHEELED:
		if e.WheelDirection > 0 {
			ev.Button = tea.MouseButtonWheelUp
			ev.Type = tea.MouseWheelUp
		} else {
			ev.Button = tea.MouseButtonWheelDown
			ev.Type = tea.MouseWheelDown
		}
	case coninput.MOUSE_HWHEELED:
		if e.WheelDirection > 0 {
			ev.Button = tea.MouseButtonWheelRight
			ev.Type = tea.MouseWheelRight
		} else {
			ev.Button = tea.MouseButtonWheelLeft
			ev.Type = tea.MouseWheelLeft
		}
	case coninput.MOUSE_MOVED:
		ev.Button, _ = mouseEventButton(p, e.ButtonState)
		ev.Action = tea.MouseActionMotion
		ev.Type = tea.MouseMotion
	}

	return ev
}

func keyType(e coninput.KeyEventRecord) tea.KeyType {
	code := e.VirtualKeyCode

	shiftPressed := e.ControlKeyState.Contains(coninput.SHIFT_PRESSED)
	ctrlPressed := e.ControlKeyState.Contains(coninput.LEFT_CTRL_PRESSED | coninput.RIGHT_CTRL_PRESSED)

	switch code { //nolint:exhaustive
	case coninput.VK_RETURN:
		return tea.KeyEnter
	case coninput.VK_BACK:
		return tea.KeyBackspace
	case coninput.VK_TAB:
		if shiftPressed {
			return tea.KeyShiftTab
		}
		return tea.KeyTab
	case coninput.VK_SPACE:
		return tea.KeyRunes // this could be tea.KeySpace but on unix space also produces tea.KeyRunes
	case coninput.VK_ESCAPE:
		return tea.KeyEscape
	case coninput.VK_UP:
		switch {
		case shiftPressed && ctrlPressed:
			return tea.KeyCtrlShiftUp
		case shiftPressed:
			return tea.KeyShiftUp
		case ctrlPressed:
			return tea.KeyCtrlUp
		default:
			return tea.KeyUp
		}
	case coninput.VK_DOWN:
		switch {
		case shiftPressed && ctrlPressed:
			return tea.KeyCtrlShiftDown
		case shiftPressed:
			return tea.KeyShiftDown
		case ctrlPressed:
			return tea.KeyCtrlDown
		default:
			return tea.KeyDown
		}
	case coninput.VK_RIGHT:
		switch {
		case shiftPressed && ctrlPressed:
			return tea.KeyCtrlShiftRight
		case shiftPressed:
			return tea.KeyShiftRight
		case ctrlPressed:
			return tea.KeyCtrlRight
		default:
			return tea.KeyRight
		}
	case coninput.VK_LEFT:
		switch {
		case shiftPressed && ctrlPressed:
			return tea.KeyCtrlShiftLeft
		case shiftPressed:
			return tea.KeyShiftLeft
		case ctrlPressed:
			return tea.KeyCtrlLeft
		default:
			return tea.KeyLeft
		}
	case coninput.VK_HOME:
		switch {
		case shiftPressed && ctrlPressed:
			return tea.KeyCtrlShiftHome
		case shiftPressed:
			return tea.KeyShiftHome
		case ctrlPressed:
			return tea.KeyCtrlHome
		default:
			return tea.KeyHome
		}
	case coninput.VK_END:
		switch {
		case shiftPressed && ctrlPressed:
			return tea.KeyCtrlShiftEnd
		case shiftPressed:
			return tea.KeyShiftEnd
		case ctrlPressed:
			return tea.KeyCtrlEnd
		default:
			return tea.KeyEnd
		}
	case coninput.VK_PRIOR:
		return tea.KeyPgUp
	case coninput.VK_NEXT:
		return tea.KeyPgDown
	case coninput.VK_DELETE:
		return tea.KeyDelete
	case coninput.VK_F1:
		return tea.KeyF1
	case coninput.VK_F2:
		return tea.KeyF2
	case coninput.VK_F3:
		return tea.KeyF3
	case coninput.VK_F4:
		return tea.KeyF4
	case coninput.VK_F5:
		return tea.KeyF5
	case coninput.VK_F6:
		return tea.KeyF6
	case coninput.VK_F7:
		return tea.KeyF7
	case coninput.VK_F8:
		return tea.KeyF8
	case coninput.VK_F9:
		return tea.KeyF9
	case coninput.VK_F10:
		return tea.KeyF10
	case coninput.VK_F11:
		return tea.KeyF11
	case coninput.VK_F12:
		return tea.KeyF12
	case coninput.VK_F13:
		return tea.KeyF13
	case coninput.VK_F14:
		return tea.KeyF14
	case coninput.VK_F15:
		return tea.KeyF15
	case coninput.VK_F16:
		return tea.KeyF16
	case coninput.VK_F17:
		return tea.KeyF17
	case coninput.VK_F18:
		return tea.KeyF18
	case coninput.VK_F19:
		return tea.KeyF19
	case coninput.VK_F20:
		return tea.KeyF20
	default:
		switch {
		case e.ControlKeyState.Contains(coninput.LEFT_CTRL_PRESSED) && e.ControlKeyState.Contains(coninput.RIGHT_ALT_PRESSED):
			// AltGr is pressed, then it's a rune.
			fallthrough
		case !e.ControlKeyState.Contains(coninput.LEFT_CTRL_PRESSED) && !e.ControlKeyState.Contains(coninput.RIGHT_CTRL_PRESSED):
			return tea.KeyRunes
		}

		switch e.Char {
		case '@':
			return tea.KeyCtrlAt
		case '\x01':
			return tea.KeyCtrlA
		case '\x02':
			return tea.KeyCtrlB
		case '\x03':
			return tea.KeyCtrlC
		case '\x04':
			return tea.KeyCtrlD
		case '\x05':
			return tea.KeyCtrlE
		case '\x06':
			return tea.KeyCtrlF
		case '\a':
			return tea.KeyCtrlG
		case '\b':
			return tea.KeyCtrlH
		case '\t':
			return tea.KeyCtrlI
		case '\n':
			return tea.KeyCtrlJ
		case '\v':
			return tea.KeyCtrlK
		case '\f':
			return tea.KeyCtrlL
		case '\r':
			return tea.KeyCtrlM
		case '\x0e':
			return tea.KeyCtrlN
		case '\x0f':
			return tea.KeyCtrlO
		case '\x10':
			return tea.KeyCtrlP
		case '\x11':
			return tea.KeyCtrlQ
		case '\x12':
			return tea.KeyCtrlR
		case '\x13':
			return tea.KeyCtrlS
		case '\x14':
			return tea.KeyCtrlT
		case '\x15':
			return tea.KeyCtrlU
		case '\x16':
			return tea.KeyCtrlV
		case '\x17':
			return tea.KeyCtrlW
		case '\x18':
			return tea.KeyCtrlX
		case '\x19':
			return tea.KeyCtrlY
		case '\x1a':
			return tea.KeyCtrlZ
		case '\x1b':
			return tea.KeyCtrlOpenBracket // tea.KeyEscape
		case '\x1c':
			return tea.KeyCtrlBackslash
		case '\x1f':
			return tea.KeyCtrlUnderscore
		}

		switch code { //nolint:exhaustive
		case coninput.VK_OEM_4:
			return tea.KeyCtrlOpenBracket
		case coninput.VK_OEM_6:
			return tea.KeyCtrlCloseBracket
		}

		return tea.KeyRunes
	}
}
