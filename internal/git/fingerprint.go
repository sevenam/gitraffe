package git

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Fingerprint condenses everything a reload would read differently into one
// string: where HEAD is, every ref, and the uncommitted changes. Two calls that
// return the same string mean reading the repository again would draw the same
// screen, which is what lets it be watched on a timer without re-reading the
// whole history each time.
//
// A changed file counts by its size and modification time, not its contents:
// the status line for a file edited twice reads the same both times, and
// diffing every changed file on every check is the cost this exists to avoid.
//
// The three commands run side by side, so the whole thing takes as long as
// the slowest of them, which is the status.
func Fingerprint(dir string) (string, error) {
	type result struct {
		out []byte
		err error
	}
	run := func(args ...string) <-chan result {
		ch := make(chan result, 1)
		go func() {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			out, err := cmd.Output()
			ch <- result{out, err}
		}()
		return ch
	}

	rootCh := run("rev-parse", "--show-toplevel")
	refsCh := run("for-each-ref", "--format=%(objectname) %(refname)")
	// --no-optional-locks: a plain status refreshes the index and takes its
	// lock to do so, and a check running unasked in the background must not
	// make the git command someone is typing in another terminal fail on it.
	// --branch puts HEAD's name and commit in the same answer.
	statusCh := run("--no-optional-locks", "status", "--porcelain=v2", "--branch", "-z")

	root, refs, status := <-rootCh, <-refsCh, <-statusCh
	for _, r := range []result{root, refs, status} {
		if r.err != nil {
			return "", r.err
		}
	}

	h := sha256.New()
	h.Write(refs.out)
	h.Write([]byte{0})
	h.Write(status.out)

	top := filepath.FromSlash(strings.TrimSpace(string(root.out)))
	for _, path := range statusPaths(status.out) {
		info, err := os.Lstat(filepath.Join(top, filepath.FromSlash(path)))
		if err != nil {
			continue // deleted: the status entry already says so
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\x00", path, info.Size(), info.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// statusPaths lists the files named in "git status --porcelain=v2 -z" output.
// Each entry type keeps its path after a fixed number of space-separated
// fields, so that a path with spaces in it is still read whole.
func statusPaths(out []byte) []string {
	var paths []string
	entries := bytes.Split(out, []byte{0})
	for i := 0; i < len(entries); i++ {
		e := string(entries[i])
		fields := 0
		switch {
		case strings.HasPrefix(e, "1 "):
			fields = 9
		case strings.HasPrefix(e, "2 "):
			fields = 10
			i++ // a rename or copy is followed by the path it came from
		case strings.HasPrefix(e, "u "):
			fields = 11
		case strings.HasPrefix(e, "? "):
			fields = 2
		default:
			continue // a "# branch" header, an ignored file, or the final empty entry
		}
		if parts := strings.SplitN(e, " ", fields); len(parts) == fields {
			paths = append(paths, parts[fields-1])
		}
	}
	return paths
}
