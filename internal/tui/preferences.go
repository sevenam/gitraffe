package tui

import (
	"github.com/sevenam/gitraffe/internal/config"
)

// applyPreferences starts the model off where the last run left it. Anything
// the file doesn't hold, or holds nonsense, keeps the built-in default: these
// are conveniences, and a hand-edited file shouldn't be able to start gitraffe
// focused on a panel that doesn't exist.
func applyPreferences(m model) model {
	s := config.Load(m.configDir)
	if s.LaneColours != nil {
		m.colourLanes = *s.LaneColours
	}
	if s.Maximised != nil {
		m.maximised = *s.Maximised
	}
	if s.Whitespace != nil {
		m.showWhitespace = *s.Whitespace
	}
	m.autoRefresh = defaultAutoRefresh
	if s.AutoRefresh != nil {
		m.autoRefresh = interval(*s.AutoRefresh, minAutoRefresh)
	}
	m.autoFetch = interval(s.AutoFetch, minAutoFetch)
	if s.FocusedBox == 1 || s.FocusedBox == 2 {
		m.focusedBox = s.FocusedBox
	}
	return m
}

// savePreferences records them again as gitraffe exits, rather than on every
// keystroke that changes one: "L" and tab are pressed often, and the file is
// only ever read at startup.
func savePreferences(m model) error {
	if m.configDir == "" {
		return nil
	}
	s := config.Load(m.configDir)
	s.LaneColours = &m.colourLanes
	s.FocusedBox = m.focusedBox
	s.Maximised = &m.maximised
	s.Whitespace = &m.showWhitespace
	return config.Save(m.configDir, s)
}
