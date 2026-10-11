package main

import (
	"slices"
	"strings"
)

// Files dragged from the Finder open where they are dropped: in a column,
// at a place of its tab strip, or in a column of their own beside one.
// Folders open as `mayoi <dir>` opens them. On a terminal, the paths are
// typed instead.

// dropFiles opens the files dropped on a column at place at, the first
// there and each other after it, and the folders in windows of their own.
func (a *app) dropFiles(paths []string, col *column, at int) {
	folders, files := splitPaths(paths)
	if len(folders) > 0 {
		a.host.openFolders(folders)
	}
	if slices.Index(a.cols, col) < 0 {
		col = a.column()
		at = col.active + 1
	}
	for i, p := range files {
		a.openFileIn(col, at, p, i == len(files)-1)
		at = col.active + 1
	}
}

// dropFilesBeside opens the files dropped on a side of a column in a
// column of their own there.
func (a *app) dropFilesBeside(paths []string, col *column, after bool) {
	if slices.Index(a.cols, col) < 0 {
		return
	}
	if _, files := splitPaths(paths); len(files) > 0 {
		nc := a.splitColumn(col, after)
		a.dropFiles(paths, nc, 0)
		if len(nc.tabs) == 0 {
			// None of them opened.
			a.removeColumn(nc)
		}
		return
	}
	a.dropFiles(paths, col, col.active+1)
}

// dropOnColumn opens what is dropped on the middle of a column after the
// tab shown, or types it into the terminal the column shows.
func (a *app) dropOnColumn(paths []string, col *column) {
	if t := col.tab(); t != nil && t.term != nil {
		a.pastePaths(t.term, paths)
		return
	}
	a.dropFiles(paths, col, col.active+1)
}

// pastePaths types the paths into a terminal, quoted for the shell, as
// the Terminal does.
func (a *app) pastePaths(s *session, paths []string) {
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = shellQuote(p)
	}
	s.term.Paste(strings.Join(quoted, " ") + " ")
}

// shellQuote quotes s for a POSIX shell, unless it needs no quotes.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
