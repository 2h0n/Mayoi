# Features

## Explorer

A lazy file tree that follows the disk, with Git-ignored files shown faint. You can create, rename, trash and reveal files
from it.

## Editor

Tabs, line numbers, a highlighted caret line, and Chroma syntax highlighting. Files changed on disk reload unless you
have unsaved edits.

### Word wrap

Long lines scroll sideways by default. **View → Word Wrap** wraps them at the edge of the editor. ⌥Z toggles wrapping
for the current tab only, until the setting changes again.

### Auto save

A changed file saves:

- one second after the last edit
- when its column switches to another tab, or the keyboard moves to another column
- when the window loses focus

Closing a changed file saves it without asking. Untitled files wait for ⌘S. Turn this off from **File → Auto Save**.

### Editing

- Tab and ⇧Tab indent and outdent, using tabs or spaces to match the file.
- Enter keeps the indent, and indents one more level after an opening bracket.
- Brackets and quotes close themselves, wrap a selection, and are stepped over when typed again.
- Commands comment lines, move, copy and delete lines, and insert a line above or below.
- Home goes to the first non-blank character of the line.
- The status bar shows the file's indentation and line endings, and you can change either from there.

### Undo

Each tab keeps its own history. It covers the app's edits as well as typing, so indenting, moving lines, replacing and
reloading from disk can all be undone. Undoing back to the saved text marks the tab clean again.

### Reading aids

- Indent guides, with the caret's block drawn brighter.
- The bracket matching the one at the caret is boxed.
- Other occurrences of the word at the caret are highlighted.
- The selection has rounded corners and shows its spaces and tabs. It stays visible, dimmed, while the keyboard is
  elsewhere.
- Sticky scroll pins the lines that open the blocks in view to the top, and breadcrumbs name the blocks around the
  caret. Click either to jump there.

### Git

The gutter marks lines added, changed or removed since the last commit. A strip along the right edge shows these
changes, find matches and the caret position for the whole file.

## Image viewer

PNG, JPEG, GIF, WebP and BMP files open in a viewer, fitted to the view on a checkerboard.

- ⌘= and ⌘- zoom, as does scrolling with ⌘ held. ⌘0 fits the image again.
- Double-click toggles between fitted and actual size.
- Drag or scroll to pan an image larger than the view.
- The image reloads when the file changes.

An SVG opens as text. ⇧⌘V, or the button at the end of the breadcrumbs, shows it as an image that updates as you type.

## Columns

The editor splits into columns side by side, each with its own tabs, and you resize them by dragging the handle between
two columns. A file can show in several columns at once. Those columns share its text and undo history, and each keeps
its own caret and scroll position. Drag a tab along its strip, into another column, or to the edge of a column to give it
a new column.

## Terminal

Ghostty's emulator (libghostty-vt), in the bottom panel or in a column tab. You can run several terminals, move one
between the panel and a column by dragging it or from its menu, and maximize the panel. Terminal colors follow the theme.

## Quick open

⌘P, or the search box in the title bar, opens a fuzzy file search with recent files first. In a Git work tree it lists
the files Git tracks, so build output stays out.

- Type `:42` to go to line 42.
- Type `@` to list the file's functions, types and headings.

## Find and replace

⌘F finds and ⌥⌘F replaces, with options for match case, whole word and regular expressions (`$1` refers to a group). In
large files, matches are found in the background as you type.

## Windows

Each folder gets its own window.

- **Open Folder…** (⌘O) opens a folder in the current window.
- **Open Folder in New Window…** (⌥⌘O), or ⌘-clicking a recent folder (or pressing ⌘↵ on it), opens it in a new window.
- Opening a folder that another window already has brings that window to the front instead.

## The `mayoi` command

**File → Install 'mayoi' Command** installs the command into `~/.local/bin` when that directory is on your `PATH`, and
otherwise into `/usr/local/bin`, which asks for your password.

- `mayoi .` opens the current folder, in a new window or the one that already has it.
- `mayoi <file>` opens a file in the window whose folder contains it.

The command returns immediately, and starts Mayoi if it isn't running.

## Appearance and persistence

Mayoi follows the system's light or dark mode, or the mode chosen in **View → Appearance**. It remembers the sidebar and
panel sizes, and every window with its folder, frame, columns and tabs. Started without a folder, Mayoi reopens the
windows that were open when it quit. Closing the last window quits the app.

## Updates

Release builds check GitHub Releases for a new version once a day, 10 seconds after launch when a check is due, and
**Mayoi → Check for Updates…** (**Help → Check for Updates…** on Linux and Windows) checks right away. A window shows the
release notes and offers to install the update, skip that version or remind you later. Updates download only what
changed when they can, are checked against Mayoi's signing key, and apply when Mayoi relaunches. Development builds, and
copies Mayoi can't write to, never check.
