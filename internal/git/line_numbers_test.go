package git

import (
	"fmt"
	"strings"
	"testing"
)

// withNumbers writes a diff with each line's numbers in front of it, so a
// test can state what it expects the way it would look on screen.
func withNumbers(body string, nums []LineNumber) string {
	cell := func(n int) string {
		if n == 0 {
			return "  ."
		}
		return fmt.Sprintf("%3d", n)
	}
	var sb strings.Builder
	for i, line := range strings.Split(body, "\n") {
		sb.WriteString(cell(nums[i].Old) + cell(nums[i].New) + " " + line + "\n")
	}
	return sb.String()
}

func TestLineNumbers(t *testing.T) {
	tests := []struct {
		name, body, want string
	}{
		{
			name: "a hunk counts each side on its own",
			body: strings.Join([]string{
				"@@ -8,4 +8,5 @@ func main() {",
				" a",
				"-b",
				"+B",
				"+B2",
				" c",
				" d",
			}, "\n"),
			want: strings.Join([]string{
				"  .  . @@ -8,4 +8,5 @@ func main() {",
				"  8  8  a",
				"  9  . -b",
				"  .  9 +B",
				"  . 10 +B2",
				" 10 11  c",
				" 11 12  d",
				"",
			}, "\n"),
		},
		{
			name: "each hunk starts from its own header",
			body: strings.Join([]string{
				"@@ -1,2 +1,1 @@",
				" a",
				"-b",
				"@@ -40,1 +39,2 @@",
				" x",
				"+y",
			}, "\n"),
			want: strings.Join([]string{
				"  .  . @@ -1,2 +1,1 @@",
				"  1  1  a",
				"  2  . -b",
				"  .  . @@ -40,1 +39,2 @@",
				" 40 39  x",
				"  . 40 +y",
				"",
			}, "\n"),
		},
		{
			name: "a count left out of the header is one",
			body: "@@ -3 +3 @@\n-a\n+b",
			want: strings.Join([]string{
				"  .  . @@ -3 +3 @@",
				"  3  . -a",
				"  .  3 +b",
				"",
			}, "\n"),
		},
		{
			name: "a new file has no old side",
			body: "@@ -0,0 +1,2 @@\n+a\n+b",
			want: strings.Join([]string{
				"  .  . @@ -0,0 +1,2 @@",
				"  .  1 +a",
				"  .  2 +b",
				"",
			}, "\n"),
		},
		{
			name: "the note about a missing newline is not a line",
			body: "@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b",
			want: strings.Join([]string{
				"  .  . @@ -1 +1 @@",
				"  1  . -a",
				"  .  . \\ No newline at end of file",
				"  .  1 +b",
				"",
			}, "\n"),
		},
		{
			// The second file's "--- a/two" starts as a removed line does;
			// only the first hunk's counts say that hunk is over.
			name: "the next file's header in a whole patch is not a removed line",
			body: strings.Join([]string{
				"diff --git a/one b/one",
				"--- a/one",
				"+++ b/one",
				"@@ -1,1 +1,1 @@",
				"-a",
				"+b",
				"diff --git a/two b/two",
				"--- a/two",
				"+++ b/two",
				"@@ -5,1 +5,1 @@",
				"-c",
				"+d",
			}, "\n"),
			want: strings.Join([]string{
				"  .  . diff --git a/one b/one",
				"  .  . --- a/one",
				"  .  . +++ b/one",
				"  .  . @@ -1,1 +1,1 @@",
				"  1  . -a",
				"  .  1 +b",
				"  .  . diff --git a/two b/two",
				"  .  . --- a/two",
				"  .  . +++ b/two",
				"  .  . @@ -5,1 +5,1 @@",
				"  5  . -c",
				"  .  5 +d",
				"",
			}, "\n"),
		},
		{
			name: "a removed line that began with two dashes is still a line",
			body: "@@ -1,2 +1,1 @@\n--- a comment\n keep",
			want: strings.Join([]string{
				"  .  . @@ -1,2 +1,1 @@",
				"  1  . --- a comment",
				"  2  1  keep",
				"",
			}, "\n"),
		},
		{
			name: "the mark where a long diff was cut ends the numbering",
			body: "@@ -1,9 +1,9 @@\n a\n... (truncated)",
			want: strings.Join([]string{
				"  .  . @@ -1,9 +1,9 @@",
				"  1  1  a",
				"  .  . ... (truncated)",
				"",
			}, "\n"),
		},
		{
			name: "a merge's combined diff is left alone",
			body: "@@@ -1,2 -1,2 +1,3 @@@\n  a\n +b\n+ c",
			want: strings.Join([]string{
				"  .  . @@@ -1,2 -1,2 +1,3 @@@",
				"  .  .   a",
				"  .  .  +b",
				"  .  . + c",
				"",
			}, "\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withNumbers(tt.body, LineNumbers(tt.body)); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// An untracked file's body has no header, so it is numbered as the file is.
func TestLineNumbersOfAnUntrackedFile(t *testing.T) {
	f := FileDiff{Untracked: true, Body: "+a\n+- b\n... (truncated)"}
	want := "  .  1 +a\n  .  2 +- b\n  .  . ... (truncated)\n"
	if got := withNumbers(f.Body, f.LineNumbers()); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The numbers are those of the files git diffed: line n of the new side is
// line n of the file on disk, and line n of the old side line n of the commit.
func TestLineNumbersMatchTheFile(t *testing.T) {
	dir, _ := stageRepo(t)
	before := strings.Split(strings.TrimSuffix(numbered(30), "\n"), "\n")
	var after []string
	after = append(after, "inserted")
	after = append(after, before[:14]...)
	after = append(after, before[16:]...) // lines 15 and 16 go
	after[len(after)-1] = "changed"
	write(t, dir, "numbered.txt", strings.Join(after, "\n")+"\n")

	files := WorkingTree(dir, 80).Files
	if len(files) != 1 {
		t.Fatalf("got %d files, want the one that changed", len(files))
	}
	body := strings.Split(files[0].Body, "\n")
	checked := 0
	for i, n := range files[0].LineNumbers() {
		if n.New > 0 {
			checked++
			if got, want := body[i][1:], after[n.New-1]; got != want {
				t.Errorf("new line %d is %q in the diff, %q in the file", n.New, got, want)
			}
		}
		if n.Old > 0 {
			checked++
			if got, want := body[i][1:], before[n.Old-1]; got != want {
				t.Errorf("old line %d is %q in the diff, %q in the file", n.Old, got, want)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("no line of the diff was numbered:\n%s", files[0].Body)
	}
}
