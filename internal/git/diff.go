package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FileDiff is one file's part of a commit, split out of the diff git already
// gave us. The commit view lists these and shows one of them at a time.
type FileDiff struct {
	// Path is what to call the file: the new path, or "old → new" for a
	// rename, since the commit view has no room to show both lines.
	Path string
	// Added and Removed count the +/- lines. They come from the diff itself
	// rather than from --numstat: a second git call is a second source of
	// truth, and a file list that disagreed with the diff beside it would be
	// worse than no counts at all.
	Added, Removed int
	// Binary marks a file git described but could not diff; Body is then the
	// line git wrote instead.
	Binary bool
	// Untracked marks a working-tree file git has never seen. It has no diff,
	// so Body is the file's contents, every line written as an addition.
	Untracked bool
	Body      string // the file's hunks, without the "diff --git" header
}

// MaxFileDiffLines caps one file's diff. The whole-commit cap this replaces
// meant a commit touching many files showed the first few and silently hid the
// rest; per file, a long file costs only its own tail.
const MaxFileDiffLines = 800

// parseFileDiffs splits "git show -p" output into one entry per file.
//
// It reads git's own headers rather than the --stat summary: the summary is a
// second rendering of the same commit, and a list that disagreed with the diff
// it sits beside would send you to the wrong file. Anything before the first
// "diff --git" is preamble and belongs to no file.
func parseFileDiffs(body string) []FileDiff {
	if strings.TrimSpace(body) == "" {
		return nil
	}

	var files []FileDiff
	var current *FileDiff
	var hunks []string

	flush := func() {
		if current == nil {
			return
		}
		if len(hunks) > MaxFileDiffLines {
			hunks = append(hunks[:MaxFileDiffLines], "... (truncated)")
		}
		current.Body = strings.Join(hunks, "\n")
		files = append(files, *current)
		current, hunks = nil, nil
	}

	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			current = &FileDiff{Path: pathFromDiffHeader(line)}
		case current == nil:
			// Preamble: git wrote it about the commit, not about a file.
		case strings.HasPrefix(line, "Binary files "):
			current.Binary = true
			hunks = append(hunks, line)
		case strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, "--- "):
			// The file names again, which the header already gave us.
		case strings.HasPrefix(line, "index "), strings.HasPrefix(line, "old mode "),
			strings.HasPrefix(line, "new mode "), strings.HasPrefix(line, "new file mode "),
			strings.HasPrefix(line, "deleted file mode "), strings.HasPrefix(line, "similarity index "),
			strings.HasPrefix(line, "rename from "), strings.HasPrefix(line, "copy from "),
			strings.HasPrefix(line, "copy to "):
			// Metadata git prints between the header and the hunks.
		default:
			if strings.HasPrefix(line, "+") {
				current.Added++
			} else if strings.HasPrefix(line, "-") {
				current.Removed++
			}
			hunks = append(hunks, line)
		}
	}
	flush()
	return files
}

// pathFromDiffHeader names the file in `diff --git a/x b/y`: x when both halves
// agree, "x → y" when they don't, which is how a rename shows up. Both ends
// come from this one line rather than from the "rename to" line further down,
// so a file is named the same whether or not git chose to call it a rename.
//
// Paths are split on " b/" rather than on whitespace, since a path may contain
// spaces. git quotes a path with stranger characters still; there is no
// reading that, so the header is shown as git wrote it.
func pathFromDiffHeader(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	if strings.HasPrefix(rest, "\"") {
		return rest
	}
	i := strings.Index(rest, " b/")
	if i < 0 {
		return strings.TrimPrefix(rest, "a/")
	}
	from := strings.TrimPrefix(rest[:i], "a/")
	to := rest[i+len(" b/"):]
	if from == to {
		return to
	}
	return from + " → " + to
}

// Diff is one commit's changes, or the working tree's, as the details panel
// and the commit view want them.
type Diff struct {
	Stat string // the --stat summary
	Body string // the patch, cut to a length the details panel can hold
	// Files is the same patch split per file. It is parsed before Body is cut,
	// so the commit view can list files the panel's text no longer reaches.
	Files []FileDiff
}

// maxDiffLines caps the patch shown in the details panel.
const maxDiffLines = 300

// ShowCommit reads what a commit changed. statWidth is the width the --stat
// summary is drawn to.
func ShowCommit(dir, hash string, statWidth int) Diff {
	var d Diff

	cmd := exec.Command("git", "show", "--format=", fmt.Sprintf("--stat=%d", statWidth), "--no-color", hash)
	cmd.Dir = dir
	if out, err := cmd.Output(); err == nil {
		d.Stat = strings.TrimSpace(strings.ReplaceAll(string(out), "\r", ""))
	}

	cmd = exec.Command("git", "show", "--format=", "--no-color", "-p", hash)
	cmd.Dir = dir
	if out, err := cmd.Output(); err == nil {
		diff := strings.ReplaceAll(string(out), "\r", "")
		// Split before the cut below: the details panel shows as much as
		// it can hold, but the commit view has a list to fill, and a file
		// it never named could not be opened.
		d.Files = parseFileDiffs(diff)
		diffLines := strings.Split(diff, "\n")
		if len(diffLines) > maxDiffLines {
			diffLines = diffLines[:maxDiffLines]
			diffLines = append(diffLines, "... (truncated)")
		}
		d.Body = strings.TrimSpace(strings.Join(diffLines, "\n"))
	}
	return d
}

// WorkingTree reads the uncommitted changes. "git diff HEAD" covers staged and
// unstaged together, matching Status's count; untracked files have no diff to
// show and are listed instead.
func WorkingTree(dir string, statWidth int) Diff {
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(strings.ReplaceAll(string(out), "\r", ""))
	}

	stat := run("diff", "HEAD", "--stat="+fmt.Sprint(statWidth), "--no-color")
	body := run("diff", "HEAD", "--no-color")
	files := parseFileDiffs(body)
	if lines := strings.Split(body, "\n"); len(lines) > maxDiffLines {
		body = strings.Join(append(lines[:maxDiffLines], "... (truncated)"), "\n")
	}

	if untracked := run("ls-files", "--others", "--exclude-standard"); untracked != "" {
		var sb strings.Builder
		sb.WriteString("Untracked files:\n")
		for _, f := range strings.Split(untracked, "\n") {
			sb.WriteString("  " + f + "\n")
			files = append(files, untrackedFile(dir, f))
		}
		if body != "" {
			sb.WriteString("\n")
		}
		body = sb.String() + body
	}

	return Diff{Stat: stat, Body: body, Files: files}
}

// untrackedFile lists a file git has never seen, with its contents as the
// body. There is no diff to ask git for, but the file itself is what you want
// to read before adding it, so it is shown whole rather than only named.
//
// Every line is written as an addition, which is what adding the file will
// make it, so it reads the same as new lines in a tracked file. It is read
// from disk rather than through "git diff --no-index", which exits 1 whenever
// there is a difference and so cannot tell a new file from a failed call.
func untrackedFile(repoPath, path string) FileDiff {
	f := FileDiff{Path: path, Untracked: true}
	data, err := os.ReadFile(filepath.Join(repoPath, filepath.FromSlash(path)))
	switch {
	case err != nil:
		f.Body = "Untracked — this file is not in git yet, and could not be read."
	// git's own test for binary: a NUL byte in the first 8000.
	case bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0:
		f.Binary = true
		f.Body = "Untracked binary file, not shown."
	case len(data) == 0:
		// Left empty: the commit view says so in its own words.
	default:
		text := strings.TrimSuffix(strings.ReplaceAll(string(data), "\r", ""), "\n")
		lines := strings.Split(text, "\n")
		f.Added = len(lines)
		for i, line := range lines {
			lines[i] = "+" + line
		}
		if len(lines) > MaxFileDiffLines {
			lines = append(lines[:MaxFileDiffLines], "... (truncated)")
		}
		f.Body = strings.Join(lines, "\n")
	}
	return f
}

// Status counts what is uncommitted: changed files, staged or not, and files
// git has never seen.
func Status(dir string) (changed, untracked int, err error) {
	out, err := Run(dir, "status", "--porcelain")
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.TrimSpace(line) == "":
		case strings.HasPrefix(line, "??"):
			untracked++
		default:
			changed++
		}
	}
	return changed, untracked, nil
}

// Patch is a commit's whole diff, uncut, for copying: the details panel's text
// stops at maxDiffLines, and a patch missing its tail would not apply. An empty
// hash means the uncommitted changes, as "git diff HEAD" sees them; untracked
// files are not in it, since git has no diff for a file it has never seen.
func Patch(dir, hash string) (string, error) {
	args := []string{"diff", "HEAD", "--no-color"}
	if hash != "" {
		args = []string{"show", "--format=", "--no-color", "-p", hash}
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// Not trimmed: git apply wants the final newline a patch ends with.
	return strings.ReplaceAll(string(out), "\r", ""), nil
}
