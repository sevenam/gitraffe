# CLAUDE.md

Gitraffe is a terminal git history viewer in Go (Bubble Tea + lipgloss). `README.md` is the user
documentation; this file is the map of the code. Use the tables below to go straight to the file
that holds a thing instead of reading packages end to end.

## Commands

```
go build -o gitraffe.exe .
go test ./internal/git                         # ~5s: parsing and graph layout
go test ./internal/tui -run TestFollowWindow   # one test; prefer this while iterating
go test ./...                                  # ~40s, nearly all in internal/tui
go vet ./... && gofmt -l .
```

The tests need `git` on the PATH: most build throwaway repositories in temp directories and run the
real loaders against them. Nothing touches the network or the user's config directory. Run the
package you changed with `-run` while iterating, and the full suite once before committing.

A release is a tag matching the `version` constant in `main.go`; `release.yml` refuses any other.
Don't change `version` unless asked.

## Working here economically

- `README.md` is about 35 KB. Don't read it whole: `grep -n '^#' README.md` lists its sections,
  then read only the one you need.
- Find a symbol with `grep -rn 'func Name' internal/` rather than opening files to look.
- Most files are under 400 lines; the largest are `internal/tui/commit_view.go` and
  `internal/tui/repo_switcher.go`.

## Layout

```
main.go              arguments, logging, wiring; the version constant
themes/              bundled *.yml themes, embedded by embed.go (adding a .yml is enough)
internal/
  git/               everything asked of git, and the parsing of what it prints
  theme/             colours: defaults, theme files, the list the picker offers
  config/            settings.yml (the Settings struct), the config directory, the log path
  selfupdate/        release check, version comparison, binary swap
  tui/               the model, Update, View, and every screen and overlay
  gittest/           throwaway repositories for tests
```

Dependencies point one way: `tui` uses all the others; `theme` uses `config` and `themes`; `git`,
`config` and `selfupdate` use nothing of ours (outside tests, which may use `gittest`).
**`internal/git` must not import Bubble Tea or lipgloss, and `internal/tui` must not run git
itself** — it calls `internal/git`. That boundary is the point of the split; keep it.

`main.go` stays in the repository root because `go install github.com/sevenam/gitraffe@latest`, the
release build and the release version check all depend on it being there.

### internal/git

| File | Holds |
| --- | --- |
| `run.go` | `Run`, the one way to run a git command; the no-prompt environment for remote calls |
| `repo.go` | opening a repository, branch and HEAD info, `Toplevel`, `Remotes` |
| `log.go` | `Commit`, `DisplayRow`, `IsCommitMarker`, `LoadGraph` (reads the history, then lays it out), `LoadCommits` (fallback) |
| `layout.go` | the graph drawing: one row per commit, each connection on the row of the commit it belongs to; `commitPaths` |
| `refs.go` | `ParseRefs`, merged-branch names, `ListRefs`, `CommitDepth` |
| `diff.go` | `ShowCommit`, `WorkingTree`, `Status`, splitting a patch per file, `Patch` (uncut, for copying) |
| `sync.go` | ahead/behind counts, `Fetch` |
| `pull.go` | `Pull`: fetch, then fast-forward or nothing |
| `remote_tags.go` | which tags the remotes hold |
| `fingerprint.go` | one string for "would a reload draw anything different", for auto-refresh |
| `remote.go` | `PullRequestNumber`, `PullRequestURL`: from commit subjects and remotes, for GitHub and Azure DevOps |

### internal/tui

| Area | Files |
| --- | --- |
| Entry and state | `run.go` (`Run`, `Version`, `LogPath`), `model.go` (all state), `types.go` (aliases for the git types), `messages.go` |
| Event loop | `update.go` (`Init`, `Update`, message handling), `keys.go` (`handleKey`: who owns the keyboard, and every key binding), `mouse.go` |
| Loading | `load.go` (repository into model), `diff_load.go`, `reload.go`, `auto_refresh.go` (the refresh and fetch timers), `more_commits.go`, `fetch.go`, `pull.go`, `remote_tags.go`, `working_tree.go` |
| Screen assembly | `view.go` (`View`), `layout.go` (`currentLayout`: how the width is shared), `boxes.go` (clipping, labels, overlays, `trimToHeight`), `scroll_marks.go`, `background.go` (`paintBackground`) |
| Main screen | `repo_info.go` (top box), `graph_panel.go` (commit list and `graphWindow`), `branch_label.go`, `details_panel.go` (`fitDetails`), `status_line.go` |
| Other screens and overlays | `commit_view.go`, `ref_picker.go`, `repo_switcher.go`, `theme_picker.go`, `search.go`, `help.go`, `pull_request.go` (also opens the browser), `copy.go` (the `y` prompt and the clipboard) |
| Looks | `styles.go` (package-level styles built from the theme), `lane_colours.go` |
| Session | `preferences.go` (what is remembered between runs), `selfupdate_tui.go` (the in-app update prompt), `terminal.go` |

### Common changes touch more than one place

- **A key binding:** `keys.go`, the hand-written `helpSections` in `help.go`, and README's
  "Keyboard Shortcuts". A key missing from the help is a key nobody finds.
- **A setting:** the `Settings` struct in `internal/config/config.go` (with `omitempty`), the code
  that reads it, and the README section that documents it.
- **A theme:** a `.yml` in `themes/`; it is embedded and listed automatically.
- **A new git query:** a function in `internal/git` with a test there, called from `internal/tui`
  through an asynchronous message that carries `repoPath`.

### Tests

A test file sits beside what it tests, in the same package: `internal/git/layout_test.go`,
`internal/tui/layout_test.go`. Tests that read git output directly are in `internal/git`; tests that
go through the model are in `internal/tui`.

`gittest.Fixture` and `gittest.NewRepo` build repositories. In `internal/tui`,
`testhelpers_test.go` holds what more than one test file uses — `testModel`, `keyPress`, `press` and
`res` drive the model — and its `TestMain` sets the default theme and a fixed `Version`, so no test
depends on the machine it runs on. Read that file before writing a `tui` test rather than inventing
new helpers.

## Things that are easy to break

- **One answer per question, shared by drawing and input.** `graphWindow` says which rows are on
  screen and `currentLayout` says how the width is divided; the renderer and the mouse both ask
  them. A second calculation that differs by one row selects the commit above the one clicked.
- **`Update` records the scroll window after every message.** The selection moves from a dozen
  places; the wrapper in `update.go` is what stops the window drifting after any of them.
- **Late answers are dropped.** Every asynchronous message carries the `repoPath` it was started
  for and is ignored if the repository has since been switched.
- **A reload nobody asked for must go unnoticed.** Auto-refresh compares fingerprints and
  reloads only on a difference, skips while anything is open over the graph, says nothing, and
  never asks the remotes for tags. Each timer is one self-renewing tick for the whole session;
  a handler that returns without asking for the next tick stops it for good.
- **Lipgloss `Height` pads but never clips.** Content is cut to size by hand (`trimToHeight`,
  `fitDetails`), and `View` must return exactly `windowHeight` lines.
- **Every styled piece ends in a reset.** That is why the selected row sets its background on each
  piece and why `paintBackground` re-applies the theme background after every reset.
- **Gitraffe draws the graph itself; it does not show `git log --graph`.** A lane stays open, in
  one column, from a commit down to the row of its parent, and every connection is drawn on the row
  of the commit it belongs to. Closing a lane early to save a column is what makes a branch look
  like it starts out of the side of a line; see `internal/git/layout.go`.
- **A line's colour is a branch path, not a column.** Columns are reused; `commitPaths` says which
  branch a commit is on, and a line takes the path of the commit at its upper end.
- **Refs are read with `--decorate=full`.** Short names cannot tell a local branch from a remote one.
- **A commit has more than one marker.** `●` for a commit and `◆` for a merge, each with a
  ringed form when selected. Code looking for "the commit on this row" asks
  `git.IsCommitMarker`, not for a particular character.
- **The theme and the styles are package-level state.** `theme.Current` holds the colours and
  `internal/tui/styles.go` the styles built from them; anything changing the theme goes through
  `setTheme` so the styles are rebuilt.
- **`settings.yml` is read, changed and written per field** (`config.Load`, change one field,
  `config.Save`), so one setting never drops another.
- **Gitraffe only does to a repository what cannot lose work or need resolving.** That is a
  fetch, and a fast-forward of the current branch; a merge, a rebase, a checkout or a push is
  not. Moving the branch always takes a key press — only fetching may run on a timer, and only
  when asked for in `settings.yml`.

## Style

Comments explain why a decision was made, especially where the obvious alternative is wrong; skip
ones that restate the code. British spelling (`colour`, `maximised`, `centre`). Gitraffe opens any
repository, so design and test against arbitrary histories rather than this repository's own.
