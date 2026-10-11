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

### Vim mode

**View → Vim Mode** edits with vim's keys. Each tab has its own mode, shown in the status bar, and starts in normal
mode, where the caret is a block.

- **Motions:** `h j k l`, `w b e ge` and `W B E gE`, `0 ^ $ g_`, `gg G`, `f F t T` with `;` and `,`, `%`, `{ }`, Enter
  and `-`. Arrows, Home and End move too. A count goes before any of them.
- **Operators:** `d c y`, `> <`, `gc` to comment, and `g~ gu gU` for case. They take a motion or a text object (`iw
  aw iW aW`, `i" a"` and the other quotes, `i( a(`, `i[`, `i{`, `i<`), or apply to lines when typed twice, as `dd`.
- **Commands:** `x X D C Y s S r J ~ p P o O`, `i a I A` to insert, `u` and ⌃R to undo and redo, `.` to repeat the
  last change, ⌃D ⌃U ⌃F ⌃B to scroll by half a view or a view, and ⌃E ⌃Y by a line.
- **Visual mode:** `v` selects characters and `V` lines. A selection made with the mouse is one too. Motions extend it,
  `o` swaps its ends, and operators, `r`, `J`, `~ u U` and `p` act on it.

Esc, ⌃[ or ⌃C leaves insert mode. A change and the text typed after it undo as one step. Yanking, deleting and pasting
use the system clipboard, and a yank of whole lines pastes as lines. In normal mode, ⌃R redoes rather than opening
recent folders. ⌘ shortcuts work as usual in every mode. Punctuation keys are read as a US keyboard types them.

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

- The branch at the top opens **Switch Branch…**. Beside it, fetch, pull and push, which show how many commits
  they would bring down or send up; a branch with no upstream gets **Publish Branch** instead.
- Click a file to see its changes, Enter opens them too, and Space stages or unstages it.
- The buttons on a row open the file, discard its changes, and stage or unstage it. A group's header does the same for
  all its files. Discarding asks first, and moves untracked files to the Trash.
- Write a message and press ⌘↵ or the commit button, which says what it will do: **Commit**, **Commit All** when nothing
  is staged (Mayoi asks before staging everything), or **Amend Commit**. Changed files save first. Past 50 characters
  the summary line shows its length, in a warning color past 72.
- The arrow beside the commit button has **Commit & Push**, **Amend Last Commit** (which fills in the last message) and
  **Undo Last Commit**, which takes the commit back but keeps its changes staged and its message in the box.
- The **⋯** menu by the title has the commit commands, stage, unstage and discard all, fetch, pull and push, and
  **View as Tree**, which lists each group's files under their folders. A folder with only one folder in it shows as
  one row, as `internal/git`, and a folder's buttons stage, unstage or discard every file in it.
- A thin bar under the title moves while a Git command runs. A toast says when a commit is made, with an **Undo**
  button, and when a push or a pull is done.
- A clean work tree says so, and offers to push or pull when the branch is ahead of its upstream or behind it.
- A folder outside a repository gets an **Initialize Repository** button.

### Commits

Under the changes, **COMMITS** lists the last 50 commits of the branch, newest first, with how long ago each was made.
A filled dot and an arrow mark the commits not pushed yet, and **Show More Commits** reads 50 more. Point at a commit to
see its author and hash.

Click a commit to see it in a tab of its own: its message, author, date and hash at the top, then each file it changed
from its first parent, as the **Changes** tab shows them. Its menu copies the hash or the message, and the newest
commit's menu can undo it.

### Diffs

Every file changed shows in one **Changes** tab, as source control lists them: the staged changes first, from HEAD
to the index, then the rest, from the index to the work tree. Each file is under a header with its path and the lines
added and removed. Click a file in source control and the tab scrolls to it; **Open Changes** from the Explorer, a
tab's menu or the **Git** menu does the same for a file. As you scroll, source control chooses the file you are
reading. The diff follows a file as you type in it.

Each header shows the file's icon, its path, a badge of its state (where a renamed file came from, too), and the lines
added and removed, in numbers and in a bar of five blocks. Click a header to fold its file; a file with more than 2,000
changed lines starts folded. Point at a header, or choose
it, for buttons that open the file, stage or unstage it, and discard its changes. Files are read as they come near the
view, so a long list of changes opens at once.

Only three unchanged lines show around each change. The rest fold into a row that says how many lines it hides, and a
click opens it. Each run of changes starts with an `@@` header naming the lines and the block it is in, as Git writes
it. **Show Full Files** in the breadcrumbs shows every line.

Point at a run of changes, or choose a line of it, and its header offers to stage those changes alone, to unstage them
when they are staged, or to discard them. Mayoi reads the index and the file again first, so it acts on them as they
are. Discarding edits an open file as a step of undo, and asks first for one that is not open. As you scroll through a
file, its header stays at the top of the view with its buttons, and a click there folds the file.

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
| `s` | Stage the changes of the chosen line or run, or unstage them |

### Branches and remotes

The branch in the status bar shows how far it is ahead of its upstream (↑) and behind it (↓). Click it to switch
branches, create one, or fetch, pull and push. Switching waits until open files are saved.

**Switch Branch…**, from the branch at the top of source control, the status bar's menu or the **Git** menu, opens a box
of the branches, local and of the remotes, the one checked out first and then the one committed to last. Each shows the
subject of its last commit and how long ago it was made. Type to find one by name, or type a new name to create a branch
from the one checked out. Choosing a remote's branch checks out a local branch that tracks it.

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

Files dragged from the Finder open where you drop them: in a column after the tab shown, on its tab strip where the
marker shows, or on the edge of a column in a new column there. Dropped elsewhere, as on the explorer, they open in the
column that has the keyboard. A dropped folder opens as `mayoi <dir>` opens it.

## Terminal

Ghostty's emulator (libghostty-vt), in the bottom panel or in a column tab. You can run several terminals, move one
between the panel and a column by dragging it or from its menu, and maximize the panel. Terminal colors follow the theme.
Files dropped on a terminal type their paths, quoted for the shell.

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
- **File → Open Recent** lists the last ten folders, except the window's own and any no longer on disk, and has
  **Clear Recently Opened**. On macOS the Dock icon's menu lists them too.
- ⌃R (or **Open Recent → More…**) finds a recent folder by name: ↵ opens it in the current window, ⌘↵ in a new window.
  When a terminal has the keyboard, ⌃R goes to the shell instead.
- Opening a folder that another window already has brings that window to the front instead.

## The `mayoi` command

**File → Install 'mayoi' Command** installs the command into `~/.local/bin` when that directory is on your `PATH`, and
otherwise into `/usr/local/bin`, which asks for your password.

- `mayoi .` opens the current folder, in a new window or the one that already has it.
- `mayoi <file>` opens a file in the window whose folder contains it.

The command returns immediately, and starts Mayoi if it isn't running.

## Appearance and persistence

Mayoi follows the system's light or dark mode, or the mode chosen in **View → Appearance**. **View → Sidebar on Right**
moves the sidebar to the right of the editors. It remembers the sidebar's side, the sidebar and panel sizes, and every
window with its folder, frame, columns and tabs. Started without a folder, Mayoi reopens the windows that were open when
it quit. Closing the last window quits the app.

## Updates

Release builds check GitHub Releases for a new version once a day, 10 seconds after launch when a check is due, and
**Mayoi → Check for Updates…** (**Help → Check for Updates…** on Linux and Windows) checks right away. A window shows the
release notes and offers to install the update, skip that version or remind you later. Updates download only what
changed when they can, are checked against Mayoi's signing key, and apply when Mayoi relaunches. Development builds, and
copies Mayoi can't write to, never check.
