package main

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/highlight"
	"mayoi/internal/linediff"
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
	// What was typed since the last frame, for undo.
	seen := d.hist.snap.Version()
	d.observe()
	snap := d.buf.Snapshot()
	edited := snap.Version() != seen
	lines := snap.LineCount()
	gutterW := f.gutterWidth(lines)
	view := d.hscroll.Bounds(c)
	textX := gutterW + gutterGap

	ui.ScrollHorizontal(c).TrackScroll(&d.hscroll).Fill().Background(p.editor).Children(func() {
		area := ui.TextAreaBufferBase(c, d.buf).
			NoWrap().Font(f.family).FontSize(f.size).FixedLineHeight(f.lineH).TextColor(p.text).
			FillHeight().
			Padding(editorPadY, textPadR, editorPadY, textX).
			Label("Editor " + d.name()).
			HandleInput(func(ev ui.InputEvent) bool { return a.editorKey(d, ev) })

		if !d.caretSeen {
			// A new area puts the caret at the end: open at the start.
			area.SetTextSelection(0, 0)
		}
		jump := d.pending != nil
		if r := d.pending; r != nil {
			area.SetTextSelection(r[0], r[1])
			d.pending = nil
		}
		selStart, caret := area.TextSelection()
		if sel := [2]int{selStart, caret}; sel != d.sel {
			if !edited {
				d.hist.typing = false // the caret moved: typing after starts a step
				if n := snap.Len(); snap.LineAt(min(caret, n)) != snap.LineAt(min(d.sel[1], n)) {
					d.closers = d.closers[:0]
				}
			}
			d.sel = sel
		}
		if len(d.cmds) > 0 {
			// Edits asked for since the last frame, now that the selection
			// is known; the next frame shows them.
			cmds := d.cmds
			d.cmds = nil
			for _, cmd := range cmds {
				cmd(d)
			}
			a.invalidate()
		}
		caretLine := snap.LineAt(min(caret, snap.Len()))
		caretMoved := caret != d.caret && d.caretSeen

		if caret != d.caret || !d.caretSeen {
			if d.caretSeen {
				a.revealColumn(d, snap, caret, view.W, textX)
			}
			d.caret, d.caretSeen = caret, true
		}
		// Show the caret's line before the area takes the offset, so that
		// it finds the caret in view and leaves the offset be.
		d.sticky = append(d.sticky[:0], a.scrollToCaret(d, snap, caretLine, view.H, jump, caretMoved || edited)...)
		var sticky []stickyRow
		if len(d.sticky) > 0 {
			sticky = a.stickyRows(d, snap, d.sticky)
		}

		// The lines in view.
		first := max(0, int((d.vscroll.Y-editorPadY)/f.lineH))
		last := min(lines-1, first+int(view.H/f.lineH)+1)
		for i := first; i <= last; i++ {
			s, e := snap.LineRange(i)
			if e-s > d.widest {
				d.widest = max(d.widest, columns(snap.Slice(s, e)))
			}
		}
		contentW := max(view.W, textX+float32(d.widest)*f.charW+textPadR)
		area.TrackScroll(&d.vscroll).Width(contentW).
			// The area is as wide as its widest line: keep its scroll bar
			// at the right of the view, wherever it scrolls sideways.
			ScrollbarInsets(2, max(0, d.hscroll.MaxX-d.hscroll.X)+2, 2, 0)

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
		deco := a.decorate(d, snap, first, last)
		a.refreshGit(d)
		marks := d.git.marks
		gitRuns := d.gitRuns()
		guideW := float32(d.indent.cols()) * charW
		rows := a.overviewRows(d, snap, view.H)
		spaces := a.selectedWhitespace(d, snap, first, last)
		area.Draw(func(pt *ui.Painter, r ui.Rect) {
			lineY := func(i int) float32 { return r.Y + editorPadY + float32(i)*lineH - d.vscroll.Y }
			if y := lineY(caretLine); y+lineH > r.Y && y < r.Y+r.H {
				pt.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: lineH}, p.line, 0)
			}
			for _, w := range deco.words {
				pt.Fill(ui.Rect{X: r.X + textX + w.x0 - 1, Y: lineY(w.line) + 1, W: w.x1 - w.x0 + 2, H: lineH - 2}, p.word, 3)
			}
			// The indent guides, at the start of each level of a line's
			// indent, and brighter along the caret's block.
			for k, level := range deco.levels {
				i := deco.first + k
				y := lineY(i)
				for l := range level {
					col := p.guide
					if l == deco.active-1 && i >= deco.activeFrom && i <= deco.activeTo {
						col = p.guideNow
					}
					pt.Fill(ui.Rect{X: r.X + textX + float32(l)*guideW, Y: y, W: 1, H: lineH}, col, 0)
				}
			}
			for _, b := range deco.brackets {
				box := ui.Rect{X: r.X + textX + b.x0 - 0.5, Y: lineY(b.line) + 1.5, W: b.x1 - b.x0 + 1, H: lineH - 3}
				pt.Fill(box, p.word, 2)
				pt.Stroke(box, p.bracket, 2, 1)
			}
			// Spaces and tabs selected show, faint.
			for _, w := range spaces {
				cy := lineY(w.line) + lineH/2
				if w.tab {
					x0, x1 := r.X+textX+w.x0+2, r.X+textX+w.x1-3
					pt.Line(x0, cy, x1, cy, 1, p.faint)
					pt.Line(x1-3, cy-3, x1, cy, 1, p.faint)
					pt.Line(x1-3, cy+3, x1, cy, 1, p.faint)
				} else {
					cx := r.X + textX + (w.x0+w.x1)/2
					pt.Fill(ui.Rect{X: cx - 1, Y: cy - 1, W: 2, H: 2}, p.faint, 1)
				}
			}
			for _, m := range matches {
				box := ui.Rect{X: r.X + textX + m.x0 - 1, Y: lineY(m.line) + 1, W: m.x1 - m.x0 + 2, H: lineH - 2}
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
			top := d.vscroll.Y
			from := max(0, int((top-editorPadY)/lineH))
			to := min(lines-1, from+int(r.H/lineH)+1)
			stuck := float32(len(sticky)) * lineH
			_, numH := pt.MeasureText(0, ui.Span{Text: "0", Font: family, Size: size})
			number := func(i int, y float32, col ui.Color) {
				label := strconv.Itoa(i + 1)
				pt.RichText(x+gutterW-14-float32(len(label))*charW, y+(lineH-numH)/2, 0,
					ui.Span{Text: label, Font: family, Size: size, Color: col})
			}

			// The lines that open the blocks in view stick at the top, over
			// the text, scrolling sideways with it.
			for k, row := range sticky {
				y := r.Y + float32(k)*lineH
				pt.Fill(ui.Rect{X: x, Y: y, W: view.W, H: lineH}, p.editor, 0)
				_, h := pt.MeasureText(0, row.spans...)
				pt.RichText(r.X+textX+row.indent, y+(lineH-h)/2, 0, row.spans...)
			}

			pt.Fill(ui.Rect{X: x, Y: r.Y, W: textX - 2, H: r.H}, p.editor, 0)
			if d.hscroll.X > 0 {
				// A shadow at the edge the text scrolls under.
				pt.FillGradient(ui.Rect{X: x + textX - 2, Y: r.Y, W: 6, H: r.H}, ui.LinearGradient{
					From: ui.RGBA(0, 0, 0, 0.18), To: ui.RGBA(0, 0, 0, 0), Angle: 90,
				}, 0)
			}
			// Along the right edge, where the scroll bar goes, the whole
			// document: the lines changed since the last commit, find's
			// matches and the caret's line.
			if lines > 1 && len(gitRuns)+len(rows) > 0 {
				right := x + view.W - 10
				for _, g := range gitRuns {
					y, h := r.Y+g.from*r.H, max((g.to-g.from)*r.H, 2)
					switch g.kind {
					case linediff.Added:
						pt.Fill(ui.Rect{X: right, Y: y, W: 3, H: h}, p.added, 0)
					case linediff.Modified:
						pt.Fill(ui.Rect{X: right, Y: y, W: 3, H: h}, p.changed, 0)
					default:
						pt.Fill(ui.Rect{X: right, Y: y - 1, W: 3, H: 3}, p.removed, 0)
					}
				}
				for _, f := range rows {
					pt.Fill(ui.Rect{X: right + 4, Y: r.Y + f*r.H, W: 5, H: 2}, p.matchRing, 1)
				}
				caretAt := float32(caretLine) / float32(lines)
				pt.Fill(ui.Rect{X: right, Y: r.Y + caretAt*r.H - 1, W: 9, H: 2}, p.text.Alpha(0.6), 1)
			}
			if top > 0 {
				// A shadow under the breadcrumbs, or the lines that stick, as
				// the text scrolls up under them.
				pt.FillGradient(ui.Rect{X: x, Y: r.Y + stuck, W: view.W, H: 6}, ui.LinearGradient{
					From: p.shadow, To: p.shadow.Alpha(0), Angle: 180,
				}, 0)
			}
			pt.Clip(ui.Rect{X: x, Y: r.Y + stuck, W: gutterW, H: r.H - stuck}, 0, func() {
				// The lines changed since the last commit: a bar by their
				// numbers, and a notch where lines were removed.
				barX := x + gutterW - 7
				for i := from; i <= min(to, len(marks)-1); i++ {
					y := r.Y + editorPadY + float32(i)*lineH - top
					switch marks[i] {
					case linediff.Added:
						pt.Fill(ui.Rect{X: barX, Y: y, W: 3, H: lineH}, p.added, 0)
					case linediff.Modified:
						pt.Fill(ui.Rect{X: barX, Y: y, W: 3, H: lineH}, p.changed, 0)
					case linediff.Deleted:
						pt.Fill(ui.Rect{X: barX, Y: y - 2.5, W: 7, H: 5}, p.removed, 2.5)
					case linediff.DeletedBelow:
						pt.Fill(ui.Rect{X: barX, Y: y + lineH - 2.5, W: 7, H: 5}, p.removed, 2.5)
					}
				}
				for i := from; i <= to; i++ {
					col := p.gutter
					if i == caretLine && focused {
						col = p.gutterNow
					} else if i == caretLine {
						col = p.muted
					}
					number(i, r.Y+editorPadY+float32(i)*lineH-top, col)
				}
			})
			for k, row := range sticky {
				number(row.line, r.Y+float32(k)*lineH, p.muted)
			}
		})
	})
}

// scrollToCaret scrolls the editor to show the caret's line, below the
// lines that stick at the top, and returns those lines. jump brings a line
// out of view a third of the way down, as going to a line does; reveal
// scrolls as little as it takes, as the caret moves.
//
// It runs before the text area takes the offset, which would otherwise
// scroll to the caret itself, under the lines that stick.
func (a *app) scrollToCaret(d *document, snap ui.TextSnapshot, line int, viewH float32, jump, reveal bool) []int {
	lineH := a.font.lineH
	sticky := a.stickyAt(d, snap, viewH)
	if viewH <= 0 || !jump && !reveal {
		return sticky
	}
	end := max(0, 2*editorPadY+float32(snap.LineCount())*lineH-viewH)
	top := editorPadY + float32(line)*lineH // in the content
	if jump {
		stuck := float32(len(sticky)) * lineH
		if top < d.vscroll.Y+stuck || top+lineH > d.vscroll.Y+viewH-lineH {
			d.vscroll.Y = clamp(top-viewH/3, 0, end)
			sticky = a.stickyAt(d, snap, viewH)
		}
	}
	// Scrolling changes which lines stick: until the two agree.
	for range 3 {
		y, was := top-d.vscroll.Y, d.vscroll.Y
		switch from, to := max(editorPadY, float32(len(sticky))*lineH), viewH-editorPadY; {
		case y < from:
			d.vscroll.Y -= from - y
		case y+lineH > to:
			d.vscroll.Y += y + lineH - to
		}
		d.vscroll.Y = clamp(d.vscroll.Y, 0, end)
		if d.vscroll.Y == was {
			break
		}
		sticky = a.stickyAt(d, snap, viewH)
	}
	return sticky
}

// stickyAt returns the lines that stick at the top of the editor where it
// is scrolled: above the first line wholly in view.
func (a *app) stickyAt(d *document, snap ui.TextSnapshot, viewH float32) []int {
	lineH := a.font.lineH
	if d.vscroll.Y <= editorPadY || !isCode(d) {
		return nil
	}
	topLine := int((d.vscroll.Y - editorPadY + lineH - 1) / lineH)
	return stickyLines(snap, topLine, min(maxSticky, int(viewH/lineH/3)), d.scopesOf)
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
