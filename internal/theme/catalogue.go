package theme

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sevenam/gitraffe/themes"
)

const (
	// DefaultName is the picker's entry for the built-in colours.
	DefaultName = "default"
	// CustomName is the picker's entry for the config directory's
	// theme.yml, named after the file so it is recognisable as the one the
	// README tells people to create.
	CustomName = "theme.yml"
)

// CurrentName is the picker's name for Current, so the picker can
// open on it. Empty when the colours came from a -theme file, which is not in
// the list.
var CurrentName string

// Choice is one entry in the theme picker.
type Choice struct {
	Name   string // shown in the picker and recorded in settings.yml
	Source string // "built in" or "yours"
	Load   func() (Colors, error)
}

// Available lists what the picker offers: the defaults, the bundled
// themes, any theme files in the config directory's themes folder, then
// theme.yml. A file of yours with a bundled theme's name replaces it, so a
// bundled theme can be tweaked by copying it there.
func Available(configDir string) []Choice {
	choices := []Choice{{
		Name:   DefaultName,
		Source: "built in",
		Load:   func() (Colors, error) { return Default(), nil },
	}}
	index := map[string]int{DefaultName: 0}
	add := func(c Choice) {
		if i, ok := index[c.Name]; ok {
			choices[i] = c
			return
		}
		index[c.Name] = len(choices)
		choices = append(choices, c)
	}

	bundled, _ := fs.Glob(themes.FS, "*.yml") // sorted; the pattern is valid
	// Variants of the default (default-light) come straight after it rather than
	// among the others by name, so the default's family stays together.
	isVariant := func(p string) bool { return strings.HasPrefix(path.Base(p), DefaultName+"-") }
	sort.SliceStable(bundled, func(i, j int) bool { return isVariant(bundled[i]) && !isVariant(bundled[j]) })
	for _, p := range bundled {
		add(Choice{
			Name:   strings.TrimSuffix(path.Base(p), ".yml"),
			Source: "built in",
			Load: func() (Colors, error) {
				data, err := themes.FS.ReadFile(p)
				if err != nil {
					return Colors{}, err
				}
				// Strict: these ship with gitraffe, and a test holds them to it.
				return Parse(data, true)
			},
		})
	}

	if configDir == "" {
		return choices
	}
	yours, _ := filepath.Glob(filepath.Join(configDir, "themes", "*.yml"))
	for _, p := range yours {
		add(Choice{
			Name:   strings.TrimSuffix(filepath.Base(p), ".yml"),
			Source: "yours",
			Load:   themeFileLoader(p),
		})
	}
	if custom := filepath.Join(configDir, "theme.yml"); fileExists(custom) {
		add(Choice{Name: CustomName, Source: "yours", Load: themeFileLoader(custom)})
	}
	return choices
}

// themeFileLoader reads one of the user's theme files. Lenient like theme.yml
// always has been: an unknown key is ignored rather than refusing the theme.
func themeFileLoader(p string) func() (Colors, error) {
	return func() (Colors, error) {
		data, err := os.ReadFile(p)
		if err != nil {
			return Colors{}, err
		}
		return Parse(data, false)
	}
}

// Find looks a theme up by the name the picker shows.
func Find(choices []Choice, name string) (Choice, bool) {
	for _, c := range choices {
		if c.Name == name {
			return c, true
		}
	}
	return Choice{}, false
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
