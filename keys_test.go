package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPageKeysMatchHalfPageKeys(t *testing.T) {
	pageDown := tea.KeyMsg{Type: tea.KeyPgDown}
	pageUp := tea.KeyMsg{Type: tea.KeyPgUp}
	d := keyPress("d")
	u := keyPress("u")

	// Starting points chosen to hit both a plain move and clamping at each end.
	for _, tc := range []struct {
		name      string
		box       int
		start     int
		page, alt tea.KeyMsg
	}{
		{"graph: page down", 1, 3, pageDown, d},
		{"graph: page down clamps at the last commit", 1, 20, pageDown, d},
		{"graph: page up", 1, 15, pageUp, u},
		{"graph: page up clamps at the first commit", 1, 4, pageUp, u},
		{"details: page down", 2, 3, pageDown, d},
		{"details: page up", 2, 15, pageUp, u},
		{"details: page up clamps at the top", 2, 4, pageUp, u},
	} {
		t.Run(tc.name, func(t *testing.T) {
			build := func() model {
				m := testModel()
				m.commits = make([]commit, 25)
				m.focusedBox = tc.box
				m.selected = tc.start
				m.detailsScroll = tc.start
				// Diffs count as loaded so the key handler starts no git process.
				for i := range m.commits {
					m.commits[i].DiffLoaded = true
				}
				return m
			}

			viaPage, _ := build().Update(tc.page)
			viaLetter, _ := build().Update(tc.alt)
			gotPage, gotLetter := viaPage.(model), viaLetter.(model)

			if gotPage.selected != gotLetter.selected || gotPage.detailsScroll != gotLetter.detailsScroll {
				t.Errorf("%s gave selected=%d scroll=%d, %s gave selected=%d scroll=%d",
					tc.page.String(), gotPage.selected, gotPage.detailsScroll,
					tc.alt.String(), gotLetter.selected, gotLetter.detailsScroll)
			}
			if gotPage.selected == tc.start && gotPage.detailsScroll == tc.start {
				t.Errorf("%s did nothing", tc.page.String())
			}
		})
	}
}
