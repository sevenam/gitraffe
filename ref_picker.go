package main

import (
	"bufio"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// refKind sorts the list: the branches you work on first, then tags, then the
// remote-tracking copies, which are the ones you least often mean.
type refKind int

const (
	refLocal refKind = iota
	refTag
	refRemote
)

func (k refKind) String() string {
	switch k {
	case refTag:
		return "tag"
	case refRemote:
		return "remote"
	}
	return "branch"
}

// refChoice is one branch or tag the picker offers.
type refChoice struct {
	name string
	kind refKind
	// commit is the commit the ref resolves to. For an annotated tag that is
	// the commit it points at, not the tag object, since the tag object is not
	// in the graph and jumping to it would find nothing.
	commit  string
	when    int64 // creator date, for ordering tags newest first
	current bool  // the branch HEAD is on
}

// refPicker is the list opened with "b": every branch and tag, narrowed by
// typing, to move the selection to a ref's commit.
//
// Scrolling to a commit works while the history is short enough to scroll.
// Past that the graph is a haystack, and a ref is the name of the needle.
type refPicker struct {
	open    bool
	choices []refChoice // every ref, in the order they were read
	shown   []int       // indexes into choices that match the filter
	cursor  int         // an index into shown
	filter  string
	err     string // why the refs could not be read; "" when they could
}

func (m model) openRefPicker() model {
	choices, err := m.loadRefChoices()
	p := refPicker{open: true, choices: choices}
	if err != nil {
		p.err = err.Error()
	}
	p.refresh()
	// Start on the branch you are on: it is the likeliest thing to come back
	// to, and it says where the list begins.
	for i, c := range p.shown {
		if choices[c].current {
			p.cursor = i
			break
		}
	}
	m.refs = p
	return m
}

// loadRefChoices reads every branch and tag from git rather than from the
// commits on screen. A ref older than the history read so far has none of its
// commits loaded, and leaving it out of the list would hide exactly the ref
// that is hardest to reach by scrolling.
func (m *model) loadRefChoices() ([]refChoice, error) {
	// %(*objectname) is the commit an annotated tag points at, and empty for
	// everything else — a tag object is not in the graph, so the tag's own
	// hash would match no row.
	out, err := m.git("for-each-ref",
		"--format=%(refname)%00%(objectname)%00%(*objectname)%00%(creatordate:unix)%00%(HEAD)",
		"refs/heads", "refs/tags", "refs/remotes")
	if err != nil {
		return nil, err
	}

	var choices []refChoice
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\x00")
		if len(parts) < 4 {
			continue
		}
		name, object, peeled, when := parts[0], parts[1], parts[2], parts[3]

		c := refChoice{commit: object}
		if peeled != "" {
			c.commit = peeled
		}
		c.when, _ = strconv.ParseInt(when, 10, 64)
		c.current = len(parts) > 4 && strings.TrimSpace(parts[4]) == "*"

		switch {
		case strings.HasPrefix(name, "refs/heads/"):
			c.name, c.kind = strings.TrimPrefix(name, "refs/heads/"), refLocal
		case strings.HasPrefix(name, "refs/tags/"):
			c.name, c.kind = strings.TrimPrefix(name, "refs/tags/"), refTag
		case strings.HasPrefix(name, "refs/remotes/"):
			c.name, c.kind = strings.TrimPrefix(name, "refs/remotes/"), refRemote
			// origin/HEAD only repeats the remote's default branch, which is
			// listed beside it.
			if strings.HasSuffix(c.name, "/HEAD") {
				continue
			}
		default:
			continue
		}
		choices = append(choices, c)
	}

	sort.SliceStable(choices, func(i, j int) bool {
		a, b := choices[i], choices[j]
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		if a.kind == refTag {
			// Newest first: a release you want is far likelier to be a recent
			// one, and tag names sort in no useful order anyway (v1.10 before
			// v1.9).
			return a.when > b.when
		}
		if a.current != b.current {
			return a.current
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})
	return choices, nil
}

// refresh rebuilds the visible list for the current filter, keeping the cursor
// in range. Matching ignores case and looks anywhere in the name, so "fix"
// finds "bugfix" as well as "fix-the-parser".
func (p *refPicker) refresh() {
	want := strings.ToLower(strings.TrimSpace(p.filter))
	p.shown = p.shown[:0]
	for i, c := range p.choices {
		if want == "" || strings.Contains(strings.ToLower(c.name), want) {
			p.shown = append(p.shown, i)
		}
	}
	p.cursor = max(0, min(p.cursor, len(p.shown)-1))
}

// selected is the ref under the cursor.
func (p refPicker) selected() (refChoice, bool) {
	if p.cursor < 0 || p.cursor >= len(p.shown) {
		return refChoice{}, false
	}
	return p.choices[p.shown[p.cursor]], true
}

// updateRefPicker handles keys while the list is open. As in the repository
// box, every printable key is text for the filter, so the arrows move and esc
// is the way out.
func (m model) updateRefPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.refs
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.refs = refPicker{}
		return m, nil
	case "up", "ctrl+p":
		p.cursor = max(0, p.cursor-1)
		return m, nil
	case "down", "ctrl+n":
		p.cursor = min(len(p.shown)-1, p.cursor+1)
		return m, nil
	case "pgup":
		p.cursor = max(0, p.cursor-10)
		return m, nil
	case "pgdown":
		p.cursor = min(len(p.shown)-1, p.cursor+10)
		return m, nil
	case "enter":
		c, ok := p.selected()
		if !ok {
			return m, nil
		}
		m.refs = refPicker{}
		return m.jumpToRef(c)
	case "backspace":
		if runes := []rune(p.filter); len(runes) > 0 {
			p.filter = string(runes[:len(runes)-1])
			p.refresh()
		}
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		p.filter += string(msg.Runes)
		p.refresh()
	}
	return m, nil
}

// jumpToRef moves the selection to the ref's commit, reading more history
// first when the commit is older than everything loaded so far.
func (m model) jumpToRef(c refChoice) (model, tea.Cmd) {
	for i, commit := range m.commits {
		if commit.FullHash == c.commit {
			m.selected = i
			m.detailsScroll = 0
			m.notice = c.kind.String() + " " + c.name + " — " + shortHash(c.commit)
			return m, m.maybeLoadDiff()
		}
	}

	// Not on screen: the ref is older than the batch read so far. Its place in
	// the log says how deep to read to reach it.
	depth := m.commitDepth(c.commit)
	if depth < 0 {
		m.notice = c.name + " is not in this repository's history"
		return m, nil
	}
	m.commitLimit = depth + 1
	next, cmd := m.reloadRepo()
	// reloadRepo keeps your place; here the point is to go somewhere else.
	next.reselect = c.commit
	next.notice = "Reading " + thousands(m.commitLimit) + " commits to reach " + c.name + "..."
	return next, cmd
}

// commitDepth is how many commits git lists before this one, or -1 when it
// lists it not at all.
//
// The order has to be the graph's own: git log --graph implies --topo-order,
// which puts commits in a different place than the date order git otherwise
// uses, and a depth counted in the wrong order would read too little history.
func (m *model) commitDepth(hash string) int {
	cmd := exec.Command("git", "log", "--all", "--topo-order", "--format=%H")
	cmd.Dir = m.repoPath
	out, err := cmd.StdoutPipe()
	if err != nil {
		return -1
	}
	if err := cmd.Start(); err != nil {
		return -1
	}
	// Killed as soon as the answer is known: the rest of a long history is
	// output nobody is waiting for.
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	depth := 0
	scan := bufio.NewScanner(out)
	for scan.Scan() {
		if strings.TrimSpace(scan.Text()) == hash {
			return depth
		}
		depth++
	}
	return -1
}

// shortHash is the seven characters git and the graph both show.
func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// render draws the list as a bordered box, scrolling to keep the cursor in
// view. maxRows caps how many refs are listed at once.
func (p refPicker) render(maxRows int) string {
	nameWidth := 0
	for _, i := range p.shown {
		nameWidth = max(nameWidth, ansi.StringWidth(p.choices[i].name))
	}
	nameWidth = min(max(nameWidth, 12), 48)

	footer := "type to filter • ↑/↓: choose • enter: jump • esc: cancel"
	contentWidth := max(nameWidth+2+7+2+len("branch"), ansi.StringWidth(footer))

	selected := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(currentTheme.SelectedFg)).
		Background(lipgloss.Color(currentTheme.SelectedBg))

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Branches and tags"))
	sb.WriteString("\n")
	sb.WriteString(p.filterLine(contentWidth))
	sb.WriteString("\n")

	maxRows = max(1, maxRows)
	start := 0
	if len(p.shown) > maxRows {
		start = max(0, min(p.cursor-maxRows/2, len(p.shown)-maxRows))
	}
	end := min(len(p.shown), start+maxRows)

	for i := start; i < end; i++ {
		c := p.choices[p.shown[i]]
		name := ansi.Truncate(c.name, nameWidth, "…")
		pad := strings.Repeat(" ", max(0, nameWidth-ansi.StringWidth(name)))
		sb.WriteString("\n")
		if i == p.cursor {
			row := "> " + name + pad + "  " + shortHash(c.commit) + "  " + c.kind.String()
			sb.WriteString(selected.Render(row + strings.Repeat(" ", max(0, contentWidth-ansi.StringWidth(row)))))
			continue
		}
		sb.WriteString("  " + refNameStyle(c).Render(name) + pad + "  " +
			commitHashStyle.Render(shortHash(c.commit)) + "  " + helpStyle.Render(c.kind.String()))
	}

	sb.WriteString("\n\n")
	if p.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.Error))
		sb.WriteString(errStyle.Render(ansi.Truncate("Can't read the refs: "+p.err, contentWidth, "…")))
		sb.WriteString("\n")
	}
	sb.WriteString(helpStyle.Render(footer))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(currentTheme.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}

// filterLine is what has been typed, or what to do when nothing has.
func (p refPicker) filterLine(width int) string {
	if p.filter == "" {
		if len(p.choices) == 0 {
			return helpStyle.Render("No branches or tags")
		}
		return helpStyle.Render(plural(len(p.choices), "ref") + " — type to narrow")
	}
	line := "/" + p.filter
	if len(p.shown) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.Error)).
			Render(ansi.Truncate(line+" — nothing matches", width, "…"))
	}
	return ansi.Truncate(line+helpStyle.Render("  "+plural(len(p.shown), "match")), width, "…")
}

// plural counts a thing in words: "1 ref", "4 refs".
func plural(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return strconv.Itoa(n) + " " + thing + "s"
}

// refNameStyle colours a ref the way the graph colours it, so a branch looks
// like a branch in both places.
func refNameStyle(c refChoice) lipgloss.Style {
	switch c.kind {
	case refTag:
		return tagStyle
	case refRemote:
		return remoteBranchStyle
	}
	return localBranchStyle
}
