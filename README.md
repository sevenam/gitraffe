# 🦒 Gitraffe

A beautiful text-based UI git graph command line tool built with Golang, Bubble Tea, go-git, and Lip Gloss.

## Features

- 📊 Visual git commit graph in your terminal (branches **and tags** are shown; the graph expands to use available space and long branch names are truncated as needed)
- 🌿 Names merged-and-deleted branches at their tip commit, recovered from merge commit messages (see [Merged branches](#merged-branches))
- 👤 Each commit's date and author in columns beside the hash (on a narrow terminal the author goes first, then the date, before branch labels are cut)
- 🔀 Ahead/behind for the current branch next to its name, e.g. `main ↑3 ↓1` (see [Ahead and behind](#ahead-and-behind))
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

Use a colour theme for this run (see [Themes](#themes)); `-theme` and `--theme` both work:

```bash
gitraffe --theme themes/tokyo-night-storm.yml
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
| Ahead count (`↑3`) | cyan | `ahead` (defaults to `author`) |
| Behind count (`↓1`) | red | `behind` (defaults to `diff_del`) |

Local and remote are told apart by their full ref path, so a local branch called
`feature/foo` is never mistaken for a branch `foo` on a remote named `feature`.
`origin/HEAD` is hidden, since it only ever duplicates the remote's default branch.

Existing themes need no changes — `local_branch` falls back to that theme's `date`
colour, so the pairing stays coherent whatever palette you use.

## Themes

Colours come from a YAML file with a `colors:` section. Any key you leave out keeps
its default, so a theme only needs the colours it changes (keys are listed in
[Branch colours](#branch-colours) and in the bundled themes):

```yaml
colors:
  branch: "#7aa2f7"
  selected_bg: "#2f334d"
```

There are two ways to use one:

- **Every run:** save it as `theme.yml` in your config directory —
  `%APPDATA%\gitraffe\theme.yml` on Windows,
  `~/Library/Application Support/gitraffe/theme.yml` on macOS,
  `~/.config/gitraffe/theme.yml` on Linux. If this file can't be read, gitraffe
  starts with the defaults and records why in `gitraffe.log`.
- **One run:** `gitraffe -theme path/to/theme.yml`, which takes precedence over the
  config file. Because you named the file, problems stop gitraffe with an error
  instead: a missing file, invalid YAML, or a misspelt key.

The `themes/` folder in this repository has ready-made Tokyo Night variants to use
either way. Themes are read at startup, so restart gitraffe after editing one.

## Ahead and behind

The branch in the info box shows how it differs from its upstream: `↑3` means three
local commits not yet pushed, `↓1` means one commit on the remote you haven't pulled.

A branch that hasn't been pushed with `-u` (or whose remote branch was deleted) has no
upstream to compare with, but it is still ahead: `↑` then counts its commits that no
remote has yet. It never shows `↓`, since there is nothing to be behind.

The counts compare against your remote-tracking branches as of your last `git fetch` —
gitraffe never fetches, so run `git fetch` first to see the remote's current state.
Nothing is shown when the branch is in sync, on a detached HEAD, or in a repository
with no remote.

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
git tag v0.1.0 origin/main
git push --tags
```

## License

MIT License - see [LICENSE](LICENSE) file for details.
