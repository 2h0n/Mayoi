package main

import (
	"github.com/egoist/mygo/ui"
)

// selRadius rounds the corners of the selection.
const selRadius = 3

// selectionShape is the selection on the lines in view, which the editor
// paints itself, in place of the text area: a box for each line, with the
// corners rounded where the line above or below does not go on past them.
// A document keeps the last it found, as its decorations.
type selectionShape struct {
	version     uint64
	sel         [2]int
	first, last int
	font        codeFont
	found       bool

	rects []runeRect
	// native is set when a line selected runs from right to left, which
	// only the text area knows the places of: it paints the selection.
	native bool
}

// selection finds the shape of the selection on lines first up to last.
func (a *app) selection(d *document, snap ui.TextSnapshot, first, last int) *selectionShape {
	sh := &d.selShape
	if sh.found && sh.version == snap.Version() && sh.sel == d.sel && sh.first == first && sh.last == last && sh.font == a.font {
		return sh
	}
	sh.version, sh.sel, sh.first, sh.last, sh.font, sh.found = snap.Version(), d.sel, first, last, a.font, true
	sh.rects, sh.native = sh.rects[:0], false
	n := snap.Len()
	sa, sb := min(d.sel[0], d.sel[1], n), min(max(d.sel[0], d.sel[1]), n)
	if sa == sb {
		return sh
	}
	f := a.font
	for i := max(first, snap.LineAt(sa)); i <= min(last, snap.LineAt(sb)); i++ {
		s, e := snap.LineRange(i)
		from, to := max(sa, s), min(sb, e)
		var x0, x1 float32
		if e-s > highlightRunes {
			// A line too long to measure for a box: by its cells.
			x0, x1 = float32(from-s)*f.charW, float32(to-s)*f.charW
		} else {
			text := snap.Slice(s, to)
			if hasRTL(text) {
				sh.rects, sh.native = sh.rects[:0], true
				return sh
			}
			x1 = f.width(text)
			if from > s {
				x0 = f.width(snap.Slice(s, from))
			}
		}
		if sb > e {
			x1 += f.charW / 2 // the line's end is selected too
		}
		if x1 > x0 {
			sh.rects = append(sh.rects, runeRect{line: i, x0: x0, x1: x1})
		}
	}
	return sh
}

// highlightRunes is the longest line the editor measures to place what it
// paints on it.
const highlightRunes = 4000

// hasRTL reports whether s has runes of a script written from right to
// left: Hebrew, Arabic and their neighbors.
func hasRTL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0xD6 {
			continue // below U+0590 in UTF-8
		}
		for _, r := range s[i:] {
			if r >= 0x0590 && r <= 0x08FF || r >= 0xFB1D && r <= 0xFDFF || r >= 0xFE70 && r <= 0xFEFF {
				return true
			}
		}
		return false
	}
	return false
}

// paint fills the selection, with x the left of the text and lineY the top
// of a line. The color is opaque, so that the boxes may overlap.
func (sh *selectionShape) paint(pt *ui.Painter, x float32, lineY func(int) float32, lineH float32, col ui.Color) {
	const r = selRadius
	for k, b := range sh.rects {
		y := lineY(b.line)
		pt.Fill(ui.Rect{X: x + b.x0, Y: y, W: b.x1 - b.x0, H: lineH}, col, r)
		// Square the corners the lines above and below go on past.
		square := func(o runeRect, cy float32) {
			if o.x0 <= b.x0 && o.x1 > b.x0 {
				pt.Fill(ui.Rect{X: x + b.x0, Y: cy, W: min(r, b.x1-b.x0), H: r}, col, 0)
			}
			if o.x0 < b.x1 && o.x1 >= b.x1 {
				w := min(r, b.x1-b.x0)
				pt.Fill(ui.Rect{X: x + b.x1 - w, Y: cy, W: w, H: r}, col, 0)
			}
		}
		if k > 0 && sh.rects[k-1].line == b.line-1 {
			square(sh.rects[k-1], y)
		}
		if k+1 < len(sh.rects) && sh.rects[k+1].line == b.line+1 {
			square(sh.rects[k+1], y+lineH-r)
		}
	}
}
