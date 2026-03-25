package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	appName = "Gitraffe"
	version = "0.3.2"

// logFileName is initialized at runtime in main so we can compute
// a platform-appropriate location (cache/log dir) instead of using the
// current working directory.
)

var logFileName string

var (
	// Styles — initialized by initStyles() after theme is loaded.
	titleStyle      lipgloss.Style
	commitHashStyle lipgloss.Style
	authorStyle     lipgloss.Style
	dateStyle       lipgloss.Style
	messageStyle    lipgloss.Style
	branchStyle     lipgloss.Style
	tagStyle        lipgloss.Style
	helpStyle       lipgloss.Style
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

	// Handle update subcommand
	if len(os.Args) > 1 && os.Args[1] == "update" {
		if err := checkUpdate(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	loadTheme()
	initStyles()

	repoPath := "."
	if len(os.Args) > 1 {
		repoPath = os.Args[1]
	}

	log.Printf("Opening repository: %s\n", repoPath)

	// Set terminal title (works on Windows 10+, macOS, Linux)
	setTerminalTitle(appName)
	defer resetTerminalTitle()

	p := tea.NewProgram(
		initialModel(repoPath),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		log.Printf("Program error: %v\n", err)
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	log.Println(appName + " exited normally")
}
