# 🦒 Gitraffe

A text-based UI git graph command line tool built with Golang, Bubble Tea, go-git, and Lip Gloss.

<img width="925" height="750" alt="image" src="https://github.com/user-attachments/assets/d1e2a064-d542-4cbc-b578-15f15114058d" />



## Features

- 📊 Visual git commit graph in your terminal (branches **and tags** are shown; the graph expands to use available space and long branch names are truncated as needed)
- 🌈 A colour per branch in the graph, held across the columns git shifts it through (press `c` to toggle; see [Graph lane colours](#graph-lane-colours))
- 🌿 Names merged-and-deleted branches at their tip commit, recovered from merge commit messages (see [Merged branches](#merged-branches))
- 👤 Each commit's date and author in columns beside the hash (on a narrow terminal the author goes first, then the date, before branch labels are cut)
- 🔀 Ahead/behind for the current branch next to its name, e.g. `main ↑3 ↓1` (see [Ahead and behind](#ahead-and-behind))
- 📝 Uncommitted changes as a row above the newest commit, with their diff (see [Uncommitted changes](#uncommitted-changes))
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

Every invocation has the same shape — flags, then an optional repository path:

```
gitraffe [flags] [repository path]
```

With no path, gitraffe opens the repository in the current directory.

### Commands

| Command | What it does |
| --- | --- |
| `gitraffe` | Open the repository in the current directory |
| `gitraffe /path/to/repo` | Open the repository at that path |
| `gitraffe --theme <file>` | Open with a colour theme for this run only (see [Themes](#themes)) |
| `gitraffe --update` | Install the latest release and exit, without opening the TUI (see [Updating](#updating)) |
| `gitraffe --version` | Print the version and exit |
| `gitraffe --help` | Print usage and exit |

Flags take one or two dashes, so `-theme` and `--theme` are the same, as are `-update`
and `--update`, and `-h` and `--help`. They may appear before or after the repository
path — `gitraffe . --theme x.yml` and `gitraffe --theme x.yml .` both work.

The two commands that do something and exit also answer to a bare word, so `gitraffe
update` and `gitraffe version` mean the same as `--update` and `--version`. Note that a
bare word shadows a repository in a directory of that name; open those with a path, as
in `gitraffe ./update`.

### Keyboard Shortcuts

- `?` - Show every keyboard shortcut (`?`, `Esc` or `q` closes it)
- `↑/↓` or `k/j` - Scroll up/down
- `PgUp/PgDn` - Page up/down
- `Home/End` - Jump to top/bottom
- Mouse wheel - Scroll the panel the pointer is over (see [Mouse](#mouse))
- `Enter` - Fill the window with the focused panel (see [One panel at a time](#one-panel-at-a-time))
- `r` or `F5` - Reload the repository (see [Reloading](#reloading))
- `f` - Fetch from the remote, then reload (see [Fetching](#fetching))
- `/` - Search commits, then `n` / `N` for next and previous (see [Searching](#searching))
- `m` - Read more of a long history (see [Long histories](#long-histories))
- `o` - Open another repository (see [Switching repository](#switching-repository))
- `t` - Pick a colour theme (see [Picking a theme](#picking-a-theme))
- `c` - Toggle lane colours in the graph (see [Graph lane colours](#graph-lane-colours))
- `U` - Update to the latest release (shown in the help line when one is available)
- `q` or `Esc` or `Ctrl+C` - Quit

### Mouse

The wheel scrolls whichever panel the pointer is over: the graph moves the selected
commit, the details panel scrolls the diff. Pointing at a panel is how a mouse says
which one you mean, so it neither needs focus nor takes it — the keyboard keeps
driving whatever it was driving.

One notch moves three lines. Maximised there is only one panel, so the wheel always
belongs to it.

### One panel at a time

Press `Enter` to give the focused panel the whole window, and `Enter` again to bring
the other one back. `1`, `2` and `tab` still choose which panel that is, so you can
swap between a full-window graph and a full-window diff without leaving fullscreen.

The graph uses the extra room rather than just stretching: branch labels that were
truncated fit, and the date and author columns come back on a terminal too narrow to
show them beside the details panel.

Whether you left gitraffe maximised is remembered (see
[Remembered preferences](#remembered-preferences)).

### Searching

Press `/` and type. The graph moves to each match as you type, so a wrong turn shows
immediately rather than after `Enter`. Matching ignores case and looks at the commit
message, the author and the hash, so a hash pasted from a bug report finds its commit.

- `Enter` keeps the query and closes the prompt; the bottom line then says which
  match you are on and how many there are.
- `Esc` puts the selection back where it was before you started.
- `n` and `N` move to the next and previous match afterwards, wrapping round the ends
  so no match is out of reach. `↑` and `↓` do the same while the prompt is open.
- A query that matches nothing says so and leaves the selection alone.

The search only covers the commits that are loaded; on a long history, `m` reads more
(see [Long histories](#long-histories)).

### Long histories

Gitraffe reads 5,000 commits at a time, enough that most repositories arrive whole
and few enough that a very old one doesn't spend seconds drawing history nobody
asked to see. When there is more, the graph ends with a line saying so:

```
   … more history — press m
```

`m` reads the next 5,000. `G` jumps to the bottom, where the line is. However much
you have loaded is kept when you reload with `r` or fetch with `f`, and starts over
at 5,000 when you open a different repository.

The whole graph is read again rather than the new commits added to it, because git
draws the lanes for the commits it is given: a second batch drawn on its own would
not join up with the first. Your place is kept, as with any reload.

### Fetching

Press `f` to run `git fetch --all --prune` and reload once it finishes, which is what
makes the ahead/behind counts and the tag marks true as of now rather than as of your
last fetch.

It is the one thing gitraffe does that writes to your repository, so it only ever
happens when you press the key — never on a timer, and never at startup. What it
writes is limited to remote-tracking refs: your branches, your tags and your working
tree are not touched, and `--prune` only drops `origin/...` refs whose branch the
remote no longer has.

It never prompts. A remote that wants a password, or an SSH key with a passphrase,
fails with git's own message on the bottom line instead of a prompt fighting the
graph for the screen, and a remote that never answers is given up on after a minute.

### Reloading

Gitraffe reads the repository when it opens it and doesn't watch for changes, so
commits you make in another terminal aren't there until you ask for them. Press `r`
or `F5` to read it again: the graph, the ahead/behind counts and the tag marks are
all rebuilt together.

Your place is kept. The commit you had selected stays selected, even though new
commits have pushed it down the list, and the same panel keeps focus. If that commit
is gone — amended or rebased away — the selection falls back to the newest commit.

### Switching repository

Press `o` to open another repository without leaving gitraffe. The box starts with
the last ten repositories you opened, most recent first: pick one with `↑/↓` and
press `Enter`. `Esc` closes the box and keeps the repository you had.

Or type a path, and the list becomes the folders at that path that match what you've
typed so far, with repositories marked `repo`:

- `Tab` completes the highlighted folder, or the first one if none is highlighted,
  and moves the list into it.
- `Enter` on a highlighted repository opens it; on any other folder it goes into
  it, as a file browser would. With nothing highlighted, it opens the typed path.
- Matching ignores case. Hidden folders are listed once you type the leading `.`.

A few more things about paths:

- A folder inside a repository opens the whole repository, as it does for git.
- A relative path is relative to the repository on screen, so `../other` opens a
  sibling. `~` means your home directory.
- A path that isn't a repository is refused with the reason, and the box stays
  open to correct it.

`o` also works from the error screen, so starting gitraffe in a folder that isn't a
repository isn't a dead end. The recent list is kept in `settings.yml` in your config
directory (see [Picking a theme](#picking-a-theme)), next to your theme choice.

### Remembered preferences

Gitraffe writes what you chose to `settings.yml` in your config directory, so the
next run starts where the last one left off:

| Key | What it holds |
| --- | --- |
| `theme` | the theme picked with `t` (see [Picking a theme](#picking-a-theme)) |
| `recent_repos` | the repositories the `o` box offers |
| `lane_colours` | whether the graph is coloured per branch (`c`) |
| `focused_box` | the panel that had focus, `1` or `2` |
| `maximised` | whether that panel filled the window (`Enter`) |

The last three are written when gitraffe exits, not as you press the keys, since `c`
and `tab` are pressed often and the file is only read at startup. Delete the file,
or any single key in it, to go back to the defaults: lane colours on, the graph
focused. A `focused_box` naming a panel that doesn't exist is ignored.

## Uncommitted changes

When the working tree isn't clean, a row sits above the newest commit with a hollow
marker and a count — `3 changed, 1 untracked` — so the graph answers "what have I
got in progress" as well as "where am I". A clean tree adds no row.

Staged and unstaged changes are one number: both are work in progress, and which is
which is a detail for the details panel. Select the row and it shows the stats and
the diff of everything against `HEAD`, staged changes included. Untracked files have
no diff, so they are listed by name instead.

Nothing here is refreshed on a timer; press `r` after editing files (see
[Reloading](#reloading)).

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

## Graph lane colours

Each branch in the graph gets its own colour, so you can follow one from the row it
splits off to the row it merges back. The trunk keeps the theme's `graph` colour, and
the branches fanning out to its right take a colour each from a fixed palette that
repeats once a repository has more than six branches open at once.

The colour follows the *branch*, not the column it is drawn in. Those are not the same
thing: git slides lanes sideways to make room, so one branch can be drawn in three
different columns on its way down the screen, and a column that one branch has merged
away from is reused by an unrelated one later. Colouring by column therefore recolours
a branch halfway down and gives two unrelated branches the same colour, which is why
gitraffe reads the lane identity out of git's own coloured graph output and then maps
each lane back to the branch it belongs to.

Press `c` to turn it off and render the whole graph in the theme's `graph` colour, the
way it looked before. The key works whichever panel has focus, and the setting is
remembered for next time (see [Remembered preferences](#remembered-preferences)) —
it is not written to your theme file.

The colours are deliberately not theme keys: most bundled themes already leave the
optional ref colours unset, so six more would in practice be six more nobody sets.
A theme that paints a light `background` gets darker versions of the same six
instead, since the usual ones are pastels made for dark backgrounds.

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

`background` is the one key the default leaves empty: without it, gitraffe draws on
your terminal's own background, so a theme's colours have to suit whatever that is.
With it, the theme paints the whole screen and looks the same in any terminal.
`default-light` sets it, which is what lets it be light grey on a dark terminal.
Text with no colour of its own (the repository name, diff context lines) is then
drawn in `foreground`, which defaults to the `message` colour: the terminal's own
text colour suits its background, not the theme's.
### Picking a theme

Press `t` to list the themes and pick one. Moving through the list shows each theme
straight away; `Enter` keeps it and `Esc` puts back the one you had. Your pick is
saved and used every time gitraffe starts.

The list holds the default colours, the themes that ship with gitraffe
(`default-light`, Atom One Dark and the Tokyo Night variants), and your own themes
(marked *yours*):

- any `.yml` file in a `themes` folder in your config directory, listed by file
  name. One named like a bundled theme replaces it, so copying a bundled theme
  there is how to tweak it.
- your `theme.yml`, if you have one (see below).

Your config directory is `%APPDATA%\gitraffe` on Windows,
`~/Library/Application Support/gitraffe` on macOS and `~/.config/gitraffe` on
Linux. The pick is saved there as `settings.yml`, never into `theme.yml`, so picking
a theme can't overwrite colours you wrote by hand. Each theme is read again as you
move onto it, so after editing one, reopen the list to see the change.

### Other ways to set a theme

- **`theme.yml`:** save a theme as `theme.yml` in your config directory. It is used
  when you haven't picked a theme with `t`; once you have, the pick wins, and
  `theme.yml` stays in the list to switch back to. If this file can't be read,
  gitraffe starts with the defaults and records why in `gitraffe.log`.
- **One run:** `gitraffe --theme path/to/theme.yml`, which takes precedence over
  both. Because you named the file, problems stop gitraffe with an error instead: a
  missing file, invalid YAML, or a misspelt key.

The `themes/` folder in this repository holds the bundled themes;
they're built into the binary, so they are in the list however you installed it.

## Ahead and behind

The branch in the info box shows how it differs from its upstream: `↑3` means three
local commits not yet pushed, `↓1` means one commit on the remote you haven't pulled.

A branch that hasn't been pushed with `-u` (or whose remote branch was deleted) has no
upstream to compare with, but it is still ahead: `↑` then counts its commits that no
remote has yet. It never shows `↓`, since there is nothing to be behind.

The counts compare against your remote-tracking branches as of your last fetch, so
press `f` (see [Fetching](#fetching)) to bring them up to date.
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
gitraffe --update
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
