package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	// maxRecentRepos caps the recent list. Past a screenful it stops being
	// quicker than typing the path.
	maxRecentRepos = 10
	// maxSuggestions caps how many folders one keystroke collects. A folder
	// with thousands of entries would otherwise slow typing, and past this
	// many, another letter narrows the list faster than scrolling does.
	maxSuggestions = 200
)

// repoSwitcher is the "open repository" box opened with "o": a path to type,
// and below it a list to pick from instead — the recent repositories while
// the path is empty, then the folders matching what has been typed.
type repoSwitcher struct {
	open    bool
	input   textinput.Model
	base    string   // what relative paths are relative to; see repoBase
	recents []string // most recent first, without the repository already open
	items   []switcherItem
	heading string // what the items are, e.g. "Recent" or "Folders in ~\git"
	empty   string // shown instead of items when there are none
	cursor  int    // highlighted item; -1 for none, when enter opens the typed path
	err     string // why the last path couldn't be opened
}

// switcherItem is one row of the switcher's list.
type switcherItem struct {
	label string // what the row shows
	path  string // absolute
	repo  bool   // a repository's root, so enter opens it rather than going into it
}

func (m model) openRepoSwitcher() model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = "type a path, or pick a recent repository"
	in.PlaceholderStyle = helpStyle
	// A blinking cursor needs a stream of tick messages for as long as the box
	// is open, all to redraw the one character.
	in.Cursor.SetMode(cursor.CursorStatic)
	in.Focus()

	var recents []string
	for _, p := range loadSettings(m.configDir).RecentRepos {
		// A repository deleted or moved since is left out rather than offered
		// only to fail.
		if !samePath(p, m.repoRoot) && dirExists(p) {
			recents = append(recents, p)
		}
	}
	m.switcher = repoSwitcher{open: true, input: in, base: m.repoBase(), recents: recents}
	m.switcher.refresh()
	return m
}

// updateRepoSwitcher handles keys while the switcher is open. Every printable
// key is text for the path, so unlike the other boxes only esc closes it.
func (m model) updateRepoSwitcher(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.switcher
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.switcher = repoSwitcher{}
		return m, nil
	case "up":
		if len(s.items) > 0 {
			s.cursor = max(0, s.cursor-1)
		}
		return m, nil
	case "down":
		if len(s.items) > 0 {
			s.cursor = min(len(s.items)-1, s.cursor+1)
		}
		return m, nil
	case "tab":
		s.complete()
		return m, nil
	case "enter":
		target := s.input.Value()
		if s.cursor >= 0 {
			item := s.items[s.cursor]
			// A folder that isn't a repository is somewhere on the way to one,
			// so enter goes into it, as a file browser would.
			if !item.repo {
				s.complete()
				return m, nil
			}
			target = item.path
		}
		if strings.TrimSpace(target) == "" {
			return m, nil
		}
		root, err := resolveRepo(target, s.base)
		if err != nil {
			s.err = err.Error()
			return m, nil
		}
		if samePath(root, m.repoRoot) {
			m.switcher = repoSwitcher{}
			m.notice = "Already showing " + displayPath(root)
			return m, nil
		}
		return m.switchRepo(root)
	}

	before := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	if s.input.Value() != before {
		s.err = ""
		s.refresh()
	}
	return m, cmd
}

// complete puts the highlighted item's path in the input, or the first
// match's when nothing is highlighted, ending in a separator so the list moves
// on to the folders inside it.
func (s *repoSwitcher) complete() {
	if len(s.items) == 0 {
		return
	}
	item := s.items[max(0, s.cursor)]
	value := item.path
	// Kept as typed, so "~\git\se" completes to "~\git\sevenam\" rather than
	// turning into an absolute path halfway through.
	if typed := strings.TrimSpace(s.input.Value()); typed != "" {
		dir, _ := splitPathInput(typed)
		value = dir + item.label
	}
	s.input.SetValue(value + string(os.PathSeparator))
	s.input.CursorEnd()
	s.err = ""
	s.refresh()
}

// refresh rebuilds the list for what is in the input: the recent repositories
// while it is empty, otherwise the folders in the directory typed so far whose
// names start with the part after the last separator. Case is ignored: the
// Windows file system ignores it too, and elsewhere it is rarely what someone
// typing a path means.
func (s *repoSwitcher) refresh() {
	s.cursor = -1
	s.items = nil
	typed := strings.TrimSpace(s.input.Value())
	if typed == "" {
		s.heading, s.empty = "Recent", "No recent repositories yet — type a path"
		for _, p := range s.recents {
			s.items = append(s.items, switcherItem{label: displayPath(p), path: p, repo: true})
		}
		return
	}

	dir, prefix := splitPathInput(typed)
	abs := expandPath(dir, s.base)
	entries, err := os.ReadDir(abs)
	if err != nil {
		s.heading, s.empty = "", "No folder at "+displayPath(abs)
		return
	}
	s.heading, s.empty = "Folders in "+displayPath(abs), "No folders here match"
	lower := strings.ToLower(prefix)
	for _, e := range entries {
		name := e.Name()
		// Hidden folders only once asked for: otherwise .git would be offered
		// inside every repository.
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(name), lower) {
			continue
		}
		full := filepath.Join(abs, name)
		// A symlink or junction reports its own type, not its target's.
		if !e.IsDir() && (e.Type()&os.ModeSymlink == 0 || !dirExists(full)) {
			continue
		}
		s.items = append(s.items, switcherItem{label: name, path: full, repo: isRepoRoot(full)})
		if len(s.items) == maxSuggestions {
			break
		}
	}
	sort.SliceStable(s.items, func(i, j int) bool {
		return strings.ToLower(s.items[i].label) < strings.ToLower(s.items[j].label)
	})
}

// splitPathInput splits a typed path into the directory to list, as typed,
// and the start of the name being typed in it.
func splitPathInput(typed string) (dir, prefix string) {
	typed = strings.TrimLeft(typed, "\"'")
	if typed == "~" {
		return typed + string(os.PathSeparator), ""
	}
	// To Windows a bare drive, "D:", means the current folder on that drive,
	// which isn't what anyone typing it into a path box means.
	if vol := filepath.VolumeName(typed); vol != "" && vol == typed {
		return typed + string(os.PathSeparator), ""
	}
	seps := "/"
	if runtime.GOOS == "windows" {
		seps = "/\\"
	}
	i := strings.LastIndexAny(typed, seps)
	if i < 0 {
		return "", typed
	}
	return typed[:i+1], typed[i+1:]
}

// expandPath resolves a typed path: quotes are dropped, since pasted Windows
// paths often have them, ~ is the home directory, and a relative
// path is relative to base.
func expandPath(p, base string) string {
	p = strings.Trim(strings.TrimSpace(p), "\"'")
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}

// isRepoRoot reports whether dir is the root of a git repository. .git is a
// file rather than a folder in a linked worktree or a submodule.
func isRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// switchRepo replaces the model with a fresh one for root and starts loading
// it. Starting fresh rather than clearing the old repository's fields means a
// field added later can't carry one repository's data into the next; only
// what belongs to the session rather than a repository is carried over.
func (m model) switchRepo(root string) (tea.Model, tea.Cmd) {
	next := initialModel(root)
	next.windowWidth, next.windowHeight = m.windowWidth, m.windowHeight
	next.latestVersion = m.latestVersion
	next.configDir = m.configDir
	next.colourLanes = m.colourLanes
	return next, tea.Batch(loadRepo(root), loadRemoteTagsCmd(root))
}

// repoBase is what a relative path in the switcher is relative to: the open
// repository, since that is where the user is looking, not wherever gitraffe
// happened to be started from.
func (m model) repoBase() string {
	if m.repoRoot != "" {
		return m.repoRoot
	}
	abs, err := filepath.Abs(m.repoPath)
	if err != nil {
		return m.repoPath
	}
	return abs
}

// rememberCurrentRepo records the open repository at the top of the recent
// list. Called once it has loaded, so only repositories that actually opened
// are offered again.
func (m *model) rememberCurrentRepo() {
	root, err := gitToplevel(m.repoPath)
	if err != nil {
		return
	}
	m.repoRoot = root
	if m.configDir == "" {
		return
	}
	s := loadSettings(m.configDir)
	recents := []string{root}
	for _, p := range s.RecentRepos {
		if !samePath(p, root) && len(recents) < maxRecentRepos {
			recents = append(recents, p)
		}
	}
	s.RecentRepos = recents
	if err := saveSettings(m.configDir, s); err != nil {
		// Only the recent list is lost; not worth interrupting anyone over.
		log.Printf("Recent repos: could not save: %v", err)
	}
}

// resolveRepo turns what was typed into the root of the repository it names.
// A folder inside a repository opens the whole repository, as git itself
// treats it.
func resolveRepo(input, base string) (string, error) {
	p := expandPath(input, base)
	if !dirExists(p) {
		return "", fmt.Errorf("no folder at %s", p)
	}
	root, err := gitToplevel(p)
	if err != nil {
		return "", fmt.Errorf("%s is not in a git repository", p)
	}
	return root, nil
}

// gitToplevel is the root of the working tree dir is in.
func gitToplevel(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// Git prints forward slashes even on Windows.
	return filepath.Clean(filepath.FromSlash(strings.TrimSpace(string(out)))), nil
}

// samePath compares two cleaned paths the way the file system does: Windows
// ignores case, so D:\Repo and d:\repo are one repository there.
func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// displayPath shortens the home directory to ~, which is most of what makes
// paths too long to show whole.
func displayPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if rel, err := filepath.Rel(home, p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return p
}

// fitLeft shortens s to width by cutting its start: the end of a path, the
// repository's own name, is the part that tells repositories apart.
func fitLeft(s string, width int) string {
	w := ansi.StringWidth(s)
	if w <= width || width < 2 {
		return s
	}
	return "…" + ansi.TruncateLeft(s, w-width+1, "")
}

// render draws the switcher as a bordered box fitting maxWidth × maxHeight.
// The list always takes the same number of rows, so the box doesn't jump
// about as typing changes how many folders match.
func (s repoSwitcher) render(maxWidth, maxHeight int) string {
	footer := "↑/↓: choose • tab: complete • enter: open • esc: cancel"
	contentWidth := max(ansi.StringWidth(footer), min(72, maxWidth-6)) // 6: border and padding
	// 12: the title, path, heading and footer lines, the blank lines between
	// them, padding and border.
	rows := max(3, min(10, maxHeight-12))

	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(currentTheme.SectionHeader))
	selected := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(currentTheme.SelectedFg)).
		Background(lipgloss.Color(currentTheme.SelectedBg))

	in := s.input
	in.Width = contentWidth - 7 // "Path:  "

	var sb strings.Builder
	sb.WriteString(titleStyle.Padding(0).Render("Open repository"))
	sb.WriteString("\n\n")
	sb.WriteString(label.Render("Path:  "))
	sb.WriteString(in.View())
	sb.WriteString("\n\n")

	heading := s.heading
	if len(s.items) > rows {
		heading += fmt.Sprintf(" (%d)", len(s.items))
	}
	sb.WriteString(label.Render(fitLeft(heading, contentWidth)))

	start := 0
	if len(s.items) > rows {
		start = max(0, min(s.cursor-rows/2, len(s.items)-rows))
	}
	for r := range rows {
		sb.WriteString("\n")
		i := start + r
		if i >= len(s.items) {
			if r == 0 && len(s.items) == 0 {
				sb.WriteString(helpStyle.Render(fitLeft(s.empty, contentWidth)))
			}
			continue
		}
		item := s.items[i]
		// Only among folders: every recent entry is a repository.
		mark := ""
		if item.repo && s.heading != "Recent" {
			mark = "  repo"
		}
		name := fitLeft(item.label, contentWidth-2-len(mark))
		if i == s.cursor {
			row := "> " + name + mark
			sb.WriteString(selected.Render(row + strings.Repeat(" ", max(0, contentWidth-ansi.StringWidth(row)))))
		} else {
			sb.WriteString("  " + name + helpStyle.Render(mark))
		}
	}

	sb.WriteString("\n\n")
	if s.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(currentTheme.Error))
		sb.WriteString(errStyle.Render(fitLeft(s.err, contentWidth)))
		sb.WriteString("\n")
	}
	sb.WriteString(helpStyle.Render(footer))

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(currentTheme.BorderActive)).
		Padding(1, 2).
		Render(sb.String())
}
