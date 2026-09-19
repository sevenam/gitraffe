package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// workingTreeMarker is the graph glyph for uncommitted changes: hollow, since
// the change isn't committed yet, against the filled dot of a real commit.
const workingTreeMarker = "○"

// addWorkingTreeRow puts a row for uncommitted changes above the newest
// commit, so "what have I got in progress" is answered by the same screen that
// answers "where am I". Nothing is added when the working tree is clean.
//
// It runs after the graph has been read, and shifts the rows' commit indexes
// along, rather than being woven into the log output: git log has nothing to
// say about uncommitted work, and the lane colouring reads git's own output,
// which a made-up row would corrupt.
func (m *model) addWorkingTreeRow() {
	summary, ok := m.workingTreeSummary()
	if !ok {
		return
	}

	m.commits = append([]commit{{
		Message:     summary,
		WorkingTree: true,
		Date:        time.Now(),
	}}, m.commits...)
	for i := range m.displayRows {
		if m.displayRows[i].CommitIdx >= 0 {
			m.displayRows[i].CommitIdx++
		}
	}
	m.displayRows = append([]displayRow{m.workingTreeRow()}, m.displayRows...)
}

// workingTreeRow draws the marker in the column the topmost commit sits in,
// with every other lane of that row continued as a bar, so the row reads as
// sitting on top of the graph rather than floating beside it.
func (m *model) workingTreeRow() displayRow {
	row := displayRow{GraphChars: workingTreeMarker, CommitIdx: 0, GraphWidth: 1, Lanes: []int{0}}
	if len(m.displayRows) == 0 {
		return row
	}

	first := m.displayRows[0]
	runes := []rune(first.GraphChars)
	marker := -1
	for i, r := range runes {
		if r == '●' || r == '◉' {
			marker = i
			break
		}
	}
	if marker < 0 {
		return row
	}
	out := make([]rune, len(runes))
	for i, r := range runes {
		switch {
		case i == marker:
			out[i] = []rune(workingTreeMarker)[0]
		case r == ' ':
			out[i] = ' '
		default:
			// Diagonals belong to the row below; above it those lanes are
			// simply carrying on.
			out[i] = '│'
		}
	}
	row.GraphChars = string(out)
	row.GraphWidth = len(out)
	row.Lanes = append([]int(nil), first.Lanes...)
	return row
}

// workingTreeSummary counts what is uncommitted, as "3 changed, 1 untracked".
// Staged and unstaged changes are one number: both are work in progress, and
// the split is a detail for the details panel, not the graph.
func (m *model) workingTreeSummary() (string, bool) {
	out, err := m.git("status", "--porcelain")
	if err != nil {
		log.Printf("Working tree: status failed: %v", err)
		return "", false
	}
	changed, untracked := 0, 0
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.TrimSpace(line) == "":
		case strings.HasPrefix(line, "??"):
			untracked++
		default:
			changed++
		}
	}
	if changed == 0 && untracked == 0 {
		return "", false
	}

	var parts []string
	if changed > 0 {
		parts = append(parts, fmt.Sprintf("%d changed", changed))
	}
	if untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d untracked", untracked))
	}
	return strings.Join(parts, ", "), true
}

// loadWorkingDiffCmd reads the uncommitted changes for the details panel.
// "git diff HEAD" covers staged and unstaged together, matching the row's
// count; untracked files have no diff to show and are listed instead.
func loadWorkingDiffCmd(repoPath string, statWidth int) tea.Cmd {
	return func() tea.Msg {
		run := func(args ...string) string {
			cmd := exec.Command("git", args...)
			cmd.Dir = repoPath
			out, err := cmd.Output()
			if err != nil {
				return ""
			}
			return strings.TrimSpace(strings.ReplaceAll(string(out), "\r", ""))
		}

		stat := run("diff", "HEAD", "--stat="+fmt.Sprint(statWidth), "--no-color")
		body := run("diff", "HEAD", "--no-color")
		if lines := strings.Split(body, "\n"); len(lines) > 300 {
			body = strings.Join(append(lines[:300], "... (truncated)"), "\n")
		}

		if untracked := run("ls-files", "--others", "--exclude-standard"); untracked != "" {
			var sb strings.Builder
			sb.WriteString("Untracked files:\n")
			for _, f := range strings.Split(untracked, "\n") {
				sb.WriteString("  " + f + "\n")
			}
			if body != "" {
				sb.WriteString("\n")
			}
			body = sb.String() + body
		}

		return diffLoadedMsg{repoPath: repoPath, commitIdx: 0, diffStat: stat, diffBody: body}
	}
}
