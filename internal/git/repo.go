package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v5"
)

// Repository is a repository opened with go-git. Only Open and ReadInfo use
// it; everything else goes through the git command line.
type Repository = gogit.Repository

// Open opens the repository at path with go-git. A failure is not final: the
// command line understands layouts go-git does not, so callers fall back to it.
func Open(path string) (*Repository, error) {
	return gogit.PlainOpen(path)
}

// Info is what the repository box shows about the open repository.
type Info struct {
	Name   string
	Branch string
	Commit string // short hash of HEAD
	// HeadHash is HEAD's full hash, for finding its row in the graph: a
	// short hash can be shared by two commits in a large history. "" when
	// there is no HEAD to read, as in a repository with no commits.
	HeadHash string
}

// repoName is the last element of the repository's path, or of the working
// directory when the path is ".".
func repoName(path string) string {
	if path == "." {
		if wd, err := os.Getwd(); err == nil {
			return wd[strings.LastIndex(wd, string(os.PathSeparator))+1:]
		}
		return path
	}
	return path[strings.LastIndex(path, string(os.PathSeparator))+1:]
}

// ReadInfo reads the branch and commit HEAD is on from an opened repository,
// or from the command line when repo is nil.
func ReadInfo(path string, repo *Repository) Info {
	if repo == nil {
		return ReadInfoCLI(path)
	}
	info := Info{Name: repoName(path)}
	if ref, err := repo.Head(); err == nil {
		if ref.Name().IsBranch() {
			info.Branch = ref.Name().Short()
		} else {
			info.Branch = "HEAD (detached)"
		}
		info.HeadHash = ref.Hash().String()
		info.Commit = info.HeadHash[:7]
	}
	return info
}

// ReadInfoCLI is ReadInfo by way of the git command line.
func ReadInfoCLI(path string) Info {
	info := Info{Name: repoName(path), Branch: "unknown", Commit: "unknown"}

	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = path
	if out, err := cmd.Output(); err == nil {
		info.Branch = strings.TrimSpace(string(out))
	}

	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = path
	if out, err := cmd.Output(); err == nil {
		if full := strings.TrimSpace(string(out)); len(full) >= 7 {
			info.HeadHash, info.Commit = full, full[:7]
		}
	}
	return info
}

// Toplevel is the root of the working tree dir is in.
func Toplevel(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// Git prints forward slashes even on Windows.
	return filepath.Clean(filepath.FromSlash(strings.TrimSpace(string(out)))), nil
}

// IsRepoRoot reports whether dir is the root of a git repository. .git is a
// file rather than a folder in a linked worktree or a submodule.
func IsRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// Remotes lists the repository's remotes by name, or nothing when it has none
// or they could not be read.
func Remotes(dir string) []string {
	out, err := Run(dir, "remote")
	if err != nil {
		return nil
	}
	return strings.Fields(out)
}
