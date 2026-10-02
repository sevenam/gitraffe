// Package theme holds the colours gitraffe draws with: the defaults, the theme
// files that override them, and the list of themes the picker offers.
package theme

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/sevenam/gitraffe/internal/config"
)

// Colors holds all color values for the application.
type Colors struct {
	// Background paints the whole screen; empty leaves the terminal's own
	// background showing, which is what every theme did before it existed.
	Background string `yaml:"background"`
	// Foreground is the colour of text that has none of its own, used only with
	// a Background; empty means the Message colour. See paintBackground.
	Foreground     string `yaml:"foreground"`
	Title          string `yaml:"title"`
	Hash           string `yaml:"hash"`
	Author         string `yaml:"author"`
	Date           string `yaml:"date"`
	Message        string `yaml:"message"`
	Branch         string `yaml:"branch"`
	LocalBranch    string `yaml:"local_branch"`
	Ahead          string `yaml:"ahead"`
	Behind         string `yaml:"behind"`
	MergedBranch   string `yaml:"merged_branch"`
	Tag            string `yaml:"tag"`
	Help           string `yaml:"help"`
	Error          string `yaml:"error"`
	SectionHeader  string `yaml:"section_header"`
	Graph          string `yaml:"graph"`
	BorderActive   string `yaml:"border_active"`
	BorderInactive string `yaml:"border_inactive"`
	SelectedFg     string `yaml:"selected_fg"`
	SelectedBg     string `yaml:"selected_bg"`
	DiffAdd        string `yaml:"diff_add"`
	DiffDel        string `yaml:"diff_del"`
	DiffHunk       string `yaml:"diff_hunk"`
	DiffHeader     string `yaml:"diff_header"`
}

type themeFile struct {
	Colors Colors `yaml:"colors"`
}

// Current is the colours in use. Load sets it; the interface rebuilds its
// styles whenever it changes.
var Current Colors

// Default is the built-in colours, which every theme file is read over.
func Default() Colors {
	return Colors{
		Title:          "#7D56F4",
		Hash:           "#FFA500",
		Author:         "#7DD3FC",
		Date:           "#A3BE8C",
		Message:        "#E5E9F0",
		Branch:         "#88C0D0",
		MergedBranch:   "#707880",
		Tag:            "#EBCB8B",
		Help:           "#626262",
		Error:          "#FF0000",
		SectionHeader:  "#7D56F4",
		Graph:          "#FFA500",
		BorderActive:   "#FFA500",
		BorderInactive: "#7D56F4",
		SelectedFg:     "#FFFFFF",
		SelectedBg:     "#3C3C3C",
		DiffAdd:        "#A3BE8C",
		DiffDel:        "#BF616A",
		DiffHunk:       "#5E81AC",
		DiffHeader:     "#E5E9F0",
	}
}

// Load applies a theme over the default colours.
//
// With a path (the -theme flag) every problem is returned, and unknown keys
// count as problems: the user named that file, so quietly showing defaults
// instead would look as if the flag had been ignored. Without one it looks for
// the theme in configDir, where having no file is the normal case and problems
// are only logged: first the one picked in the app (settings.yml), then
// theme.yml. A picked theme wins because picking is the more recent, explicit
// choice; theme.yml stays in the picker for switching back.
func Load(path, configDir string) error {
	Current = Default()
	CurrentName = DefaultName

	if path != "" {
		CurrentName = ""
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("theme: %w", err)
		}
		colors, err := Parse(data, true)
		if err != nil {
			return fmt.Errorf("theme %s: %w", path, err)
		}
		Current = colors
		log.Printf("Theme: loaded from %s (-theme)", path)
		return nil
	}

	if configDir == "" {
		return nil
	}

	if name := config.Load(configDir).Theme; name != "" {
		choice, ok := Find(Available(configDir), name)
		if !ok {
			log.Printf("Theme: picked theme %q no longer exists, falling back", name)
		} else if colors, err := choice.Load(); err != nil {
			log.Printf("Theme: picked theme %q: %v, falling back", name, err)
		} else {
			Current = colors
			CurrentName = name
			log.Printf("Theme: %s (picked in the app)", name)
			return nil
		}
	}

	themePath := filepath.Join(configDir, "theme.yml")
	log.Printf("Theme: looking for %s", themePath)

	data, err := os.ReadFile(themePath)
	if err != nil {
		log.Printf("Theme: no file at %s, using defaults", themePath)
		return nil
	}

	colors, err := Parse(data, false)
	if err != nil {
		log.Printf("Theme: parse error in %s: %v", themePath, err)
		return nil
	}

	Current = colors
	CurrentName = CustomName
	log.Printf("Theme: loaded from %s", themePath)
	return nil
}

// Parse reads a theme file's colours over the defaults, so a file only has
// to name the colours it changes. strict rejects unknown keys, which is how a
// misspelt key gets caught instead of silently doing nothing.
func Parse(data []byte, strict bool) (Colors, error) {
	tf := themeFile{Colors: Default()}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(strict)
	// An empty file decodes to io.EOF: nothing to override, not an error.
	if err := dec.Decode(&tf); err != nil && !errors.Is(err, io.EOF) {
		return Colors{}, err
	}
	return tf.Colors, nil
}

// IsLight reports whether a "#rrggbb" colour is light enough that
// colours picked for a dark background would wash out on it. Anything else,
// including no colour at all, counts as dark: that is what the bundled
// palettes assume.
func IsLight(hex string) bool {
	if len(hex) != 7 || hex[0] != '#' {
		return false
	}
	v, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return false
	}
	r, g, b := float64(v>>16&0xff), float64(v>>8&0xff), float64(v&0xff)
	return (0.299*r+0.587*g+0.114*b)/255 > 0.5
}
