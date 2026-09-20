package main

import (
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// refRepo has a branch, a lightweight tag, an annotated tag and a remote, so
// every kind the picker lists is in it.
func refRepo(t *testing.T) string {
	t.Helper()
	dir, git, commit := gitFixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("tag", "v0.1.0")
	commit("second")
	git("tag", "-a", "v0.2.0", "-m", "the annotated one")
	git("checkout", "-qb", "side-branch")
	commit("third")
	git("checkout", "-q", "main")
	// A remote-tracking branch without a remote to fetch from: the ref is what
	// the picker reads, and writing it directly keeps the test offline.
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	git("update-ref", "refs/remotes/origin/HEAD", "HEAD")
	return dir
}

func refNames(p refPicker) []string {
	var out []string
	for _, i := range p.shown {
		out = append(out, p.choices[i].name)
	}
	return out
}

func findRef(t *testing.T, p refPicker, name string) refChoice {
	t.Helper()
	for _, c := range p.choices {
		if c.name == name {
			return c
		}
	}
	t.Fatalf("%q is not among %v", name, refNames(p))
	return refChoice{}
}

func openRefs(t *testing.T, dir string) model {
	t.Helper()
	m := loadedModel(t, dir)
	m.windowWidth, m.windowHeight = 100, 30
	m = press(m, keyPress("b"))
	if !m.refs.open {
		t.Fatal("b did not open the ref picker")
	}
	return m
}

func TestBOpensAndEscCloses(t *testing.T) {
	m := openRefs(t, refRepo(t))
	if m.refs.err != "" {
		t.Fatalf("reading the refs failed: %s", m.refs.err)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Branches and tags") {
		t.Error("the picker is not drawn over the graph")
	}
	if closed := press(m, esc); closed.refs.open {
		t.Error("esc did not close the picker")
	}
}

func TestPickerListsBranchesTagsAndRemotes(t *testing.T) {
	m := openRefs(t, refRepo(t))
	got := refNames(m.refs)

	for _, want := range []string{"main", "side-branch", "v0.1.0", "v0.2.0", "origin/main"} {
		if !contains(got, want) {
			t.Errorf("%q is missing from %v", want, got)
		}
	}
	// origin/HEAD only repeats the branch listed beside it.
	if contains(got, "origin/HEAD") {
		t.Errorf("origin/HEAD is listed: %v", got)
	}

	// The branch you are on comes first, so the list starts where you are.
	if got[0] != "main" {
		t.Errorf("the list starts at %q, want the current branch", got[0])
	}
	if c, _ := m.refs.selected(); c.name != "main" {
		t.Errorf("the cursor starts on %q, want the current branch", c.name)
	}
	// Branches, then tags, then the remote copies.
	if i, j := indexOf(got, "side-branch"), indexOf(got, "v0.2.0"); i > j {
		t.Errorf("tags come before branches: %v", got)
	}
	if i, j := indexOf(got, "v0.1.0"), indexOf(got, "origin/main"); i > j {
		t.Errorf("remotes come before tags: %v", got)
	}
	// Newest tag first: a release you are looking for is likelier a recent one.
	if i, j := indexOf(got, "v0.2.0"), indexOf(got, "v0.1.0"); i > j {
		t.Errorf("tags are oldest first: %v", got)
	}
}

// An annotated tag is an object of its own, and it is not in the graph — the
// commit it points at is.
func TestAnnotatedTagResolvesToItsCommit(t *testing.T) {
	dir := refRepo(t)
	m := openRefs(t, dir)
	tag := findRef(t, m.refs, "v0.2.0")

	out, err := exec.Command("git", "-C", dir, "rev-parse", "v0.2.0^{commit}").Output()
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(out))
	if tag.commit != want {
		t.Errorf("v0.2.0 points at %s, want the commit %s", tag.commit, want)
	}
	// And the tag object itself, which must not be what we jump to.
	out, _ = exec.Command("git", "-C", dir, "rev-parse", "v0.2.0").Output()
	if object := strings.TrimSpace(string(out)); tag.commit == object {
		t.Error("v0.2.0 resolved to the tag object, which is in no graph row")
	}
}

func TestTypingFiltersTheList(t *testing.T) {
	m := openRefs(t, refRepo(t))

	m = press(m, keyPress("v0."))
	if got := refNames(m.refs); len(got) != 2 {
		t.Errorf("filtering by \"v0.\" left %v, want the two tags", got)
	}
	// Anywhere in the name, ignoring case.
	m = press(m, esc, keyPress("b"), keyPress("BRANCH"))
	if got := refNames(m.refs); len(got) != 1 || got[0] != "side-branch" {
		t.Errorf("filtering by \"BRANCH\" left %v, want side-branch", got)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.refs.filter != "BRANC" {
		t.Errorf("filter = %q after backspace, want BRANC", m.refs.filter)
	}

	m = press(m, keyPress("zzz"))
	if len(m.refs.shown) != 0 {
		t.Errorf("shown = %v, want nothing to match", refNames(m.refs))
	}
	if !strings.Contains(ansi.Strip(m.View()), "nothing matches") {
		t.Error("the picker does not say that nothing matches")
	}
	// Enter with nothing to choose leaves the picker where it is.
	if after := press(m, enter); !after.refs.open {
		t.Error("enter on an empty list closed the picker")
	}
}

// Every printable key is filter text, so the graph's keys must not act behind
// the box — the same bargain the repository box makes.
func TestKeysDoNotLeakThroughTheRefPicker(t *testing.T) {
	m := openRefs(t, refRepo(t))
	m.selected = 1
	before := m

	for _, key := range []string{"j", "k", "G", "c", "2"} {
		after := press(before, keyPress(key))
		if after.selected != before.selected {
			t.Errorf("%q moved the graph's selection %d→%d", key, before.selected, after.selected)
		}
		if after.colourLanes != before.colourLanes {
			t.Errorf("%q toggled lane colours behind the picker", key)
		}
		if after.refs.filter != key {
			t.Errorf("%q gave filter %q, want it typed into the filter", key, after.refs.filter)
		}
	}
}

func TestEnterJumpsToTheRefsCommit(t *testing.T) {
	dir := refRepo(t)
	m := openRefs(t, dir)
	m.selected = 0

	tag := findRef(t, m.refs, "v0.1.0") // the oldest commit
	m = press(m, keyPress("v0.1"))
	res, cmd := m.Update(enter)
	got := res.(model)

	if got.refs.open {
		t.Error("the picker stayed open after jumping")
	}
	if got.commits[got.selected].FullHash != tag.commit {
		t.Errorf("selected %q, want the commit v0.1.0 points at", got.commits[got.selected].Message)
	}
	if got.detailsScroll != 0 {
		t.Errorf("detailsScroll = %d, want the details back at the top", got.detailsScroll)
	}
	if cmd == nil {
		t.Error("no command, want the commit's diff loaded")
	}
	if !strings.Contains(got.notice, "v0.1.0") {
		t.Errorf("notice = %q, want it to name the ref jumped to", got.notice)
	}
}

// The refs worth jumping to are the ones too old to scroll to, and those have
// no commit loaded at all until more history is read.
func TestJumpingToARefOlderThanTheLoadedHistory(t *testing.T) {
	dir, git, commit := gitFixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("tag", "v0.1.0")
	for i := range 6 {
		commit(string(rune('a' + i)))
	}

	// Only the newest two commits read, as a big repository's first batch is a
	// small part of its history.
	m := testModel()
	m.repoPath = dir
	m.commitLimit = 2
	res, _ := m.Update(loadRepo(dir)())
	m = res.(model)
	if len(m.commits) != 2 {
		t.Fatalf("loaded %d commits, want the 2 asked for", len(m.commits))
	}

	m = press(m, keyPress("b"))
	tag := findRef(t, m.refs, "v0.1.0")
	for _, c := range m.commits {
		if c.FullHash == tag.commit {
			t.Fatal("v0.1.0 is already loaded; the test proves nothing")
		}
	}

	m = press(m, keyPress("v0.1"))
	res, cmd := m.Update(enter)
	m = res.(model)

	if m.commitLimit < 7 {
		t.Errorf("commitLimit = %d, want enough to reach a commit 7 deep", m.commitLimit)
	}
	if m.reselect != tag.commit {
		t.Errorf("reselect = %q, want the tag's commit", m.reselect)
	}
	if !strings.Contains(m.notice, "v0.1.0") {
		t.Errorf("notice = %q, want it to say what it is reading for", m.notice)
	}
	if cmd == nil {
		t.Fatal("no command, want the history read again")
	}

	// Finish the reload the way the program does, and the selection lands.
	res, _ = m.Update(loadRepo(dir)())
	m = res.(model)
	if got := m.commits[m.selected].FullHash; got != tag.commit {
		t.Errorf("after reading more history the selection is %s, want the tag's commit %s",
			shortHash(got), shortHash(tag.commit))
	}
}

// The depth has to be counted in the order the graph is drawn in: git log
// --graph implies --topo-order, and the date order git otherwise uses puts
// commits somewhere else.
func TestCommitDepthMatchesTheGraphsOrder(t *testing.T) {
	dir, git, commit := gitFixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	git("checkout", "-qb", "side")
	commit("on the side")
	git("checkout", "-q", "main")
	commit("on main")
	git("merge", "-q", "--no-ff", "side", "-m", "merge the side")

	m := loadedModel(t, dir)
	out, err := exec.Command("git", "-C", dir, "log", "--all", "--topo-order", "--format=%H").Output()
	if err != nil {
		t.Fatal(err)
	}
	order := strings.Fields(strings.ReplaceAll(string(out), "\r", ""))

	for want, hash := range order {
		if got := m.commitDepth(hash); got != want {
			t.Errorf("commitDepth(%s) = %d, want %d", shortHash(hash), got, want)
		}
	}
	if got := m.commitDepth(strings.Repeat("0", 40)); got != -1 {
		t.Errorf("commitDepth of a commit that isn't there = %d, want -1", got)
	}
}

// A repository with no branch at all still has to open something readable.
func TestPickerOnARepositoryWithNoRefs(t *testing.T) {
	dir, git, _ := gitFixture(t)
	git("init", "-q", "-b", "main")
	git("commit", "-q", "--allow-empty", "-m", "first")
	git("update-ref", "-d", "refs/heads/main")
	git("checkout", "-q", "--orphan", "empty")

	m := loadedModel(t, dir)
	m.windowWidth, m.windowHeight = 100, 30
	m = m.openRefPicker()
	if len(m.refs.choices) != 0 {
		t.Fatalf("choices = %v, want none", refNames(m.refs))
	}
	if !strings.Contains(ansi.Strip(m.View()), "No branches or tags") {
		t.Error("the picker does not say the repository has no refs")
	}
	// Enter with nothing to choose does nothing at all, rather than closing
	// the box or reaching into an empty list.
	after := press(m, enter)
	if !after.refs.open || after.selected != m.selected {
		t.Errorf("enter left open=%v selected=%d, want the picker untouched", after.refs.open, after.selected)
	}
}

func contains(list []string, want string) bool {
	return indexOf(list, want) >= 0
}

func indexOf(list []string, want string) int {
	for i, s := range list {
		if s == want {
			return i
		}
	}
	return -1
}
