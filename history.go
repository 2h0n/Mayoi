package main

import (
	"cmp"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

// history is a document's undo and redo, which the app keeps rather than
// its text area, so that the edits the app makes (indenting, moving lines,
// replacing) undo as typing does. A step is the text before an edit and
// the selection then: the buffer is a persistent tree, so that a step
// shares all but the chunks edited with the text after it.
type history struct {
	undo, redo []histStep
	snap       ui.TextSnapshot // the text as last seen
	seen       bool
	// typing is set while the last step is typing that more typing joins:
	// it ends with a pause, a move of the caret, or an edit of the app's.
	typing bool
	at     time.Time
}

type histStep struct {
	snap ui.TextSnapshot
	sel  [2]int
}

const (
	maxUndo     = 1000
	typingPause = 1200 * time.Millisecond
)

func (h *history) push(s histStep) {
	h.undo = append(h.undo, s)
	if len(h.undo) > maxUndo {
		h.undo = append(h.undo[:0], h.undo[len(h.undo)-maxUndo:]...)
	}
	clear(h.redo)
	h.redo = h.redo[:0]
}

// observe records the edits made to d's text since it was last seen,
// which are typing: typing without a pause joins the step before.
func (d *document) observe() {
	h := &d.hist
	snap := d.buf.Snapshot()
	if !h.seen {
		h.snap, h.seen = snap, true
		return
	}
	if snap.Version() == h.snap.Version() {
		return
	}
	now := time.Now()
	old, sel := h.snap, d.sel
	if !h.typing || now.Sub(h.at) > typingPause || len(h.undo) == 0 {
		h.push(histStep{h.snap, d.sel})
	} else {
		clear(h.redo)
		h.redo = h.redo[:0]
	}
	h.snap, h.typing, h.at = snap, true, now
	// Brackets and quotes typed close, in the same step.
	if d.autoClose(old, snap, sel) {
		h.snap = d.buf.Snapshot()
	}
	d.keepClosers(h.snap)
}

// textEdit replaces the runes start up to end with text.
type textEdit struct {
	start, end int
	text       string
}

// edit makes edits of the app's as one step of undo, then selects sel.
// The edits must not overlap; they are made from the last.
func (d *document) edit(sel [2]int, edits ...textEdit) {
	d.observe()
	h := &d.hist
	h.push(histStep{h.snap, d.sel})
	sortEdits(edits)
	for _, e := range edits {
		d.buf.Replace(e.start, e.end, e.text)
	}
	h.snap, h.typing = d.buf.Snapshot(), false
	d.closers = d.closers[:0]
	n := h.snap.Len()
	sel = [2]int{min(max(sel[0], 0), n), min(max(sel[1], 0), n)}
	d.selectRange(sel[0], sel[1], false)
	d.sel = sel
}

// sortEdits orders edits from the last in the text to the first, so that
// making each keeps the offsets of those before it.
func sortEdits(edits []textEdit) {
	slices.SortStableFunc(edits, func(a, b textEdit) int { return cmp.Compare(b.start, a.start) })
}

// undo takes back the last step, reporting whether there was one.
func (d *document) undo() bool {
	d.observe()
	h := &d.hist
	if len(h.undo) == 0 {
		return false
	}
	s := h.undo[len(h.undo)-1]
	h.undo = h.undo[:len(h.undo)-1]
	h.redo = append(h.redo, histStep{d.buf.Snapshot(), d.sel})
	d.restore(s)
	return true
}

// redo makes the step undone last again.
func (d *document) redo() bool {
	d.observe()
	h := &d.hist
	if len(h.redo) == 0 {
		return false
	}
	s := h.redo[len(h.redo)-1]
	h.redo = h.redo[:len(h.redo)-1]
	h.undo = append(h.undo, histStep{d.buf.Snapshot(), d.sel})
	d.restore(s)
	return true
}

// restore brings back the text and the selection of a step. The text as
// it was on disk is clean again.
func (d *document) restore(s histStep) {
	d.buf.Restore(s.snap)
	if s.snap == d.savedSnap {
		d.markSaved()
	}
	d.hist.snap, d.hist.typing = d.buf.Snapshot(), false
	d.closers = d.closers[:0]
	n := s.snap.Len()
	d.selectRange(min(s.sel[0], n), min(s.sel[1], n), false)
	d.sel = [2]int{min(s.sel[0], n), min(s.sel[1], n)}
	d.measureWidest()
}

// markSaved takes the text as it is for the one on disk.
func (d *document) markSaved() {
	d.savedSnap = d.buf.Snapshot()
	d.saved = d.savedSnap.Version()
}
