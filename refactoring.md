# Restructuring

Gitraffe grew as one flat `package main`: 25 source files (~6,900 lines) and 30 test files
(~6,000 lines) in the repository root. This document records why that became hard to navigate, the
structure it was moved to, and what is left. `CLAUDE.md` holds the current file map.

## What made the code hard to navigate

- **Catch-all files.** `ui.go` (1,130 lines) held `View`, the panel layout maths, the status line,
  the graph window, the details panel and the box helpers. `git.go` (553 lines) mixed running git,
  parsing its output, graph building and label-width sizing.
- **Names that misled.** `input.go` was the Bubble Tea `Update` while `update.go` was the
  self-updater; `theme_catalogue.go` also held `settings.yml` and the preferences; `messages.go` ran
  `git show`; `main.go` declared the style globals; `repo_switcher.go` owned the reload logic;
  `lanes.go` mixed lane parsing with lipgloss palettes.
- **Git was called from eight files in three styles** — `m.git(...)`, raw `exec.Command`, and
  closures inside commands — so nothing said where git access lived.
- **Tests named by feature, not by source.** 16 of 30 test files had no matching source file, and
  shared helpers lived in whichever test happened to need them first.
- **Everything hung off `model`**: 83 methods across 15 files, plus the theme and style globals.
- **No map.** The README is user documentation; nothing described the code's layout.

## The structure now

```
main.go                  arguments, logging, wiring, version const
themes/                  *.yml + embed.go (exports the embedded FS)
internal/
  git/                   everything that shells out to git or parses its output
  theme/                 colours, parsing, the catalogue the picker offers
  config/                settings.yml, config and log directories
  selfupdate/            release check, version compare, binary swap
  tui/                   model, Update, View; one file per screen or overlay
  gittest/               the shared test repository fixtures
```

Constraints that shaped it:

- **`main.go` stays in the root.** `go install github.com/sevenam/gitraffe@latest`, the
  `go build .` in `release.yml`, and the release check that reads the version out of `main.go` all
  depend on it. A `cmd/gitraffe` layout would break all three.
- **`themes/` stays in the root.** The README points users at it, and `go:embed` cannot reach up out
  of a subpackage, so a small `embed.go` there exports the files.
- **`tui` stays one package.** The pickers, search and commit view are all methods on `model`;
  splitting them per overlay would mean exporting most of the model for no gain.
- **`git` does not import Bubble Tea or lipgloss, and `tui` does not run git.** This is the
  boundary worth having the compiler enforce: it stops git calls spreading through the UI again.

## What was done

### Step 1 — pure moves inside the root package

- [x] Split `ui.go` into `view.go`, `layout.go`, `status_line.go`, `repo_info.go`,
      `graph_panel.go`, `details_panel.go`, `branch_label.go` and `boxes.go`.
- [x] Split `git.go` by what it read: repository, log, refs, upstream sync, diffs.
- [x] Rename `update.go` → `selfupdate.go` and `input.go` → `update.go`; gather the self-update
      prompt and commands into `selfupdate_tui.go`.
- [x] Move settings, the style globals, the reload logic and the lane palette into files named
      for them.
- [x] Collect shared test helpers into `testhelpers_test.go`; rename test files to sit beside
      what they test.
- [x] Add `CLAUDE.md`.

### Step 2 — the leaf packages

- [x] `internal/selfupdate`. `Check` takes the running version as an argument instead of reading
      a constant.
- [x] `internal/config`: `settings.yml`, the config directory, the log path.
- [x] `themes/embed.go` and `internal/theme`: colours, theme files, the catalogue.

### Step 3 — `internal/git`

- [x] `git.Run` is the one way to run a git command; no `exec.Command("git", …)` is left in
      `internal/tui`.
- [x] The loaders return data instead of filling in the model: `LoadGraph`, `LoadCommits`,
      `ShowCommit`, `WorkingTree`, `Status`, `UpstreamSync`, `Fetch`, `RemoteTags`, `ListRefs`,
      `CommitDepth`, `ReadInfo`. The model keeps thin methods that assign the results.
- [x] The repository-loaded handling that was written out twice in `Update`, once per message,
      is one `finishLoad`.
- [x] `internal/gittest` takes the repository fixtures, so `git` and `tui` tests share them.
- [x] Tests that read git output directly moved with the code; tests that go through the model
      stayed in `internal/tui`.

### Step 4 — `internal/tui`

- [x] Everything else moved; `main.go` is arguments, logging and wiring, and calls `tui.Run`.
- [x] The key handling is out of `Update`, in `keys.go`.

## Left for later

- **The theme is still package-level state.** `theme.Current` and the styles in
  `internal/tui/styles.go` are globals, as they were. Carrying them on the model would let the
  renderers stop depending on package state, but it means threading a value through several dozen
  free functions and their tests — a change of its own, not part of moving files.
- **`tui.Version` and `tui.LogPath` are set by `main`.** The version constant has to live in
  `main.go` for the release workflow, so the interface is told it rather than owning it.
- **`internal/tui/types.go` aliases the git types** (`commit`, `displayRow`, `fileDiff`,
  `tagRef`) so the rendering code and its tests read as before. Spelling out `git.Commit` is a
  mechanical follow-up if the aliases ever confuse more than they save.
- **`renderCommitList` is about 260 lines with nested closures.** Moving it did not shorten it.
- **`go.mod` marks every dependency `// indirect`**, including Bubble Tea; `go mod tidy` corrects it.
