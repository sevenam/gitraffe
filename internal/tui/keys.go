package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleKey routes a key press to whatever currently owns the keyboard: a
// prompt, an overlay, the commit view, and only then the main screen.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// The confirmation prompt owns the keyboard until it is answered, so a
	// stray "j" can't scroll the graph behind a pending yes/no.
	if m.updateState == updateConfirming {
		switch msg.String() {
		case "y", "Y", "enter":
			m.updateState = updateDownloading
			m.updateMessage = "Downloading " + m.latestVersion + "..."
			return m, performUpdateCmd()
		default:
			m.updateState = updateIdle
			m.updateMessage = ""
			return m, nil
		}
	}

	// Mid-download the binary is being swapped underneath us; only quitting
	// is meaningful, and it stays available in case the download hangs.
	if m.updateState == updateDownloading {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}

	// Any keystroke dismisses a lingering notice.
	m.updateMessage = ""
	m.notice = ""

	// The copy prompt is answered by the next key, wherever it was opened.
	if m.copying {
		return m.answerCopyPrompt(msg)
	}
	if m.push.asking {
		return m.answerPush(msg)
	}
	if m.push.naming {
		return m.updatePushNaming(msg)
	}
	if m.checkout.open {
		return m.answerCheckout(msg)
	}
	if m.branchPrompt.open {
		return m.updateBranchPrompt(msg)
	}
	if m.commitPrompt.open {
		return m.updateCommitPrompt(msg)
	}

	if m.picker.open {
		return m.updateThemePicker(msg)
	}
	if m.deletePicker.open {
		return m.updateDeletePicker(msg)
	}
	if m.switcher.open {
		return m.updateRepoSwitcher(msg)
	}
	if m.refs.open {
		return m.updateRefPicker(msg)
	}
	if m.search.active {
		return m.updateSearch(msg)
	}
	if m.files.open {
		return m.updateFilePicker(msg)
	}

	// The help overlay covers the panels, so keys acting on them would change
	// things the user can't see. Esc and q close it rather than quit: pressed
	// while reading help they mean "back", and quitting would lose the place.
	if m.showHelp {
		switch msg.String() {
		case "?", "esc", "q":
			m.showHelp = false
		case "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}

	// The commit view has the screen to itself, so it answers every key:
	// the graph it replaced is not there to be steered.
	if m.commitView.open {
		return m.updateCommitView(msg)
	}

	switch msg.String() {
	// Not esc: everywhere else it means "back", so it gets pressed once too
	// often on the way out of a view and would throw away the whole session.
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "t":
		return m.openThemePicker(), nil
	case "O":
		// Capital: it leaves the repository on screen for another, which
		// should take more than a slip of the finger.
		// Not while loading: the load under way would finish into the model
		// the switch replaced it with.
		if !m.ready {
			return m, nil
		}
		return m.openRepoSwitcher(), nil
	case "enter":
		// Whichever panel has focus fills the window; enter again puts the
		// other one back. Gitraffe starts maximised, so from a fresh run the
		// first press is the one that brings the details panel out.
		m.maximised = !m.maximised
		return m, nil
	case "B":
		if !m.ready {
			return m, nil
		}
		return m.openRefPicker(), nil
	case "p":
		if !m.ready {
			return m, nil
		}
		return m.startPull()
	case "P":
		// Capital: it is the one key here that sends your work somewhere it
		// cannot be taken back from, which should take more than a slip of
		// the finger. It asks before making a new branch or tag on the remote.
		return m.openPush()
	case "o":
		// Lower case: it opens a page and changes nothing, here or in the
		// repository.
		if !m.ready {
			return m, nil
		}
		return m.openPullRequest()
	case "y":
		return m.openCopyPrompt(), nil
	case " ":
		// Space, not ctrl+enter: most terminals cannot tell ctrl+enter
		// from enter, so only the newer keyboard protocols would report
		// the difference and the binding would do nothing for everyone
		// else. Bubble Tea reports a space as its own key type, which
		// String reports as " ".
		return m.openCommitView()
	case "f":
		if !m.ready {
			return m, nil
		}
		return m.startFetch()
	case "/":
		if !m.ready {
			return m, nil
		}
		return m.openSearch(), nil
	case "h":
		// The same key as in the commit view's file list, so "history" is
		// one key wherever you are; there the file is already chosen, here
		// it is picked from the list.
		if !m.ready {
			return m, nil
		}
		return m.openFilePicker()
	case "esc":
		// Esc means "back" everywhere, and on the graph the only place to go
		// back to is the whole history.
		return m.clearFilter()
	case "n":
		return m.searchNext(1)
	case "N":
		return m.searchNext(-1)
	case "m":
		if !m.ready {
			return m, nil
		}
		return m.loadMoreCommits()
	case "r", "f5":
		if !m.ready {
			return m, nil
		}
		return m.refresh()
	case "U":
		return m.startUpdate(), nil
	case "c":
		// Lower case although it moves HEAD: the switch is refused while
		// there are uncommitted changes, so a slip loses nothing and "c"
		// on the branch left behind undoes it. In the commit view "c"
		// commits; that view never hands its keys on to the graph.
		return m.openCheckout()
	case "b":
		// Lower case although it makes a branch: it only opens the box, and
		// nothing is made until a name there is given with enter.
		return m.openBranchPrompt()
	case "d":
		// Lower case although it deletes: it only opens the list, and
		// nothing goes until a row there is picked with enter.
		return m.openDelete(), nil
	case "L":
		// Global rather than per-box: the graph stays visible whichever
		// box has focus, so the colours should be reachable from both.
		m.colourLanes = !m.colourLanes
		return m, nil
	case "1":
		m.focusedBox = 1
		return m, nil
	case "2":
		m.focusedBox = 2
		return m, nil
	case "tab":
		if m.focusedBox == 1 {
			m.focusedBox = 2
		} else {
			m.focusedBox = 1
		}
		return m, nil
	case "shift+tab":
		if m.focusedBox == 2 {
			m.focusedBox = 1
		} else {
			m.focusedBox = 2
		}
		return m, nil
	}

	// Handle scrolling within the focused box
	if m.ready && len(m.commits) > 0 {
		switch m.focusedBox {
		case 1: // commit list / graph
			switch msg.String() {
			case "j", "down":
				if m.selected < len(m.commits)-1 {
					m.selected++
					m.detailsScroll = 0
				}
				return m, m.maybeLoadDiff()
			case "k", "up":
				if m.selected > 0 {
					m.selected--
					m.detailsScroll = 0
				}
				return m, m.maybeLoadDiff()
			case "ctrl+d", "pgdown":
				m.selected += 10
				if m.selected >= len(m.commits) {
					m.selected = len(m.commits) - 1
				}
				m.detailsScroll = 0
				return m, m.maybeLoadDiff()
			case "ctrl+u", "pgup":
				m.selected -= 10
				if m.selected < 0 {
					m.selected = 0
				}
				m.detailsScroll = 0
				return m, m.maybeLoadDiff()
			case "g", "ctrl+home":
				m.selected = 0
				m.detailsScroll = 0
				return m, m.maybeLoadDiff()
			case "G", "ctrl+end":
				m.selected = len(m.commits) - 1
				m.detailsScroll = 0
				return m, m.maybeLoadDiff()
			// Home and end stay on the screen you are reading, as they do
			// in a text editor; the ctrl forms above leave it for the ends.
			case "home", "end":
				first, last := m.graphTopAndBottom()
				if msg.String() == "home" {
					m.selected = first
				} else {
					m.selected = last
				}
				m.detailsScroll = 0
				return m, m.maybeLoadDiff()
			}
		case 2: // commit details
			switch msg.String() {
			case "j", "down":
				m.detailsScroll++
				return m, nil
			case "k", "up":
				if m.detailsScroll > 0 {
					m.detailsScroll--
				}
				return m, nil
			case "ctrl+d", "pgdown":
				m.detailsScroll += 10
				return m, nil
			case "ctrl+u", "pgup":
				m.detailsScroll -= 10
				if m.detailsScroll < 0 {
					m.detailsScroll = 0
				}
				return m, nil
			case "g", "home", "ctrl+home":
				m.detailsScroll = 0
				return m, nil
			}
		}
	}

	return m, nil
}
