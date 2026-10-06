package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// PushOutcome is how a push ended, when nothing went wrong.
type PushOutcome int

const (
	// PushSent: the remote branch was moved forward to the local one.
	PushSent PushOutcome = iota
	// PushCreated: the remote had no such branch or tag, and now has.
	PushCreated
	// PushUpToDate: the remote already had everything.
	PushUpToDate
	// PushBehind: the remote branch has commits the local one lacks, so
	// sending would take a forced push. Nothing was changed.
	PushBehind
	// PushTagTaken: the remote has a tag of that name on another commit.
	// Nothing was changed.
	PushTagTaken
)

// PushResult is what a push did.
type PushResult struct {
	Outcome PushOutcome
	Remote  string // the remote pushed to, e.g. "origin"
	Branch  string // the local branch pushed; "" for a tag
	// RemoteBranch is the branch's name on the remote, which is the local name
	// unless it was pushed under another.
	RemoteBranch string
	Tag          string // the tag pushed; "" for a branch
	// Commits is how many commits a PushSent moved the remote branch by.
	Commits int
	// Detail is what git or a hook wrote when the push failed.
	Detail string
}

// Upstream names the remote branch as the graph labels it: "origin/main".
func (r PushResult) Upstream() string {
	return r.Remote + "/" + r.RemoteBranch
}

// ErrBadBranchName is a name git would not accept for a branch.
var ErrBadBranchName = errors.New("not a valid branch name")

// PushPlan is what pushing the current branch would mean, read without asking
// any remote.
type PushPlan struct {
	Branch string // the branch HEAD is on; "" when detached
	// Remote and RemoteBranch are what the branch tracks, when it tracks a
	// branch that the remote, as last fetched, still has. With both empty a
	// push would have to create one.
	Remote, RemoteBranch string
	// Remotes are all the repository's remotes, and NewBranchRemote the one a
	// new branch or a tag would go to unless another is picked.
	Remotes         []string
	NewBranchRemote string
}

// Upstream names the tracked branch as the graph labels it, or "" for none.
func (p PushPlan) Upstream() string {
	if p.Remote == "" {
		return ""
	}
	return p.Remote + "/" + p.RemoteBranch
}

// ReadPushPlan works out where the current branch would be pushed.
//
// A branch whose remote branch has been deleted still tracks it on paper, in
// its configuration. It is read as tracking nothing: pushing would create the
// branch again, and creating a branch is asked about first.
func ReadPushPlan(dir string) PushPlan {
	p := PushPlan{Remotes: Remotes(dir)}
	if branch, err := Run(dir, "symbolic-ref", "-q", "--short", "HEAD"); err == nil {
		p.Branch = branch
	}
	if p.Branch != "" {
		if _, err := Run(dir, "rev-parse", "--verify", "--quiet", "@{upstream}"); err == nil {
			out, _ := Run(dir, "for-each-ref", "--format=%(upstream:remotename)%09%(upstream:remoteref)", "refs/heads/"+p.Branch)
			remote, ref, _ := strings.Cut(out, "\t")
			// "." is a branch that tracks another local branch, which is
			// nowhere to push to.
			if branch, ok := strings.CutPrefix(ref, "refs/heads/"); ok && contains(p.Remotes, remote) {
				p.Remote, p.RemoteBranch = remote, branch
			}
		}
	}
	p.NewBranchRemote = defaultPushRemote(dir, p.Branch, p.Remotes)
	return p
}

// defaultPushRemote picks the remote for something that has no remote of its
// own yet, the way git itself would: what the configuration names for pushes,
// then the remote the branch is fetched from, then origin.
func defaultPushRemote(dir, branch string, remotes []string) string {
	keys := []string{"remote.pushDefault"}
	if branch != "" {
		keys = []string{"branch." + branch + ".pushRemote", "remote.pushDefault", "branch." + branch + ".remote"}
	}
	for _, key := range keys {
		if name, err := Run(dir, "config", "--get", key); err == nil && contains(remotes, name) {
			return name
		}
	}
	if contains(remotes, "origin") {
		return "origin"
	}
	if len(remotes) > 0 {
		return remotes[0]
	}
	return ""
}

// ValidBranchName reports whether git would accept name for a branch.
func ValidBranchName(dir, name string) bool {
	if name == "" || strings.HasPrefix(name, "-") {
		return false
	}
	_, err := Run(dir, "check-ref-format", "--branch", name)
	return err == nil
}

// Push sends the current branch to the branch it tracks, and only ever by
// moving that branch forward.
//
// It is never forced. When the remote branch has commits the local one lacks,
// git refuses and changes nothing, and that is reported, not overcome:
// reconciling the two is a merge or a rebase, which is work for a terminal.
// The repository's pre-push hook runs and is never skipped.
//
// What is pushed is named in full, one branch to one branch, so that
// push.default and the like in someone's config cannot make it push anything
// else; for the same reason tags are not sent along with it.
func Push(dir string) (PushResult, error) {
	p := ReadPushPlan(dir)
	r := PushResult{Remote: p.Remote, Branch: p.Branch, RemoteBranch: p.RemoteBranch}
	switch {
	case p.Branch == "":
		return r, ErrDetached
	case p.Remote == "":
		return r, fmt.Errorf("%s tracks no remote branch", p.Branch)
	}
	return pushRef(dir, r, "refs/heads/"+p.Branch+":refs/heads/"+p.RemoteBranch, false)
}

// PushNewBranch sends the current branch to remote under name, and has the
// branch track it from then on. It is meant for a name the remote does not
// have; if it has, the push is the same fast-forward-or-nothing Push makes.
func PushNewBranch(dir, remote, name string) (PushResult, error) {
	r := PushResult{Remote: remote, RemoteBranch: name}
	branch, err := Run(dir, "symbolic-ref", "-q", "--short", "HEAD")
	if err != nil || branch == "" {
		return r, ErrDetached
	}
	r.Branch = branch
	if !ValidBranchName(dir, name) {
		return r, ErrBadBranchName
	}
	return pushRef(dir, r, "refs/heads/"+branch+":refs/heads/"+name, true)
}

// PushTag sends one tag to remote. A tag the remote has under the same name
// on another commit is left as it is there: moving it would be a forced push.
func PushTag(dir, remote, tag string) (PushResult, error) {
	r := PushResult{Remote: remote, Tag: tag}
	return pushRef(dir, r, "refs/tags/"+tag+":refs/tags/"+tag, false)
}

// pushRef pushes one refspec and reads what became of it from git's
// --porcelain report, which is made to be read by a program: one line per
// ref, starting with a flag for what happened to it.
func pushRef(dir string, r PushResult, refspec string, track bool) (PushResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	args := []string{"push", "--porcelain", "--no-follow-tags"}
	if track {
		args = append(args, "--set-upstream")
	}
	args = append(args, r.Remote, refspec)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = noPromptEnv(dir)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		r.Detail = fmt.Sprintf("no answer after %s", fetchTimeout)
		return r, errors.New(r.Detail)
	}

	flag, summary := pushStatus(out.String())
	switch {
	case flag == '=':
		r.Outcome = PushUpToDate
		return r, nil
	case flag == '*' && err == nil:
		r.Outcome = PushCreated
		return r, nil
	case flag == ' ' && err == nil:
		r.Outcome = PushSent
		r.Commits = commitsIn(dir, summary)
		return r, nil
	case flag == '!' && strings.Contains(summary, "already exists"):
		r.Outcome = PushTagTaken
		return r, nil
	// "non-fast-forward" when the last fetch already showed the remote's
	// commits, "fetch first" when it has some this repository has not seen.
	case flag == '!' && (strings.Contains(summary, "non-fast-forward") || strings.Contains(summary, "fetch first")):
		r.Outcome = PushBehind
		return r, nil
	}

	// Refused by the remote or by a hook, or never got there. What the remote
	// or the hook said is on stderr; failing that, the reason git gave for
	// the ref.
	r.Detail = strings.TrimSpace(errOut.String())
	if r.Detail == "" {
		r.Detail = summary
	}
	if err == nil {
		err = errors.New("git push reported nothing")
	}
	return r, err
}

// pushStatus finds the one ref's line in a --porcelain report,
// "<flag>\t<from>:<to>\t<summary>", and returns its flag and summary. The
// flag is 0 when there is no such line, as when the remote was never reached.
func pushStatus(report string) (flag byte, summary string) {
	for _, line := range strings.Split(strings.ReplaceAll(report, "\r", ""), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) == 3 && len(fields[0]) == 1 && strings.Contains(fields[1], ":") {
			return fields[0][0], fields[2]
		}
	}
	return 0, ""
}

// commitsIn counts the commits in the "old..new" range git reports for a
// branch it moved forward. Both ends are in this repository: the new one was
// just pushed from it, and the old one is an ancestor of that.
func commitsIn(dir, commitRange string) int {
	out, err := Run(dir, "rev-list", "--count", strings.TrimSpace(commitRange))
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(out)
	return n
}
