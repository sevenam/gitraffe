# CLAUDE.md

Guidance for working in this repository. `README.md` is the user documentation; this file is the map
of the code. `refactoring.md` records how the code got from one flat package to this layout, and
what was deliberately left for later.

## Commands

```
go build -o gitraffe.exe .
go test ./...
go test ./internal/git
go test ./internal/tui -run TestFollowWindow
go vet ./...
```

The tests need `git` on the PATH: most build throwaway repositories in temp directories and run the
real loaders against them, so the full run takes about a minute (nearly all of it in `internal/tui`).
Nothing touches the network or the user's config directory.

A release is a tag matching the `version` constant in `main.go`; `release.yml` refuses any other.

## Layout

```
main.go              arguments, logging, wiring; the version constant
themes/              bundled *.yml themes, embedded by embed.go
internal/
  git/               everything asked of git, and the parsing of what it prints
  theme/             colours: defaults, theme files, the list the picker offers
  config/            settings.yml, the config directory, the log path
  selfupdate/        release check, version comparison, binary swap
  tui/               the model, Update, View, and every screen and overlay
  gittest/           throwaway repositories for tests
```

Dependencies point one way: `tui` uses all the others; `theme` uses `config` and `themes`; `git`,
`config` and `selfupdate` use nothing of ours. **`internal/git` must not import Bubble Tea or
lipgloss, and `internal/tui` must not run git itself** — it calls `internal/git`. That boundary is
the point of the split; keep it.

`main.go` stays in the repository root because `go install github.com/sevenam/gitraffe@latest`, the
release build and the release version check all depend on it being there.

### internal/git

| File | Holds |
| --- | --- |
| `run.go` | `Run`, the one way to run a git command; the no-prompt environment for remote calls |
| `repo.go` | opening a repository, branch and HEAD info, `Toplevel`, `Remotes` |
| `log.go` | `Commit`, `DisplayRow`, `LoadGraph` (history with its graph), `LoadCommits` (fallback) |
| `lanes.go` | which lane each graph character belongs to, read from git's own colours |
| `refs.go` | `ParseRefs`, merged-branch names, `ListRefs`, `CommitDepth` |
| `diff.go` | `ShowCommit`, `WorkingTree`, `Status`, splitting a patch per file |
| `sync.go` | ahead/behind counts, `Fetch` |
| `remote_tags.go` | which tags the remotes hold |
| `remote.go` | pull request numbers and web addresses from commit subjects and remotes |

### internal/tui

| Area | Files |
| --- | --- |
| Entry and state | `run.go` (`Run`, `Version`, `LogPath`), `model.go` (all state), `types.go` (aliases for the git types), `messages.go` |
| Event loop | `update.go` (`Init`, `Update`, message handling), `keys.go` (who owns the keyboard), `mouse.go` |
| Loading | `load.go` (repository into model), `diff_load.go`, `reload.go`, `more_commits.go`, `fetch.go`, `remote_tags.go`, `working_tree.go` |
| Screen assembly | `view.go` (`View`), `layout.go` (how the width is shared), `boxes.go` (clipping, labels, overlays), `scroll_marks.go`, `background.go` |
| Main screen | `repo_info.go` (top box), `graph_panel.go` (commit list and its scroll window), `branch_label.go`, `details_panel.go`, `status_line.go` |
| Other screens and overlays | `commit_view.go`, `ref_picker.go`, `repo_switcher.go`, `theme_picker.go`, `search.go`, `help.go`, `pull_request.go` |
| Looks | `styles.go` (package-level styles built from the theme), `lane_colours.go` |
| Session | `preferences.go` (what is remembered between runs), `selfupdate_tui.go` (the in-app update prompt), `terminal.go` |

### Tests

A test file sits beside what it tests, in the same package: `internal/git/lanes_test.go`,
`internal/tui/layout_test.go`. Tests that read git output directly are in `internal/git`; tests that
go through the model are in `internal/tui`.

`gittest.Fixture` and `gittest.NewRepo` build repositories. In `internal/tui`,
`testhelpers_test.go` holds what more than one test file uses — `testModel`, `keyPress`, `press` and
`res` drive the model — and its `TestMain` sets the default theme and a fixed `Version`, so no test
depends on the machine it runs on.

## Things that are easy to break

- **One answer per question, shared by drawing and input.** `graphWindow` says which rows are on
  screen and `currentLayout` says how the width is divided; the renderer and the mouse both ask
  them. A second calculation that differs by one row selects the commit above the one clicked.
- **`Update` records the scroll window after every message.** The selection moves from a dozen
  places; the wrapper in `update.go` is what stops the window drifting after any of them.
- **Late answers are dropped.** Every asynchronous message carries the `repoPath` it was started
  for and is ignored if the repository has since been switched.
- **Lipgloss `Height` pads but never clips.** Content is cut to size by hand (`trimToHeight`,
  `fitDetails`), and `View` must return exactly `windowHeight` lines.
- **Every styled piece ends in a reset.** That is why the selected row sets its background on each
  piece and why `paintBackground` re-applies the theme background after every reset.
- **A lane is identified by the colour git gave it, not its column.** Git moves lanes sideways and
  reuses columns; see `graphLanes` in `internal/git/lanes.go`.
- **Refs are read with `--decorate=full`.** Short names cannot tell a local branch from a remote one.
- **The theme and the styles are package-level state.** `theme.Current` holds the colours and
  `internal/tui/styles.go` the styles built from them; anything changing the theme goes through
  `setTheme` so the styles are rebuilt.
- **`settings.yml` is read, changed and written per field**, so one setting never drops another.

## Style

Comments explain why a decision was made, especially where the obvious alternative is wrong; skip
ones that restate the code. British spelling (`colour`, `maximised`, `centre`). Gitraffe opens any
repository, so design and test against arbitrary histories rather than this repository's own.
