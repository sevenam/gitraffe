package git

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/sevenam/gitraffe/internal/gittest"
)

// The editor git would use is the one asked for: GIT_EDITOR above
// core.editor, and core.editor above EDITOR.
func TestEditorFollowsGit(t *testing.T) {
	dir := gittest.NewRepo(t)
	// Set first so that each is put back afterwards; unset, since git takes
	// an empty GIT_EDITOR as one set to nothing.
	for _, name := range []string{"GIT_EDITOR", "VISUAL"} {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	t.Setenv("EDITOR", "nano -w")
	// The machine's own core.editor would come first otherwise.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	cmd := func(args ...string) {
		t.Helper()
		if _, err := Run(dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	if got, err := Editor(dir); err != nil || got != "nano -w" {
		t.Fatalf("with EDITOR: got %q, %v; want nano -w", got, err)
	}
	cmd("config", "core.editor", "code --wait")
	if got, err := Editor(dir); err != nil || got != "code --wait" {
		t.Fatalf("with core.editor: got %q, %v; want code --wait", got, err)
	}
	t.Setenv("GIT_EDITOR", "zed --wait")
	if got, err := Editor(dir); err != nil || got != "zed --wait" {
		t.Fatalf("with GIT_EDITOR: got %q, %v; want zed --wait", got, err)
	}
}

func TestSplitCommand(t *testing.T) {
	for _, c := range []struct {
		in      string
		windows bool
		want    []string
	}{
		{"vim", false, []string{"vim"}},
		{"  code   --wait ", false, []string{"code", "--wait"}},
		{"'C:/Program Files/Notepad++/notepad++.exe' -multiInst -nosession", false,
			[]string{"C:/Program Files/Notepad++/notepad++.exe", "-multiInst", "-nosession"}},
		{`"/opt/my editor/ed" -x`, false, []string{"/opt/my editor/ed", "-x"}},
		{`vim -c "set tw=\"72\""`, false, []string{"vim", "-c", `set tw="72"`}},
		{`my\ editor --wait`, false, []string{"my editor", "--wait"}},
		{`''`, false, []string{""}},
		{`a'b c'd`, false, []string{"ab cd"}},
		// A Windows path keeps its backslashes, quoted or not.
		{`C:\Tools\vim.exe -u NONE`, true, []string{`C:\Tools\vim.exe`, "-u", "NONE"}},
		{`"C:\Program Files\Microsoft VS Code\Code.exe" --wait`, true,
			[]string{`C:\Program Files\Microsoft VS Code\Code.exe`, "--wait"}},
		{"", false, nil},
	} {
		got, err := splitCommand(c.in, c.windows)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitCommand(%q, windows=%v) = %q, %v; want %q", c.in, c.windows, got, err, c.want)
		}
	}
	for _, in := range []string{`vim 'x`, `"code --wait`, `a "b\"`} {
		if _, err := splitCommand(in, false); !errors.Is(err, ErrUnclosedQuote) {
			t.Errorf("splitCommand(%q) err = %v; want ErrUnclosedQuote", in, err)
		}
	}
}
