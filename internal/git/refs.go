package git

import (
	"bufio"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// ParseRefs splits the raw --pretty=%D string into local branches,
// remote-tracking branches and tags, each stripped of its ref prefix for display.
//
// It expects the full ref paths produced by --decorate=full. Git's short names
// are ambiguous: "feature/foo" is both a plausible local branch and a plausible
// branch "foo" on a remote named "feature", and only the full path
// (refs/heads/... versus refs/remotes/...) settles it.
func ParseRefs(refs string) (local, remote, tags []string) {
	if refs == "" {
		return
	}
	for _, ref := range strings.Split(refs, ", ") {
		ref = strings.TrimSpace(ref)
		ref = strings.TrimPrefix(ref, "HEAD -> ")
		if ref == "" || ref == "HEAD" {
			continue
		}
		ref = strings.TrimPrefix(ref, "tag: ")

		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			local = append(local, strings.TrimPrefix(ref, "refs/heads/"))

		case strings.HasPrefix(ref, "refs/remotes/"):
			name := strings.TrimPrefix(ref, "refs/remotes/")
			// origin/HEAD is a symbolic pointer to the remote's default branch,
			// so it always duplicates a branch already listed beside it.
			if strings.HasSuffix(name, "/HEAD") {
				continue
			}
			remote = append(remote, name)

		case strings.HasPrefix(ref, "refs/tags/"):
			tags = append(tags, strings.TrimPrefix(ref, "refs/tags/"))

		default:
			// Anything else under refs/ (notes, stash, fetched PR refs). Show it
			// rather than dropping it silently.
			local = append(local, strings.TrimPrefix(ref, "refs/"))
		}
	}
	return
}

// mergedBranchName recovers the branch name recorded in a merge commit's subject.
//
// Git stores no branch name on a commit — a branch is just a ref, and deleting it
// erases the only pointer. The auto-generated merge subject is therefore the one
// place the name of a merged-then-deleted branch survives in the repository, and
// it exists only for real merge commits: squash and rebase merges leave nothing
// behind to recover.
func mergedBranchName(subject string) string {
	// GitHub: "Merge pull request #33 from owner/branch-name"
	if strings.HasPrefix(subject, "Merge pull request ") {
		_, after, ok := strings.Cut(subject, " from ")
		if !ok {
			return ""
		}
		fields := strings.Fields(after)
		if len(fields) == 0 {
			return ""
		}
		// Strip the owner (or fork owner) prefix; the remainder is the branch,
		// which may itself contain slashes.
		if _, branch, ok := strings.Cut(fields[0], "/"); ok {
			return branch
		}
		return fields[0]
	}

	// git: "Merge branch 'foo'", "Merge branch 'foo' into bar",
	// "Merge remote-tracking branch 'origin/foo'"
	if strings.HasPrefix(subject, "Merge branch ") || strings.HasPrefix(subject, "Merge remote-tracking branch ") {
		if _, after, ok := strings.Cut(subject, "'"); ok {
			if name, _, ok := strings.Cut(after, "'"); ok {
				return name
			}
		}
	}

	return ""
}

// labelMergedBranches tags each merge commit's second parent — the tip of the
// branch that was merged — with that branch's name.
func labelMergedBranches(commits []Commit) {
	byHash := make(map[string]int, len(commits))
	for i, c := range commits {
		byHash[c.Hash] = i
	}

	for _, c := range commits {
		// The second parent is the merged branch's tip; the first is the branch
		// that absorbed it.
		if len(c.Parents) < 2 {
			continue
		}
		name := mergedBranchName(c.Message)
		if name == "" {
			continue
		}
		idx, ok := byHash[c.Parents[1]]
		if !ok {
			continue
		}
		// A branch that still exists — locally or on a remote — already labels
		// itself via its ref.
		if local, remote, _ := ParseRefs(commits[idx].Refs); len(local) > 0 || len(remote) > 0 {
			continue
		}
		commits[idx].MergedBranch = name
	}
}

// RefKind sorts the list: the branches you work on first, then tags, then the
// remote-tracking copies, which are the ones you least often mean.
type RefKind int

const (
	RefLocal RefKind = iota
	RefTag
	RefRemote
)

func (k RefKind) String() string {
	switch k {
	case RefTag:
		return "tag"
	case RefRemote:
		return "remote"
	}
	return "branch"
}

// Ref is one branch or tag.
type Ref struct {
	Name string
	Kind RefKind
	// Commit is the commit the ref resolves to. For an annotated tag that is
	// the commit it points at, not the tag object, since the tag object is not
	// in the graph and jumping to it would find nothing.
	Commit  string
	When    int64 // creator date, for ordering tags newest first
	Current bool  // the branch HEAD is on
}

// ListRefs reads every branch and tag from git rather than from the
// commits on screen. A ref older than the history read so far has none of its
// commits loaded, and leaving it out of the list would hide exactly the ref
// that is hardest to reach by scrolling.
func ListRefs(dir string) ([]Ref, error) {
	// %(*objectname) is the commit an annotated tag points at, and empty for
	// everything else — a tag object is not in the graph, so the tag's own
	// hash would match no row.
	out, err := Run(dir, "for-each-ref",
		"--format=%(refname)%00%(objectname)%00%(*objectname)%00%(creatordate:unix)%00%(HEAD)",
		"refs/heads", "refs/tags", "refs/remotes")
	if err != nil {
		return nil, err
	}

	var choices []Ref
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\x00")
		if len(parts) < 4 {
			continue
		}
		name, object, peeled, when := parts[0], parts[1], parts[2], parts[3]

		c := Ref{Commit: object}
		if peeled != "" {
			c.Commit = peeled
		}
		c.When, _ = strconv.ParseInt(when, 10, 64)
		c.Current = len(parts) > 4 && strings.TrimSpace(parts[4]) == "*"

		switch {
		case strings.HasPrefix(name, "refs/heads/"):
			c.Name, c.Kind = strings.TrimPrefix(name, "refs/heads/"), RefLocal
		case strings.HasPrefix(name, "refs/tags/"):
			c.Name, c.Kind = strings.TrimPrefix(name, "refs/tags/"), RefTag
		case strings.HasPrefix(name, "refs/remotes/"):
			c.Name, c.Kind = strings.TrimPrefix(name, "refs/remotes/"), RefRemote
			// origin/HEAD only repeats the remote's default branch, which is
			// listed beside it.
			if strings.HasSuffix(c.Name, "/HEAD") {
				continue
			}
		default:
			continue
		}
		choices = append(choices, c)
	}

	sort.SliceStable(choices, func(i, j int) bool {
		a, b := choices[i], choices[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Kind == RefTag {
			// Newest first: a release you want is far likelier to be a recent
			// one, and tag names sort in no useful order anyway (v1.10 before
			// v1.9).
			return a.When > b.When
		}
		if a.Current != b.Current {
			return a.Current
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return choices, nil
}

// CommitDepth is how many commits git lists before this one, or -1 when it
// lists it not at all.
//
// The order has to be the graph's own: git log --graph implies --topo-order,
// which puts commits in a different place than the date order git otherwise
// uses, and a depth counted in the wrong order would read too little history.
func CommitDepth(dir, hash string) int {
	cmd := exec.Command("git", "log", "--all", "--topo-order", "--format=%H")
	cmd.Dir = dir
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
