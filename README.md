# 🦒 Gitraffe

A beautiful text-based UI git graph command line tool built with Golang, Bubble Tea, go-git, and Lip Gloss.

## Features

- 📊 Visual git commit graph in your terminal (branches **and tags** are shown; the graph expands to use available space and long branch names are truncated as needed)
- 🌿 Names merged-and-deleted branches at their tip commit, recovered from merge commit messages (see [Merged branches](#merged-branches))
- 🎨 Beautiful styling with Lip Gloss
- ⌨️  Keyboard navigation (arrow keys, vim-style)
- 🖱️  Mouse wheel scrolling support
- 📱 Cross-platform (Linux, macOS, Windows)
- 🚀 Fast and lightweight

## Installation

### From Source

```bash
go install github.com/sevenam/gitraffe@latest
```

Or clone and build:

```bash
git clone https://github.com/sevenam/gitraffe.git
cd gitraffe
go build -o gitraffe.exe .
```

## Usage

Navigate to a git repository and run:

```bash
gitraffe
```

Or specify a repository path:

```bash
gitraffe /path/to/repo
```

### Keyboard Shortcuts

- `↑/↓` or `k/j` - Scroll up/down
- `PgUp/PgDn` - Page up/down
- `Home/End` - Jump to top/bottom
- `U` - Update to the latest release (shown in the help line when one is available)
- `q` or `Esc` or `Ctrl+C` - Quit

## Merged branches

Git does not store a branch name on a commit — a branch is only a movable pointer,
so deleting it erases the name. Gitraffe recovers it from the merge commit instead:
the auto-generated subject (`Merge pull request #33 from you/add-feature`, or
`Merge branch 'add-feature'`) holds the name, and the merge's second parent is the
branch's tip. That name is shown at the tip in a dimmer colour than live branches.

This only works for real merge commits. Squash and rebase merges keep no second
parent and no branch name, and fast-forwards create no merge commit at all — for
those the name is genuinely gone from the repository.

Set `merged_branch` in your theme to restyle these labels.

## Branch colours

Refs are coloured by what they are:

| Ref | Colour | Theme key |
| --- | --- | --- |
| Local branch (`main`) | green | `local_branch` (defaults to `date`) |
| Remote-tracking branch (`origin/main`) | blue | `branch` |
| Tag (`v1.0`) | yellow | `tag` |
| Merged-and-deleted branch | dim grey | `merged_branch` |

Local and remote are told apart by their full ref path, so a local branch called
`feature/foo` is never mistaken for a branch `foo` on a remote named `feature`.
`origin/HEAD` is hidden, since it only ever duplicates the remote's default branch.

Existing themes need no changes — `local_branch` falls back to that theme's `date`
colour, so the pairing stays coherent whatever palette you use.

## Tag sync

A tag that exists on only one side is marked (colour already says "tag", so the
mark is a symbol instead):

| Label | Meaning |
| --- | --- |
| `v1.0` | On your machine and on a remote — in sync |
| `v1.0↑` | Only local — not pushed to any remote |
| `v1.0↓` | Only on a remote — not fetched |

Tags are compared by name *and* commit, so a tag that was moved shows at both ends:
`↑` where it now points locally, `↓` where the remote still has it.

Git keeps no remote-tracking copy of tags the way it does for branches, so this has
to ask each remote with `git ls-remote`. That runs in the background on startup,
never prompts for credentials, and gives up after 15 seconds. Until every remote has
answered — or if there is no remote, or one can't be reached — tags stay unmarked
rather than all appearing unpushed.

## Updating

Gitraffe checks for a newer release on startup and shows it in the title bar.
Press `U` to install it — you'll be asked to confirm, and gitraffe quits once the
download finishes so the new binary can take its place. Restart to pick it up.

The same thing works without the TUI:

```bash
gitraffe update
```

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Style definitions for nice terminal layouts
- [go-git](https://github.com/go-git/go-git) - Pure Go implementation of Git
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components for Bubble Tea

## Release

Bump version number in `main.go`:

```go
const (
	appName = "Gitraffe"
	version = "0.4.1"
)
```

```bash
git tag v0.1.0
git push --tags
```

## License

MIT License - see [LICENSE](LICENSE) file for details.
