<p align="center">
  <img src="resources/icon.png" width="128" height="128" alt="Mayoi icon">
</p>

<h1 align="center">Mayoi</h1>

<p align="center">迷い — a small, fast code editor written in Go. It draws a native UI on the GPU with
<a href="https://github.com/egoist/mygo">MyGo</a>, with no webview and no Electron, and the packaged app is about 18 MB.</p>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/screenshot-dark.png">
  <img alt="Mayoi editing its own source in two columns" src="docs/screenshot-light.png">
</picture>

## Features

- **Editor:** tabs, Chroma syntax highlighting, indent guides, sticky scroll, breadcrumbs and word wrap
- **Smart editing:** auto-indent, auto-closing brackets and quotes, line commands, and per-tab undo
- **Vim mode:** normal, insert and visual modes, with motions, operators, text objects, counts and `.`
- **Columns:** split the editor side by side and drag tabs between columns
- **Explorer:** a live file tree that follows the disk and dims Git-ignored files
- **Quick open:** fuzzy file search, go to line (`:42`), go to symbol (`@`) and a command palette (`>`, ⇧⌘P)
- **Find and replace:** case, whole-word and regex matching, searched in the background in large files
- **Search:** find and replace across the folder's files (⇧⌘F, ⇧⌘H), with files to include and exclude, and a
  preview of each replacement
- **Git:** a source control view to stage, commit, amend, undo, pull and push, every change in one diff, folded
  around the changes, inline or side by side, a graph of the commits with their branches and tags, each with its own
  diff, gutter marks that revert or stage a change, branches and blame
- **Terminal:** Ghostty's emulator (libghostty-vt), in the bottom panel or in a tab
- **Image viewer:** PNG, JPEG, GIF, WebP and BMP, plus a live SVG preview
- **Auto save,** light, dark, The Matrix, Claude and Qoder themes, one window per folder, and a `mayoi` shell command
- **Settings:** JSON files applied as you save them, with the code's and the terminal's fonts, weight, ligatures and
  line height, and keyboard shortcuts in VS Code's format (see [docs/SETTINGS.md](docs/SETTINGS.md))

See [docs/FEATURES.md](docs/FEATURES.md) for the details, [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for a tour of the
source and how fast it runs, and [CONTRIBUTING.md](CONTRIBUTING.md) to build, test and release Mayoi.

## Requirements

- macOS 13 or later
- Go 1.27 or later

Mayoi also builds for Linux and Windows.

## Getting started

```sh
go run ./cmd/mayoi ~/some/project    # open a folder or a file
go tool mygo build                   # build build/darwin-arm64/Mayoi.app and a .dmg
```

To open folders from a shell, choose **File → Install 'mayoi' Command**, then run `mayoi .` or `mayoi <file>`.

## Keyboard shortcuts

| Shortcut | Action |
|---|---|
| ⌘P / ⇧⌘O / ⌃G | Quick open / go to symbol / go to line |
| ⌘F / ⌥⌘F / ⌘G | Find / replace / next match |
| ⇧⌘F / ⇧⌘H | Find in files / replace in files |
| ⌘S / ⇧⌘S / ⌥⌘S | Save / save as / save all |
| ⌘O / ⌥⌘O | Open folder / open folder in a new window |
| ⌘N / ⌘W / ⌘1…9 / ⌃Tab | New file / close tab / go to tab / next tab |
| ⌘\\ / ⌥⌘→ / ⌥⌘← | Split right / next column / previous column |
| ⌘B / ⌃\` / ⌃⇧\` | Toggle sidebar / toggle terminal panel / new terminal |
| ⌘Z / ⇧⌘Z | Undo / redo |
| Tab / ⇧Tab, ⌘] / ⌘[ | Indent / outdent |
| ⌘/ | Toggle line comment |
| ⌥↑ / ⌥↓, ⇧⌥↑ / ⇧⌥↓ | Move line / copy line |
| ⇧⌘K / ⌘↵ / ⇧⌘↵ | Delete line / insert line below / insert line above |
| ⌘L / ⇧⌘\\ | Select line / go to bracket |
| ⌘= / ⌘- / ⌘0 | Zoom text or image in / out / reset |
| ⇧⌘V | Toggle SVG preview |
| ⌥Z | Toggle word wrap for the current tab |

## License

[MIT](LICENSE)
