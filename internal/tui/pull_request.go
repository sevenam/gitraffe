package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
)

// pullRequestFoundMsg is what was found out about a commit's pull request.
type pullRequestFoundMsg struct {
	repoPath string // the repository asked; see the handler
	remote   string
	number   int // 0 when the commit names no pull request
	// What starting one would take, looked into only when there is none.
	start git.NewPullRequest
}

// openPullRequest looks for the pull request the commit on screen came from,
// and failing that for the branch one could be started from. Finding either
// means asking the history — which merge brought the commit in, whether the
// default branch already holds it — so it runs in the background and
// finishPullRequest opens the page.
func (m model) openPullRequest() (model, tea.Cmd) {
	c, ok := m.commitOnScreen()
	if !ok {
		return m, nil
	}
	if c.WorkingTree {
		m.notice = "These are your uncommitted changes — pick a commit to open its pull request"
		return m, nil
	}
	return m, findPullRequestCmd(m.repoPath, c, m.currentBranch)
}

func findPullRequestCmd(repoPath string, c commit, currentBranch string) tea.Cmd {
	return func() tea.Msg {
		// The remote comes first because it says how to read a subject:
		// GitHub and Azure DevOps each write a merge their own way.
		remote := git.BrowserRemote(repoPath)
		msg := pullRequestFoundMsg{
			repoPath: repoPath,
			remote:   remote,
			number:   git.PullRequestFor(repoPath, c.FullHash, c.Message, remote),
		}
		if msg.number == 0 {
			msg.start = git.NewPullRequestFor(repoPath, c.FullHash, c.Refs, currentBranch)
		}
		return msg
	}
}

// openBrowser is openURL, except in tests, which must not open one.
var openBrowser = openURL

// finishPullRequest opens the page, or says why it cannot rather than
// appearing to do nothing: there are several ordinary reasons a commit has no
// page to open.
func (m model) finishPullRequest(msg pullRequestFoundMsg) model {
	if msg.number == 0 {
		return m.startPullRequest(msg)
	}
	if msg.remote == "" {
		m.notice = "No remote to build a pull request address from"
		return m
	}
	address := git.PullRequestURL(msg.remote, msg.number)
	if address == "" {
		m.notice = "The remote " + msg.remote + " has no page on the web"
		return m
	}
	if err := openBrowser(address); err != nil {
		m.notice = "Could not open a browser: " + err.Error()
		return m
	}
	m.notice = fmt.Sprintf("Opening pull request #%d in your browser", msg.number)
	return m
}

// startPullRequest opens the page that starts a pull request from the
// commit's branch, for a commit that came from none. Only a branch the remote
// has can start one, and a commit with no such branch says which of the
// ordinary reasons applies.
func (m model) startPullRequest(msg pullRequestFoundMsg) model {
	switch {
	case msg.start.Unpushed != "":
		m.notice = "No pull request on this commit, and " + msg.start.Unpushed +
			" is not on the remote yet — push it (P) to open one"
		return m
	case msg.start.Branch == "":
		m.notice = "No pull request on this commit, and no branch on it to open one from"
		return m
	}
	address := git.NewPullRequestURL(msg.remote, msg.start.Branch)
	if address == "" {
		m.notice = "The remote " + msg.remote + " has no page on the web"
		return m
	}
	if err := openBrowser(address); err != nil {
		m.notice = "Could not open a browser: " + err.Error()
		return m
	}
	m.notice = "Opening a new pull request for " + msg.start.Branch + " in your browser"
	return m
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
