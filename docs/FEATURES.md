# Features

## Explorer

A lazy file tree that follows the disk, with Git-ignored files shown faint. You can create, rename, trash and reveal files
from it. In a Git work tree, a changed file shows its letter (M, A, D, U for untracked, C for a conflict) and its name in a
color, and a folder with changes inside gets a dot.

## Editor

Tabs, line numbers, a highlighted caret line, and Chroma syntax highlighting. Files changed on disk reload unless you
have unsaved edits.

⇧⌘T reopens the last tab you closed, in its column and place, with its caret and scroll position. Press it again for
the one before. Files and terminals both come back, a terminal as a new shell in the folder it started in. Opening or
closing a folder forgets the tabs closed.

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

Click a mark to see the lines as they were at HEAD, in a box under the change. From there you can revert the change,
which is a step of undo, or stage it, which puts only those lines in the index. ⌥F5 and ⇧⌥F5 go to the next and previous
change, and the **Git** menu has the same commands for the change at the caret.

The caret's line shows who changed it last, when, and the commit's summary, a moment after the caret stops. Turn this
off from **Git → Inline Blame**.

## Source control

The button beside **EXPLORER** at the top of the sidebar (⌃⇧G) switches it to the changes of the folder's Git work
tree, with a count of them. Files are grouped as merge changes, staged changes, changes and untracked files.

- Click a file to see its changes, Enter opens them too, and Space stages or unstages it.
- The buttons on a row open the file, discard its changes, and stage or unstage it. A group's header does the same for
  all its files. Discarding asks first, and moves untracked files to the Trash.
- Write a message and press ⌘↵ or **Commit**. With nothing staged, Mayoi offers to stage everything and commit it.
  Changed files save first.
- The **⋯** menu has **Commit & Push**, **Amend Last Commit** (which fills in the last message), stage, unstage and
  discard all, and fetch, pull and push.
- A folder outside a repository gets an **Initialize Repository** button.

### Diffs

Every file changed shows in one **Changes** tab, as source control lists them: the staged changes first, from HEAD
to the index, then the rest, from the index to the work tree. Each file is under a header with its path and the lines
added and removed. Click a file in source control and the tab scrolls to it; **Open Changes** from the Explorer, a
tab's menu or the **Git** menu does the same for a file. As you scroll, source control chooses the file you are
reading. The diff follows a file as you type in it.

Click a header to fold its file; a file with more than 2,000 changed lines starts folded. Point at a header, or choose
it, for buttons that open the file, stage or unstage it, and discard its changes. Files are read as they come near the
view, so a long list of changes opens at once.

Only three unchanged lines show around each change. The rest fold into a row that says how many lines it hides, and a
click opens it. Each run of changes starts with an `@@` header naming the lines and the block it is in, as Git writes
it. **Show Full Files** in the breadcrumbs shows every line.

Removed and added lines show in one column, or side by side with the lines of a change across from each other. The
layout button in the breadcrumbs chooses side by side, one column, or side by side when each side has room for 80
columns, which is the default. Words that changed within a line are tinted,
and a bar along the left is brightest for the change you are on. Long lines wrap as the editor's do: **Word Wrap**
sets them all, and ⌥Z or the wrap button just the tab.

Double-click a line to open the file there. The arrows go from change to change, and the status bar counts the lines
added and removed. With the diff focused, keys work as in a pager:

| Key | Does |
| --- | --- |
| `j` `k`, ↓ ↑ | Choose the next or previous row |
| `]` `[` | Go to the next or previous change |
| `.` `,` | Go to the next or previous file |
| Space, `f`, `b`, `d`, `u` | Scroll a page down or up, or half a page |
| `g` `G` | Go to the top or the bottom |
| `z` | Open the folded lines chosen, or fold them again |
| Enter | Open folded lines, fold a file, or open the file at a line |
| `e` | Open the file at the chosen line |

### Branches and remotes

The branch in the status bar shows how far it is ahead of its upstream (↑) and behind it (↓). Click it to switch
branches, create one, or fetch, pull and push. Switching waits until open files are saved.

Pull only fast-forwards. Push sets the upstream the first time. Git never prompts for a password inside Mayoi: it uses
your SSH agent and credential helper, and if those fail, the error says to run the command once in the terminal.

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
