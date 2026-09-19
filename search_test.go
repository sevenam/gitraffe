package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// searchRepo has commits by two authors with messages that overlap, so a
// query can match one, several or none.
func searchRepo(t *testing.T) string {
	t.Helper()
	dir, git, _ := gitFixture(t)
	git("init", "-q", "-b", "main")
	for i, c := range []struct{ author, message string }{
		{"ada", "add the parser"},   // oldest
		{"grace", "fix the parser"}, //
		{"ada", "tidy up"},          //
		{"grace", "fix the lexer"},  // newest
	} {
		write(t, dir, c.message, c.message)
		git("add", "-A")
		// gitFixture's own GIT_AUTHOR_NAME would win over -c user.name, so the
		// author is set the same way: through the environment.
		at := fmt.Sprintf("2026-02-01T00:%02d:00", i)
		cmd := exec.Command("git", "commit", "-qm", c.message)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME="+c.author, "GIT_AUTHOR_EMAIL="+c.author+"@t",
			"GIT_COMMITTER_NAME="+c.author, "GIT_COMMITTER_EMAIL="+c.author+"@t",
			"GIT_AUTHOR_DATE="+at, "GIT_COMMITTER_DATE="+at,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v\n%s", err, out)
		}
	}
	return dir
}

func selectedMessage(m model) string {
	if m.selected < 0 || m.selected >= len(m.commits) {
		return "<none>"
	}
	return m.commits[m.selected].Message
}

func search(m model, query string) model {
	m = press(m, keyPress("/"))
	return typeText(m, query)
}

func TestSearchJumpsAsYouType(t *testing.T) {
	m := loadedModel(t, searchRepo(t))
	if got := selectedMessage(m); got != "fix the lexer" {
		t.Fatalf("starts on %q, want the newest commit", got)
	}

	m = search(m, "parser")
	if got := selectedMessage(m); got != "fix the parser" {
		t.Errorf("searching \"parser\" landed on %q, want the newest match", got)
	}
	// Deleting a letter widens the search again rather than stranding you.
	m = press(m, tea.KeyMsg{Type: tea.KeyBackspace})
	if !m.search.found {
		t.Error("\"parse\" found nothing, though it is a prefix of a match")
	}
}

func TestSearchMatchesAuthorAndHash(t *testing.T) {
	m := loadedModel(t, searchRepo(t))

	byAuthor := search(m, "ada")
	if got := selectedMessage(byAuthor); got != "tidy up" {
		t.Errorf("searching an author landed on %q, want their newest commit", got)
	}

	hash := m.commits[2].FullHash[:6]
	byHash := search(m, hash)
	if got := byHash.selected; got != 2 {
		t.Errorf("searching a hash selected %d (%q), want commit 2", got, selectedMessage(byHash))
	}
}

func TestSearchSaysWhenNothingMatches(t *testing.T) {
	m := loadedModel(t, searchRepo(t))
	was := m.selected
	m = search(m, "zzz")

	if m.search.found {
		t.Error("a query with no matches reported one")
	}
	if m.selected != was {
		t.Error("a query with no matches moved the selection")
	}
	if line := ansi.Strip(m.renderStatusLine()); !strings.Contains(line, "no match") {
		t.Errorf("prompt = %q, want it to say there is no match", line)
	}
}

func TestEscapeRestoresWhereYouWere(t *testing.T) {
	m := loadedModel(t, searchRepo(t))
	m.selected = 2
	was := selectedMessage(m)

	m = search(m, "lexer")
	if selectedMessage(m) == was {
		t.Fatal("the search did not move anywhere to come back from")
	}
	m = press(m, esc)
	if m.search.active || selectedMessage(m) != was {
		t.Errorf("active=%v selected=%q; want the prompt closed and the selection back",
			m.search.active, selectedMessage(m))
	}
}

func TestEnterKeepsTheQueryForNextAndPrevious(t *testing.T) {
	m := loadedModel(t, searchRepo(t))
	m = press(search(m, "parser"), enter)

	if m.search.active || m.search.query != "parser" {
		t.Fatalf("active=%v query=%q; want the prompt closed and the query kept", m.search.active, m.search.query)
	}
	if !strings.Contains(m.notice, "Match 1 of 2") {
		t.Errorf("notice = %q, want which match this is and how many there are", m.notice)
	}

	// n goes down the graph, into older commits; N comes back.
	res, _ := m.Update(keyPress("n"))
	next := res.(model)
	if got := selectedMessage(next); got != "add the parser" {
		t.Errorf("n landed on %q, want the older match", got)
	}
	res, _ = next.Update(keyPress("N"))
	if got := selectedMessage(res.(model)); got != "fix the parser" {
		t.Errorf("N landed on %q, want the newer match again", got)
	}
}

// A search that stopped at the last match would hide the ones above it.
func TestNextWrapsRound(t *testing.T) {
	m := loadedModel(t, searchRepo(t))
	m = press(search(m, "parser"), enter)

	res, _ := m.Update(keyPress("n")) // older match
	res, _ = res.(model).Update(keyPress("n"))
	if got := selectedMessage(res.(model)); got != "fix the parser" {
		t.Errorf("n at the last match landed on %q, want it back at the first", got)
	}
}

func TestSearchKeysTypeRatherThanAct(t *testing.T) {
	m := loadedModel(t, searchRepo(t))
	m = search(m, "qtro")
	if !m.search.active || m.search.input.Value() != "qtro" {
		t.Errorf("active=%v value=%q; shortcut letters must type into the query",
			m.search.active, m.search.input.Value())
	}
	if m.showHelp || m.picker.open || m.switcher.open {
		t.Error("a key typed into the query opened something")
	}
}

func TestSearchSkipsTheWorkingTreeRow(t *testing.T) {
	dir, _, _ := dirtyRepo(t)
	write(t, dir, "first", "changed")
	m := loadedModel(t, dir)
	if !m.commits[0].WorkingTree {
		t.Fatal("no working tree row to skip")
	}
	m = search(m, "changed")
	if m.search.found {
		t.Error("the working tree row answered a search for a commit")
	}
}
