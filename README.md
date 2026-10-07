# 🦒 Gitraffe

A text-based UI git graph command line tool built with Golang, Bubble Tea, go-git, and Lip Gloss.

<img width="951" height="738" alt="image" src="https://github.com/user-attachments/assets/ec1c1a22-0047-46ea-a4d5-bc95e5a2b893" />

## Features

- 📊 Visual git commit graph in your terminal (branches **and tags** are shown; the graph expands to use available space and long branch names are truncated as needed)
- 🌈 A colour per branch in the graph, held across the columns git shifts it through (press `L` to toggle; see [Graph lane colours](#graph-lane-colours))
- 🧭 Every branch line runs to the commit it started from and the commit it was merged into, and merges are drawn as a diamond (see [Reading the graph](#reading-the-graph))
- 🌿 Names merged-and-deleted branches at their tip commit, recovered from merge commit messages (see [Merged branches](#merged-branches))
- 👤 Each commit's date and author in columns beside the hash (on a narrow terminal the author goes first, then the date, before branch labels are cut)
- 🔀 Ahead/behind for the current branch next to its name, e.g. `main ↑3 ↓1` (see [Ahead and behind](#ahead-and-behind))
- 📝 Uncommitted changes as a row above the newest commit, with their diff (see [Uncommitted changes](#uncommitted-changes))
- 🔍 A commit view on `Space`: what the commit is, every file it touched, and one file's diff at a time (see [The commit view](#the-commit-view))
- ✍️ Stage files, hunks or single lines from the commit view and commit them with `c`, without leaving gitraffe (see [Staging and committing](#staging-and-committing))
- ⬇️ `p` pulls: fetch, then fast-forward your branch — and nothing riskier than that (see [Pulling](#pulling))
- ⬆️ `P` pushes your branch — never by force — and asks before creating a branch or a tag on the remote (see [Pushing](#pushing))
- 🔀 `c` checks out the selected commit's branch, and refuses while you have uncommitted changes (see [Checking out](#checking-out))
- 🌱 `b` starts a new branch where you are and switches to it (see [Creating a branch](#creating-a-branch))
- 🗑️ `d` deletes the selected commit's branch — local, remote or both — and refuses when its commits are on no other branch (see [Deleting branches](#deleting-branches))
- 🌐 `o` opens the commit's pull request in your browser, on GitHub or Azure DevOps, read from the merge subject and the remote (see [Opening a pull request](#opening-a-pull-request))
- 📋 `y` copies the commit's hash, subject or whole diff (see [Copying](#copying))
- 🔖 `B` jumps to any branch or tag, typing to narrow the list (see [Jumping to a branch or tag](#jumping-to-a-branch-or-tag))
- 🔄 Keeps itself up to date: changes on your machine appear without a keypress, and it can fetch on a timer too (see [Auto-refresh](#auto-refresh))
- 🎨 Beautiful styling with Lip Gloss
- ⌨️  Keyboard navigation (arrow keys, vim-style)
- 🖱️  Mouse support: click a commit to select it, wheel to scroll (see [Mouse](#mouse))
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
- `PgUp/PgDn` or `Ctrl+U/Ctrl+D` - Move ten rows, as vim's half-page keys do
- `Home/End` - Go to the top/bottom of what is on screen; in a box of text, its start/end
- `Ctrl+Home/Ctrl+End` or `g/G` - Go to the first/last commit, or the first/last file in the commit view
- Mouse wheel - Scroll the panel the pointer is over (see [Mouse](#mouse))
- Click - Select the commit under the pointer (see [Mouse](#mouse))
- `Enter` - Show one panel or both (see [One panel at a time](#one-panel-at-a-time))
- `Space` - Open the commit view, `Esc` to come back (see [The commit view](#the-commit-view))
- `p` - Pull: fetch, then fast-forward this branch (see [Pulling](#pulling))
- `P` - Push this branch, or the selected commit's unpushed tag (see [Pushing](#pushing))
- `c` - Check out this commit's branch (see [Checking out](#checking-out))
- `b` - New branch from where you are, switched to (see [Creating a branch](#creating-a-branch))
- `d` - Delete this commit's branch: local, remote or both (see [Deleting branches](#deleting-branches))
- `o` - Open this commit's pull request in a browser (see [Opening a pull request](#opening-a-pull-request))
- `y` - Copy this commit's hash (`y` again), subject (`s`) or diff (`d`) (see [Copying](#copying))
- `r` or `F5` - Reload the repository (see [Reloading](#reloading))
- `f` - Fetch from the remote, then reload (see [Fetching](#fetching))
- `/` - Search commits, then `n` / `N` for next and previous (see [Searching](#searching))
- `h` - Show only the commits that changed a file or directory, picked from a list; `Esc` for all of them again (see [A file's history](#a-files-history))
- `m` - Read more of a long history (see [Long histories](#long-histories))
- `B` - Jump to a branch or tag (see [Jumping to a branch or tag](#jumping-to-a-branch-or-tag))
- `O` - Open another repository (see [Switching repository](#switching-repository))
- `t` - Pick a colour theme (see [Picking a theme](#picking-a-theme))
- `L` - Toggle lane colours in the graph (see [Graph lane colours](#graph-lane-colours))
- `U` - Update to the latest release (shown in the help line when one is available)
- `q` or `Ctrl+C` - Quit

The right end of the bottom line shows where you are in the history, as in
`1,234/5,000 · 24%` — counted from the newest commit, with uncommitted changes as 0.
While the history is cut short (see [Long histories](#long-histories)) it reads
`1,234/5,000+` instead, without a percentage, since the bottom of what has been read
isn't the bottom of the history.

A box with more to read than fits in it says so in its border: `⇣` in the middle of the
bottom edge when there is more below, `⇡` along the top when you have scrolled past the
start. A box whose content fits has neither, so a plain border means you have seen it all.

### Mouse

The wheel scrolls whichever panel the pointer is over: the graph moves the selected
commit, the details panel scrolls the diff. Pointing at a panel is how a mouse says
which one you mean, so it neither needs focus nor takes it — the keyboard keeps
driving whatever it was driving.

One notch moves three lines. Maximised there is only one panel, so the wheel always
belongs to it.

In the commit view the wheel scrolls whichever box the pointer is over, and clicking
a file selects it there too.

Clicking a commit selects it, the same as walking to it with the arrow keys: the
details panel follows and the diff is read afresh. Like the wheel it leaves focus
alone — a click names a commit, not a panel. Clicking where there is no commit does
nothing: the blank rows under a short history, a line of graph between two commits,
the note saying the history was cut short, and the details panel itself all leave the
selection where it is.

### One panel at a time

Gitraffe starts fullscreen: the focused panel has the whole window, which on a fresh
run is the graph. Press `Enter` to bring the other panel back beside it, and `Enter`
again to go back to one. `1`, `2` and `tab` still choose which panel that is, so you
can swap between a full-window graph and a full-window diff without leaving
fullscreen.

The graph uses the extra room rather than just stretching: branch labels that were
truncated fit, the date and author columns come back on a terminal too narrow to show
them beside the details panel, and each commit's message is added at the end of its
row, since the details panel is no longer there to show it. Whatever is left over
after the other columns goes to the message, and it is cut with an `…` when the row
runs out; a window too narrow for a readable message leaves it out entirely.

Whether you left gitraffe maximised is remembered, so a split view stays split the
next time too (see [Remembered preferences](#remembered-preferences)).

### The commit view

Press `Space` to step into one commit. The graph goes away and the window is given over
to three boxes: what the commit is, every file it touched, and the diff of whichever
file is chosen. `Esc` or `q` comes back to the graph, exactly as it was.

```
╭──────────────────────────────────────────────────────────────────────╮
│ Commit: 4a938a6  fix the sign flip on refunds                        │
╰──────────────────────────────────────────────────────────────────────╯
╭[1]-commit───────────╮╭[3]-diff─────────────────────────────────────╮
│ SHA:     4a938a6... ││ parser.go                                   │
│ Author:  sevenam    ││                                             │
│ ─── Message ─────── ││    @@ -18,6 +18,9 @@                        │
│ fix the sign flip   ││ 21 +  if amount < 0 {                       │
╰─────────────────────╯│ 24 -  amount = -amount                      │
╭[2]-files-(3)────────╮│                                             │
│ > parser.go   +2 -2 ││                                             │
│   …/helper.go +1 -0 ││                                             │
│   README.md   +2 -0 ││                                             │
╰─────────────────────╯╰─────────────────────────────────────────────╯
esc: back • 1/2/3: focus box • tab: cycle • ↑/↓/j/k: move      1/3 · 33%
```

`1`, `2` and `3` choose a box and `tab` cycles them; `↑/↓` or `j/k` then move in the
one that has focus — down the file list, or through the details and the diff. Moving
to another file puts its diff at the top, since a line count from the file before it
would mean nothing. Clicking a file selects it, and the wheel scrolls whichever box
the pointer is over.

The right end of the bottom line says which file of the list is chosen, as in
`3/12 · 25%` — the same corner that says where you are in the history on the graph
screen, so a list longer than its box still shows how far down it you are.

The file list is git's own account of the commit rather than a second reading of it:
both the list and the diffs are split out of the same `git show`, so a file cannot be
listed with a diff that belongs to somewhere else. A rename is named at both ends
(`old.go → new.go`), a binary file says `binary` where its counts would be, and a
path too long for the column is cut from the front, keeping the file's own name.

Each line of a diff carries its line number, so a change can be found in the file:
the line's number in the file as the commit left it, or for a removed line the number
it had before. The details panel beside the graph numbers its diff the same way. The
`@@` header stays, for the name of the function git found above the change.

It works on uncommitted changes too: the working-tree row opens like any other
commit, and untracked files are listed — marked `untracked`, since git has never seen
them and so has nothing to compare them with. Selecting one shows the whole file as
added lines, `+` and green, the way new lines in a tracked file look; a binary file
is only named. There the view does more than show: see
[Staging and committing](#staging-and-committing).

Pressing `?` here lists this screen's keys rather than the graph's, and `o` opens the
commit's pull request (see [Opening a pull request](#opening-a-pull-request)).

Why a screen of its own, rather than more boxes beside the graph? The graph is what
gitraffe is for, and splitting the details panel three ways would have taken room
from it on every screen to answer a question you ask on some of them. Here the commit
has the window, and `Esc` gives the graph back untouched.

### Staging and committing

Open the uncommitted changes (`Space` on the row at the top of the graph) and the commit
view becomes the place to put a commit together: choose what goes in, type a message,
commit. Nothing else in gitraffe writes a commit, and this only does on `c` then `Enter`.

```
╭[1]-commit─────────────────╮╭[3]-diff────────────────────────────────╮
│ Uncommitted changes       ││ parser.go  not staged                  │
│ 2 changed, 1 untracked    ││                                        │
╰───────────────────────────╯│ @@ -18,6 +18,9 @@                      │
╭[2]-files-(1-of-3-staged)──╮│  func parse() {                        │
│   ● README.md       +2 -0 ││ -  amount = -amount                    │
│ > ○ parser.go       +2 -2 ││ +  if amount < 0 {                     │
│   ○ notes.txt   untracked ││                                        │
╰───────────────────────────╯╰────────────────────────────────────────╯
esc: back • s: stage file • S: all • c: commit • 3: diff, for hunks and lines • ?: help
```

Each row of the file list says which side it is on: `●` is staged — what the next
commit will hold — and `○` is not. Staged files come first, then unstaged, then
untracked. A file with some changes staged and some not is listed twice, once on each
side, so the diff beside a row is exactly what `s` on that row would move.

| Where | Key | Does |
| --- | --- | --- |
| files | `s` | stage the selected file, or unstage it if it is staged |
| diff | `s` | stage or unstage the hunk under the cursor |
| diff | `v`, move, `s` | pick lines, then stage or unstage just those; `Esc` lets go of them |
| any | `S` | stage everything; when everything is staged already, unstage it all |
| any | `c` | commit what is staged |

Here the diff box has a cursor, which `↑/↓`, `j/k`, the wheel and a click all move; the
box follows it. With nothing picked, `s` takes the hunk the cursor is in, from its
`@@` line to the next. For less than a hunk, press `v` on a line and move: the lines
from there to the cursor are picked, and `s` stages the changed ones among them — so
`v` `s` on one line stages that line. After `s` the cursor stays put and the next hunk
moves up under it, so pressing it again works down the file; on the file list the
selection steps to the next file in the same way.

`c` opens a box for the message, with a subject line and a body:

```
Commit  2 staged files to main

Subject  29
Stop negating refunds

Body  45
The sign was flipped twice.

enter: commit • tab: body • esc: cancel
```

`Enter` in the subject commits. `Tab` moves to the body, where `Enter` is a new line;
`Tab` again goes back to the subject to commit. `Esc` closes the box and keeps what
you typed, so `c` finds it again.

The numbers count down to the 50/72 rule: 50 characters for the subject, which is what
fits in a one-line log, and 72 for each line of the body, which leaves room for git's
indent in an 80-column terminal. `Subject  29` means 29 more fit. Past the limit the
number goes below zero and turns red — `-4` is four too many — but nothing stops you:
they are conventions, not limits. The body is drawn 72 columns wide, so a line that
wraps on screen is one that has run over; the wrap is only on screen, and the line is
committed as you typed it. The body's number is for the line you are typing;
with the cursor in the subject it is for the longest line, so one that runs over is
not forgotten.

After the commit the graph is read again. If changes are left, you stay in the view
with what remains, ready for the next commit; if none are, you are back on the graph
with the new commit selected.

What it will and won't do:

- **Only what is staged is committed.** With nothing staged, `c` says
  `Nothing staged` rather than committing everything.
- **Staging never touches your files.** It copies changes into git's index, and
  unstaging takes them out again; the files on disk stay as they are either way.
- **Hooks run, and are obeyed.** A `pre-commit` or `commit-msg` hook that refuses
  stops the commit, and what it printed is shown in the message box with your message
  still there. Gitraffe never passes `--no-verify`.
- **The message is committed as typed.** It is passed to git directly, no editor
  opens, and a line starting with `#` is kept: there is no template whose comments
  would need stripping.
- **Not during a merge, rebase, cherry-pick or revert, or with unresolved
  conflicts.** There, staging a file means "this conflict is resolved" and a commit
  concludes the operation; both are refused, with the reason on the bottom line, and
  the files are listed as one diff to read. Finish it in a terminal.
- **Not on a detached HEAD.** You can stage, but `c` is refused: a commit made on no
  branch is easy to lose. Check out a branch first (see [Checking out](#checking-out)).
- **A file that changed since it was read is shown again, not staged.** If you edit a
  file after its diff was drawn, the line numbers on screen are of a diff that no
  longer exists; `s` then stages nothing and redraws it.
- **A rename is a deletion and an addition here.** They are two rows, staged one at
  a time; git still records it as a rename when both are committed.

A diff longer than 800 lines is cut on screen, and only the lines shown can be picked;
`s` on the file list stages all of it.

### Jumping to a branch or tag

Press `B` for a list of every branch and tag in the repository, and `Enter` to put the
selection on the one you pick. Typing narrows the list — the match is anywhere in the
name and ignores case, so `fix` finds `bugfix` as well as `fix-the-parser` — and the
arrow keys move through what is left. `Esc` closes it and keeps the commit you had.

```
╭───────────────────────────────────────────────────────────╮
│  Branches and tags                                        │
│  43 refs — type to narrow                                 │
│                                                           │
│  > main          1ff4317  branch                          │
│    add-file-view 5333ac6  branch                          │
│    v0.29.0       1ff4317  tag                             │
│    v0.28.0       5333ac6  tag                             │
│    origin/main   1ff4317  remote                          │
│                                                           │
│  type to filter • ↑/↓: choose • enter: jump • esc: cancel │
╰───────────────────────────────────────────────────────────╯
```

The branch you are on comes first, so the list opens where you are. Then the rest of
the local branches, then tags newest first — a release you are looking for is far
likelier to be a recent one, and tag names sort in no useful order anyway, with `v1.10`
landing before `v1.9`. The remote-tracking copies come last, being the ones you least
often mean.

An annotated tag is resolved to the commit it points at rather than to the tag object,
since the tag object is in no row of the graph.

The list is read from git, not from the commits on screen, so it holds refs whose
commits have not been loaded yet — which are exactly the ones worth jumping to, being
too old to scroll to. Picking one reads enough history to reach it first: gitraffe
works out how deep the commit is and reads that far, then lands on it. The bottom line
says so while it happens, since reading a long history takes a moment.

### Copying

Press `y` and the bottom line asks what to copy from the commit: `y` again for its full
hash (so `yy`, as in vim, gives you what a `git revert` or `git cherry-pick` wants), `s`
for its subject, or `d` for its whole diff. Any other key cancels. It works in the commit
view too, on the commit that view is open on.

The diff is read afresh rather than taken from the details panel, which stops after a
few hundred lines, so what you paste is the complete patch and `git apply` takes it. On
the uncommitted-changes row, `d` copies `git diff HEAD`; there is no hash or subject.

Gitraffe uses the system clipboard where there is one. Where there isn't — on Linux
without `xclip`, `xsel` or `wl-copy`, or over SSH — it asks the terminal to hold the text
instead (OSC 52), and says it was sent rather than copied, since a terminal that doesn't
support that ignores it silently.

### Opening a pull request

Press `o` on a commit that came from a pull request and its page opens in your
browser. It works on the graph and in the commit view, on whichever commit that
screen is about.

Nothing in git records the pull request a commit came from — a PR is the forge's
idea, not git's. What survives in the repository is the subject the forge wrote when
it merged. Gitraffe reads GitHub's and Azure DevOps's:

```
Merge pull request #59 from sevenam/add-graph-colors   ← GitHub, a merge commit
Teach the parser about nested groups (#59)             ← GitHub, a squashed pull request
Merged PR 59: Teach the parser about nested groups     ← Azure DevOps, however it was completed
```

A commit *inside* a pull request has no such subject, so gitraffe looks for the merge
that brought it onto the branch you are on and reads that one's. `o` on any commit
of a merged pull request opens the pull request.

That merge is asked before a `(#59)` at the end of the commit's own subject. The
brackets are how GitHub marks a squashed pull request, but they are also how people
mention an issue — `Fix the sign flip (#12)` — and following that number would open
issue 12. So the brackets are only believed on a commit no merge brought in, which is
where squashed pull requests are. On a squashed commit that names an issue by hand,
the issue is still what opens: nothing in the repository tells the two apart.

The rest of the address comes from the remote, so `git@github.com:sevenam/gitraffe.git`
and `https://github.com/sevenam/gitraffe.git` both lead to the same page. `origin` is
used when there is one, otherwise the first remote git lists; any username or password
written into the remote is dropped rather than carried into a browser.

The remote also says which of the two it is, so there is nothing to configure. An
Azure DevOps remote is recognised however it is written, and each of these opens
`…/_git/gitraffe/pullrequest/59`:

```
https://sevenam@dev.azure.com/sevenam/tools/_git/gitraffe
git@ssh.dev.azure.com:v3/sevenam/tools/gitraffe
https://sevenam.visualstudio.com/tools/_git/gitraffe
https://tfs.example.com/tfs/Collection/tools/_git/gitraffe   ← a server of your own
```

Any other host is read the GitHub way, with the address `/pull/<number>`. Each host's
subjects are only read on that host's remotes: a repository moved from GitHub to
Azure DevOps keeps its old merge commits, and their numbers would be someone else's
pull request there.

Some commits leave nothing behind to find, and gitraffe says so on the bottom line
rather than appearing to ignore the key, as it does when the repository has no
remote: a rebase merge on either host, a commit not yet merged, or an Azure DevOps pull request whose commit
message was rewritten without the `Merged PR 59:` at the front.

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

### A file's history

To see what happened to one file, press `h` on the graph and start typing its name.
The list offers every file git tracks, and every directory, best match first: a name
that starts with what you typed, then one that contains it, then a path that does,
then a path holding its letters in order, so `tuikeys` finds `internal/tui/keys.go`.

- `↑` / `↓` choose, and `Enter` shows the chosen file's history.
- `Tab` puts the chosen path in the box, so `Tab` on a directory lists what is
  under it.
- When nothing matches, `Enter` uses the path as typed, from the top of the
  repository: a file deleted since is not in the list, but its history is there.
- `Enter` on an empty box clears the filter, and `Esc` closes the list unchanged.

`h` does the same in the commit view, where the file is the one selected in its file
list. Either way the graph then holds only the commits that changed that file (or
anything under that directory), on every branch, joined up as they descend from each
other.

The top box says what the graph is filtered to, and `Esc` on the graph brings every
commit back, with the one you were on still selected. Reloading, fetching and `m`
keep the filter; opening another repository drops it. Opening a commit while
filtered starts its file list on the filtered file.

The row of uncommitted changes is left out while filtered, since it counts every
change in the working tree, not just the file's. A rename is not followed: the
history is of the file under its current name, as `git log -- path` gives it.

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

Fetching writes to your repository, so unless you turn
on [auto-fetch](#auto-refresh) it only ever happens when you press the key — never on
a timer, and never at startup. What it writes is limited to remote-tracking refs: your
branches, your tags and your working
tree are not touched, and `--prune` only drops `origin/...` refs whose branch the
remote no longer has.

It never prompts. A remote that wants a password, or an SSH key with a passphrase,
fails with git's own message on the bottom line instead of a prompt fighting the
graph for the screen, and a remote that never answers is given up on after a minute.

### Pulling

Press `p` to catch your branch up with its upstream. It fetches, exactly as `f`
does, and then moves the branch forward to where the remote is — a fast-forward, and
only ever a fast-forward.

That limit is the point. A fast-forward makes no commit and cannot conflict, and git
refuses it, changing nothing, if an uncommitted edit of yours is in the way. Anything
more than that — merging, rebasing — can stop halfway in a state that has to be
sorted out by hand, and gitraffe is not the place to do that. So when it can't
fast-forward it leaves everything as it was and says why on the bottom line:

| The bottom line says | What happened |
| --- | --- |
| `Pulled 3 commits from origin/main` | the branch moved forward |
| `Already up to date with origin/main` | there was nothing new; being ahead counts as up to date |
| `Not pulled: main and origin/main have diverged (↑2 ↓3) — merge or rebase in a terminal` | both sides have commits the other lacks; nothing was changed |
| `Pull failed: …` | git's own reason — most often an uncommitted edit to a file the incoming commits change |
| `Nothing to pull — …` | the branch tracks no remote branch, or `HEAD` is not on a branch |

It doesn't read `pull.rebase` or any other pull setting: what `p` does is the same
in every repository. And it is never done for you — [auto-fetch](#auto-refresh) can
show that there is something to pull, but moving your branch always takes the key.

### Pushing

Press `P` to send your branch to the remote branch it tracks. It is the one key that
puts your work somewhere it cannot be taken back from, so it is a capital — and it only
ever moves the remote branch *forward*.

That limit is the same one [pulling](#pulling) has, seen from the other side. If the
remote branch has commits yours lacks, sending yours would mean overwriting them, and
gitraffe never forces a push: git refuses, nothing changes at either end, and the bottom
line says to pull first. The repository's `pre-push` hook runs as it would from a
terminal, and is never skipped.

Two things are asked about before anything is sent, because each puts a new name on the
remote for everyone else to see:

- **An unpushed tag on the selected commit.** If the commit carries a tag marked `↑`
  (see [Tag sync](#tag-sync)), the bottom line offers it in place of the branch:
  `Push: t tag v1.2 • b branch main • esc cancel`. `t` pushes the tag and nothing else,
  `b` pushes the branch as usual, and any other key sends nothing. Several unpushed tags
  are numbered, as the branches of a [checkout](#checking-out) are.
- **A branch with no branch on the remote.** A branch that tracks nothing has no remote
  branch to move, so a box asks what to call the one to create. Your branch's own name
  is already filled in: `Enter` accepts it, or type another first. With more than one
  remote, `Tab` moves between them; it starts on the one git itself would push to
  (`branch.<name>.pushRemote`, `remote.pushDefault`, else `origin`). From then on the
  branch tracks what was created, and `P` asks nothing.

| The bottom line says | What happened |
| --- | --- |
| `Pushed 3 commits to origin/main` | the remote branch moved forward |
| `Pushed feature to origin/feature, a new branch it now tracks` | the branch was created on the remote |
| `Pushed tag v1.2 to origin` | the tag was created on the remote; the branch was not touched |
| `Nothing to push — origin/main already has every commit of main` | there was nothing new to send |
| `Not pushed: origin/main has commits main lacks — pull first (p), or merge or rebase in a terminal` | the push would have overwritten them; nothing was changed |
| `Not pushed: origin already has a tag v1.2, on another commit` | moving a tag on the remote is a forced push; nothing was changed |
| `Push failed: …` | git's own reason, or the hook's: no access, a protected branch, a remote that can't be reached |
| `Nothing to push — …` | `HEAD` is not on a branch, or the repository has no remote |

What `P` sends is exactly one branch to one branch, or one tag: `push.default`,
`push.followTags` and the like are not read, so it does the same in every repository.
Like every call to a remote it never prompts for a password. And it is never done for
you: nothing is pushed without the key.

### Checking out

Press `c` on a commit to switch to its branch. It switches straight away and says
where it went on the bottom line: `Switched to feature`. Only a commit with several
branches asks which, with a numbered list —
`Check out: 1 main • 2 origin/release • esc cancel` — where any key but a number
cancels.

- A **local branch** is switched to as it is.
- A **remote branch** with no local branch of the same name on that commit is
  switched to through a new local branch that tracks it, so `origin/feature` gives
  you `feature`, set up to pull from `origin/feature`. When the local branch is on the
  same commit, only the local one is offered.
- A commit with **no branch** is checked out on no branch (a "detached HEAD"), which
  the bottom line says afterwards. A commit that has a branch is never offered detached: commits
  made on no branch are easy to lose.

It uses `git switch`, git's newer command for changing branches (since 2.23), rather
than `git checkout`, which also restores files and can overwrite edits if given the
wrong argument.

Like [pulling](#pulling), it only does what cannot lose work. While any tracked file
has uncommitted changes it changes nothing and says
`Not checked out: you have uncommitted changes — commit or stash them first`; git
would carry such changes over to the other branch, which is how work gets committed
in the wrong place. Untracked files don't count: they belong to no branch, and git
refuses on its own if the switch would overwrite one. If git refuses for any other
reason, the bottom line gives git's reason after `Checkout failed:`.

### Creating a branch

Press `b` to start a new branch. A box asks for its name; `Enter` makes the branch and
switches to it, and the bottom line says `Created feature and switched to it`. `Esc`
closes the box with nothing made.

```
╭─────────────────────────────────────────────────────────────╮
│                                                             │
│  New branch  starts where you are, not at the selection     │
│                                                             │
│  From  main  4a938a6 fix the sign flip on refunds           │
│  Name  feature/refunds                                      │
│                                                             │
│  enter: create it and switch to it • esc: cancel            │
│                                                             │
╰─────────────────────────────────────────────────────────────╯
```

- **It starts where you are**: at the commit you have checked out, named in the box
  by its branch, hash and subject, and not at the commit selected in the graph. The
  selection is wherever reading the history last left it, and a branch started there
  would start somewhere you never chose. To branch from another commit, check it out
  with `c` first, then press `b`.
- **Uncommitted changes come along.** Unlike [checking out](#checking-out), nothing
  is refused over them: no file is touched, staged or not, so they are exactly as
  they were, on a branch that is the old one under a new name. That makes `b` the way
  to move work you started on the wrong branch.
- **On a detached HEAD** it is the way back onto a branch, keeping whatever was
  committed there. The box then reads `From  HEAD (detached)  4a938a6 …`, the commit
  being all there is to say where the branch starts.
- **The name is checked while the box is open**: one git would not accept, or one a
  branch already has, is said in the box, which stays open for another.
- **The new branch tracks nothing**, whatever `branch.autoSetupMerge` says, so the
  first `P` asks where to create it on the remote (see [Pushing](#pushing)) rather
  than pushing to the old branch's upstream.
- **Not during a merge or a rebase**: the operation belongs to the branch it was
  started on, so the bottom line says to finish it first.

It is `git switch --no-track -c <name>`.

### Deleting branches

Press `d` on a commit to delete a branch that is on it. A list opens with a row for
each thing that can go:

```
Delete branch

> local (feature)
  remote (origin/feature)
  local & remote (feature + origin/feature)
```

`↑`/`↓` choose, `Enter` deletes the row picked, and `Esc` closes the list with
nothing deleted. A commit with several branches lists each of them.

- A local branch and a remote branch of the same name are offered together only when
  both are on the selected commit. If the remote one is on another commit, select
  that commit to delete it.
- The branch you have checked out is not offered locally; its remote branch still is.
- Deleting a remote branch is a push to that remote, so it needs the same access a
  `git push` does.

Like [pulling](#pulling) and [checking out](#checking-out), it only does what cannot
lose work:

- A branch whose commits are on no other branch or tag is not deleted; the bottom
  line says `Not deleted: its commits are on no other branch`. Merge it first, or
  delete it in a terminal if you mean to throw the commits away. For "local and
  remote" that means a third branch must hold them, since the two would otherwise be
  each other's only copy.
- A remote branch is deleted only if it is still where your last fetch saw it. If
  someone has pushed to it since, nothing is deleted and the bottom line asks you to
  fetch.

When both are asked for, the remote branch goes first: if that fails, nothing has
changed.

### Reloading

Press `r` or `F5` to read the repository again: the graph, the ahead/behind counts
and the tag marks are all rebuilt together. Changes made on this machine are picked
up without asking (see [Auto-refresh](#auto-refresh)), so the key is for when you
don't want to wait, or have turned that off.

The screen stays as it is while that happens: only the bottom line changes, to
`Refreshing…` and then `Refreshed`, so a refresh that found nothing new leaves
everything else exactly where it was.

Your place is kept. The commit you had selected stays selected, even though new
commits have pushed it down the list, and the same panel keeps focus, with the same
layout and scroll position. If that commit is gone — amended or rebased away — the
selection falls back to the newest commit.

### Auto-refresh

A commit made in another terminal, a checkout, an edit: gitraffe notices these on its
own. Every 30 seconds, and whenever its terminal window gets the focus back, it
checks whether anything changed — where `HEAD` is, the branches and tags, the
uncommitted files — and reads the repository again only if something did. A check
that finds nothing draws nothing, and one that finds something changes only what
changed: nothing is said on the bottom line, and your place is kept as it is for `r`.

The check is a couple of quick git commands run in the background, and it never
takes git's index lock, so it won't make a git command you are typing elsewhere fail.
It waits while a box is open over the graph — the help, a picker, the search prompt —
and catches up once it is closed.

Auto-refresh only looks at your machine. Seeing what is new on the remote still
takes a fetch, which you can also have done for you. Both are set in `settings.yml`
in your config directory, in seconds:

```yaml
auto_refresh: 30   # the default; 0 turns it off
auto_fetch: 300    # off unless you add it; fetch every five minutes
```

Auto-fetch is off by default because fetching reaches the network and writes to
the repository, and that is yours to opt into. When on, it runs the same fetch as
`f`, silently: what it brings shows up in the graph, and nothing is said if there
was nothing new. If a fetch fails — no network, a key that needs unlocking — the
bottom line says so once and auto-fetch stops, rather than failing again every
interval; pressing `f` tries again, and a fetch that works starts it off again.

Intervals shorter than 2 seconds for refreshing and 30 seconds for fetching are
raised to those.

### Switching repository

Press `O` to open another repository without leaving gitraffe. The box starts with
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

`O` also works from the error screen, so starting gitraffe in a folder that isn't a
repository isn't a dead end. The recent list is kept in `settings.yml` in your config
directory (see [Picking a theme](#picking-a-theme)), next to your theme choice.

### Remembered preferences

Gitraffe writes what you chose to `settings.yml` in your config directory, so the
next run starts where the last one left off:

| Key | What it holds |
| --- | --- |
| `theme` | the theme picked with `t` (see [Picking a theme](#picking-a-theme)) |
| `recent_repos` | the repositories the `O` box offers |
| `lane_colours` | whether the graph is coloured per branch (`L`) |
| `focused_box` | the panel that had focus, `1` or `2` |
| `maximised` | whether that panel filled the window (`Enter`) |

`auto_refresh` and `auto_fetch` live in the same file but are yours to write: see
[Auto-refresh](#auto-refresh).

`lane_colours`, `focused_box` and `maximised` are written when gitraffe exits, not as you press the keys, since `L`
and `tab` are pressed often and the file is only read at startup. Delete the file,
or any single key in it, to go back to the defaults: fullscreen, lane colours on, the
graph focused. A `focused_box` naming a panel that doesn't exist is ignored.

## Uncommitted changes

When the working tree isn't clean, a row sits above the newest commit with a hollow
marker and a count — `3 changed, 1 untracked` — so the graph answers "what have I
got in progress" as well as "where am I". A clean tree adds no row.

Staged and unstaged changes are one number: both are work in progress. Select the row
and it shows the stats and the diff of everything against `HEAD`, staged changes
included. Untracked files have no diff, so they are listed by name instead. Open the
row (`Space`) to read them, to see what is staged and what is not, and to stage and
commit (see [Staging and committing](#staging-and-committing)).

The row follows your edits on its own (see [Auto-refresh](#auto-refresh)); press `r`
if you don't want to wait for the next check.

## Reading the graph

Every commit has one row, and every line is drawn to the commit it belongs to:

```
◆─╮        ← a merge: the branch on the right was merged into this commit
│ ●        ← a commit on that branch
│ ●
│ │ ●      ← a commit on another branch, not merged yet
●─┴─╯      ← both branches were started from this commit
```

A branch's line runs from the commit it was branched from to the commit it was merged
into, in one column and one colour, however many other commits are listed in between.
So where a line meets a commit is where it really starts or ends.

| Drawn | Meaning |
| --- | --- |
| `>` | the commit that is checked out (HEAD), in the column left of the labels; the selected commit is the highlighted row |
| `●` | a commit; `◉` when selected |
| `◆` | a merge commit, with more than one parent; `◈` when selected |
| `○` | uncommitted changes (see [Uncommitted changes](#uncommitted-changes)) |
| `─╮` | a line leaving a merge for a branch that was merged in |
| `─╯` | a branch's line arriving at the commit it was started from |
| `─┴─` | another branch arriving at the same commit, with one beyond it |
| `─┤` | a branch ends at this commit and another one, merged here, begins — both in one column, which is what a run of pull requests looks like |
| `├─◆` | a merge whose other parent is on a line that was already there, as when main is merged into a branch |
| `─│─` | a line crossing one it has nothing to do with |

Gitraffe draws this itself instead of showing `git log --graph`. Git puts each
connection on a row of its own between the commits, and folds a branch's line into
its neighbour as soon as the branch has no more commits — rows above the commit it
was branched from, so the branch looks as if it starts out of the side of a line.
The commits are listed in the same order git's graph lists them.

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
thing: a column one branch has finished with is used again by an unrelated one further
down, so colouring by column would give two unrelated branches the same colour. A
branch here means a commit and the first parents it leads back through; a line takes
the colour of the branch at its upper end, and the stroke from a merge to the branch
it merged in takes that branch's.

Press `L` to turn it off and render the whole graph in the theme's `graph` colour, the
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
press `f` (see [Fetching](#fetching)) to bring them up to date, and `P` to push what is
ahead (see [Pushing](#pushing)).
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

Pressing `P` on a commit with a tag marked `↑` offers to push that tag (see
[Pushing](#pushing)).

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
