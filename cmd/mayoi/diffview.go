package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/highlight"
	"mayoi/internal/linediff"
)

// diffBase is what a diff compares: which version of a file with which.
type diffBase int

const (
	diffHead   diffBase = iota // HEAD with the work tree
	diffIndex                  // the index with the work tree: the changes not staged
	diffStaged                 // HEAD with the index: the changes staged
)

// sides names the two versions a diff compares.
func (b diffBase) sides() (old, new string) {
	switch b {
	case diffIndex:
		return "Index", "Working Tree"
	case diffStaged:
		return "HEAD", "Index"
	}
	return "HEAD", "Working Tree"
}

// key is how the settings name the base.
func (b diffBase) key() string { return [...]string{"head", "index", "staged"}[b] }

func parseDiffBase(s string) (diffBase, bool) {
	for b := diffHead; b <= diffStaged; b++ {
		if b.key() == s {
			return b, true
		}
	}
	return 0, false
}

// diffRow is a row of a diff as it shows: a line of each side, -1 where a
// side has none, and the hunk it is part of, -1 for lines unchanged. In
// the unified layout a row has one side when it changed, and peer is the
// line of the other side it was modified from, or into, -1 for none.
type diffRow struct{ old, new, hunk, peer int }

// diffSide is a version of the file a diff shows.
type diffSide struct {
	text  string
	ok    bool // the version has the file
	lines []string
	src   *lineSource
	hl    *highlight.Highlighter
}

// diffView shows the changes of a file between two versions, in a tab:
// the lines removed and added interleaved, or side by side.
type diffView struct {
	path  string
	base  diffBase
	split bool

	old, new diffSide
	binary   bool
	hunks    []linediff.Hunk
	// unified and pairs are the rows of the two layouts, and their first
	// rows of each hunk.
	unified, pairs           []diffRow
	unifiedHunks, pairsHunks []int
	widest                   int // the longest line, in columns
	loaded                   bool
	err                      string

	// gitStale has the sides Git keeps read again, diskStale the work
	// tree's file; docVersion is the version of the open document's text
	// compared last.
	gitStale, diskStale bool
	docVersion          uint64
	running, waiting    bool
	started             time.Time

	scroll ui.ScrollState
	pane   ui.Handle
	// current is the hunk gone to last; jump a row to scroll to in the
	// next frame, -1 for none.
	current, jump int
	words         map[[2]int][2][]linediff.Span
	viewW         float32
}

func newDiffView(path string, base diffBase, split bool) *diffView {
	return &diffView{path: path, base: base, split: split, gitStale: true, diskStale: true, current: -1, jump: -1}
}

func (v *diffView) name() string {
	old, new := v.base.sides()
	return fmt.Sprintf("%s (%s ↔ %s)", filepath.Base(v.path), old, new)
}

// rows returns the rows of the layout shown, and their first rows of each
// hunk.
func (v *diffView) rows() ([]diffRow, []int) {
	if v.split {
		return v.pairs, v.pairsHunks
	}
	return v.unified, v.unifiedHunks
}

// stop lets go of the highlighters.
func (v *diffView) stop() {
	v.old.hl.Close()
	v.new.hl.Close()
}

// lineSource is lines of text as the highlighter reads them.
type lineSource struct {
	text    string
	starts  []int // the byte offset of each line
	version uint64
}

var sourceVersions atomic.Uint64

func newLineSource(lines []string) *lineSource {
	s := &lineSource{text: strings.Join(lines, "\n"), starts: make([]int, len(lines)), version: sourceVersions.Add(1)}
	at := 0
	for i, l := range lines {
		s.starts[i] = at
		at += len(l) + 1
	}
	return s
}

func (s *lineSource) Version() uint64 { return s.version }
func (s *lineSource) LineCount() int  { return len(s.starts) }
func (s *lineSource) LineRange(i int) (int, int) {
	if i+1 < len(s.starts) {
		return s.starts[i], s.starts[i+1] - 1
	}
	return s.starts[i], len(s.text)
}
func (s *lineSource) Slice(start, end int) string { return s.text[start:end] }

// diffViewPause is the least time between two comparisons as a document
// is typed in.
const diffViewPause = 150 * time.Millisecond

// openDiff shows the changes of a file in a tab of the column that has the
// keyboard, after the tab shown, or the tab that shows them already.
func (a *app) openDiff(path string, base diffBase, focus bool) {
	col := a.column()
	if i := col.showingDiff(path, base); i >= 0 {
		a.activate(col, i, focus)
		return
	}
	at := min(col.active+1, len(col.tabs))
	a.addTab(col, at, &colTab{diff: newDiffView(path, base, a.settings.DiffSplit)})
	if a.tree != nil {
		a.tree.Watch(path)
	}
	a.activate(col, at, focus)
}

// openChanges shows the changes of the file of the tab shown since the
// last commit.
func (a *app) openChanges() {
	t := a.activeTab()
	switch {
	case t == nil || t.path() == "":
	case t.diff != nil:
		a.openFileAt(t.diff.path, a.diffTopLine(t.diff), true)
	default:
		a.openDiff(t.path(), diffHead, true)
	}
}

// showingDiff returns the place of the tab of the column that shows the
// changes of path against base, -1 when none does.
func (col *column) showingDiff(path string, base diffBase) int {
	for i, t := range col.tabs {
		if t.diff != nil && t.diff.path == path && t.diff.base == base {
			return i
		}
	}
	return -1
}

// diffDoc returns the open document of the file a diff shows, whose text
// is the work tree's side, nil when it is not open or the diff compares
// the index.
func (a *app) diffDoc(v *diffView) *document {
	if v.base == diffStaged {
		return nil
	}
	if i := a.docIndex(v.path); i >= 0 {
		return a.docs[i]
	}
	return nil
}

// refreshDiff compares the sides of a diff again in the background when
// one may have changed.
func (a *app) refreshDiff(v *diffView) {
	r := a.git.repo
	if v.running || v.waiting {
		return
	}
	if r == nil {
		if a.git.looked && !v.loaded {
			v.loaded, v.err = true, "The file is not in a Git repository."
		}
		return
	}
	doc := a.diffDoc(v)
	var snap ui.TextSnapshot
	edited := false
	if doc != nil {
		snap = doc.buf.Snapshot()
		edited = snap.Version() != v.docVersion
	}
	if v.loaded && !v.gitStale && !edited && !(v.diskStale && doc == nil) {
		return
	}
	if wait := diffViewPause - time.Since(v.started); v.loaded && !v.gitStale && wait > 0 {
		v.waiting = true
		time.AfterFunc(wait, func() { a.post(func() { v.waiting = false }) })
		return
	}
	v.running, v.started = true, time.Now()
	readGit, readDisk := v.gitStale || !v.loaded, doc == nil && (v.diskStale || !v.loaded)
	v.gitStale, v.diskStale = false, false
	if doc != nil {
		v.docVersion = snap.Version()
	}
	old, new := v.old, v.new
	path, base := v.path, v.base
	opening := !v.loaded
	go func() {
		var err error
		read := func(rev string) (string, bool) {
			text, ok, e := r.Show(rev, path)
			if e != nil && err == nil {
				err = e
			}
			return text, ok
		}
		if readGit {
			switch base {
			case diffIndex:
				old.text, old.ok = read("")
			default:
				old.text, old.ok = read("HEAD")
			}
			if base == diffStaged {
				new.text, new.ok = read("")
			}
		}
		switch {
		case doc != nil:
			new.text, new.ok = snap.String(), true
		case readDisk && base != diffStaged:
			data, e := os.ReadFile(path)
			new.text, new.ok = strings.ReplaceAll(string(data), "\r\n", "\n"), e == nil
			if e != nil && !errors.Is(e, os.ErrNotExist) && err == nil {
				err = e
			}
		}
		binary := isBinary([]byte(old.text)) || isBinary([]byte(new.text))
		oldLines, newLines := splitLines(old.text, old.ok), splitLines(new.text, new.ok)
		var hunks []linediff.Hunk
		if !binary {
			hunks = linediff.Hunks(oldLines, newLines)
		}
		unified, uh, pairs, ph := diffRows(oldLines, newLines, hunks)
		widest := 0
		for _, ls := range [][]string{oldLines, newLines} {
			for _, l := range ls {
				if len(l) > widest {
					widest = max(widest, columns(l[:min(len(l), maxDiffLine)]))
				}
			}
		}
		a.post(func() {
			v.running = false
			if err != nil {
				v.err = err.Error()
			} else {
				v.err = ""
			}
			v.loaded, v.binary, v.hunks, v.widest = true, binary, hunks, widest
			v.unified, v.unifiedHunks, v.pairs, v.pairsHunks = unified, uh, pairs, ph
			v.words = nil
			a.setDiffSide(v, &v.old, old, oldLines)
			a.setDiffSide(v, &v.new, new, newLines)
			if v.current >= len(hunks) {
				v.current = -1
			}
			// A diff just opened shows its first change, unless it is in
			// view already.
			if opening && len(hunks) > 0 {
				v.stepHunk(1)
			}
		})
	}()
}

// setDiffSide takes a side's new text, to highlight.
func (a *app) setDiffSide(v *diffView, s *diffSide, read diffSide, lines []string) {
	s.ok = read.ok
	if s.src != nil && s.text == read.text {
		return
	}
	s.text, s.lines, s.src = read.text, lines, newLineSource(lines)
	if s.hl == nil {
		s.hl = highlight.New(highlight.Lexer(v.path), a.invalidate)
	}
}

// splitLines returns the lines of a version of a file, without the empty
// one after its last newline; none when it has no file.
func splitLines(text string, ok bool) []string {
	if !ok || text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// diffRows lays out the lines of two sides and the hunks between them:
// in one column, the lines of each hunk removed before those added, and
// side by side, the lines of a hunk across from each other. It returns
// each layout's rows and their first rows of each hunk.
func diffRows(old, new []string, hunks []linediff.Hunk) (unified []diffRow, uh []int, pairs []diffRow, ph []int) {
	unified = make([]diffRow, 0, max(len(old), len(new)))
	pairs = make([]diffRow, 0, max(len(old), len(new)))
	i, j := 0, 0
	same := func(to int) {
		for ; i < to; i, j = i+1, j+1 {
			r := diffRow{i, j, -1, -1}
			unified, pairs = append(unified, r), append(pairs, r)
		}
	}
	for k, h := range hunks {
		same(h.A)
		uh, ph = append(uh, len(unified)), append(ph, len(pairs))
		m := min(h.ALen, h.BLen)
		for n := range h.ALen {
			peer := -1
			if n < m {
				peer = h.B + n
			}
			unified = append(unified, diffRow{h.A + n, -1, k, peer})
		}
		for n := range h.BLen {
			peer := -1
			if n < m {
				peer = h.A + n
			}
			unified = append(unified, diffRow{-1, h.B + n, k, peer})
		}
		for n := range max(h.ALen, h.BLen) {
			r := diffRow{-1, -1, k, -1}
			if n < h.ALen {
				r.old = h.A + n
			}
			if n < h.BLen {
				r.new = h.B + n
			}
			pairs = append(pairs, r)
		}
		i, j = h.A+h.ALen, h.B+h.BLen
	}
	same(len(old))
	return unified, uh, pairs, ph
}

// maxDiffLine is how much of a line a diff shows, in bytes: the rest of
// a longer one, as of a minified file, is cut.
const maxDiffLine = 2000

// diffGutter is the room of the line numbers of n lines in a diff.
func (a *app) diffGutter(n int) float32 {
	return float32(max(len(strconv.Itoa(n)), 3))*a.font.charW + 16
}

// diffLayout is where a diff's columns are, for its width.
type diffLayout struct {
	numW  float32 // the width of a column of line numbers
	textX float32 // where the text starts, from the side's left
	half  float32 // the width of a side, side by side
	viewW float32
}

func (a *app) diffLayout(v *diffView, viewW float32) diffLayout {
	numW := a.diffGutter(max(len(v.old.lines), len(v.new.lines)))
	l := diffLayout{numW: numW, viewW: viewW}
	if v.split {
		l.half = float32(int(viewW / 2))
		l.textX = numW + 22
	} else {
		l.textX = 2*numW + 22
	}
	return l
}

// diffViewUI shows a diff: its rows painted in a pane that scrolls both
// ways, keeping the line numbers in place.
func (a *app) diffViewUI(c *ui.Context, v *diffView) {
	p, f := a.pal, a.font
	a.refreshDiff(v)
	if !v.loaded {
		ui.Box(c).Fill().Background(p.editor)
		return
	}
	if v.binary || v.err != "" || !v.old.ok && !v.new.ok {
		msg := "The file is binary."
		switch {
		case v.err != "":
			msg = v.err
		case !v.binary:
			msg = "Neither version has the file."
		}
		ui.Column(c).Fill().Background(p.editor).Center().Children(func() {
			ui.Text(c, msg).FontSize(13).TextColor(p.muted)
		})
		return
	}
	rows, starts := v.rows()
	lineH, charW := f.lineH, f.charW
	view := v.pane.Bounds(c)
	v.viewW = view.W
	l := a.diffLayout(v, view.W)
	textW := float32(v.widest)*charW + textPadR
	contentW := max(view.W, l.textX+textW)
	if v.split {
		contentW = view.W + max(0, textW-(l.half-l.textX))
	}
	contentH := 2*editorPadY + float32(len(rows))*lineH
	if j := v.jump; j >= 0 && view.H > 0 {
		v.jump = -1
		y := editorPadY + float32(j)*lineH
		if y < v.scroll.Y || y+lineH > v.scroll.Y+view.H {
			v.scroll.Y = clamp(y-view.H/3, 0, max(0, contentH-view.H))
		}
	}
	// The rows in view, and the lines of each side they show, to
	// highlight.
	first := max(0, int((v.scroll.Y-editorPadY)/lineH))
	last := min(len(rows)-1, first+int(view.H/lineH)+1)
	for k, s := range []*diffSide{&v.old, &v.new} {
		lo, hi := -1, -1
		for i := first; i <= last && i < len(rows); i++ {
			n := rows[i].old
			if k == 1 {
				n = rows[i].new
			}
			if n >= 0 {
				if lo < 0 {
					lo = n
				}
				hi = n
			}
		}
		if lo >= 0 && s.src != nil && s.hl != nil {
			s.hl.Request(s.src, lo, hi)
		}
	}
	ui.ScrollBoth(c.Key("diff")).TrackScroll(&v.scroll).Bind(&v.pane).Fill().Background(p.editor).
		Focusable().FocusRing(false).Label("Changes " + v.name()).Children(func() {
		box := ui.Box(c).Width(contentW).Height(contentH)
		if box.DoubleClicked() {
			if _, y, ok := box.PointerPosition(); ok {
				if i := int((y - editorPadY) / lineH); i >= 0 && i < len(rows) {
					a.openFileAt(v.path, newLineNear(rows, i), true)
				}
			}
		}
		box.Draw(func(pt *ui.Painter, r ui.Rect) {
			vx, vy := r.X+v.scroll.X, r.Y+v.scroll.Y // the view's corner
			_, numH := pt.MeasureText(0, ui.Span{Text: "0", Font: f.family, Size: f.size})
			number := func(n int, right, y float32, col ui.Color) {
				label := strconv.Itoa(n + 1)
				pt.RichText(right-float32(len(label))*charW, y+(lineH-numH)/2, 0,
					ui.Span{Text: label, Font: f.family, Size: f.size, Color: col})
			}
			for i := first; i <= last; i++ {
				row := rows[i]
				y := r.Y + editorPadY + float32(i)*lineH
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
							switch {
							case n < 0:
								a.paintFiller(pt, ui.Rect{X: x0, Y: y, W: w, H: lineH})
								return
							case row.hunk >= 0:
								pt.Fill(ui.Rect{X: x0, Y: y, W: w, H: lineH}, tint.Alpha(0.13), 0)
							}
							peer := -1
							if row.hunk >= 0 && row.old >= 0 && row.new >= 0 {
								peer = row.new
								if side == 1 {
									peer = row.old
								}
							}
							pt.Clip(ui.Rect{X: x0 + l.textX - 4, Y: vy, W: w - l.textX + 4, H: view.H}, 0, func() {
								a.paintDiffLine(pt, v, side == 1, n, peer, r.X+float32(side)*l.half+l.textX, y, tint)
							})
							number(n, x0+l.numW, y, p.gutter)
							if row.hunk >= 0 {
								mark := "-"
								if side == 1 {
									mark = "+"
								}
								pt.RichText(x0+l.numW+8, y+(lineH-numH)/2, 0, ui.Span{Text: mark, Font: f.family, Size: f.size, Color: tint})
							}
						})
					}
					continue
				}
				tint := p.added
				if row.new < 0 {
					tint = p.removed
				}
				if row.hunk >= 0 {
					pt.Fill(ui.Rect{X: r.X, Y: y, W: max(r.W, view.W), H: lineH}, tint.Alpha(0.13), 0)
				}
				n := row.new
				if n < 0 {
					n = row.old
				}
				pt.Clip(ui.Rect{X: vx + l.textX - 4, Y: vy, W: view.W - l.textX + 4, H: view.H}, 0, func() {
					a.paintDiffLine(pt, v, row.new >= 0, n, row.peer, r.X+l.textX, y, tint)
				})
				// The gutter, which stays as the text scrolls sideways.
				gut := ui.Rect{X: vx, Y: y, W: l.textX - 4, H: lineH}
				pt.Fill(gut, p.editor, 0)
				if row.hunk >= 0 {
					pt.Fill(gut, tint.Alpha(0.13), 0)
				}
				if row.old >= 0 {
					number(row.old, vx+l.numW, y, p.gutter)
				}
				if row.new >= 0 {
					number(row.new, vx+2*l.numW, y, p.gutter)
				}
				if row.hunk >= 0 {
					mark := "+"
					if row.new < 0 {
						mark = "-"
					}
					pt.RichText(vx+2*l.numW+8, y+(lineH-numH)/2, 0, ui.Span{Text: mark, Font: f.family, Size: f.size, Color: tint})
				}
			}
			if v.split {
				pt.Fill(ui.Rect{X: vx + l.half - 0.5, Y: vy, W: 1, H: view.H}, p.border, 0)
			}
			// Along the right edge, the hunks of the whole file.
			if len(rows) > 1 {
				right := vx + view.W - 10
				for k, at := range starts {
					h := v.hunks[k]
					n := h.ALen + h.BLen
					if v.split {
						n = max(h.ALen, h.BLen)
					}
					y0 := vy + float32(at)/float32(len(rows))*view.H
					hh := max(float32(n)/float32(len(rows))*view.H, 2)
					col := p.changed
					switch {
					case h.ALen == 0:
						col = p.added
					case h.BLen == 0:
						col = p.removed
					}
					pt.Fill(ui.Rect{X: right, Y: y0, W: 3, H: hh}, col, 0)
				}
			}
		})
	})
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

// paintDiffLine paints line n of a side of a diff at x, y: its words that
// changed from the line peer of the other side, then its text in the
// colors of its tokens.
func (a *app) paintDiffLine(pt *ui.Painter, v *diffView, newSide bool, n, peer int, x, y float32, tint ui.Color) {
	f, p := a.font, a.pal
	s := &v.old
	if newSide {
		s = &v.new
	}
	if n < 0 || n >= len(s.lines) {
		return
	}
	text := s.lines[n]
	if len(text) > maxDiffLine {
		text = text[:maxDiffLine]
		for len(text) > 0 && !utf8.RuneStart(text[len(text)-1]) {
			text = text[:len(text)-1]
		}
	}
	shown, at := expandTabs(text)
	if peer >= 0 {
		key := [2]int{n, peer}
		if !newSide {
			key = [2]int{peer, n}
		}
		ws, ok := v.words[key]
		if !ok && key[0] < len(v.old.lines) && key[1] < len(v.new.lines) {
			o, w := linediff.Words(v.old.lines[key[0]], v.new.lines[key[1]])
			ws = [2][]linediff.Span{o, w}
			if v.words == nil {
				v.words = map[[2]int][2][]linediff.Span{}
			}
			v.words[key] = ws
		}
		spans := ws[0]
		if newSide {
			spans = ws[1]
		}
		for _, sp := range spans {
			if sp.Start >= len(text) {
				continue
			}
			x0 := f.width(shown[:at(sp.Start)])
			x1 := f.width(shown[:at(min(sp.End, len(text)))])
			pt.Fill(ui.Rect{X: x + x0, Y: y + 1, W: max(x1-x0, 2), H: f.lineH - 2}, tint.Alpha(0.32), 2)
		}
	}
	var spans []ui.Span
	plain := func(s string) {
		if s != "" {
			spans = append(spans, ui.Span{Text: s, Font: f.family, Size: f.size, Color: p.text})
		}
	}
	var toks []highlight.Token
	if s.hl != nil && len(s.lines[n]) <= highlight.MaxLineRunes {
		toks = s.hl.Line(n, s.lines[n])
	}
	if len(toks) == 0 {
		plain(shown)
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
			plain(shown[done:b0])
			spans = append(spans, ui.Span{Text: shown[b0:b1], Font: f.family, Size: f.size, Color: p.syntax[t.Kind]})
			done = b1
		}
		plain(shown[done:])
	}
	if len(spans) > 0 {
		_, h := pt.MeasureText(0, spans[0])
		pt.RichText(x, y+(f.lineH-h)/2, 0, spans...)
	}
}

// expandTabs returns a line with its tabs as spaces to the next stop, and
// where each byte of the line went in it.
func expandTabs(s string) (string, func(b int) int) {
	if strings.IndexByte(s, '\t') < 0 {
		return s, func(b int) int { return b }
	}
	var out bytes.Buffer
	at := make([]int, len(s)+1)
	col := 0
	for i := 0; i < len(s); {
		at[i] = out.Len()
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == '\t' {
			k := tabWidth - col%tabWidth
			out.WriteString(strings.Repeat(" ", k))
			col += k
		} else {
			out.WriteString(s[i : i+n])
			col++
		}
		for j := 1; j < n; j++ {
			at[i+j] = out.Len()
		}
		i += n
	}
	at[len(s)] = out.Len()
	return out.String(), func(b int) int { return at[min(max(b, 0), len(s))] }
}

// newLineNear returns the line of the new side at row i, or the one after
// the nearest above it, for a row of the old side's lines only.
func newLineNear(rows []diffRow, i int) int {
	for k := i; k >= 0; k-- {
		if rows[k].new >= 0 {
			if k < i {
				return rows[k].new + 1
			}
			return rows[k].new
		}
	}
	return 0
}

// diffTopLine returns the line of the new side at the top of a diff's
// view.
func (a *app) diffTopLine(v *diffView) int {
	rows, _ := v.rows()
	if len(rows) == 0 {
		return 0
	}
	i := min(len(rows)-1, max(0, int((v.scroll.Y-editorPadY)/a.font.lineH)+1))
	return newLineNear(rows, i)
}

// stepHunk goes to the next hunk of a diff, or the previous one for a
// step of -1, round the ends.
func (v *diffView) stepHunk(step int) {
	_, starts := v.rows()
	if len(starts) == 0 {
		return
	}
	v.current = (v.current + step + len(starts)) % len(starts)
	if v.current < 0 {
		v.current += len(starts)
	}
	v.jump = starts[v.current]
}

// openFileAt shows a file with the caret at the start of a line.
func (a *app) openFileAt(path string, line int, focus bool) {
	a.openFile(path, focus)
	if d := a.activeEditor(); d != nil && d.path == path {
		snap := d.buf.Snapshot()
		line = min(max(line, 0), snap.LineCount()-1)
		s, _ := snap.LineRange(line)
		d.selectRange(s, s, true)
	}
}

// diffStatus is what the status bar says of a diff: how many lines it
// adds and removes, and in how many hunks.
func (v *diffView) diffStatus() string {
	if !v.loaded || v.binary {
		return ""
	}
	if len(v.hunks) == 0 {
		return "No changes"
	}
	add, del := 0, 0
	for _, h := range v.hunks {
		add, del = add+h.BLen, del+h.ALen
	}
	return fmt.Sprintf("%s · +%d −%d", plural(len(v.hunks), "change"), add, del)
}

// diffButtons are the buttons at the end of a diff's breadcrumbs: the
// versions compared, going from hunk to hunk, the layout, and staging.
func (a *app) diffButtons(c *ui.Context, t *colTab) {
	p := a.pal
	v := t.diff
	old, new := v.base.sides()
	ui.Text(c, old+" ↔ "+new).FontSize(11.5).TextColor(p.faint).SingleLine().Shrink(1).Padding(0, 6, 0, 0)
	button := func(icon *ui.SVG, label string, fn func()) {
		b := ui.ButtonBase(c.Key(label)).Height(crumbsH-4).Padding(0, 6).Radius(4).Shrink(0).
			AlignItems(ui.Center).Label(label).Tooltip(label)
		if b.Hovered() {
			b.Background(p.hover)
		}
		b.Children(func() { ui.Icon(c, icon).FontSize(13).TextColor(p.muted) })
		if b.Clicked() {
			a.post(fn)
		}
	}
	button(iconArrowUp, "Previous Change (⇧⌥F5)", func() { v.stepHunk(-1) })
	button(iconArrowDown, "Next Change (⌥F5)", func() { v.stepHunk(1) })
	if v.split {
		button(iconRows, "Show Inline", func() { a.setDiffSplit(v, false) })
	} else {
		button(iconColumns, "Show Side by Side", func() { a.setDiffSplit(v, true) })
	}
	if a.git.repo != nil {
		if v.base == diffStaged {
			button(iconMinus, "Unstage File", func() { a.unstage(v.path) })
		} else {
			button(iconPlus, "Stage File", func() { a.stage(v.path) })
		}
	}
	if v.new.ok || v.base != diffStaged {
		button(iconOpenFile, "Open File", func() { a.openFileAt(v.path, a.diffTopLine(v), true) })
	}
}

// setDiffSplit shows a diff side by side, or inline, as the next diffs
// opened will be.
func (a *app) setDiffSplit(v *diffView, split bool) {
	if v.split == split {
		return
	}
	// Keep the row at the top in view.
	rows, _ := v.rows()
	top := int(max(0, v.scroll.Y-editorPadY) / a.font.lineH)
	line, old := -1, false
	if top < len(rows) {
		line = rows[top].new
		if line < 0 {
			line, old = rows[top].old, true
		}
	}
	v.split = split
	a.settings.DiffSplit = split
	a.settings.Changed()
	a.rememberTabs()
	rows, _ = v.rows()
	for i, r := range rows {
		if !old && r.new == line || old && r.old == line {
			v.scroll.Y = editorPadY + float32(i)*a.font.lineH
			break
		}
	}
}

// diffsChanged has the diffs read the sides Git keeps again.
func (a *app) diffsChanged() {
	for _, col := range a.cols {
		for _, t := range col.tabs {
			if t.diff != nil {
				t.diff.gitStale = true
			}
		}
	}
}

// diffsOnDisk has the diffs of files changed on disk read them again.
func (a *app) diffsOnDisk(changed []string) {
	for _, col := range a.cols {
		for _, t := range col.tabs {
			if t.diff != nil {
				for _, p := range changed {
					if p == t.diff.path {
						t.diff.diskStale = true
					}
				}
			}
		}
	}
}

// stepChange goes to the next change of the tab shown, or the previous
// one: a diff's hunk, or the lines of an editor changed since the last
// commit.
func (a *app) stepChange(step int) {
	t := a.activeTab()
	switch {
	case t == nil:
	case t.diff != nil:
		t.diff.stepHunk(step)
	case t.ed != nil:
		a.queueEdit(t.ed, func(d *editor) { a.stepEditorHunk(d, step) })
	}
}
