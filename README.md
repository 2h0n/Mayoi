# Mayoi

迷い — a small, quick code editor in Go, drawn by [MyGo](https://github.com/egoist/mygo)'s
native UI on the GPU. It has no webview and no Electron. The packaged app is about 18 MB.

- **Explorer:** a lazy file tree that follows the disk, with what Git ignores shown faint. You can create, rename, trash and reveal files from it.
- **Editor:** tabs, line numbers, the caret's line, and Chroma syntax highlighting. Long lines scroll sideways instead of wrapping. Files changed on disk reload unless you have unsaved edits.
  - **Auto save:** a changed file saves a second after the last edit, when its column shows another tab or the keyboard moves to another column, and when the window loses focus. Closing a changed file saves it without asking. Untitled files wait for ⌘S. Turn it off from File → Auto Save.
  - **Editing:** Tab and ⇧Tab indent and outdent, with tabs or spaces as the file already uses. Enter keeps the indent, and indents further after an opening bracket. Brackets and quotes close themselves, wrap a selection, and are stepped over when typed again. There are commands to comment lines, move, copy and delete lines, and insert a line above or below. Home goes to the first character of the line. The status bar shows the file's indentation and line endings, and you can change either from there.
  - **Undo:** each tab keeps its own history, and it covers the app's edits as well as typing: indenting, moving lines, replacing, and reloading from disk can all be undone. Undoing back to the saved text makes the tab clean again.
  - **Reading:** indent guides, with the caret's block brighter. The bracket paired with the one at the caret is boxed, and other occurrences of the word at the caret are highlighted. The selection has rounded corners, shows its spaces and tabs, and stays in sight, fainter, while the keyboard is elsewhere. Sticky scroll pins the lines that open the blocks in view to the top, and the breadcrumbs name the caret's blocks. Clicking either jumps there.
  - **Git:** the gutter marks lines added, changed or removed since the last commit. A strip along the right edge shows these changes, find's matches and the caret for the whole file.
- **Columns:** the editor splits into columns side by side, each with its own tabs, and a handle between two resizes them. A file shows in several columns at once: they share its text and its undo history, and each keeps its own caret and place. Drag a tab along its strip, to another column, or to the side of one for a column of its own.
- **Terminal:** Ghostty's emulator (libghostty-vt), in the panel or in a tab of a column. You can run several terminals, move one between the panel and a column by dragging it or from its menu, maximize the panel, and colors follow the theme.
- **Quick open** (⌘P, or the search box in the title bar): fuzzy file search, with recent files first. In a Git work tree it lists what Git does, so build output stays out. Type `:42` to go to a line, or `@` for the file's functions, types and headings.
- **Find and replace** (⌘F, ⌥⌘F): match case, whole word, and regular expressions with `$1` groups. In a large file the matches are found in the background as you type.
- **Windows:** a window for each folder. Open Folder… (⌘O) opens one in the window, and Open Folder in New Window… (⌥⌘O), or a recent folder clicked with ⌘ (or ⌘↵), in a window of its own. A folder another window has open brings that window to the front instead.
- **`mayoi` command:** File → Install 'mayoi' Command puts it in `~/.local/bin` when the shell looks there, else in `/usr/local/bin`, which asks for the password. `mayoi .` opens the folder you are in, in a new window or the one that has it; `mayoi <file>` opens a file in the window whose folder holds it. It returns at once, and starts Mayoi if it is not running.
- **Persistence:** follows the system's light or dark mode, or the one chosen in View → Appearance. It remembers the sidebar and panel sizes, and the windows, each with its folder, frame, and columns with their tabs. Started with no folders, Mayoi opens the windows open when it quit; closing the last window quits it.

## Run

```sh
go tool mygo dev               # live reload
go run . ~/some/project        # open a folder (or a file); a running Mayoi takes it
go tool mygo build             # build/darwin-arm64/Mayoi.app and a .dmg
go test ./...                  # headless UI tests (-short skips the shell)
go test -short -run XXX -bench Frame   # the cost of a frame of the editor
go run ./tools/icongen         # redraw resources/icon.png
```

The app needs Go 1.27+ and macOS 13+, because of the terminal library. It also builds for Linux and Windows.

## Keys

| | |
|---|---|
| ⌘P / ⇧⌘O / ⌃G | Quick open / go to symbol / go to line |
| ⌘F / ⌥⌘F / ⌘G | Find / replace / next match |
| ⌘S / ⇧⌘S / ⌥⌘S | Save / save as / save all |
| ⌘O / ⌥⌘O | Open folder / in a new window |
| ⌘N / ⌘W / ⌘1…9 / ⌃Tab | New file / close tab / show tab / next tab |
| ⌘\\ / ⌥⌘→ / ⌥⌘← | Split right / next column / previous column |
| ⌘B / ⌃\` / ⌃⇧\` | Sidebar / terminal panel / new terminal |
| ⌘Z / ⇧⌘Z | Undo / redo |
| Tab / ⇧Tab, ⌘] / ⌘[ | Indent / outdent |
| ⌘/ | Toggle line comment |
| ⌥↑ / ⌥↓, ⇧⌥↑ / ⇧⌥↓ | Move line / copy line |
| ⇧⌘K / ⌘↵ / ⇧⌘↵ | Delete line / insert line below / above |
| ⌘L / ⇧⌘\\ | Select line / go to bracket |
| ⌘= / ⌘- / ⌘0 | Text size |

## How it is built

| File | |
|---|---|
| `history.go` | Undo and redo belong to the app, not to the text area. The buffer is a persistent tree, so each step keeps the text before an edit at little cost. Typing that the area does is found when its frame starts, and joins the step before it until a pause or a move of the caret. |
| `editing.go`, `autoclose.go` | Editing commands are functions of a document and its selection. Keys reach them before the text area does, and they run in the next frame, once the selection is known. Brackets close by following up the edit that typing them made, within the same undo step. |
| `decorations.go`, `selection.go`, `sticky.go` | What is painted around the text: indent guides, bracket pairs, other occurrences of a word, and the selection with its whitespace. Each is found for the lines in view only, and found again only when the text, the caret or the view changes. They are placed by measuring the text as the text area lays it out, tabs at the platform's stops. Sticky lines and breadcrumbs come from the indentation, so they work for any language. |
| `gitmarks.go`, `internal/linediff` | The text at HEAD is read with `git show`, and the lines are compared with Myers' diff in the background after each edit. A few edits in a 100,000-line file take about 0.25 ms to compare. |
| `editor.go` | The editor is a `TextAreaBufferBase` that never wraps, inside a container that scrolls sideways. Every line has the same height, so line numbers are painted into its left padding at paint time and stay in step while scrolling. Only the lines in view get `TextRanges` colors, kept from frame to frame and merged into as few ranges as color the same runes. |
| `internal/highlight` | Chroma lexing on a background goroutine: the whole file up to 6,000 lines, and a window around the view beyond that. A line whose text has changed shows plain until the next pass, so stale colors never land on the wrong text. |
| `internal/workspace` | Directories are read as they open and watched with fsnotify, with bursts of changes batched. Also the file index and the fuzzy matcher for quick open. |
| `document.go`, `columns.go` | A document is a file's text, its undo history and its Git marks; an editor is a document as one tab shows it, with its own selection, scroll and paint caches. A column holds tabs, each an editor or a terminal. Typing is credited to the editor that has the keyboard, and the selections of the document's other editors move with the text. A text area scrolls to its caret when its text changes under it, so an editor without the keyboard parks the area's caret in view and keeps its own selection, and stays where it is scrolled. |
| `view.go` | The layout. The sidebar, the columns and the panel keep stable keys as panes come and go, so editors never lose their state. Tabs are dragged as values within the window; the places a column takes one show only during a drag. |
| `app.go`, `commands.go` | State and commands. An `app` is a window's; the windows share the settings and the menu bar, whose items run the command of the window they are chosen in. Menus, accelerators and tests all run the same commands. |
| `main.go`, `cli.go` | The windows. Folders and files reach the app from `open`, as the `mayoi` script hands them over by the app's identifier, and from a second Mayoi started with them, which passes them on and exits. Each folder goes to the window that has it, or a new one; a file to the window whose folder holds it. |
| `theme.go` | The palette, light and dark, for the window, the code and the terminal. |

Measured in tests on an M-series Mac, a 100,000-line file opens in about 20 ms. Scrolling and typing cost under 2 ms per frame, guides, sticky lines and Git marks included, and about 3 ms with find open on it. Quick open searches 100,000 paths in about 1.5 ms.
