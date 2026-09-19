package main

import (
	"path/filepath"
	"strconv"
	"testing"
)

func TestPreferencesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := initialModel(".")
	m.configDir = dir
	m.colourLanes = false
	m.focusedBox = 2
	if err := savePreferences(m); err != nil {
		t.Fatal(err)
	}

	fresh := initialModel(".")
	fresh.configDir = dir
	next := applyPreferences(fresh)
	if next.colourLanes || next.focusedBox != 2 {
		t.Errorf("colourLanes=%v focusedBox=%d, want false and 2", next.colourLanes, next.focusedBox)
	}
}

func TestPreferencesKeepTheDefaultsWhenUnset(t *testing.T) {
	fresh := initialModel(".")
	fresh.configDir = t.TempDir()
	got := applyPreferences(fresh)
	// Lane colours are on and the graph has focus until something says otherwise.
	if !got.colourLanes || got.focusedBox != 1 {
		t.Errorf("colourLanes=%v focusedBox=%d, want true and 1", got.colourLanes, got.focusedBox)
	}
}

// The file is hand-editable, so it can name a panel that doesn't exist.
func TestPreferencesIgnoreAnImpossiblePanel(t *testing.T) {
	for _, box := range []int{0, 3, -1} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "settings.yml"), "focused_box: "+strconv.Itoa(box)+"\n")
		m := initialModel(".")
		m.configDir = dir
		if got := applyPreferences(m); got.focusedBox != 1 {
			t.Errorf("focused_box %d gave %d, want the graph panel", box, got.focusedBox)
		}
	}
}

// Lane colours off has to survive a round trip: written as a plain false it
// would be indistinguishable from never having been saved.
func TestPreferencesRememberLaneColoursOff(t *testing.T) {
	dir := t.TempDir()
	m := initialModel(".")
	m.configDir = dir
	m.colourLanes = false
	if err := savePreferences(m); err != nil {
		t.Fatal(err)
	}
	if s := loadSettings(dir); s.LaneColours == nil || *s.LaneColours {
		t.Errorf("lane colours = %v, want a saved false", s.LaneColours)
	}
}

func TestPreferencesKeepTheOtherSettings(t *testing.T) {
	dir := t.TempDir()
	if err := saveThemeChoice(dir, "default-light"); err != nil {
		t.Fatal(err)
	}
	repo := newRepo(t)
	r := initialModel(repo)
	r.configDir = dir
	r.rememberCurrentRepo()

	m := initialModel(".")
	m.configDir = dir
	m.focusedBox = 2
	if err := savePreferences(m); err != nil {
		t.Fatal(err)
	}

	s := loadSettings(dir)
	if s.Theme != "default-light" || len(s.RecentRepos) != 1 {
		t.Errorf("theme=%q recents=%v; saving preferences dropped them", s.Theme, s.RecentRepos)
	}
}

func TestPreferencesWithoutAConfigDir(t *testing.T) {
	m := initialModel(".")
	if err := savePreferences(m); err != nil {
		t.Errorf("saving without a config directory returned %v, want it skipped quietly", err)
	}
	if got := applyPreferences(m); !got.colourLanes || got.focusedBox != 1 {
		t.Error("no config directory should leave the defaults alone")
	}
}
