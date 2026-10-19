package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// markdownSync keeps a Markdown preview in step with the text of its
// document shown in another column, as VS Code's does: scrolling the text
// scrolls the preview to the same place, and scrolling the preview scrolls
// the text, block by block and within a block by its share of its lines.
type markdownSync struct {
	// rows are the blocks' rows, as the last frame laid them out.
	rows []ui.Handle
	// textY is the text's offset as the preview last followed it, or set
	// it, and resync has the preview follow it again, as its rows moved.
	textY  float32
	resync bool
	// ownY is the preview's offset as the last frame found it, and again
	// has the next frame read the rows once more after it moved.
	ownY  float32
	again bool
	// tries counts the frames that went on placing the preview after the
	// text last moved.
	tries int
	// userAt is when the user last scrolled the preview, which leads
	// while it scrolls: the text follows it, and not the other way.
	userAt time.Time
}

// markdownLead is how long after the user last scrolled the preview, as
// its momentum goes on, it leads the text.
const markdownLead = 400 * time.Millisecond

// markdownSource is the editor showing the text of a preview's document in
// another column, nil when none does.
func (a *app) markdownSource(v *viewer) *editor {
	for _, col := range a.cols {
		if t := col.tab(); t != nil && t.ed != nil && t.ed.document == v.doc && t.ed.preview == nil {
			return t.ed
		}
	}
	return nil
}

// markdownTouched notes the user scrolling the preview, or pressing in
// it, which then leads the text for a moment.
func (m *markdownPreview) markdownTouched(ev ui.InputEvent) {
	switch ev.Kind {
	case ui.InputScroll, ui.InputPointerDown, ui.InputKeyDown:
		m.sync.userAt = time.Now()
	}
}

// markdownGroup is the run of blocks from i that start on the same line,
// as an alert's title does with its text, and the line after them.
func (m *markdownPreview) markdownGroup(i, lines int) (first, last, next int) {
	first, last = i, i
	for first > 0 && m.blocks[first-1].line == m.blocks[i].line {
		first--
	}
	for last+1 < len(m.blocks) && m.blocks[last+1].line == m.blocks[i].line {
		last++
	}
	next = lines
	if last+1 < len(m.blocks) {
		next = m.blocks[last+1].line
	}
	return first, last, max(next, m.blocks[i].line+1)
}

// markdownRowY is where row i of the preview starts below the top of the
// preview, as the last frame laid it out, and whether it did.
func (a *app) markdownRowY(c *ui.Context, v *viewer, i int) (float32, bool) {
	m := v.markdown
	if i < 0 || i >= len(m.sync.rows) {
		return 0, false
	}
	first, last := m.list.Visible()
	if i < first || i > last {
		return 0, false
	}
	r := m.sync.rows[i].Bounds(c)
	if r.W == 0 && r.H == 0 {
		return 0, false
	}
	return r.Y - v.pane.Bounds(c).Y, true
}

// markdownExtent is where the blocks from first to last start below the
// top of the preview, and how far they go, to where the next starts.
func (a *app) markdownExtent(c *ui.Context, v *viewer, first, last int) (top, height float32, ok bool) {
	top, ok = a.markdownRowY(c, v, first)
	if !ok {
		return 0, 0, false
	}
	if next, ok := a.markdownRowY(c, v, last+1); ok {
		return top, next - top, true
	}
	r := v.markdown.sync.rows[last].Bounds(c)
	end, ok := a.markdownRowY(c, v, last)
	return top, end + r.H - top, ok
}

// syncMarkdown scrolls the preview with the text shown in another column,
// or the text with the preview while the user scrolls it, before the
// preview's frame is built.
func (a *app) syncMarkdown(c *ui.Context, v *viewer) {
	m := v.markdown
	if len(m.sync.rows) != len(m.blocks) {
		m.sync.rows = make([]ui.Handle, len(m.blocks))
	}
	ed := a.markdownSource(v)
	if ed == nil || len(m.blocks) == 0 {
		m.sync.resync = true
		return
	}
	lead := time.Since(m.sync.userAt) < markdownLead
	if !lead {
		m.sync.ownY = m.scroll.Y
		if ed.vscroll.Y == m.sync.textY && !m.sync.resync {
			return
		}
	}
	lines := ed.buf.Snapshot().LineCount()
	g := a.geometry(ed, lines)
	if lead {
		a.textFromPreview(c, v, ed, g, lines)
		return
	}
	if ed.vscroll.Y != m.sync.textY {
		m.sync.tries = 0
	}
	m.sync.textY = ed.vscroll.Y
	m.sync.resync = false
	y := ed.vscroll.Y - editorPadY
	switch {
	case y <= 0:
		m.scroll.Y = 0
		return
	case ed.vscroll.MaxY > 0 && ed.vscroll.Y >= ed.vscroll.MaxY-0.5:
		m.list.ScrollToEnd()
		return
	}
	// The line at the top of the text, and how far down it.
	at := g.at(y)
	line := float32(at)
	if h := g.top(at+1) - g.top(at); h > 0 {
		line += clamp((y-g.top(at))/h, 0, 1)
	}
	i := 0
	for i+1 < len(m.blocks) && float32(m.blocks[i+1].line) <= line {
		i++
	}
	first, last, next := m.markdownGroup(i, lines)
	top, height, ok := a.markdownExtent(c, v, first, last)
	if !ok {
		// The block is out of view: go there, and to the place within it
		// in the next frame, once it is laid out.
		m.list.ScrollTo(first, ui.Start)
		m.sync.resync = true
		a.invalidate()
		return
	}
	// The preview is where the last frame laid it out, as the user did
	// not scroll it.
	share := clamp((line-float32(m.blocks[first].line))/float32(next-m.blocks[first].line), 0, 1)
	by := top + share*height
	if by > -0.5 && by < 0.5 {
		return
	}
	m.scroll.Y = max(0, m.scroll.Y+by)
	// The list places rows it had not measured where it guessed they
	// were: look again in the next frame, until the place holds.
	if m.sync.tries < 4 {
		m.sync.tries++
		m.sync.resync = true
		a.invalidate()
	}
}

// textFromPreview scrolls the text to the place at the top of the
// preview, which the user scrolls. It reads the rows where the last frame
// laid them out, which the preview's offset may have left since, and
// takes the next frame for that.
func (a *app) textFromPreview(c *ui.Context, v *viewer, ed *editor, g geometry, lines int) {
	m := v.markdown
	// Only as the preview moves, and once more after, as the rows read
	// are the frame's before: a click in it leaves the text where it is.
	if m.scroll.Y == m.sync.ownY && !m.sync.again {
		return
	}
	m.sync.again = m.scroll.Y != m.sync.ownY
	m.sync.ownY = m.scroll.Y
	if m.sync.again {
		a.invalidate()
	}
	to := float32(0)
	switch {
	case m.scroll.Y <= 0:
	case m.list.AtEnd():
		to = max(ed.vscroll.MaxY, editorPadY+g.height())
	default:
		first, last := m.list.Visible()
		i := -1
		for k := max(first, 0); k <= last; k++ {
			if top, ok := a.markdownRowY(c, v, k); ok && top <= 0 {
				i = k
			}
		}
		if i < 0 {
			return
		}
		first, last, next := m.markdownGroup(i, lines)
		top, height, ok := a.markdownExtent(c, v, first, last)
		if !ok || height <= 0 {
			return
		}
		line := float32(m.blocks[first].line) + clamp(-top/height, 0, 1)*float32(next-m.blocks[first].line)
		at := min(int(line), lines-1)
		to = editorPadY + g.top(at) + (line-float32(at))*(g.top(at+1)-g.top(at))
	}
	if to == m.sync.textY {
		return
	}
	// Before the next frame: the text may be laid out in this one already,
	// which would put back where it was.
	m.sync.textY = to
	a.post(func() { ed.vscroll.Y = to })
}
