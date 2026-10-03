package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sevenam/gitraffe/internal/git"
)

// Gitraffe keeps itself up to date in two ways, both off the keyboard:
//
// Auto-refresh notices what changed on this machine: a commit made in another
// terminal, a checkout, an edit. It does not re-read the history on a timer,
// which on a large repository is a pause every half minute for nothing. It
// takes a fingerprint in the background (see git.Fingerprint) and reloads only
// when that differs from the one taken at the last load. A tick that finds
// nothing changed costs no redraw at all.
//
// Auto-fetch asks the remotes, and is off unless settings.yml turns it on:
// fetching reaches the network and writes to the repository, which is not
// something to start doing to someone unasked.
//
// Each has one timer for the life of the program, not one per repository. The
// next tick is asked for only when the last one's work is done, so a slow
// repository is checked less often rather than having checks pile up, and the
// handlers ask for it whichever model is current, so switching repository
// neither stops the timer nor starts a second.

const (
	defaultAutoRefresh = 30 * time.Second
	// Below these a hand-edited setting is raised rather than obeyed: a check
	// every second is a process storm, and a fetch every few is a way to get
	// rate-limited by one's own git host.
	minAutoRefresh = 2 * time.Second
	minAutoFetch   = 30 * time.Second
)

// interval reads a settings.yml value in seconds. Zero and below mean off.
func interval(seconds int, least time.Duration) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return max(time.Duration(seconds)*time.Second, least)
}

type (
	autoRefreshTickMsg struct{}
	autoFetchTickMsg   struct{}
)

// repoStateMsg is a fingerprint taken in the background, to compare with the
// one the screen was drawn from.
type repoStateMsg struct {
	repoPath    string
	fingerprint string // "" when it could not be read
	fromTick    bool   // the timer's own check, which starts the next tick
}

func (m model) autoRefreshTick() tea.Cmd {
	if m.autoRefresh <= 0 {
		return nil
	}
	return tea.Tick(m.autoRefresh, func(time.Time) tea.Msg { return autoRefreshTickMsg{} })
}

func (m model) autoFetchTick() tea.Cmd {
	if m.autoFetch <= 0 {
		return nil
	}
	return tea.Tick(m.autoFetch, func(time.Time) tea.Msg { return autoFetchTickMsg{} })
}

func checkRepoStateCmd(repoPath string, fromTick bool) tea.Cmd {
	return func() tea.Msg {
		fingerprint, _ := git.Fingerprint(repoPath)
		return repoStateMsg{repoPath: repoPath, fingerprint: fingerprint, fromTick: fromTick}
	}
}

// canReloadUnasked reports whether a reload nobody pressed a key for would go
// unnoticed. A reload starts from a fresh model, so anything open over the
// graph — a picker, the search prompt, the help — would close under the
// user's hands; and while something else is already loading, fetching or
// updating, it is that work's turn. Whatever is skipped here is found again by
// the next check, since the fingerprint still differs.
func (m model) canReloadUnasked() bool {
	return m.ready && m.err == nil &&
		!m.fetching && !m.pulling && m.updateState == updateIdle &&
		!m.showHelp && !m.picker.open && !m.switcher.open && !m.refs.open && !m.search.active &&
		!m.filterPrompt.active
}

func (m model) onAutoRefreshTick() (model, tea.Cmd) {
	if !m.canReloadUnasked() {
		return m, m.autoRefreshTick()
	}
	return m, checkRepoStateCmd(m.repoPath, true)
}

// onFocus checks at once when the terminal comes back to the front: having
// just committed in another window is exactly when the graph is out of date,
// and exactly when waiting out the rest of the interval would be noticed.
func (m model) onFocus() (model, tea.Cmd) {
	if m.autoRefresh <= 0 || !m.canReloadUnasked() {
		return m, nil
	}
	return m, checkRepoStateCmd(m.repoPath, false)
}

func (m model) onRepoState(msg repoStateMsg) (model, tea.Cmd) {
	var tick tea.Cmd
	if msg.fromTick {
		tick = m.autoRefreshTick()
	}
	// An unknown fingerprint on either side is no evidence of a change, and
	// reloading on it would reload on every tick.
	if msg.repoPath != m.repoPath || msg.fingerprint == "" || m.fingerprint == "" ||
		msg.fingerprint == m.fingerprint || !m.canReloadUnasked() {
		return m, tick
	}
	next, cmd := m.reloadUnasked(msg.fingerprint)
	return next, tea.Batch(cmd, tick)
}

// reloadUnasked is reloadRepo for a reload nobody asked for. It says nothing
// on the bottom line and leaves what is there alone, keeps the search that n
// and N continue, and does not ask the remotes for their tags again: that is a
// network call per remote, which a refresh every few seconds has no business
// making.
func (m model) reloadUnasked(fingerprint string) (model, tea.Cmd) {
	next, _ := m.reloadRepo()
	next.notice = m.notice
	next.updateMessage = m.updateMessage
	next.search = m.search
	return next, loadRepoKnowing(m.repoPath, fingerprint)
}

func (m model) onAutoFetchTick() (model, tea.Cmd) {
	if m.autoFetchStopped || m.autoFetching || !m.canReloadUnasked() {
		return m, m.autoFetchTick()
	}
	m.autoFetching = true
	return m, autoFetchCmd(m.repoPath)
}

// autoFetchCmd is fetchCmd for the timer. Having no remote is found out here
// rather than before starting, since asking is a git call of its own and this
// runs in the background.
func autoFetchCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		if len(git.Remotes(repoPath)) == 0 {
			return fetchFinishedMsg{repoPath: repoPath, auto: true, skipped: true}
		}
		stderr, err := git.Fetch(repoPath)
		return fetchFinishedMsg{repoPath: repoPath, auto: true, err: err, detail: firstLine(stderr)}
	}
}

// finishAutoFetch handles a fetch nobody is waiting on. When it worked, what
// it brought is shown the same way any other change is: by comparing
// fingerprints, so a fetch that found nothing redraws nothing. The tags are
// asked for again alongside, because a fetch is what changes them.
//
// When it failed, it is the last one tried. The usual reasons — no network, a
// key that needs unlocking, a remote that wants a password — are not going to
// fix themselves by the next tick, and each try can cost a prompt on a
// security key or a line in a server's log. Pressing f tries again, and
// succeeding starts the timer's fetches off again.
func (m model) finishAutoFetch(msg fetchFinishedMsg) (model, tea.Cmd) {
	m.autoFetching = false
	if msg.skipped {
		return m, nil
	}
	if msg.err != nil {
		m.autoFetchStopped = true
		m.notice = "Auto-fetch stopped: " + fetchFailure(msg) + " — press f to try again"
		return m, nil
	}
	return m, tea.Batch(checkRepoStateCmd(m.repoPath, false), loadRemoteTagsCmd(m.repoPath))
}
