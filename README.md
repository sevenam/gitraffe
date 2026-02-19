# 🦒 Gitraffe

A beautiful text-based UI git graph command line tool built with Golang, Bubble Tea, go-git, and Lip Gloss.

## Features

- 📊 Visual git commit graph in your terminal
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
go build -o gitraffe main.go
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
- `q` or `Esc` or `Ctrl+C` - Quit

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Style definitions for nice terminal layouts
- [go-git](https://github.com/go-git/go-git) - Pure Go implementation of Git
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components for Bubble Tea

## License

MIT License - see [LICENSE](LICENSE) file for details.