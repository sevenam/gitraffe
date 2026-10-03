package git

import "strings"

// ListFiles names every file git tracks in the repository, relative to its
// top and with forward slashes, the form a Filter's Path takes. It is the list
// the file-history picker completes from.
//
// Tracked files rather than the working tree's: an ignored build output has
// no history to show. A file deleted since is not listed either, though its
// history is still there to be asked for by typing its path.
func ListFiles(dir string) ([]string, error) {
	// --full-name and the ":/" pathspec make the list the whole repository's,
	// from the top, even when gitraffe was started in a subdirectory. -z keeps
	// a name with a newline, or one git would otherwise quote, as it is.
	out, err := Run(dir, "ls-files", "-z", "--full-name", "--", ":/")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}
