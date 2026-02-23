package main

import "fmt"

// setTerminalTitle sets the terminal window title using ANSI escape sequence
// Works on Windows 10+, macOS, and Linux
func setTerminalTitle(title string) {
	fmt.Printf("\033]0;%s\007", title)
}

// resetTerminalTitle clears the terminal title back to default
func resetTerminalTitle() {
	fmt.Printf("\033]0;\007")
}
