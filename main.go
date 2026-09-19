package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	appName = "Gitraffe"
	version = "0.16.2"

// logFileName is initialized at runtime in main so we can compute
// a platform-appropriate location (cache/log dir) instead of using the
// current working directory.
)

var logFileName string

var (
	// Styles — initialized by initStyles() after theme is loaded.
	titleStyle        lipgloss.Style
	commitHashStyle   lipgloss.Style
	authorStyle       lipgloss.Style
	dateStyle         lipgloss.Style
	messageStyle      lipgloss.Style
	localBranchStyle  lipgloss.Style
	aheadStyle        lipgloss.Style
	behindStyle       lipgloss.Style
	remoteBranchStyle lipgloss.Style
	mergedBranchStyle lipgloss.Style
	tagStyle          lipgloss.Style
	helpStyle         lipgloss.Style
)

// getLogFilePath returns a suitable path for the application's log file.
// It uses the OS-specific cache directory (as returned by os.UserCacheDir)
// and creates a "gitraffe" subdirectory. Falling back to the current
// directory on error keeps behaviour safe.
func getLogFilePath() string {
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

type cliOptions struct {
	repoPath    string
	themePath   string
	update      bool
	showVersion bool
}

// errUsage marks a command line parseArgs has already reported.
var errUsage = errors.New("invalid arguments")

// parseArgs reads the command line: flags plus an optional repository path.
// Flags are accepted before or after the path.
// Go's flag package on its own stops at the first non-flag argument, so
// "gitraffe . -theme x.yml" would start with the theme silently ignored.
func parseArgs(args []string, output io.Writer) (cliOptions, error) {
	opts := cliOptions{repoPath: "."}

	fs := flag.NewFlagSet("gitraffe", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&opts.themePath, "theme", "",
		"colour theme `file` to use instead of your theme config, e.g. themes/tokyo-night-storm.yml")
	fs.BoolVar(&opts.update, "update", false,
		"install the latest release and exit, instead of opening a repository")
	fs.BoolVar(&opts.showVersion, "version", false,
		"print the version and exit")
	fs.Usage = func() {
		fmt.Fprintf(output, "Usage:\n  gitraffe [flags] [repository path]\n\nFlags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(output, "\nFlags take one or two dashes: -theme and --theme are the same.\n")
		fmt.Fprintf(output, "The bare words 'gitraffe update' and 'gitraffe version' work too.\n")
	}

	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return opts, err // flag has printed the error and the usage
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}

	switch len(positional) {
	case 0:
	case 1:
		// The flags that do something and exit also answer to a bare word:
		// "update" shipped before -update existed, and "version" matches it so
		// the pair does not surprise anyone who tries the other. A bare word
		// shadows a repository in a directory of that name; "./update" opens it.
		switch positional[0] {
		case "update":
			opts.update = true
		case "version":
			opts.showVersion = true
		default:
			opts.repoPath = positional[0]
		}
	default:
		fmt.Fprintf(output, "expected at most one repository path, got %q\n\n", positional)
		fs.Usage()
		return opts, errUsage
	}
	return opts, nil
}

func main() {
	// Determine where the log file should live based on OS conventions
	logFileName = getLogFilePath()

	// Set up logging to file for debugging
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	log.Println("Starting " + appName + "...")

	opts, err := parseArgs(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		// parseArgs has already printed the problem and the usage.
		os.Exit(2)
	}

	// Answered before -update so it never waits on the network.
	if opts.showVersion {
		fmt.Printf("%s v%s\n", appName, version)
		return
	}

	if opts.update {
		if err := checkUpdate(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Before the TUI starts, so a bad -theme is reported on the normal screen.
	configDir := themeConfigDir()
	if err := loadTheme(opts.themePath, configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	initStyles()

	repoPath := opts.repoPath

	log.Printf("Opening repository: %s\n", repoPath)

	// Set terminal title (works on Windows 10+, macOS, Linux)
	setTerminalTitle(appName)
	defer resetTerminalTitle()

	m := initialModel(repoPath)
	m.configDir = configDir

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	finalModel, err := p.Run()
	if err != nil {
		log.Printf("Program error: %v\n", err)
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Reported here rather than from the TUI so it lands on the normal screen
	// that Bubble Tea has just restored, instead of the alt screen it tore down.
	if m, ok := finalModel.(model); ok && m.updatedTo != "" {
		fmt.Printf("Updated to %s — restart gitraffe to use the new version.\n", m.updatedTo)
	}

	log.Println(appName + " exited normally")
}
