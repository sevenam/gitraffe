package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
)

// openPullRequest opens the pull request the commit on screen came from. It
// says why when it cannot, rather than appearing to do nothing: there are
// several ordinary reasons a commit has no page to open.
func (m model) openPullRequest() (model, tea.Cmd) {
	c, ok := m.commitOnScreen()
	if !ok {
		return m, nil
	}
	number := git.PullRequestNumber(c.Message)
	if number == 0 {
		m.notice = "No pull request on this commit — its message doesn't name one"
		return m, nil
	}
	remote := git.BrowserRemote(m.repoPath)
	if remote == "" {
		m.notice = "No remote to build a pull request address from"
		return m, nil
	}
	address := git.PullRequestURL(remote, number)
	if address == "" {
		m.notice = "The remote " + remote + " has no page on the web"
		return m, nil
	}

	if err := openURL(address); err != nil {
		m.notice = "Could not open a browser: " + err.Error()
		return m, nil
	}
	m.notice = fmt.Sprintf("Opening pull request #%d in your browser", number)
	return m, nil
}

// openURL hands an address to whatever the desktop opens links with.
//
// The URL is passed as an argument rather than through a shell, so a remote or
// a commit subject cannot smuggle a command into it, and only http(s) is
// handed over at all: a remote is repository data, and file:// or anything
// stranger is not something to open on someone's behalf.
func openURL(address string) error {
	if !strings.HasPrefix(address, "https://") && !strings.HasPrefix(address, "http://") {
		return fmt.Errorf("refusing to open %q: not a web address", address)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Not "cmd /c start", which reads its argument as a command line and
		// treats & as a separator.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	case "darwin":
		cmd = exec.Command("open", address)
	default:
		cmd = exec.Command("xdg-open", address)
	}
	// Started, not run: the browser outlives gitraffe, and waiting for it
	// would hang the interface for as long as someone reads the page.
	return cmd.Start()
}
