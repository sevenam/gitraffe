// Package config reads and writes what gitraffe keeps in the user's config
// directory, and says where that directory and the log file are.
package config

import (
	"errors"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Dir is gitraffe's directory in the user's config directory, where
// theme.yml, settings.yml and the themes folder live. Empty when the OS has no
// config directory, which disables all three.
func Dir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("Theme: config dir unavailable: %v", err)
		return ""
	}
	return filepath.Join(dir, "gitraffe")
}

// LogPath returns a suitable path for the application's log file.
// It uses the OS-specific cache directory (as returned by os.UserCacheDir)
// and creates a "gitraffe" subdirectory. Falling back to the current
// directory on error keeps behaviour safe.
func LogPath() string {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		// last-resort fallback
		return "gitraffe.log"
	}
	// create subdirectory for our logs
	dir = filepath.Join(dir, "gitraffe")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "gitraffe.log")
}

// Settings is settings.yml: choices made inside the app. It is kept apart from
// theme.yml so picking a theme never overwrites colours someone wrote by hand.
type Settings struct {
	Theme string `yaml:"theme,omitempty"`
	// RecentRepos lists repository roots, most recently opened first; the
	// repository switcher offers them.
	RecentRepos []string `yaml:"recent_repos,omitempty"`
	// LaneColours, Maximised and FocusedBox carry the state of the "c" and
	// enter toggles and which panel had focus into the next run. A nil pointer
	// and a zero mean "never saved", which is what keeps the interface's
	// defaults the defaults rather than "split", "off" and "no panel":
	// both toggles are on by default, so a saved false has to be told apart
	// from an absent key.
	LaneColours *bool `yaml:"lane_colours,omitempty"`
	FocusedBox  int   `yaml:"focused_box,omitempty"`
	Maximised   *bool `yaml:"maximised,omitempty"`
	// AutoRefresh is how often, in seconds, the open repository is checked for
	// changes. It is a pointer for the same reason as the toggles above: absent
	// means the default interval, and 0 means never.
	AutoRefresh *int `yaml:"auto_refresh,omitempty"`
	// AutoFetch is how often, in seconds, to fetch unasked. Absent and 0 both
	// mean never: reaching the network is something to opt into.
	AutoFetch int `yaml:"auto_fetch,omitempty"`
}

// Path is where settings.yml lives in configDir.
func Path(configDir string) string {
	return filepath.Join(configDir, "settings.yml")
}

// Load reads settings.yml. A missing or unreadable file is the same as
// an empty one: it only holds preferences, so it is never worth refusing to
// start over.
func Load(configDir string) Settings {
	var s Settings
	if configDir == "" {
		return s
	}
	data, err := os.ReadFile(Path(configDir))
	if err != nil {
		return s
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		log.Printf("Settings: parse error in %s: %v", Path(configDir), err)
		return Settings{}
	}
	return s
}

// SaveTheme records the picked theme so it is used on the next start.
func SaveTheme(configDir, name string) error {
	s := Load(configDir)
	s.Theme = name
	return Save(configDir, s)
}

// Save writes settings.yml. Callers read it first and change only
// their own field, so one setting never drops another.
func Save(configDir string, s Settings) error {
	if configDir == "" {
		return errors.New("no config directory")
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(Path(configDir), data, 0o644)
}
