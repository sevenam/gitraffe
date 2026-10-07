package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/sevenam/gitraffe/internal/config"
	"github.com/sevenam/gitraffe/internal/selfupdate"
	"github.com/sevenam/gitraffe/internal/theme"
	"github.com/sevenam/gitraffe/internal/tui"
)

// The release workflow reads version out of this file and refuses a tag that
// doesn't match it, so the constant has to stay here.
const (
	appName = "Gitraffe"
	version = "0.49.4"
)

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
	logPath := config.LogPath()

	// Set up logging to file for debugging
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
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
		if err := selfupdate.Check(version); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Before the TUI starts, so a bad -theme is reported on the normal screen.
	configDir := config.Dir()
	if err := theme.Load(opts.themePath, configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	log.Printf("Opening repository: %s\n", opts.repoPath)

	tui.Version = version
	tui.LogPath = logPath
	updatedTo, err := tui.Run(opts.repoPath, configDir)
	if err != nil {
		log.Printf("Program error: %v\n", err)
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Reported here rather than from the TUI so it lands on the normal
	// screen that Bubble Tea has just restored, instead of the alt screen
	// it tore down.
	if updatedTo != "" {
		fmt.Printf("Updated to %s — restart gitraffe to use the new version.\n", updatedTo)
	}

	log.Println(appName + " exited normally")
}
