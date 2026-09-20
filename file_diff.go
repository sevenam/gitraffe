package main

import (
	"strings"
)

// fileDiff is one file's part of a commit, split out of the diff git already
// gave us. The commit view lists these and shows one of them at a time.
type fileDiff struct {
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
	// so it is listed to be counted, not to be read.
	Untracked bool
	Body      string // the file's hunks, without the "diff --git" header
}

// maxFileDiffLines caps one file's diff. The whole-commit cap this replaces
// meant a commit touching many files showed the first few and silently hid the
// rest; per file, a long file costs only its own tail.
const maxFileDiffLines = 800

// parseFileDiffs splits "git show -p" output into one entry per file.
//
// It reads git's own headers rather than the --stat summary: the summary is a
// second rendering of the same commit, and a list that disagreed with the diff
// it sits beside would send you to the wrong file. Anything before the first
// "diff --git" is preamble and belongs to no file.
func parseFileDiffs(body string) []fileDiff {
	if strings.TrimSpace(body) == "" {
		return nil
	}

	var files []fileDiff
	var current *fileDiff
	var hunks []string

	flush := func() {
		if current == nil {
			return
		}
		if len(hunks) > maxFileDiffLines {
			hunks = append(hunks[:maxFileDiffLines], "... (truncated)")
		}
		current.Body = strings.Join(hunks, "\n")
		files = append(files, *current)
		current, hunks = nil, nil
	}

	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			current = &fileDiff{Path: pathFromDiffHeader(line)}
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
