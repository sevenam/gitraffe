package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// fakeClipboard stands in for the clipboard for one test and returns what
// was copied, in order.
func fakeClipboard(t *testing.T, how clipboardResult, err error) *[]string {
	t.Helper()
	var copied []string
	prev := writeClipboard
	writeClipboard = func(text string) (clipboardResult, error) {
		copied = append(copied, text)
		return how, err
	}
	t.Cleanup(func() { writeClipboard = prev })
	return &copied
}

func copyModel() model {
	m := testModel()
	m.commits = []commit{
		{Hash: "abc1234", FullHash: "abc1234def5678abc1234def5678abc1234def56", Message: "Fix the thing", DiffLoaded: true},
		{Hash: "0001111", FullHash: "0001111222233334444555566667777888899990", Message: "Older", DiffLoaded: true},
	}
	return m
}

func TestCopyHashAndSubject(t *testing.T) {
	for _, tc := range []struct {
		key, want, notice string
	}{
		{"y", "abc1234def5678abc1234def5678abc1234def56", "Copied the hash abc1234"},
		{"s", "Fix the thing", "Copied the subject"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			copied := fakeClipboard(t, copiedToSystem, nil)
			m := press(copyModel(), keyPress("y"), keyPress(tc.key))
			if len(*copied) != 1 || (*copied)[0] != tc.want {
				t.Errorf("copied %q, want %q", *copied, tc.want)
			}
			if m.notice != tc.notice {
				t.Errorf("notice = %q, want %q", m.notice, tc.notice)
			}
			if m.copying {
				t.Error("the prompt is still open after it was answered")
			}
		})
	}
}

// The prompt says what each key copies while it waits, since nothing else
// on screen does.
func TestCopyPromptIsOnTheStatusLine(t *testing.T) {
	m := press(copyModel(), keyPress("y"))
	line := ansi.Strip(m.renderStatusLine())
	for _, want := range []string{"y hash", "s subject", "d diff"} {
		if !strings.Contains(line, want) {
			t.Errorf("status line %q does not offer %q", line, want)
		}
	}
}

// Any other key closes the prompt and does nothing else: it was pressed as an
// answer, so it must not also move the selection.
func TestCopyPromptCancels(t *testing.T) {
	copied := fakeClipboard(t, copiedToSystem, nil)
	for _, k := range []tea.KeyMsg{{Type: tea.KeyEsc}, keyPress("j")} {
		m := press(copyModel(), keyPress("y"), k)
		if m.copying {
			t.Errorf("%s left the prompt open", k)
		}
		if m.selected != 0 {
			t.Errorf("%s moved the selection to %d", k, m.selected)
		}
	}
	if len(*copied) != 0 {
		t.Errorf("copied %q, want nothing", *copied)
	}
}

// The uncommitted changes have no hash or subject; copying an empty string
// would quietly replace whatever the clipboard held.
func TestCopyFromWorkingTreeRowSaysWhy(t *testing.T) {
	copied := fakeClipboard(t, copiedToSystem, nil)
	m := copyModel()
	m.commits = append([]commit{{WorkingTree: true, DiffLoaded: true}}, m.commits...)
	for _, key := range []string{"y", "s"} {
		got := press(m, keyPress("y"), keyPress(key))
		if !strings.Contains(got.notice, "Uncommitted changes have no") {
			t.Errorf("y%s: notice = %q, want it to say why nothing was copied", key, got.notice)
		}
	}
	if len(*copied) != 0 {
		t.Errorf("copied %q, want nothing", *copied)
	}
}

// Where no system clipboard answers, the text goes to the terminal, and the
// notice can only say it was sent.
func TestCopyThroughTheTerminalSaysSent(t *testing.T) {
	fakeClipboard(t, copiedToTerminal, nil)
	m := press(copyModel(), keyPress("y"), keyPress("y"))
	if !strings.Contains(m.notice, "terminal") {
		t.Errorf("notice = %q, want it to say the terminal was asked", m.notice)
	}
}

func TestCopyFailureSaysSo(t *testing.T) {
	fakeClipboard(t, copiedToSystem, errors.New("no clipboard"))
	m := press(copyModel(), keyPress("y"), keyPress("s"))
	if !strings.Contains(m.notice, "Could not copy") {
		t.Errorf("notice = %q, want the failure", m.notice)
	}
}

// The diff is the whole patch, read again, not the details panel's text cut
// to a few hundred lines.
func TestCopyDiffIsTheWholePatch(t *testing.T) {
	copied := fakeClipboard(t, copiedToSystem, nil)
	dir, git, commit := gittest.Fixture(t)
	git("init", "-q", "-b", "main")
	commit("first")
	writeFile(t, dir+"/long.txt", strings.Repeat("line\n", 1000))
	git("add", "-A")
	git("commit", "-qm", "long")

	m := loadedModel(t, dir)
	for i := range m.commits {
		m.commits[i].DiffLoaded = true
	}
	m = press(m, keyPress("y"))
	next, cmd := m.Update(keyPress("d"))
	if cmd == nil {
		t.Fatal("d started nothing")
	}
	m = res(next.Update(cmd()))
	if len(*copied) != 1 {
		t.Fatalf("copied %d times, want once", len(*copied))
	}
	if got := strings.Count((*copied)[0], "\n+line"); got != 1000 {
		t.Errorf("copied diff has %d added lines, want all 1000", got)
	}
	if m.notice != "Copied the diff" {
		t.Errorf("notice = %q", m.notice)
	}
}

// A diff read for a repository since switched away from is of a commit no
// longer on screen.
func TestCopyDiffFromAnotherRepositoryIsDropped(t *testing.T) {
	copied := fakeClipboard(t, copiedToSystem, nil)
	m := copyModel()
	m.repoPath = "/here"
	m = res(m.Update(copyDiffMsg{repoPath: "/elsewhere", patch: "diff --git a/x b/x\n"}))
	if len(*copied) != 0 {
		t.Errorf("copied %q, want nothing", *copied)
	}
}

// The commit view answers "y" too, for the commit it is open on rather than
// whatever the graph had selected.
func TestCopyFromTheCommitView(t *testing.T) {
	copied := fakeClipboard(t, copiedToSystem, nil)
	m := copyModel()
	m.selected = 1
	m, _ = m.openCommitView()
	m.selected = 0
	m = press(m, keyPress("y"), keyPress("y"))
	if len(*copied) != 1 || (*copied)[0] != m.commits[1].FullHash {
		t.Errorf("copied %q, want the viewed commit's hash", *copied)
	}
	if !m.commitView.open {
		t.Error("answering the prompt closed the commit view")
	}
}
