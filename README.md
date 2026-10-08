# Mayoi

迷い — a small, quick code editor in Go, drawn by [MyGo](https://github.com/egoist/mygo)'s
native UI on the GPU. It has no webview and no Electron. The packaged app is about 18 MB.

- **Explorer:** a lazy file tree that follows the disk. You can create, rename, trash and reveal files from it.
- **Editor:** tabs, line numbers, the caret's line, and Chroma syntax highlighting. Long lines scroll sideways instead of wrapping. Each tab keeps its own undo history. Files changed on disk reload unless you have unsaved edits.
- **Terminal panel:** Ghostty's emulator (libghostty-vt). You can run several terminals, maximize the panel, and colors follow the theme.
- **Quick open** (⌘P): fuzzy file search, with recent files first. Type `:42` to go to a line.
- **Find and replace** (⌘F, ⌥⌘F): match case, whole word, and regular expressions with `$1` groups.
- **Persistence:** follows the system's light or dark mode. It remembers the sidebar and panel sizes, the folder, and the open tabs.

## Run

```sh
go tool mygo dev               # live reload
go run . ~/some/project        # open a folder (or a file)
go tool mygo build             # build/darwin-arm64/Mayoi.app and a .dmg
go test ./...                  # headless UI tests (-short skips the shell)
go run ./tools/icongen         # redraw resources/icon.png
```

The app needs Go 1.27+ and macOS 13+, because of the terminal library. It also builds for Linux and Windows.

## Keys

| | |
|---|---|
| ⌘P / ⌃G | Quick open / go to line |
| ⌘F / ⌥⌘F / ⌘G | Find / replace / next match |
| ⌘S / ⇧⌘S / ⌥⌘S | Save / save as / save all |
| ⌘N / ⌘W / ⌘1…9 / ⌃Tab | New file / close tab / show tab / next tab |
| ⌘B / ⌃\` / ⌃⇧\` | Sidebar / terminal panel / new terminal |
| ⌘= / ⌘- / ⌘0 | Text size |

## How it is built

| File | |
|---|---|
| `editor.go` | The editor is a `TextAreaBufferBase` that never wraps, inside a container that scrolls sideways. Every line has the same height, so line numbers are painted into its left padding at paint time and stay in step while scrolling. Only the lines in view get `TextRanges` colors. |
| `internal/highlight` | Chroma lexing on a background goroutine: the whole file up to 6,000 lines, and a window around the view beyond that. A line whose text has changed shows plain until the next pass, so stale colors never land on the wrong text. |
| `internal/workspace` | Directories are read as they open and watched with fsnotify, with bursts of changes batched. Also the file index and the fuzzy matcher for quick open. |
| `view.go` | The layout. The sidebar, editor and panel keep stable keys as panes come and go, so editors never lose their state. |
| `app.go`, `commands.go` | State, documents and commands. Menus, accelerators and tests all run the same commands. |
| `theme.go` | The palette, light and dark, for the window, the code and the terminal. |

Measured in tests on an M-series Mac, a 100,000-line file opens in about 20 ms. Scrolling and typing cost about 2 ms per frame.
