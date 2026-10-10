# Changelog

The update window shows the section of the version it offers. Every release
needs a `## <version>` section here, or its build fails.

## Unreleased

- Source control: the sidebar lists the files changed, staged and not, to
  stage, unstage, discard and commit, with Commit & Push and Amend.
- Diffs open in a tab, in one column or side by side, with the words that
  changed tinted, and follow the file as you type.
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
