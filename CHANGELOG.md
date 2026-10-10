# Changelog

The update window shows the section of the version it offers. Every release
needs a `## <version>` section here, or its build fails.

## 0.2.1

- Source control lists the last commits of the branch, with those not yet
  pushed marked. Click one to see its message and its changes in a tab of
  its own; its menu copies the hash or the message.
- The commit button says what it will do (**Commit**, **Commit All**,
  **Amend Commit**), and its menu has Commit & Push, Amend and the new
  **Undo Last Commit**, which keeps the changes staged and the message.
- **Switch Branch…** finds a branch by name among the local ones and the
  remotes', the one committed to last first, with its last commit, and
  creates a branch of the name typed. The branch at the top of source
  control opens it, and the pull and push buttons beside it show how many
  commits they would move. A branch with no upstream gets **Publish
  Branch**.
- **View as Tree**, in the **⋯** menu of source control, lists the changes
  under their folders, which stage, unstage or discard all they hold.
- A clean work tree says so, and offers to push or pull when the branch is
  ahead or behind. A folder outside a repository explains itself.
- The summary line of a commit message shows its length past 50
  characters, in a warning color past 72.
- A thin bar under the title moves while a Git command runs, and a toast
  says when a commit, push or pull is done. The commit's toast has an
  **Undo** button.
- Stage, unstage or discard one change at a time from the Changes tab:
  point at a run of changes for its buttons, or press `s` on a line.
- A file's header stays at the top of the diff as you scroll through it.
- Diff headers show the file's icon, a badge of its state, where a renamed
  file came from, and a bar of the lines added and removed. Folded lines
  read "Show N unchanged lines" and light up under the pointer.
- Source control and the Changes tab stay quick with thousands of changed
  files: an idle frame of 2,000 changes went from 0.39 ms to 0.22 ms and
  allocates a tenth as much, and reading the status again when nothing
  changed no longer lays the diff out again.
- **File → Open Recent** lists the folders opened last, but the window's
  and those not on the disk, with **Clear Recently Opened**. ⌃R, or
  **More…**, finds one by name: ⏎ opens it in the window, ⌘⏎ in a new
  one. A terminal keeps ⌃R for its shell. On macOS the Dock icon's menu
  lists them too.

## 0.2.0

- Source control: the sidebar lists the files changed, staged and not, to
  stage, unstage, discard and commit, with Commit & Push and Amend.
- Diffs open in a tab, in one column or side by side, with the words that
  changed tinted, and follow the file as you type.
- Diffs fold the unchanged lines away from the changes, head each change
  with its `@@` line and the block it is in, wrap long lines, and go side
  by side when there is room.
- Every file changed shows in one **Changes** tab, staged and not, each
  under a header that folds it, stages it or discards it. Clicking a file
  in source control scrolls the tab to it.
- In a diff, `j`/`k`, `]`/`[`, `.`/`,`, Space, `g`/`G` and `z` move about as
  in a pager.
- Click a gutter mark to see what the lines were, and revert or stage that
  change alone. ⌥F5 goes to the next change.
- The Explorer shows each changed file's letter and color.
- The branch in the status bar switches and creates branches, says how far
  it is ahead and behind, and fetches, pulls and pushes.
- The caret's line shows who changed it last.
- ⇧⌘T (**File → Reopen Closed Tab**) reopens the last tab closed, where it
  was and scrolled as it was. Terminals come back as a new shell.
- Highlighting keeps up with typing: after a keystroke only the lines around
  the edit are lexed again, under 1 ms in a 6,000-line file where the whole
  file took 170 ms, and lines an edit moves keep their colors meanwhile.
- A large file opened shows the colors of the lines in view first.
- The tab strip has an edge along its bottom, which the tab shown breaks.
- Line numbers are easier to read, the find and replace fields line up, and
  the status bar says "1 line", names languages consistently, and shows a
  terminal's folder and size.

## 0.1.0

- The first release: tabs, columns, the file tree, quick open, find and
  replace, Git marks, a terminal, an image viewer and auto save.
- Mayoi updates itself from GitHub Releases, from **Mayoi → Check for
  Updates…** or in the background once a day.
