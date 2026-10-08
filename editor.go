package main

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/highlight"
)

const (
	tabWidth   = 4
	editorPadY = 8  // above the first line
	gutterGap  = 10 // between the line numbers and the text
	textPadR   = 32 // right of the widest line
)

// codeFont is the editor's font and the metrics of its monospaced cells.
type codeFont struct {
	family string
	size   float32
	lineH  float32
	charW  float32
}

func newCodeFont(family string, size float32) codeFont {
	l := ui.ShapeText(strings.Repeat("0", 64), ui.Font{Family: family, Size: size}, 0)
	w, _ := l.Size()
	return codeFont{family: family, size: size, lineH: float32(int(size*1.55 + 0.5)), charW: w / 64}
}

// gutterWidth is the width of the line numbers of a document of n lines.
func (f codeFont) gutterWidth(n int) float32 {
	digits := max(len(strconv.Itoa(n)), 3)
	return float32(digits)*f.charW + 28
}

// editorView shows a document: its text in a text area that never wraps,
// inside a container that scrolls it sideways, with the line numbers
// painted over the area's left padding, where they stay as it scrolls.
//
// The lines are all lineH high, so that line i is at editorPadY+i*lineH
// in the area's content, which places the line numbers and the caret's
// line, and finds the lines in view to highlight.
func (a *app) editorView(c *ui.Context, d *document) {
	p, f := a.pal, a.font
	snap := d.buf.Snapshot()
	lines := snap.LineCount()
	gutterW := f.gutterWidth(lines)
	view := d.hscroll.Bounds(c)
	textX := gutterW + gutterGap

	// A selection the app asked for: scroll to it, unless it shows.
	if r := d.pending; r != nil && view.H > 0 {
		line := snap.LineAt(min(r[0], snap.Len()))
		top := editorPadY + float32(line)*f.lineH
		if top < d.vscroll.Y || top+f.lineH > d.vscroll.Y+view.H-f.lineH {
			d.vscroll.Y = max(0, top-view.H/3)
		}
	}

	// The lines in view, and the caret.
	first := max(0, int((d.vscroll.Y-editorPadY)/f.lineH))
	last := min(lines-1, first+int(view.H/f.lineH)+1)
	for i := first; i <= last; i++ {
		s, e := snap.LineRange(i)
		if e-s > d.widest {
			d.widest = max(d.widest, columns(snap.Slice(s, e)))
		}
	}
	contentW := max(view.W, textX+float32(d.widest)*f.charW+textPadR)

	ui.ScrollHorizontal(c).TrackScroll(&d.hscroll).Fill().Background(p.editor).Children(func() {
		area := ui.TextAreaBufferBase(c, d.buf).
			NoWrap().Font(f.family).FontSize(f.size).FixedLineHeight(f.lineH).TextColor(p.text).
			Width(contentW).FillHeight().
			Padding(editorPadY, textPadR, max(view.H-3*f.lineH, 0), textX).
			TrackScroll(&d.vscroll).
			// The area is as wide as its widest line: keep its scroll bar
			// at the right of the view, wherever it scrolls sideways.
			ScrollbarInsets(2, max(0, d.hscroll.MaxX-d.hscroll.X)+2, 2, 0).
			Label("Editor " + d.name())

		if !d.caretSeen {
			// A new area puts the caret at the end: open at the start.
			area.SetTextSelection(0, 0)
		}
		if r := d.pending; r != nil {
			area.SetTextSelection(r[0], r[1])
			d.pending = nil
		}
		selStart, caret := area.TextSelection()
		d.sel = [2]int{selStart, caret}
		caretLine := snap.LineAt(min(caret, snap.Len()))
		if caret != d.caret || !d.caretSeen {
			if d.caretSeen {
				a.revealColumn(d, snap, caret, view.W, textX)
			}
			d.caret, d.caretSeen = caret, true
		}

		if d.hl != nil && d.hl.Lexer() != nil {
			d.hl.Request(snap, first, last)
			ranges := a.ranges[:0]
			for i := first; i <= last; i++ {
				s, e := snap.LineRange(i)
				if e-s > highlight.MaxLineRunes {
					continue
				}
				for _, tok := range d.hl.Line(i, snap.Slice(s, e)) {
					ranges = append(ranges, ui.TextRange{Start: s + tok.Start, End: s + tok.End, Color: p.syntax[tok.Kind]})
				}
			}
			area.TextRanges(ranges...)
			a.ranges = ranges
		}

		lineH, charW, size, family := f.lineH, f.charW, f.size, f.family
		focused := d.vscroll.Focused(c)
		matches := a.visibleMatches(d, first, last)
		area.Draw(func(pt *ui.Painter, r ui.Rect) {
			y := r.Y + editorPadY + float32(caretLine)*lineH - d.vscroll.Y
			if y+lineH > r.Y && y < r.Y+r.H {
				pt.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: lineH}, p.line, 0)
			}
			for _, m := range matches {
				y := r.Y + editorPadY + float32(m.line)*lineH - d.vscroll.Y
				box := ui.Rect{X: r.X + textX + m.x0 - 1, Y: y + 1, W: m.x1 - m.x0 + 2, H: lineH - 2}
				if m.cur {
					pt.Fill(box, p.matchNow, 3)
					pt.Stroke(box, p.matchRing, 3, 1)
				} else {
					pt.Fill(box, p.match, 3)
				}
			}
		})
		area.DrawOver(func(pt *ui.Painter, r ui.Rect) {
			x := r.X + d.hscroll.X
			pt.Fill(ui.Rect{X: x, Y: r.Y, W: textX - 2, H: r.H}, p.editor, 0)
			if d.hscroll.X > 0 {
				// A shadow at the edge the text scrolls under.
				pt.FillGradient(ui.Rect{X: x + textX - 2, Y: r.Y, W: 6, H: r.H}, ui.LinearGradient{
					From: ui.RGBA(0, 0, 0, 0.18), To: ui.RGBA(0, 0, 0, 0), Angle: 90,
				}, 0)
			}
			top := d.vscroll.Y
			from := max(0, int((top-editorPadY)/lineH))
			to := min(lines-1, from+int(r.H/lineH)+1)
			pt.Clip(ui.Rect{X: x, Y: r.Y, W: gutterW, H: r.H}, 0, func() {
				for i := from; i <= to; i++ {
					label := strconv.Itoa(i + 1)
					col := p.gutter
					if i == caretLine && focused {
						col = p.gutterNow
					} else if i == caretLine {
						col = p.muted
					}
					y := r.Y + editorPadY + float32(i)*lineH - top
					span := ui.Span{Text: label, Font: family, Size: size, Color: col}
					_, h := pt.MeasureText(0, span)
					pt.RichText(x+gutterW-14-float32(len(label))*charW, y+(lineH-h)/2, 0, span)
				}
			})
		})
	})
}

// revealColumn scrolls the editor sideways, as little as it takes, to show
// the caret.
func (a *app) revealColumn(d *document, snap ui.TextSnapshot, caret int, viewW, textX float32) {
	if viewW <= 0 {
		return
	}
	line := snap.LineAt(min(caret, snap.Len()))
	s, _ := snap.LineRange(line)
	// Measure the line as the area lays it out, tabs at the platform's
	// stops.
	prefix := snap.Slice(s, min(caret, snap.Len()))
	w := float32(len(prefix)) * a.font.charW
	if strings.ContainsRune(prefix, '\t') || !isASCII(prefix) {
		w, _ = ui.ShapeText(prefix, ui.Font{Family: a.font.family, Size: a.font.size}, 0).Size()
	}
	x := textX + w
	left, right := d.hscroll.X+textX, d.hscroll.X+viewW-textPadR
	switch {
	case x < left:
		d.hscroll.X = max(0, x-textX-4*a.font.charW)
	case x > right:
		d.hscroll.X = x - viewW + textPadR + 4*a.font.charW
	}
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
