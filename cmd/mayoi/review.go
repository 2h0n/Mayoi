package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/git"
	"mayoi/internal/remote"
)

// reviewNote is a comment on lines of a file, as a review of the changes
// an agent made, to paste back to it with where the lines are and what
// they were.
type reviewNote struct {
	path string
	// commit is the commit whose diff the note is on, "" for the changes
	// of the work tree; staged is set for one on the changes staged, whose
	// new side is the index.
	commit string
	staged bool
	// oldSide is set when the lines are of the old side, the note being on
	// lines removed only. from and to are the lines of the side, to after
	// the last.
	oldSide  bool
	from, to int
	// diff is set when the snippet is of a diff, each line marked as added,
	// removed or the same. lines are the lines of the side from from to to
	// as they were, which the note finds again as the file changes, and
	// outdated is set when they are no longer in it.
	diff     bool
	snippet  []string
	lines    []string
	text     string
	outdated bool
	// version is the version of the open document the note was last
	// found in.
	version uint64
}

// review is the notes of a project, until they are sent.
type review struct {
	notes []*reviewNote
	// draft is the note being written, a new one or one of notes, nil for
	// none: text is its comment as typed, and ed the editor whose box
	// shows it, nil for a diff's.
	draft *reviewNote
	text  string
	ed    *editor
	// gen counts the changes to the notes, which the diffs lay out their
	// rows again for.
	gen int
	// typed is the frame the draft's box last had the keyboard in.
	typed int
}

// noteAt returns note k of a diff's rows: of the notes, or the draft, a
// new one, after them.
func (r *review) noteAt(k int) *reviewNote {
	switch {
	case k >= 0 && k < len(r.notes):
		return r.notes[k]
	case k == len(r.notes):
		return r.draft
	}
	return nil
}

// work reports whether a note is on lines of the work tree: of an editor,
// or of the changes not staged.
func (n *reviewNote) work() bool { return n.commit == "" && !n.staged }

// inDiff reports whether a note is on lines of file f of a diff.
func (n *reviewNote) inDiff(v *diffView, f *diffFile) bool {
	if n.path != f.path {
		return false
	}
	if v.commit != nil {
		return n.commit == v.commit.hash
	}
	return n.commit == "" && n.staged == (f.base == diffStaged)
}

// same reports whether two notes are on the same lines.
func (n *reviewNote) same(o *reviewNote) bool {
	return n.path == o.path && n.commit == o.commit && n.staged == o.staged && n.oldSide == o.oldSide &&
		n.from == o.from && n.to == o.to
}

// relocate finds lines in text, at from or as near it as they are, and
// reports whether it found them.
func relocate(lines, text []string, from int) (int, bool) {
	n := len(lines)
	at := func(i int) bool { return i >= 0 && i+n <= len(text) && slices.Equal(text[i:i+n], lines) }
	if n == 0 || at(from) {
		return from, n > 0
	}
	for d := 1; d <= max(from, len(text)-from); d++ {
		if at(from - d) {
			return from - d, true
		}
		if at(from + d) {
			return from + d, true
		}
	}
	return from, false
}

// follow finds a note's lines again in text, the lines of its side now,
// or marks it outdated.
func (n *reviewNote) follow(text []string) {
	at, ok := relocate(n.lines, text, n.from)
	n.outdated = !ok
	if ok {
		n.from, n.to = at, at+len(n.lines)
	}
}

// where names a note's lines, as its card says.
func (n *reviewNote) where() string {
	s := fmt.Sprintf("Line %d", n.from+1)
	if n.to-n.from > 1 {
		s = fmt.Sprintf("Lines %d–%d", n.from+1, n.to)
	}
	if n.oldSide {
		s += " (removed)"
	}
	return s
}

// ref is where a note's lines are, as an agent reads it: the path and the
// lines, and what they are of.
func (n *reviewNote) ref(rel func(string) string) string {
	at := fmt.Sprint(n.from + 1)
	if n.to-n.from > 1 {
		at += fmt.Sprintf("-%d", n.to)
	}
	s := rel(n.path) + ":" + at
	var tags []string
	if n.oldSide {
		tags = append(tags, "removed lines, numbered as before the change")
	}
	switch {
	case n.commit != "":
		tags = append(tags, "in commit "+git.ShortHash(n.commit))
	case n.staged:
		tags = append(tags, "staged")
	}
	if n.outdated {
		tags = append(tags, "outdated: the lines have changed since")
	}
	if len(tags) > 0 {
		s += " (" + strings.Join(tags, "; ") + ")"
	}
	return s
}

// fence returns a fence of backticks longer than any run of them in
// lines.
func fence(lines []string) string {
	most := 2
	for _, l := range lines {
		run := 0
		for _, r := range l {
			if r != '`' {
				run = 0
				continue
			}
			run++
			most = max(most, run)
		}
	}
	return strings.Repeat("`", most+1)
}

// formatReview writes notes as a message to an agent: where each is, its
// lines in a block of code and its comment, numbered when there are more
// than one. rel spells a path.
func formatReview(notes []*reviewNote, rel func(string) string) string {
	var b strings.Builder
	for i, n := range notes {
		if i > 0 {
			b.WriteString("\n")
		}
		if len(notes) > 1 {
			fmt.Fprintf(&b, "%d. ", i+1)
		}
		b.WriteString(n.ref(rel) + "\n")
		f := fence(n.snippet)
		lang := "diff"
		if !n.diff {
			lang = strings.TrimPrefix(filepath.Ext(n.path), ".")
		}
		b.WriteString(f + lang + "\n")
		for _, l := range n.snippet {
			b.WriteString(l + "\n")
		}
		b.WriteString(f + "\n")
		if t := strings.TrimSpace(n.text); t != "" {
			b.WriteString(t + "\n")
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// commentRows returns the rows a comment shows in, cols columns wide, as
// its lines wrap.
func commentRows(text string, cols int) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		shown, _ := expandTabs(line)
		at := 0
		for _, k := range wrapAt(shown, cols) {
			out = append(out, shown[at:k])
			at = k
		}
		out = append(out, shown[at:])
	}
	return out
}

// noteHeight is how many lines tall a note's card is, cols columns wide:
// its header and its comment, or, while it is written, the box it is
// typed in, as tall as the text up to a point, and its buttons.
func noteHeight(n *reviewNote, editing bool, text string, cols int) int {
	if editing {
		return 3 + min(max(len(commentRows(text, cols)), 3), 12)
	}
	return 2 + len(commentRows(n.text, cols))
}

// selection returns the rows of a diff chosen, from lo to hi: those from
// the anchor to the cursor, or the cursor's, or the top row's; -1 for
// none.
func (v *diffView) selection() (lo, hi int) {
	c := v.focusRow()
	if c < 0 {
		return -1, -1
	}
	if v.anchor < 0 || v.anchor >= len(v.rows) || v.cursor < 0 {
		return c, c
	}
	return min(v.anchor, c), max(v.anchor, c)
}

// selected reports whether row i is of the rows chosen, more than one.
func (v *diffView) selected(i int) bool {
	if v.anchor < 0 || v.cursor < 0 || v.anchor == v.cursor {
		return false
	}
	return i >= min(v.anchor, v.cursor) && i <= max(v.anchor, v.cursor)
}

// extend moves the cursor of a diff by step rows, keeping where the rows
// chosen start.
func (v *diffView) extend(step int) {
	if v.cursor < 0 || v.cursor >= len(v.rows) {
		v.setCursor(v.focusRow())
	}
	at := v.anchor
	if at < 0 {
		at = v.cursor
	}
	v.moveCursor(step)
	v.anchor = at
}

// excerpts returns notes, without comments, of the lines of rows lo to
// hi of a diff: one for each file they are of. A header of a run of
// changes stands for the lines under it.
func (v *diffView) excerpts(lo, hi int) []*reviewNote {
	var out []*reviewNote
	for i := max(lo, 0); i <= hi && i < len(v.rows); {
		file := v.rows[i].file
		olds, news := map[int]bool{}, map[int]bool{}
		add := func(r diffRow) {
			if r.kind != rowLine {
				return
			}
			if r.old >= 0 {
				olds[r.old] = true
			}
			if r.new >= 0 {
				news[r.new] = true
			}
		}
		for ; i <= hi && i < len(v.rows) && v.rows[i].file == file; i++ {
			if v.rows[i].kind == rowHunk {
				for k := i + 1; k < len(v.rows) && v.rows[k].file == file && (v.rows[k].kind == rowLine || v.rows[k].kind == rowComment); k++ {
					add(v.rows[k])
				}
			}
			add(v.rows[i])
		}
		if n := v.excerpt(v.files[file], olds, news); n != nil {
			out = append(out, n)
		}
	}
	return out
}

// excerpt returns a note, without a comment, of the lines of a file of a
// diff, of each side: its lines as a diff, in the order of the unified
// layout, with … where lines between them are left out.
func (v *diffView) excerpt(f *diffFile, olds, news map[int]bool) *reviewNote {
	if len(olds)+len(news) == 0 {
		return nil
	}
	n := &reviewNote{path: f.path, staged: f.base == diffStaged, diff: true}
	if v.commit != nil {
		n.commit, n.staged = v.commit.hash, false
	}
	last := -1
	for k, r := range f.unified {
		if !(r.old >= 0 && olds[r.old] || r.new >= 0 && news[r.new]) {
			continue
		}
		if last >= 0 && k > last+1 {
			n.snippet = append(n.snippet, "…")
		}
		last = k
		switch {
		case r.old >= 0 && r.new >= 0:
			n.snippet = append(n.snippet, " "+clipLine(f.new.lines[r.new]))
		case r.new >= 0:
			n.snippet = append(n.snippet, "+"+clipLine(f.new.lines[r.new]))
		default:
			n.snippet = append(n.snippet, "-"+clipLine(f.old.lines[r.old]))
		}
	}
	lines, set := f.new.lines, news
	if len(news) == 0 {
		lines, set, n.oldSide = f.old.lines, olds, true
	}
	n.from, n.to = len(lines), 0
	for l := range set {
		n.from, n.to = min(n.from, l), max(n.to, l+1)
	}
	if n.from >= n.to || n.to > len(lines) {
		return nil
	}
	n.lines = slices.Clone(lines[n.from:n.to])
	return n
}

// editorExcerpt returns a note, without a comment, of the lines of an
// editor's selection, or of its caret's line; nil for a file not saved.
func editorExcerpt(d *editor) *reviewNote {
	if d.path == "" {
		return nil
	}
	snap := d.buf.Snapshot()
	lo, hi := min(d.sel[0], d.sel[1], snap.Len()), min(max(d.sel[0], d.sel[1]), snap.Len())
	from, to := snap.LineAt(lo), snap.LineAt(hi)
	// A selection to the start of a line ends on the line before.
	if to > from {
		if s, _ := snap.LineRange(to); s == hi {
			to--
		}
	}
	n := &reviewNote{path: d.path, from: from, to: to + 1, version: d.buf.Version()}
	for i := from; i <= to; i++ {
		s, e := snap.LineRange(i)
		n.lines = append(n.lines, strings.TrimRight(snap.Slice(s, e), "\r\n"))
	}
	n.snippet = n.lines
	return n
}

// relPath spells a path as an agent in the project's folder reads it:
// from the folder, or whole for one out of it.
func (a *app) relPath(path string) string {
	if a.tree != nil {
		if rel := a.tree.Rel(path); rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel) {
			return rel
		}
	}
	return hostPath(path)
}

// followNote finds a note on the work tree again in its file as it is
// now: in its open document, or on disk.
func (a *app) followNote(n *reviewNote) {
	if !n.work() {
		return
	}
	var text string
	if i := a.docIndex(n.path); i >= 0 {
		doc := a.docs[i]
		if doc.buf.Version() == n.version {
			return
		}
		n.version = doc.buf.Version()
		text = doc.buf.Snapshot().String()
	} else if remote.Is(n.path) {
		return
	} else {
		t, _, err := readWork(n.path)
		if err != nil {
			n.outdated = true
			return
		}
		text = t
	}
	n.follow(strings.Split(text, "\n"))
}

// addReviewComment starts a comment on what is chosen: the lines of the
// diff shown, or of the editor's selection. On the lines of a note, it
// edits the note.
func (a *app) addReviewComment() {
	if t := a.activeTab(); t != nil && t.diff != nil {
		a.commentDiff(t.diff)
		return
	}
	d := a.textEditor()
	if d == nil {
		return
	}
	n := editorExcerpt(d)
	if n == nil {
		a.tell(note{message: "Save the file to comment on it."})
		return
	}
	for _, o := range a.review.notes {
		a.followNote(o)
		if o.work() && o.path == n.path && o.from <= n.from && o.to >= n.to {
			a.editNote(o, d)
			return
		}
	}
	a.startDraft(n, d)
}

// commentDiff starts a comment on the lines of a diff chosen, or edits
// the note chosen.
func (a *app) commentDiff(v *diffView) {
	lo, hi := v.selection()
	if lo < 0 {
		return
	}
	if c := v.cursor; c >= 0 && c < len(v.rows) && v.rows[c].kind == rowComment && lo == hi {
		if n := a.review.noteAt(v.rows[c].n); n != nil {
			a.editNote(n, nil)
		}
		return
	}
	ns := v.excerpts(lo, hi)
	if len(ns) == 0 {
		a.tell(note{message: "Choose the lines to comment on."})
		return
	}
	// Of the file of the cursor, when the rows are of more than one.
	n := ns[len(ns)-1]
	if c := v.cursor; c >= 0 && c < len(v.rows) {
		for _, o := range ns {
			if o.path == v.files[v.rows[c].file].path {
				n = o
			}
		}
	}
	for _, o := range a.review.notes {
		if o.same(n) {
			a.editNote(o, nil)
			return
		}
	}
	v.anchor = -1
	a.startDraft(n, nil)
	a.revealDraft(v)
}

// revealDraft scrolls a diff to the card of the note being written.
func (a *app) revealDraft(v *diffView) {
	a.layoutRows(v, a.diffGeometry(v, v.split, v.viewW))
	for i, r := range v.rows {
		if r.kind == rowComment && a.review.noteAt(r.n) == a.review.draft {
			v.jumpRow(i, jumpReveal)
			return
		}
	}
}

// startDraft has a note written, in the box of editor ed, or of a diff
// for nil.
func (a *app) startDraft(n *reviewNote, ed *editor) {
	r := &a.review
	r.draft, r.text, r.ed = n, n.text, ed
	r.gen++
}

// editNote has a note written again.
func (a *app) editNote(n *reviewNote, ed *editor) {
	a.startDraft(n, ed)
	if t := a.activeTab(); ed == nil && t != nil && t.diff != nil {
		a.revealDraft(t.diff)
	}
}

// saveDraft keeps the comment typed for its note; an empty one deletes
// the note.
func (a *app) saveDraft() {
	r := &a.review
	n := r.draft
	if n == nil {
		return
	}
	if text := strings.TrimSpace(r.text); text == "" {
		r.notes = slices.DeleteFunc(r.notes, func(o *reviewNote) bool { return o == n })
	} else {
		n.text = text
		if !slices.Contains(r.notes, n) {
			r.notes = append(r.notes, n)
		}
	}
	a.endDraft()
}

// endDraft closes the box of the note being written, and gives the
// keyboard back to where it was.
func (a *app) endDraft() {
	r := &a.review
	r.draft, r.text, r.ed = nil, "", nil
	r.gen++
	a.focusColumn()
}

// deleteNote drops a note.
func (a *app) deleteNote(n *reviewNote) {
	r := &a.review
	r.notes = slices.DeleteFunc(r.notes, func(o *reviewNote) bool { return o == n })
	if r.draft == n {
		r.draft, r.text, r.ed = nil, "", nil
	}
	r.gen++
}

// reviewText is the notes as a message to an agent, each found again in
// its file first.
func (a *app) reviewText() string {
	for _, n := range a.review.notes {
		a.followNote(n)
	}
	return formatReview(a.review.notes, a.relPath)
}

// copyReview puts the notes on the clipboard.
func (a *app) copyReview() {
	k := len(a.review.notes)
	if k == 0 {
		a.tell(note{message: "No review comments to copy."})
		return
	}
	a.services.WriteClipboard(a.reviewText())
	a.tell(note{message: "Copied " + plural(k, "review comment")})
}

// sendReview pastes the notes into the terminal used last, as a message
// to the agent in it, without pressing Enter, and clears them. With no
// terminal, it copies them.
func (a *app) sendReview() {
	k := len(a.review.notes)
	if k == 0 {
		a.tell(note{message: "No review comments to send."})
		return
	}
	s := a.reviewTerminal()
	if s == nil {
		a.services.WriteClipboard(a.reviewText())
		a.tell(note{message: "No terminal is open: copied " + plural(k, "review comment") + " instead"})
		return
	}
	s.term.Paste(a.reviewText() + "\n")
	a.clearReview()
	a.showTerminal(s)
	a.tell(note{message: "Sent " + plural(k, "review comment") + " to the terminal"})
}

// clearReview drops every note.
func (a *app) clearReview() {
	r := &a.review
	r.notes, r.draft, r.text, r.ed = nil, nil, "", nil
	r.gen++
}

// reviewTerminal is the terminal the notes go to: the one that had the
// keyboard last, or the panel's.
func (a *app) reviewTerminal() *session {
	if s := a.keyTerm; s != nil && slices.Contains(a.terms, s) {
		return s
	}
	if a.panelTerm != nil {
		return a.panelTerm
	}
	if len(a.terms) > 0 {
		return a.terms[len(a.terms)-1]
	}
	return nil
}

// showTerminal shows a terminal and gives it the keyboard: in its tab, or
// in the panel.
func (a *app) showTerminal(s *session) {
	if s.tab != nil {
		if col, i := a.locate(s.tab); i >= 0 {
			a.activate(col, i, true)
		}
		return
	}
	a.panelTerm = s
	a.window.PanelOpen, a.window.TerminalOpen = true, true
	a.settings.Changed()
	a.focusTerminal()
}

// copyExcerpt puts the lines of a diff chosen on the clipboard, with
// where they are, as a message to an agent.
func (a *app) copyExcerpt(v *diffView) {
	lo, hi := v.selection()
	ns := v.excerpts(lo, hi)
	if len(ns) == 0 {
		return
	}
	a.services.WriteClipboard(formatReview(ns, a.relPath))
	refs := make([]string, len(ns))
	for i, n := range ns {
		refs[i] = n.ref(a.relPath)
	}
	a.tell(note{message: "Copied " + strings.Join(refs, ", ")})
}

// goToNote opens the file of a note at its lines.
func (a *app) goToNote(n *reviewNote) {
	a.followNote(n)
	a.openFileAt(n.path, n.from, true)
}

// reviewMenu lists the notes, to go to each, and copies, sends or clears
// them.
func (a *app) reviewMenu(m *ui.Menu) {
	for _, n := range a.review.notes {
		text := strings.ReplaceAll(n.text, "\n", " ")
		if r := []rune(text); len(r) > 48 {
			text = string(r[:47]) + "…"
		}
		label := fmt.Sprintf("%s:%d  %s", filepath.Base(n.path), n.from+1, text)
		if m.Item(label).Chosen() {
			a.goToNote(n)
		}
	}
	m.Separator()
	if m.Item("Copy Review Comments").Chosen() {
		a.copyReview()
	}
	if m.Item("Send Review Comments to Terminal").Chosen() {
		a.sendReview()
	}
	if m.Item("Clear Review Comments").Chosen() {
		a.clearReview()
	}
}

// noteButton is a small button of a note's card, of text, which reports
// whether it was clicked.
func (a *app) noteButton(c *ui.Context, label string, primary bool) bool {
	p := a.pal
	b := ui.ButtonBase(c.Key("note:"+label)).Height(a.font.lineH-4).Padding(0, 8).Radius(4).AlignItems(ui.Center).Shrink(0).Label(label)
	col := p.text
	switch {
	case primary:
		b.Background(p.accent)
		col = p.editor
		if b.Hovered() {
			b.Background(mix(p.accent, p.text, 0.15))
		}
	case b.Hovered():
		b.Background(p.hover)
	}
	b.Children(func() { ui.Text(c, label).FontSize(11.5).TextColor(col).SingleLine() })
	return b.Clicked()
}

// noteCard shows the inside of a note's card, its text cols columns wide:
// where its lines are and its comment, with buttons to edit or delete it.
// While the note is written in the box of editor ed, or of a diff for
// nil, it shows the box to type it in.
func (a *app) noteCard(c *ui.Context, n *reviewNote, ed *editor, cols int) {
	p, f := a.pal, a.font
	r := &a.review
	editing := r.draft == n && r.ed == ed
	ui.Row(c).Height(f.lineH).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Icon(c, iconComment).FontSize(12).TextColor(p.accent)
		ui.Text(c, n.where()).FontSize(11.5).TextColor(p.muted).SingleLine().Shrink(1)
		if n.outdated {
			ui.Text(c, "Outdated").FontSize(11).TextColor(p.changed).SingleLine().Tooltip("The lines have changed since")
		}
		ui.Spacer(c)
		if editing {
			return
		}
		if a.smallButton(c, iconPencil, "Edit Comment") {
			a.post(func() { a.editNote(n, ed) })
		}
		if a.smallButton(c, iconTrash, "Delete Comment") {
			a.post(func() { a.deleteNote(n) })
		}
	})
	if !editing {
		for _, s := range commentRows(n.text, cols) {
			ui.Text(c, s).Font(f.family).FontSize(f.size).FontWeight(f.weight).FontFeatures(f.featureList()...).
				Height(f.lineH).SingleLine().TextColor(p.text).Shrink(0)
		}
		return
	}
	in := a.textKeys(ui.TextArea(c.Key("review-text"), &r.text)).AutoFocus().Font(f.family).FontSize(f.size).
		Grow(1).MinHeight(0).Placeholder("Comment for the agent…").Label("Review Comment")
	if in.Focused() {
		r.typed = a.frame
	}
	if in.Shortcut(ui.Cmd, ui.KeyEnter) {
		a.post(a.saveDraft)
	}
	if in.Shortcut(0, ui.KeyEscape) {
		a.post(a.endDraft)
	}
	ui.Row(c).Height(f.lineH + 2).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, keyText("CmdOrCtrl+Enter")+" to save · Esc to cancel").FontSize(11).TextColor(p.faint).SingleLine().Shrink(1)
		ui.Spacer(c)
		if a.noteButton(c, "Cancel", false) {
			a.post(a.endDraft)
		}
		if a.noteButton(c, "Comment", true) {
			a.post(a.saveDraft)
		}
	})
}

// typingNote reports whether the box of the note being written has the
// keyboard, whose keys a diff leaves to it.
func (a *app) typingNote() bool {
	return a.review.draft != nil && a.review.typed >= a.frame-1
}

// noteCardBox is the card of a note, at (x, y) w wide and h tall.
func (a *app) noteCardBox(c *ui.Context, key string, x, y, w, h float32) ui.Element {
	p := a.pal
	return ui.Column(c.Key(key)).Absolute().Left(x).Top(y).Width(w).Height(h).Radius(6).Clip().
		Background(p.panel).Border(1, p.border).Padding(0, 10)
}

// noteCols is how many columns of a comment fit in a card w wide.
func (a *app) noteCols(w float32) int {
	return max(int((w-20)/a.font.charW), 10)
}

// diffNoteGeometry is where the cards of notes go in a diff whose view is
// viewW wide: their left, from the view's, and their width.
func diffNoteGeometry(viewW float32) (x, w float32) {
	x = diffRailW + 10
	return x, max(viewW-x-18, 120)
}

// noteRows adds the rows of the notes on file f of a diff, number file,
// to the rows from start, which are of f: each under the last line it is
// on, or the gap that folds it. cols are the columns of a card.
func (a *app) noteRows(v *diffView, f *diffFile, file, start, cols int) {
	r := &a.review
	shown := r.notes
	// The note being written in an editor's box shows there alone.
	if r.draft != nil && r.ed == nil && !slices.Contains(shown, r.draft) {
		shown = append(slices.Clip(shown), r.draft)
	}
	for k, n := range shown {
		if !n.inDiff(v, f) {
			continue
		}
		if f.loaded && !(r.draft == n && r.ed == nil) {
			side := f.new.lines
			if n.oldSide {
				side = f.old.lines
			}
			n.follow(side)
		}
		at, last := -1, n.to-1
		for i := start; i < len(v.rows); i++ {
			row := v.rows[i]
			line := row.new
			if n.oldSide {
				line = row.old
			}
			if (row.kind == rowLine || row.kind == rowGap) && line >= 0 && line <= last {
				at = i
			}
		}
		if at < 0 {
			continue
		}
		for at+1 < len(v.rows) && v.rows[at+1].kind == rowComment {
			at++
		}
		h := noteHeight(n, r.draft == n && r.ed == nil, r.text, cols)
		v.rows = slices.Insert(v.rows, at+1, diffRow{kind: rowComment, file: file, old: -1, new: -1, hunk: -1, peer: -1, gap: -1, n: k, h: h})
	}
}

// notedLines returns the runs of lines of an editor's file that notes
// are on, found again in its text as it changed.
func (a *app) notedLines(d *editor) []span {
	var out []span
	for _, n := range a.review.notes {
		if n.work() && n.path == d.path && d.path != "" {
			a.followNote(n)
			if !n.outdated {
				out = append(out, span{n.from, n.to})
			}
		}
	}
	return out
}

// reviewBoxView is the box under the lines of an editor a note is being
// written on.
func (a *app) reviewBoxView(c *ui.Context, d *editor) {
	r := &a.review
	n := r.draft
	if n == nil || r.ed != d {
		return
	}
	snap := d.buf.Snapshot()
	count := snap.LineCount()
	g := a.geometry(d, count)
	view := d.hscroll.Bounds(c)
	left := d.gutterW - 4
	w := view.W - left - 14
	cols := a.noteCols(w)
	h := float32(noteHeight(n, true, r.text, cols)) * a.font.lineH
	y := editorPadY + g.top(min(n.to, count)) - d.vscroll.Y
	// It stays in view, as its lines scroll away.
	y = clamp(y, 0, max(0, view.H-h))
	box := a.floating(c, a.noteCardBox(c, "review-box", left, y+1, w, h), false).Transition(popIn).Label("Review Comment")
	box.Children(func() { a.noteCard(c, n, d, cols) })
}

// reviewStatus is the part of the status bar that counts the notes, with
// their menu.
func (a *app) reviewStatus(c *ui.Context, text func(string) ui.Element) {
	k := len(a.review.notes)
	if k == 0 {
		return
	}
	p := a.pal
	tip := plural(k, "review comment") + ": copy, or send to the terminal"
	b := ui.ButtonBase(c.Key("status:review")).Height(statusBarH-6).Padding(0, 6).Gap(5).Radius(4).
		AlignItems(ui.Center).Label(tip).Tooltip(tip)
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() {
		ui.Icon(c, iconComment).FontSize(12).TextColor(p.statusFg)
		text(fmt.Sprint(k))
	})
	b.Menu(a.reviewMenu)
}
