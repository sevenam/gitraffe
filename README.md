# 🦒 Gitraffe

A beautiful text-based UI git graph command line tool built with Golang, Bubble Tea, go-git, and Lip Gloss.

## Features

- 📊 Visual git commit graph in your terminal (branches **and tags** are shown; the graph expands to use available space and long branch names are truncated as needed)
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
