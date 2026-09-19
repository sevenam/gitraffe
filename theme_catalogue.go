package main

import (
	"embed"
	"errors"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The bundled themes are compiled in: "go install" builds from the module
// cache and copies only the binary, so a themes/ folder on disk would exist
// for people who cloned the repository and nobody else.
//
//go:embed themes/*.yml
var bundledThemes embed.FS

const (
	defaultThemeName = "default"
	// customThemeName is the picker's entry for the config directory's
	// theme.yml, named after the file so it is recognisable as the one the
	// README tells people to create.
	customThemeName = "theme.yml"
)

// currentThemeName is the picker's name for currentTheme, so the picker can
// open on it. Empty when the colours came from a -theme file, which is not in
// the list.
var currentThemeName string

// themeChoice is one entry in the theme picker.
type themeChoice struct {
	name   string // shown in the picker and recorded in settings.yml
	source string // "built in" or "yours"
	load   func() (ThemeColors, error)
}

// themeConfigDir is gitraffe's directory in the user's config directory, where
// theme.yml, settings.yml and the themes folder live. Empty when the OS has no
// config directory, which disables all three.
func themeConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("Theme: config dir unavailable: %v", err)
		return ""
	}
	return filepath.Join(dir, "gitraffe")
}

// availableThemes lists what the picker offers: the defaults, the bundled
// themes, any theme files in the config directory's themes folder, then
// theme.yml. A file of yours with a bundled theme's name replaces it, so a
// bundled theme can be tweaked by copying it there.
func availableThemes(configDir string) []themeChoice {
	choices := []themeChoice{{
		name:   defaultThemeName,
		source: "built in",
		load:   func() (ThemeColors, error) { return defaultTheme(), nil },
	}}
	index := map[string]int{defaultThemeName: 0}
	add := func(c themeChoice) {
		if i, ok := index[c.name]; ok {
			choices[i] = c
			return
		}
		index[c.name] = len(choices)
		choices = append(choices, c)
	}

	bundled, _ := fs.Glob(bundledThemes, "themes/*.yml") // sorted; the pattern is valid
	// Variants of the default (default-light) come straight after it rather than
	// among the others by name, so the default's family stays together.
	isVariant := func(p string) bool { return strings.HasPrefix(path.Base(p), defaultThemeName+"-") }
	sort.SliceStable(bundled, func(i, j int) bool { return isVariant(bundled[i]) && !isVariant(bundled[j]) })
	for _, p := range bundled {
		add(themeChoice{
			name:   strings.TrimSuffix(path.Base(p), ".yml"),
			source: "built in",
			load: func() (ThemeColors, error) {
				data, err := bundledThemes.ReadFile(p)
				if err != nil {
					return ThemeColors{}, err
				}
				// Strict: these ship with gitraffe, and a test holds them to it.
				return parseTheme(data, true)
			},
		})
	}

	if configDir == "" {
		return choices
	}
	yours, _ := filepath.Glob(filepath.Join(configDir, "themes", "*.yml"))
	for _, p := range yours {
		add(themeChoice{
			name:   strings.TrimSuffix(filepath.Base(p), ".yml"),
			source: "yours",
			load:   themeFileLoader(p),
		})
	}
	if custom := filepath.Join(configDir, "theme.yml"); fileExists(custom) {
		add(themeChoice{name: customThemeName, source: "yours", load: themeFileLoader(custom)})
	}
	return choices
}

// themeFileLoader reads one of the user's theme files. Lenient like theme.yml
// always has been: an unknown key is ignored rather than refusing the theme.
func themeFileLoader(p string) func() (ThemeColors, error) {
	return func() (ThemeColors, error) {
		data, err := os.ReadFile(p)
		if err != nil {
			return ThemeColors{}, err
		}
		return parseTheme(data, false)
	}
}

func findTheme(choices []themeChoice, name string) (themeChoice, bool) {
	for _, c := range choices {
		if c.name == name {
			return c, true
		}
	}
	return themeChoice{}, false
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// settings is settings.yml: choices made inside the app. It is kept apart from
// theme.yml so picking a theme never overwrites colours someone wrote by hand.
type settings struct {
	Theme string `yaml:"theme,omitempty"`
	// RecentRepos lists repository roots, most recently opened first; the
	// repository switcher offers them. See rememberRepo.
	RecentRepos []string `yaml:"recent_repos,omitempty"`
	// LaneColours and FocusedBox carry the state of the "c" toggle and which
	// panel had focus into the next run. A pointer and a zero mean "never
	// saved", which is what keeps the defaults in initialModel the defaults
	// rather than "off" and "no panel". See applyPreferences.
	LaneColours *bool `yaml:"lane_colours,omitempty"`
	FocusedBox  int   `yaml:"focused_box,omitempty"`
	// Maximised needs no pointer: not maximised is both the default and the
	// zero value, so an absent key and a saved false mean the same thing.
	Maximised bool `yaml:"maximised,omitempty"`
}

func settingsPath(configDir string) string {
	return filepath.Join(configDir, "settings.yml")
}

// loadSettings reads settings.yml. A missing or unreadable file is the same as
// an empty one: it only holds preferences, so it is never worth refusing to
// start over.
func loadSettings(configDir string) settings {
	var s settings
	if configDir == "" {
		return s
	}
	data, err := os.ReadFile(settingsPath(configDir))
	if err != nil {
		return s
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		log.Printf("Settings: parse error in %s: %v", settingsPath(configDir), err)
		return settings{}
	}
	return s
}

// saveThemeChoice records the picked theme so it is used on the next start.
func saveThemeChoice(configDir, name string) error {
	s := loadSettings(configDir)
	s.Theme = name
	return saveSettings(configDir, s)
}

// applyPreferences starts the model off where the last run left it. Anything
// the file doesn't hold, or holds nonsense, keeps the built-in default: these
// are conveniences, and a hand-edited file shouldn't be able to start gitraffe
// focused on a panel that doesn't exist.
func applyPreferences(m model) model {
	s := loadSettings(m.configDir)
	if s.LaneColours != nil {
		m.colourLanes = *s.LaneColours
	}
	m.maximised = s.Maximised
	if s.FocusedBox == 1 || s.FocusedBox == 2 {
		m.focusedBox = s.FocusedBox
	}
	return m
}

// savePreferences records them again as gitraffe exits, rather than on every
// keystroke that changes one: "c" and tab are pressed often, and the file is
// only ever read at startup.
func savePreferences(m model) error {
	if m.configDir == "" {
		return nil
	}
	s := loadSettings(m.configDir)
	s.LaneColours = &m.colourLanes
	s.FocusedBox = m.focusedBox
	s.Maximised = m.maximised
	return saveSettings(m.configDir, s)
}

// saveSettings writes settings.yml. Callers read it first and change only
// their own field, so one setting never drops another.
func saveSettings(configDir string, s settings) error {
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
	return os.WriteFile(settingsPath(configDir), data, 0o644)
}

// setTheme makes c the colours in use. The styles are package-level and built
// from currentTheme, so they have to be rebuilt too.
func setTheme(name string, c ThemeColors) {
	currentTheme = c
	currentThemeName = name
	initStyles()
}
