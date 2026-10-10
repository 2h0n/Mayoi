package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/highlight"
	"mayoi/internal/linediff"
)

// diffGeometry is where a diff's columns are, for its width.
type diffGeometry struct {
	numW  float32 // the width of a column of line numbers
	textX float32 // where the text starts, from the side's left
	half  float32 // the width of the left side, side by side
	viewW float32
	// cols are how many columns of text each side has room for, as the
	// lines wrap: of the one column only, in the unified layout.
	cols [2]int
}

// diffRailW is the width of the bar along the left of a diff's rows,
// colored as their lines changed.
const diffRailW = 3

// diffGutter is the room of the line numbers of n lines in a diff.
func (a *app) diffGutter(n int) float32 {
	return float32(max(len(strconv.Itoa(n)), 3))*a.font.charW + 16
}

func (a *app) diffGeometry(v *diffView, split bool, viewW float32) diffGeometry {
	n := 0
	for _, f := range v.files {
		n = max(n, len(f.old.lines), len(f.new.lines))
	}
	numW := a.diffGutter(n)
	l := diffGeometry{numW: numW, viewW: viewW}
	room := func(w float32) int { return max(int((w-l.textX-14)/a.font.charW), 8) }
	if split {
		l.half = float32(int(viewW / 2))
		l.textX = diffRailW + numW + 22
		l.cols = [2]int{room(l.half), room(viewW - l.half)}
	} else {
		l.textX = diffRailW + 2*numW + 22
		l.cols = [2]int{room(viewW), 0}
	}
	return l
}

// diffViewUI shows a diff: its rows painted in a pane that scrolls both
// ways, keeping the line numbers in place, or down only as its lines wrap.
func (a *app) diffViewUI(c *ui.Context, v *diffView) {
	p, f := a.pal, a.font
	a.refreshDiff(v)
	message := func(msg string) {
		ui.Column(c).Fill().Background(p.editor).Center().Children(func() {
			ui.Text(c, msg).FontSize(13).TextColor(p.muted)
		})
	}
	if len(v.files) == 0 {
		switch {
		case a.git.looked && a.git.repo == nil:
			message("The folder is not in a Git repository.")
		case a.git.status != nil:
			message("No changes.")
		default:
			ui.Box(c).Fill().Background(p.editor)
		}
		return
	}
	lineH, charW := f.lineH, f.charW
	view := v.pane.Bounds(c)
	if view.W > 0 {
		v.viewW, v.viewH = view.W, view.H
	}
	v.split = a.splits(v, v.viewW)
	l := a.diffGeometry(v, v.split, v.viewW)
	a.layoutRows(v, l)
	rows := v.rows
	wrap := v.built.wrap
	widest := 0
	for _, file := range v.files {
		widest = max(widest, file.widest)
	}
	textW := float32(widest)*charW + textPadR
	contentW := max(view.W, l.textX+textW)
	if v.split {
		contentW = view.W + max(0, textW-(l.half-l.textX))
	}
	if wrap {
		contentW, v.scroll.X = view.W, 0
	}
	contentH := 2*editorPadY + float32(v.tops[len(rows)])*lineH
	if j := v.jump; j >= 0 && view.H > 0 {
		v.jump = -1
		if j < len(rows) {
			y := editorPadY + float32(v.tops[j])*lineH
			h := float32(rows[j].h) * lineH
			top := v.scroll.Y
			switch v.jumpTo {
			case jumpTop:
				top = y
			case jumpReveal:
				if y < top {
					top = y
				} else if y+h > top+view.H {
					top = y + h - view.H
				}
			default:
				if y < top || y+h > top+view.H {
					top = y - view.H/3
				}
			}
			v.scroll.Y = clamp(top, 0, max(0, contentH-view.H))
		}
	}
	first, last := 0, -1
	if len(rows) > 0 {
		first, last = v.rowAt(v.scroll.Y, lineH), v.rowAt(v.scroll.Y+view.H, lineH)
	}
	// The files read are those within a view's height of it, and the
	// source control view chooses the file chosen, or at its top.
	if len(rows) > 0 {
		pad := int(view.H/lineH) + 1
		v.near = [2]int{rows[max(0, first-pad)].file, rows[min(len(rows)-1, last+pad)].file + 1}
		at := v.focusRowAt(v.topRow(lineH))
		if v.cursor >= first && v.cursor <= last {
			at = v.cursor
		}
		if at >= 0 {
			if fl := v.files[rows[at].file]; fl != v.synced {
				v.synced = fl
				a.post(func() { a.chooseScmFile(fl) })
			}
		}
	}
	// The lines of each side of each file in view, to highlight.
	for k := first; k <= last; {
		file := rows[k].file
		lo, hi := [2]int{-1, -1}, [2]int{-1, -1}
		for ; k <= last && rows[k].file == file; k++ {
			if rows[k].kind != rowLine {
				continue
			}
			for side, n := range [2]int{rows[k].old, rows[k].new} {
				if n >= 0 {
					if lo[side] < 0 {
						lo[side] = n
					}
					hi[side] = n
				}
			}
		}
		fl := v.files[file]
		for side := range 2 {
			if s := fl.side(side == 1); lo[side] >= 0 && s.src != nil {
				a.highlighter(fl, s).Request(s.src, lo[side], hi[side])
			}
		}
	}
	focused := v.pane.FocusWithin(c)
	ui.ScrollBoth(c.Key("diff")).TrackScroll(&v.scroll).Bind(&v.pane).Fill().Background(p.editor).
		Focusable().FocusRing(false).Label("Diff of the Changes").
		HandleInput(func(ev ui.InputEvent) bool { return a.diffInput(v, ev) }).Children(func() {
		box := ui.Box(c).Width(contentW).Height(contentH)
		// The buttons of the file's header under the pointer, or chosen.
		hovered := -1
		if box.Hovered() {
			if _, y, ok := box.PointerPosition(); ok {
				hovered = v.rowAt(y, lineH)
			}
		}
		var buttons ui.Rect
		box.Children(func() {
			for _, i := range []int{hovered, v.cursor} {
				if i < first || i > last || rows[i].kind != rowFile || buttons.W > 0 {
					continue
				}
				buttons = a.fileButtons(c, v, v.files[rows[i].file], v.scroll.X+view.W, editorPadY+float32(v.tops[i])*lineH)
			}
		})
		// A click chooses a row, and opens a gap or folds a file; a double
		// click opens the file at a line, unless its first click did that.
		if clicked, double := box.Clicked(), box.DoubleClicked(); clicked || double {
			if x, y, ok := box.PointerPosition(); ok && len(rows) > 0 && !buttons.Contains(x, y) {
				i := v.rowAt(y, lineH)
				a.post(func() {
					v.pane.Focus()
					if i >= len(v.rows) {
						return
					}
					if double {
						if !v.acted {
							a.activateRow(v, i, true)
						}
						return
					}
					v.setCursor(i)
					v.acted = v.rows[i].kind != rowLine && v.rows[i].kind != rowHunk
					a.activateRow(v, i, false)
				})
			}
		}
		box.Draw(func(pt *ui.Painter, r ui.Rect) {
			vx, vy := r.X+v.scroll.X, r.Y+v.scroll.Y // the view's corner
			_, numH := pt.MeasureText(0, ui.Span{Text: "0", Font: f.family, Size: f.size})
			mid := (lineH - numH) / 2
			txt := func(x, y float32, s string, col ui.Color) float32 {
				span := ui.Span{Text: s, Font: f.family, Size: f.size, Color: col}
				w, _ := pt.MeasureText(0, span)
				pt.RichText(x, y+mid, 0, span)
				return w
			}
			number := func(n int, right, y float32, col ui.Color) {
				label := strconv.Itoa(n + 1)
				txt(right-float32(len(label))*charW, y, label, col)
			}
			rail := func(x, y, h float32, col ui.Color) {
				pt.Fill(ui.Rect{X: x, Y: y, W: diffRailW, H: h}, col, 0)
			}
			dim := func(col ui.Color) ui.Color { return col.Mix(p.editor, 0.55) }
			neutral := p.gutter.Alpha(0.45)
			for i := first; i <= last; i++ {
				row := &rows[i]
				file := v.files[row.file]
				y := r.Y + editorPadY + float32(v.tops[i])*lineH
				h := float32(row.h) * lineH
				full := ui.Rect{X: vx, Y: y, W: view.W, H: h}
				switch row.kind {
				case rowSpace:
					continue
				case rowFile:
					a.paintFileHeader(pt, file, full, txt)
				case rowSection:
					pt.Fill(full, p.chrome, 0)
					label, sides := "CHANGES", "Index ↔ Working Tree"
					if file.base == diffStaged {
						label, sides = "STAGED CHANGES", "HEAD ↔ Index"
					}
					n := 0
					for _, o := range v.files {
						if o.base == file.base {
							n++
						}
					}
					at := vx + diffRailW + 8
					at += txt(at, y, label, p.muted)
					at += txt(at+charW, y, fmt.Sprint(n), p.faint) + 2*charW
					txt(at+charW, y, sides, p.faint)
				case rowNote:
					txt(vx+l.textX, y, row.label, p.faint)
				case rowHunk:
					pt.Fill(full, p.text.Alpha(0.035), 0)
					rail(vx, y, h, neutral)
					at := vx + diffRailW + 10
					head, scope := row.label, ""
					if k := strings.Index(row.label[min(3, len(row.label)):], " @@"); k >= 0 {
						head, scope = row.label[:k+6], row.label[k+6:]
					}
					at += txt(at, y, head, p.faint)
					txt(at, y, scope, p.muted)
				case rowGap:
					pt.Fill(full, p.text.Alpha(0.025), 0)
					rail(vx, y, h, neutral)
					txt(vx+diffRailW+10, y, fmt.Sprintf("▸ %s", plural(row.n, "unchanged line")), p.muted)
				case rowLine:
					if v.split {
						for side := range 2 {
							x0 := vx + float32(side)*l.half
							w := l.half
							if side == 1 {
								w = view.W - l.half
							}
							n, tint := row.old, p.removed
							if side == 1 {
								n, tint = row.new, p.added
							}
							pt.Clip(ui.Rect{X: x0, Y: vy, W: w, H: view.H}, 0, func() {
								if n < 0 {
									a.paintFiller(pt, ui.Rect{X: x0, Y: y, W: w, H: h})
									return
								}
								changed := row.hunk >= 0
								if changed {
									pt.Fill(ui.Rect{X: x0, Y: y, W: w, H: h}, tint.Alpha(0.16), 0)
								}
								peer := -1
								if changed && row.old >= 0 && row.new >= 0 {
									peer = row.new
									if side == 1 {
										peer = row.old
									}
								}
								pt.Clip(ui.Rect{X: x0 + l.textX - 4, Y: vy, W: w - l.textX + 4, H: view.H}, 0, func() {
									a.paintDiffLine(pt, file, side == 1, n, peer, r.X+float32(side)*l.half+l.textX, y, tint, l.cols[side], wrap)
								})
								numCol := p.gutter
								switch {
								case changed && v.inCurrent(row):
									rail(x0, y, h, tint)
									numCol = tint
								case changed:
									rail(x0, y, h, dim(tint))
									numCol = tint
								default:
									rail(x0, y, h, neutral)
								}
								number(n, x0+diffRailW+l.numW, y, numCol)
								if changed {
									mark := "-"
									if side == 1 {
										mark = "+"
									}
									txt(x0+diffRailW+l.numW+8, y, mark, tint)
								}
							})
						}
						pt.Fill(ui.Rect{X: vx + l.half - 0.5, Y: y, W: 1, H: h}, p.border, 0)
						break
					}
					changed := row.hunk >= 0
					tint := p.added
					if row.new < 0 {
						tint = p.removed
					}
					if changed {
						pt.Fill(ui.Rect{X: r.X, Y: y, W: max(r.W, view.W), H: h}, tint.Alpha(0.16), 0)
					}
					n, newSide := row.line()
					pt.Clip(ui.Rect{X: vx + l.textX - 4, Y: vy, W: view.W - l.textX + 4, H: view.H}, 0, func() {
						a.paintDiffLine(pt, file, newSide, n, row.peer, r.X+l.textX, y, tint, l.cols[0], wrap)
					})
					// The gutter, which stays as the text scrolls sideways.
					gut := ui.Rect{X: vx, Y: y, W: l.textX - 4, H: h}
					pt.Fill(gut, p.editor, 0)
					numCol := p.gutter
					switch {
					case changed:
						pt.Fill(gut, tint.Alpha(0.16), 0)
						numCol = tint
						if v.inCurrent(row) {
							rail(vx, y, h, tint)
						} else {
							rail(vx, y, h, dim(tint))
						}
					default:
						rail(vx, y, h, neutral)
					}
					if row.old >= 0 {
						number(row.old, vx+diffRailW+l.numW, y, numCol)
					}
					if row.new >= 0 {
						number(row.new, vx+diffRailW+2*l.numW, y, numCol)
					}
					if changed {
						mark := "+"
						if row.new < 0 {
							mark = "-"
						}
						txt(vx+diffRailW+2*l.numW+8, y, mark, tint)
					}
				}
				if i == v.cursor {
					col := p.text.Alpha(0.05)
					if focused {
						col = p.text.Alpha(0.09)
					}
					pt.Fill(full, col, 0)
					pt.Fill(ui.Rect{X: vx, Y: y, W: diffRailW, H: h}, p.accent, 0)
				}
			}
			// Along the right edge, the hunks of every file.
			if total := v.tops[len(rows)]; total > 1 {
				right := vx + view.W - 10
				for _, at := range v.starts {
					s := rows[at]
					end := at
					for end < len(rows) && rows[end].changed() && rows[end].hunk == s.hunk && rows[end].file == s.file {
						end++
					}
					hk := v.files[s.file].hunks[s.hunk]
					y0 := vy + float32(v.tops[at])/float32(total)*view.H
					hh := max(float32(v.tops[end]-v.tops[at])/float32(total)*view.H, 2)
					col := p.changed
					switch {
					case hk.ALen == 0:
						col = p.added
					case hk.BLen == 0:
						col = p.removed
					}
					pt.Fill(ui.Rect{X: right, Y: y0, W: 3, H: hh}, col, 0)
				}
			}
		})
	})
}

// paintFileHeader paints the header of a file in the diff of every file:
// whether its lines show, its path and letter, and the lines its changes
// add and remove.
func (a *app) paintFileHeader(pt *ui.Painter, f *diffFile, r ui.Rect, txt func(x, y float32, s string, col ui.Color) float32) {
	p := a.pal
	pt.Fill(r, p.chrome, 0)
	pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, p.border, 0)
	pt.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - 1, W: r.W, H: 1}, p.border, 0)
	chevron := "▾"
	if f.folded {
		chevron = "▸"
	}
	x := r.X + diffRailW + 8
	txt(x, r.Y, chevron, p.muted)
	x += 2 * a.font.charW
	col := p.text
	if f.letter != 0 {
		col = gitColor(p, f.letter)
	}
	rel := a.relOrPath(f.path)
	if k := strings.LastIndexAny(rel, `/\`); k >= 0 {
		x += txt(x, r.Y, rel[:k+1], p.muted)
		rel = rel[k+1:]
	}
	x += txt(x, r.Y, rel, col)
	if f.letter != 0 {
		txt(x+a.font.charW, r.Y, string(f.letter), col)
	}
	if !f.loaded || f.binary {
		return
	}
	add, del := f.counts()
	plus, minus := fmt.Sprintf("+%d", add), fmt.Sprintf(" −%d", del)
	w := float32(len([]rune(plus+minus))) * a.font.charW
	at := r.X + r.W - 18 - w
	at += txt(at, r.Y, plus, p.added)
	txt(at, r.Y, minus, p.removed)
}

// paintFiller paints the room across from lines the other side has.
func (a *app) paintFiller(pt *ui.Painter, r ui.Rect) {
	p := a.pal
	pt.Fill(r, p.text.Alpha(0.025), 0)
	var stripes ui.Path
	for x := r.X - r.H; x < r.X+r.W; x += 8 {
		stripes.MoveTo(x, r.Y+r.H).LineTo(x+r.H, r.Y)
	}
	pt.StrokePath(&stripes, 1, p.text.Alpha(0.04))
}

// paintDiffLine paints line n of a side of a file of a diff at x, y: its
// words that changed from the line peer of the other side, then its text
// in the colors of its tokens, on rows cols wide as it wraps.
func (a *app) paintDiffLine(pt *ui.Painter, file *diffFile, newSide bool, n, peer int, x, y float32, tint ui.Color, cols int, wrap bool) {
	f, p := a.font, a.pal
	s := file.side(newSide)
	if n < 0 || n >= len(s.lines) {
		return
	}
	text := clipLine(s.lines[n])
	shown, at := expandTabs(text)
	// Where each row starts in shown, and ends.
	breaks := []int{0}
	if wrap {
		breaks = append(breaks, wrapAt(shown, cols)...)
	}
	breaks = append(breaks, len(shown))
	// piece calls fn with each part of the run from b0 up to b1 of shown,
	// on its row: the row, and where the part starts and ends on it.
	piece := func(b0, b1 int, fn func(k int, x0, x1 float32, s0, s1 int)) {
		for k := 0; k+1 < len(breaks); k++ {
			lo, hi := breaks[k], breaks[k+1]
			s0, s1 := max(b0, lo), min(b1, hi)
			if s0 >= s1 && !(b0 == b1 && b0 == lo) {
				continue
			}
			fn(k, f.width(shown[lo:s0]), f.width(shown[lo:s1]), s0, s1)
		}
	}
	if peer >= 0 {
		key := [2]int{n, peer}
		if !newSide {
			key = [2]int{peer, n}
		}
		ws, ok := file.words[key]
		if !ok && key[0] < len(file.old.lines) && key[1] < len(file.new.lines) {
			o, w := linediff.Words(file.old.lines[key[0]], file.new.lines[key[1]])
			ws = [2][]linediff.Span{o, w}
			if file.words == nil {
				file.words = map[[2]int][2][]linediff.Span{}
			}
			file.words[key] = ws
		}
		spans := ws[0]
		if newSide {
			spans = ws[1]
		}
		for _, sp := range spans {
			if sp.Start >= len(text) {
				continue
			}
			piece(at(sp.Start), at(min(sp.End, len(text))), func(k int, x0, x1 float32, _, _ int) {
				pt.Fill(ui.Rect{X: x + x0, Y: y + float32(k)*f.lineH, W: max(x1-x0, 2), H: f.lineH}, tint.Alpha(0.30), 0)
			})
		}
	}
	// The runs of shown in the colors of their tokens.
	type run struct {
		from, to int
		col      ui.Color
	}
	var runs []run
	plain := func(from, to int) {
		if to > from {
			runs = append(runs, run{from, to, p.text})
		}
	}
	var toks []highlight.Token
	if s.hl != nil && len(s.lines[n]) <= highlight.MaxLineRunes {
		toks = s.hl.Line(n, s.lines[n])
	}
	if len(toks) == 0 {
		plain(0, len(shown))
	} else {
		// Tokens are in runes from the line's start.
		runeAt := make([]int, 0, len(text)+1)
		for i := range text {
			runeAt = append(runeAt, i)
		}
		runeAt = append(runeAt, len(text))
		byteOf := func(r int) int { return runeAt[min(r, len(runeAt)-1)] }
		done := 0
		for _, t := range toks {
			b0, b1 := at(byteOf(t.Start)), at(byteOf(t.End))
			if b0 < done || b1 <= b0 {
				continue
			}
			plain(done, b0)
			runs = append(runs, run{b0, b1, p.syntax[t.Kind]})
			done = b1
		}
		plain(done, len(shown))
	}
	rowSpans := make([][]ui.Span, len(breaks)-1)
	for _, rn := range runs {
		piece(rn.from, rn.to, func(k int, _, _ float32, s0, s1 int) {
			if s1 > s0 {
				rowSpans[k] = append(rowSpans[k], ui.Span{Text: shown[s0:s1], Font: f.family, Size: f.size, Color: rn.col})
			}
		})
	}
	for k, spans := range rowSpans {
		if len(spans) > 0 {
			_, h := pt.MeasureText(0, spans[0])
			pt.RichText(x, y+float32(k)*f.lineH+(f.lineH-h)/2, 0, spans...)
		}
	}
}

// fileButtons are the buttons of a file's header, whose right is at right
// and top at y in the diff's content: opening the file, staging or
// unstaging it, and discarding its changes. It returns where they are.
func (a *app) fileButtons(c *ui.Context, v *diffView, f *diffFile, right, y float32) ui.Rect {
	e := f.entry
	type button struct {
		icon  *ui.SVG
		label string
		fn    func()
	}
	var bs []button
	if f.letter != 'D' || f.base == diffStaged {
		bs = append(bs, button{iconOpenFile, "Open File", func() { a.openFileAt(f.path, 0, true) }})
	}
	if f.base == diffStaged {
		bs = append(bs, button{iconMinus, "Unstage Changes", func() { a.unstage(entryPaths(e, true)...) }})
	} else {
		bs = append(bs, button{iconUndo, "Discard Changes…", func() { a.discard(e) }},
			button{iconPlus, "Stage Changes", func() { a.stage(e.Path) }})
	}
	add, del := f.counts()
	counts := float32(len([]rune(fmt.Sprintf("+%d −%d", add, del)))) * a.font.charW
	w := float32(len(bs)) * 22
	r := ui.Rect{X: right - 18 - counts - 12 - w, Y: y, W: w, H: a.font.lineH}
	ui.Row(c.Key("file-buttons")).Absolute().Left(r.X).Top(y + (a.font.lineH-20)/2).Gap(2).Children(func() {
		for _, b := range bs {
			if a.smallButton(c, b.icon, b.label) {
				a.post(b.fn)
			}
		}
	})
	return r
}
