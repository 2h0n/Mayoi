package main

import (
	"os"
	"slices"
)

// maxClosed is how many closed tabs the window remembers.
const maxClosed = 30

// closedTab is a tab closed, as ⇧⌘T opens it again: a file, an image or a
// terminal, where it was, and where an editor was in its text.
type closedTab struct {
	path    string // the file or image; "" for a terminal
	termDir string // where the terminal started
	col, at int    // the column's id and the tab's place there
	preview bool   // an SVG showed as an image
	sel     [2]int
	caret   int
	x, y    float32
	wrap    *bool
	wrapGen int
}

// rememberClosed keeps a tab about to close, to open again. An untitled
// document's tab is not kept: its text goes with it.
func (a *app) rememberClosed(t *colTab) {
	col, i := a.locate(t)
	if col == nil {
		return
	}
	c := closedTab{col: col.id, at: i}
	switch {
	case t.term != nil:
		c.termDir = t.term.dir
	case t.path() == "":
		return
	default:
		c.path = t.path()
	}
	if ed := t.ed; ed != nil {
		c.preview = ed.preview != nil
		c.sel, c.caret = ed.sel, ed.caret
		c.x, c.y = ed.hscroll.X, ed.vscroll.Y
		c.wrap, c.wrapGen = ed.wrap, ed.wrapGen
	}
	a.closed = append(a.closed, c)
	if len(a.closed) > maxClosed {
		a.closed = slices.Delete(a.closed, 0, len(a.closed)-maxClosed)
	}
}

// reopenClosed opens the tab closed last again, in its column and place
// when the column is still there, and otherwise in the column that has
// the keyboard. Files gone from the disk are passed over.
func (a *app) reopenClosed() {
	for len(a.closed) > 0 {
		c := a.closed[len(a.closed)-1]
		a.closed = a.closed[:len(a.closed)-1]
		col := a.column()
		if i := slices.IndexFunc(a.cols, func(k *column) bool { return k.id == c.col }); i >= 0 {
			col = a.cols[i]
		}
		if c.path == "" {
			if a.newTerminalTabAt(col, c.at, c.termDir, true) {
				return
			}
			continue
		}
		if _, err := os.Stat(c.path); err != nil {
			continue
		}
		n := len(col.tabs)
		a.openFileIn(col, c.at, c.path, true)
		if len(col.tabs) == n {
			// The column showed it already, or it could not open.
			return
		}
		if ed := col.tab().ed; ed != nil {
			ed.sel, ed.caret = c.sel, c.caret
			ed.vscroll.Y, ed.hscroll.X = c.y, c.x
			ed.wrap, ed.wrapGen = c.wrap, c.wrapGen
			if c.preview {
				a.setPreview(ed, true)
			}
			a.focusTab(col.tab())
		}
		return
	}
}
