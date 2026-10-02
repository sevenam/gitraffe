// Package git is everything gitraffe asks of git: running the command line and
// reading what it prints. It knows nothing about the interface, so it can be
// tested against real repositories without a model or a screen.
package git

import (
	"os"
	"os/exec"
	"strings"
)

// Run runs a read-only git command in dir and returns its stdout.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// noPromptEnv is the environment for a git command that talks to a remote
// while the TUI owns the terminal, where a username, password or SSH
// passphrase prompt would either hang or scribble over the screen.
func noPromptEnv(repoPath string) []string {
	env := append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // HTTPS: fail rather than ask for credentials
		"GCM_INTERACTIVE=never", // Git Credential Manager: no sign-in window
	)
	if usesDefaultSSH(repoPath) {
		// BatchMode makes ssh fail instead of asking for a passphrase. Only
		// injected for plain ssh: GIT_SSH_COMMAND would override a user's own
		// GIT_SSH / core.sshCommand (e.g. plink), breaking their setup.
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return env
}

func usesDefaultSSH(repoPath string) bool {
	if os.Getenv("GIT_SSH_COMMAND") != "" || os.Getenv("GIT_SSH") != "" {
		return false
	}
	cmd := exec.Command("git", "config", "--get", "core.sshCommand")
	cmd.Dir = repoPath
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out)) == ""
}

// parseLsRemoteTags reads `git ls-remote --tags` output. An annotated tag is
