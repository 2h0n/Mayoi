# Architecture

Mayoi is a single Go package (`main`) with three internal packages, drawn by [MyGo](https://github.com/egoist/mygo).
This page walks through the source by area.

## Windows and commands

**`main.go`, `cli.go`.** These manage the windows. Folders and files reach the app in two ways. The `mayoi` script hands
them to `open` with the app's identifier, and a second Mayoi started with paths passes them on and exits. Each folder
goes to the window that already has it, or to a new one. A file goes to the window whose folder contains it.

**`app.go`, `commands.go`.** These hold state and commands. Each window has its own `app`, and the windows share the
settings and the menu bar. A menu item runs the command of the window it was chosen in. Menus, accelerators and tests
all run the same commands.

**`view.go`.** This is the layout. The sidebar, the columns and the panel keep stable keys as panes come and go, so
editors never lose their state. Tabs are dragged as values within the window, and the drop targets a column offers only
show during a drag.

## Documents, editors and columns

**`document.go`, `columns.go`.** A document is a file's text, its undo history and its Git marks. An editor is a
document as one tab shows it, with its own selection, scroll position and paint caches. A column holds tabs, each of
which is an editor or a terminal.

Typing is credited to the editor that has the keyboard, and the selections of the document's other editors move with
the text. A text area scrolls to its caret when its text changes. So an editor without the keyboard parks the area's
caret in view, keeps its own selection, and stays where it was scrolled.

**`editor.go`, `wrap.go`.** The editor is a `TextAreaBufferBase` inside a container that scrolls sideways.

- **Unwrapped,** every line has the same height. Line numbers are painted into the area's left padding at paint time, so
  they stay in step while scrolling.
- **Wrapped,** the area is as wide as the view and reports where it laid out each line, using MyGo's `TrackLines`.
  Decorations are placed along a line as if it did not wrap, then split onto its visual rows.

Only the lines in view get `TextRanges` colors. Those colors are kept from frame to frame, and merged into as few ranges
as possible.

## Editing and undo

**`history.go`.** Undo and redo belong to the app, not to the text area. The buffer is a persistent tree, so each step
keeps the text from before an edit at little cost. Typing done by the text area is detected when its frame starts, and
joins the previous step until a pause or a caret move.

**`editing.go`, `autoclose.go`.** Editing commands are functions of a document and its selection. Keys reach them
before the text area does, and they run in the next frame, once the selection is known. Brackets close by following up
the edit that typed them, within the same undo step.

## Decorations

**`decorations.go`, `selection.go`, `sticky.go`.** These paint what goes around the text: indent guides, bracket pairs,
other occurrences of a word, and the selection with its whitespace.

- Each decoration is computed for the visible lines only, and recomputed only when the text, the caret or the view
  changes.
- Decorations are placed by measuring the text the way the text area lays it out, with tabs at the platform's stops.
- Sticky lines and breadcrumbs are derived from indentation, so they work in any language.

## Image viewer

**`viewer.go`.** An image tab holds a viewer instead of a document. The bitmap is decoded off the main thread and
painted over a checkerboard, at a scale and offset the viewer keeps.

An SVG preview is a viewer shown in place of its editor. It parses the text shortly after each edit and keeps the last
image that parsed. The text area stays built, out of sight, so the editor keeps its place.

## Git

**`gitmarks.go`, `internal/linediff`.** The text at `HEAD` is read with `git show`, and the lines are compared using
Myers' diff in the background after each edit. Comparing a few edits in a 100,000-line file takes about 0.25 ms.

## Internal packages

**`internal/highlight`.** Chroma lexing runs on a background goroutine. It lexes the whole file up to 6,000 lines, and
a window around the view beyond that. A line whose text has changed shows plain until the next pass, so stale colors
never land on the wrong text.

**`internal/workspace`.** Directories are read as they are expanded and watched with fsnotify, with bursts of changes
batched together. This package also holds the file index and the fuzzy matcher for quick open.

## Theme

**`theme.go`.** The light and dark palettes for the window, the code and the terminal.
