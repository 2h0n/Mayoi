package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/highlight"
)

const (
	titleBarH  = 38
	tabStripH  = 35
	crumbsH    = 24
	statusBarH = 24
	panelHeadH = 34
)

// view builds the window: a title bar, the explorer beside the editor
// and the terminal panel below it, and a status bar.
func (a *app) view(c *ui.Context) {
	a.mu.Lock()
	if !a.hasServices {
		a.services, a.hasServices = c.Services(), true
	}
	a.mu.Unlock()
	if !a.started {
		// Before what was posted meanwhile, as opening the file the app was
		// started with: it opens in the columns of the last session.
		a.started = true
		a.start()
	}
	a.drain()
	a.pal = applyTheme(c)
	p := a.pal
	if f := a.settings; a.font.charW == 0 || a.font.size != f.FontSize || a.font.family != f.FontFamily {
		a.font = newCodeFont(f.FontFamily, f.FontSize)
	}
	if now := c.Now(); now != a.frameAt {
		a.frameAt = now
		a.frame++
		a.dragging, a.dragNow = a.dragNow, false
	}
	a.shortcuts(c)
	// Which pane has the keyboard focus, as the frame before left it, and
	// which column: its editor is the one typing is of.
	for i, col := range a.cols {
		switch t := col.tab(); {
		case t == nil:
		case t.ed != nil && t.ed.vscroll.FocusWithin(c):
			a.focus, a.col = paneEditor, i
			t.ed.typist = t.ed
		case t.term != nil && t.term.view.FocusWithin(c):
			a.focus, a.col = paneEditor, i
		case t.view != nil && t.view.pane.FocusWithin(c):
			a.focus, a.col = paneEditor, i
		case t.diff != nil && t.diff.pane.FocusWithin(c):
			a.focus, a.col = paneEditor, i
		case t.ed != nil && t.ed.preview != nil && t.ed.preview.pane.FocusWithin(c):
			a.focus, a.col = paneEditor, i
		}
	}
	a.autoSaveTick()
	switch {
	case a.panelTerm != nil && a.panelTerm.view.FocusWithin(c):
		a.focus = paneTerminal
	case a.files.List.FocusWithin(c):
		a.focus = paneFiles
	}

	ui.Column(c).Fill().Background(p.chrome).Children(func() {
		a.titleBar(c)
		// The panes keep their places in the tree, by keys, as the sidebar
		// and the panel come and go, so that the editors keep their state.
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if a.settings.SidebarOpen {
				s := a.settings
				ui.Column(c.Key("sidebar")).Width(s.SidebarWidth).Shrink(0).BorderWidth(0, 1, 0, 0).BorderColor(p.border).Children(func() {
					a.sidebar(c)
					a.splitter(c, edgeRight, func(d float32) {
						w, _ := c.Size()
						s.SidebarWidth = clamp(s.SidebarWidth+d, 160, max(w-320, 160))
					})
				})
			}
			ui.Column(c.Key("main")).Grow(1).MinWidth(0).Children(func() { a.main(c) })
		})
		a.statusBar(c)
	})
	a.quickOpenView(c)
	a.promptView(c)
	a.confirmView(c)
	for _, msg := range a.errors {
		c.Toast(msg)
	}
	a.errors = a.errors[:0]
	a.tick()
}

// main is the columns of editors and, below them, the panel.
func (a *app) main(c *ui.Context) {
	p, s := a.pal, a.settings
	panel := a.window.PanelOpen
	editor := ui.Column(c.Key("editor")).Grow(1).MinHeight(0).Background(p.editor).Clip()
	if panel && a.panelMax {
		// Out of sight, but built, so that the editors keep their state.
		editor.Grow(0).Height(0).Invisible()
	}
	editor.Children(func() { a.columns(c) })
	if !panel {
		return
	}
	_, h := c.Size()
	room := h - titleBarH - statusBarH
	height := clamp(s.PanelHeight, 80, max(room-80, 80))
	box := ui.Column(c.Key("panel")).Shrink(0).BorderWidth(1, 0, 0, 0).BorderColor(p.border)
	if a.panelMax {
		box.Grow(1)
	} else {
		box.Height(height)
	}
	box.Children(func() {
		a.panel(c)
		if !a.panelMax {
			a.splitter(c, edgeTop, func(d float32) {
				s.PanelHeight = clamp(height-d, 80, max(room-80, 80))
			})
		}
	})
}

// edge is an edge of a pane, which its splitter is along.
type edge int

const (
	edgeRight edge = iota // the sidebar's
	edgeTop               // the panel's
	edgeLeft              // a column's, after the first
)

// splitter is the handle along an edge of a pane that resizes it. drag
// gets how far the pointer moved, across the edge.
func (a *app) splitter(c *ui.Context, at edge, drag func(d float32)) {
	h := ui.Box(c).Absolute()
	switch at {
	case edgeTop:
		h.Top(0).Left(0).Right(0).Height(5).Cursor(ui.CursorResizeNS)
	case edgeLeft:
		h.Top(0).Bottom(0).Left(0).Width(5).Cursor(ui.CursorResizeEW)
	default:
		h.Top(0).Bottom(0).Right(0).Width(5).Cursor(ui.CursorResizeEW)
	}
	lit := h.Hovered() || h.Dragging()
	h.Children(func() {
		if !lit {
			return
		}
		line := ui.Box(c).Absolute().Background(a.pal.accentDim)
		switch at {
		case edgeTop:
			line.Top(0).Left(0).Right(0).Height(2)
		case edgeLeft:
			line.Top(0).Bottom(0).Left(0).Width(2)
		default:
			line.Top(0).Bottom(0).Right(0).Width(2)
		}
	})
	dx, dy, held := h.Dragged()
	if held && (dx != 0 || dy != 0) {
		if at == edgeTop {
			drag(dy)
		} else {
			drag(dx)
		}
		a.settings.Changed()
	}
}

func (a *app) titleBar(c *ui.Context) {
	p := a.pal
	bar := c.TitleBar()
	ui.Row(c).Height(max(bar.Height, titleBarH)).Padding(0, bar.Right+8, 0, bar.Left+10).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.border).DragWindow().Children(func() {
		ui.Row(c).Width(120).Gap(2).Children(func() {
			if a.iconButton(c, iconSidebar, "Toggle Sidebar (⌘B)", a.settings.SidebarOpen).Clicked() {
				a.toggleSidebar()
			}
		})
		ui.Row(c).Grow(1).MinWidth(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			a.commandCenter(c)
		})
		ui.Row(c).Width(120).Justify(ui.End).Gap(2).Children(func() {
			if a.iconButton(c, iconPanel, "Toggle Terminal (⌃`)", a.window.PanelOpen).Clicked() {
				a.togglePanel()
			}
		})
	})
}

// commandCenter is the box in the middle of the title bar: it names the
// document and the folder, and opens quick open.
func (a *app) commandCenter(c *ui.Context) {
	p := a.pal
	b := ui.ButtonBase(c.Key("command-center")).Height(24).Width(420).MaxWidthPercent(100).Shrink(1).Padding(0, 10).Gap(8).
		Radius(6).Justify(ui.Center).AlignItems(ui.Center).Border(1, p.border).Label("Quick Open").Tooltip("Quick Open (⌘P)")
	b.Background(p.hover)
	hovered := b.Hovered()
	if hovered {
		b.Background(p.hover.Alpha(2)).Border(1, mix(p.border, p.muted, 0.3))
	}
	b.Children(func() {
		col := p.faint
		if hovered {
			col = p.muted
		}
		ui.Icon(c, iconSearch).FontSize(12).TextColor(col)
		name, folder := "", ""
		if t := a.activeTab(); t != nil {
			name = t.name()
			if t.ed != nil && t.ed.dirty() {
				name = "● " + name
			}
		}
		if a.root != "" {
			folder = filepath.Base(a.root)
		}
		switch {
		case name != "" && folder != "":
			ui.Text(c, name).FontSize(12).FontWeight(500).TextColor(p.text).SingleLine().Shrink(1)
			ui.Text(c, folder).FontSize(12).TextColor(p.faint).SingleLine().Shrink(1)
		case name != "" || folder != "":
			ui.Text(c, name+folder).FontSize(12).FontWeight(500).TextColor(p.muted).SingleLine().Shrink(1)
		default:
			ui.Text(c, "Mayoi").FontSize(12).FontWeight(500).TextColor(p.muted).SingleLine()
		}
	})
	if b.Clicked() {
		a.openQuick("")
	}
}

// iconButton is a small square button showing an icon, its tooltip the
// label.
func (a *app) iconButton(c *ui.Context, icon *ui.SVG, label string, on bool) ui.Element {
	p := a.pal
	b := ui.ButtonBase(c).Size(26, 24).Radius(5).Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(label)
	col := p.muted
	switch {
	case b.Pressed():
		b.Background(p.hover)
		col = p.text
	case b.Hovered():
		b.Background(p.hover)
		col = p.text
	}
	if on {
		col = p.text
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(15).TextColor(col) })
	return b
}

// minColumnW is the least width a column is dragged to.
const minColumnW = 160

// columns is the editor area: its columns side by side, which share its
// width by their weights, with a handle between two that moves the width
// from one to the other.
func (a *app) columns(c *ui.Context) {
	p := a.pal
	width := a.colsView.Bounds(c).W
	var total float32
	for _, col := range a.cols {
		total += col.weight
	}
	ui.Row(c.Key("columns")).Bind(&a.colsView).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		for i, col := range a.cols {
			box := ui.Column(c.Key(col.id)).Basis(0).Grow(col.weight).MinWidth(0)
			if i > 0 {
				box.BorderWidth(0, 0, 0, 1).BorderColor(p.border)
			}
			box.Children(func() {
				a.columnView(c, col)
				if i == 0 || width <= 0 {
					return
				}
				prev := a.cols[i-1]
				a.splitter(c, edgeLeft, func(d float32) {
					least := minColumnW / width * total
					by := d / width * total
					if lo, hi := least-prev.weight, col.weight-least; lo <= hi {
						by = clamp(by, lo, hi)
						prev.weight, col.weight = prev.weight+by, col.weight-by
						a.rememberTabs()
					}
				})
			})
		}
	})
}

// columnView is a column: its tab strip, the breadcrumbs, and the editor
// or the terminal of the tab shown, or the welcome page.
func (a *app) columnView(c *ui.Context, col *column) {
	if len(col.tabs) == 0 {
		ui.Column(c).Bind(&col.body).Grow(1).MinHeight(0).Children(func() {
			a.welcome(c)
			a.dropZones(c, col)
		})
		return
	}
	a.tabStrip(c, col)
	shown := col.tab()
	if shown != nil && shown.term == nil {
		a.breadcrumbs(c, shown)
	}
	// Every tab keeps its editor, so that its selection and its place stay
	// as tabs change: those not shown are built invisible, out of the way.
	// The key keeps them as the breadcrumbs above come and go.
	ui.Box(c.Key("body")).Bind(&col.body).Grow(1).Children(func() {
		for _, t := range col.tabs {
			box := ui.Column(c.Key(t.id)).Absolute().Top(0).Left(0).Right(0).Bottom(0)
			if t != shown {
				box.Invisible()
			}
			box.Children(func() {
				switch {
				case t.term != nil:
					// A terminal keeps its state itself.
					if t == shown {
						a.terminalView(c, t.term)
					}
				case t.view != nil:
					if t == shown {
						a.viewerView(c, t.view, t.name())
					}
				case t.diff != nil:
					if t == shown {
						a.diffViewUI(c, t.diff)
					}
				case t == shown && t.ed.preview != nil:
					// The text keeps its place, out of sight, under the picture.
					ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Invisible().Children(func() {
						a.hiddenEditor(c, t.ed)
					})
					a.viewerView(c, t.ed.preview, t.name())
				case t == shown:
					a.editorView(c, t.ed)
					a.hunkPeekView(c, t.ed)
					if col == a.column() {
						a.findView(c, t.ed)
					}
				default:
					a.hiddenEditor(c, t.ed)
				}
			})
		}
		a.dropZones(c, col)
	})
}

// hiddenEditor keeps the state of an editor's text area while another
// tab is shown. The area scrolls to its caret when its text changes, as
// an edit made in another column changes it: it stays where it was.
func (a *app) hiddenEditor(c *ui.Context, d *editor) {
	if !d.hidden {
		d.hidden, d.hiddenY = true, d.vscroll.Y
	}
	d.vscroll.Y = d.hiddenY
	ui.ScrollHorizontal(c).TrackScroll(&d.hscroll).Fill().Children(func() {
		// Laid out as the editor shows it, so that its place stays.
		area := ui.TextAreaBufferBase(c, d.buf).Font(a.font.family).FontSize(a.font.size).FixedLineHeight(a.font.lineH).
			Fill().TrackScroll(&d.vscroll)
		if a.wraps(d) {
			// The rows the editor measured stay measured only at its width
			// and in its style.
			gutterW := a.font.gutterWidth(d.buf.Snapshot().LineCount())
			area.Padding(editorPadY, textPadR, editorPadY, gutterW+gutterGap).TextColor(a.pal.text).TrackLines(&d.lines)
			if w := d.hscroll.Bounds(c).W; w > 0 {
				area.Width(w)
			}
		} else {
			area.NoWrap()
		}
	})
}

// dragItem is what is dragged to a column: one of its tabs, or of
// another's, or a terminal of the panel.
type dragItem struct {
	tab  *colTab
	term *session
}

// dropInColumn puts what was dragged at place at of a column.
func (a *app) dropInColumn(it dragItem, col *column, at int) {
	if slices.Index(a.cols, col) < 0 {
		return
	}
	switch {
	case it.term != nil:
		a.terminalToColumn(it.term, col, at)
	case it.tab != nil:
		a.moveTab(it.tab, col, at)
	}
}

// dropBeside puts what was dragged in a column of its own, before a
// column or after it.
func (a *app) dropBeside(it dragItem, col *column, after bool) {
	if slices.Index(a.cols, col) < 0 {
		return
	}
	if from, _ := a.locate(it.tab); from == col && len(col.tabs) == 1 {
		return // it is the column
	}
	a.dropInColumn(it, a.splitColumn(col, after), 0)
}

// dropZones are where a tab dragged over a column is dropped: on a side,
// for a column of its own there, or between them, for a tab of this one.
// They are there only while a tab is dragged, over the editor.
func (a *app) dropZones(c *ui.Context, col *column) {
	if !a.dragging {
		return
	}
	w := col.body.Bounds(c).W
	side := min(w/4, 140)
	zone := func(key string, left, width, markLeft, markW float32, drop func(it dragItem)) {
		z := ui.Box(c.Key(key)).Absolute().Top(0).Bottom(0).Left(left).Width(width)
		if _, ok := ui.DragOver[dragItem](z); ok {
			ui.Box(c.Key(key+"-mark")).Absolute().Top(0).Bottom(0).Left(markLeft).Width(markW).PassThrough().
				Background(a.pal.accent.Alpha(0.14)).Border(1, a.pal.accentDim)
		}
		if it, ok := ui.Drop[dragItem](z); ok {
			a.post(func() { drop(it) })
		}
	}
	zone("drop-before", 0, side, 0, w/2, func(it dragItem) { a.dropBeside(it, col, false) })
	zone("drop-in", side, w-2*side, 0, w, func(it dragItem) { a.dropInColumn(it, col, len(col.tabs)) })
	zone("drop-after", w-side, side, w/2, w/2, func(it dragItem) { a.dropBeside(it, col, true) })
}

// dropMark tints a pane that takes what is dragged over it.
func (a *app) dropMark(c *ui.Context) {
	ui.Box(c).Absolute().Top(0).Bottom(0).Left(0).Right(0).PassThrough().
		Background(a.pal.accent.Alpha(0.14)).Border(1, a.pal.accentDim)
}

// breadcrumbs name the file a tab shows and, in an editor, the blocks
// its caret is in. An SVG has a button there to show it as a picture or
// as text.
func (a *app) breadcrumbs(c *ui.Context, t *colTab) {
	p := a.pal
	d := t.ed
	var parts []string
	if path := t.path(); path != "" {
		rel := path
		if a.root != "" && strings.HasPrefix(path, a.root+string(filepath.Separator)) {
			rel = path[len(a.root)+1:]
		}
		parts = strings.Split(filepath.ToSlash(rel), "/")
	} else {
		parts = []string{t.name()}
	}
	// The blocks the caret is in, after the file: the innermost two.
	var snap ui.TextSnapshot
	var scope []int
	if d != nil && d.preview == nil && isCode(d.document) {
		snap = d.buf.Snapshot()
		scope = d.scopesOf(snap, snap.LineAt(min(d.sel[1], snap.Len())))
		scope = scope[max(0, len(scope)-2):]
	}
	ui.Row(c).Height(crumbsH).Padding(0, 14).Gap(4).AlignItems(ui.Center).Background(p.editor).Clip().Children(func() {
		for i, part := range parts {
			last := i == len(parts)-1
			if last {
				icon, col := fileIcon(p, part)
				ui.Icon(c, icon).FontSize(13).TextColor(col)
			}
			t := ui.Text(c, part).FontSize(12).SingleLine().TextColor(p.muted).Shrink(0)
			if last {
				t.TextColor(p.text)
			}
			if !last || len(scope) > 0 {
				ui.Icon(c, iconChevronRight).FontSize(11).TextColor(p.faint)
			}
		}
		for i, line := range scope {
			label := scopeLabel(snap, line)
			b := ui.ButtonBase(c.Key(fmt.Sprintf("scope:%d", i))).Height(crumbsH-6).Padding(0, 5).Radius(4).
				AlignItems(ui.Center).Shrink(1).MinWidth(0).Label("Go to " + label)
			if b.Hovered() {
				b.Background(p.hover)
			}
			b.Children(func() {
				ui.Text(c, label).FontSize(12).Font(a.font.family).SingleLine().TextColor(p.muted)
			})
			if b.Clicked() {
				a.queueEdit(d, func(d *editor) { a.jumpToScope(d, line) })
			}
			if i < len(scope)-1 {
				ui.Icon(c, iconChevronRight).FontSize(11).TextColor(p.faint)
			}
		}
		if d != nil && isSVG(d.document) {
			ui.Spacer(c)
			a.previewButton(c, t, d)
		}
		if t.diff != nil {
			ui.Spacer(c)
			a.diffButtons(c, t)
		}
	})
}

// previewButton shows an SVG as a picture, or as text again.
func (a *app) previewButton(c *ui.Context, t *colTab, d *editor) {
	p := a.pal
	label, icon := "Show Preview (⇧⌘V)", iconImage
	if d.preview != nil {
		label, icon = "Show Source (⇧⌘V)", iconFileCode
	}
	b := ui.ButtonBase(c.Key("preview")).Height(crumbsH-4).Padding(0, 6).Radius(4).Shrink(0).
		AlignItems(ui.Center).Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(13).TextColor(p.muted) })
	if b.Clicked() {
		a.post(func() {
			a.setPreview(d, d.preview == nil)
			if col, i := a.locate(t); col != nil {
				a.activate(col, i, true)
			}
		})
	}
}

func (a *app) statusBar(c *ui.Context) {
	p := a.pal
	ui.Row(c).Height(statusBarH).Padding(0, 6).Gap(2).AlignItems(ui.Center).Background(p.status).
		BorderWidth(1, 0, 0, 0).BorderColor(p.border).Children(func() {
		text := func(s string) ui.Element {
			return ui.Text(c, s).FontSize(11.5).TextColor(p.statusFg).SingleLine()
		}
		// item is a part of the bar, a button when it does something.
		item := func(key, label, tip string, icon *ui.SVG, fn func()) {
			if fn == nil {
				ui.Row(c.Key("status:"+key)).Height(statusBarH).Padding(0, 6).Gap(5).AlignItems(ui.Center).Children(func() {
					if icon != nil {
						ui.Icon(c, icon).FontSize(12).TextColor(p.statusFg)
					}
					text(label)
				})
				return
			}
			b := ui.ButtonBase(c.Key("status:"+key)).Height(statusBarH-6).Padding(0, 6).Gap(5).Radius(4).
				AlignItems(ui.Center).Label(tip).Tooltip(tip)
			if b.Hovered() {
				b.Background(p.hover)
			}
			b.Children(func() {
				if icon != nil {
					ui.Icon(c, icon).FontSize(12).TextColor(p.statusFg)
				}
				text(label)
			})
			if b.Clicked() {
				fn()
			}
		}
		// menuItem is a part of the bar that opens a menu.
		menuItem := func(key, label, tip string, build func(m *ui.Menu)) {
			b := ui.ButtonBase(c.Key("status:"+key)).Height(statusBarH-6).Padding(0, 6).Radius(4).
				AlignItems(ui.Center).Label(tip).Tooltip(tip)
			if b.Hovered() {
				b.Background(p.hover)
			}
			b.Children(func() { text(label) })
			b.Menu(build)
		}
		if b := a.branch(); b != "" {
			bb := ui.ButtonBase(c.Key("status:branch")).Height(statusBarH-6).Padding(0, 6).Gap(5).Radius(4).
				AlignItems(ui.Center).Label("Branch " + b).Tooltip("Branch " + b + ": switch, fetch, pull or push")
			if bb.Hovered() {
				bb.Background(p.hover)
			}
			bb.Children(func() {
				ui.Icon(c, iconBranch).FontSize(12).TextColor(p.statusFg)
				text(a.branchLabel())
			})
			bb.Menu(a.branchMenu)
		}
		if busy := a.git.busy; busy != "" {
			ui.Row(c.Key("status:busy")).Height(statusBarH).Padding(0, 6).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(11, 11)
				text(busy)
			})
		}
		if a.root != "" {
			item("folder", filepath.Base(a.root), "Reveal "+shortenHome(a.root)+" in Finder", iconFolder, func() { a.host.reveal(a.root) })
		}
		ui.Spacer(c)
		if v := a.activeViewer(); v != nil {
			a.viewerStatus(c, v, item, text)
			return
		}
		if t := a.activeTab(); t != nil && t.diff != nil {
			if s := t.diff.diffStatus(); s != "" {
				item("diff", s, "", iconDiff, nil)
			}
			return
		}
		if t := a.activeTab(); t != nil && t.term != nil {
			// A terminal: where its shell is, and its size in cells.
			term := t.term.term
			if dir := term.Dir(); dir != "" {
				item("cwd", shortenHome(dir), "", iconTerminal, nil)
			}
			if cols, rows := term.Size(); cols > 0 {
				item("size", fmt.Sprintf("%d × %d", cols, rows), "", nil, nil)
			}
			return
		}
		d := a.activeEditor()
		if d == nil {
			return
		}
		if d.diskGone {
			ui.Row(c).Padding(0, 6).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconTrash).FontSize(12).TextColor(p.danger)
				text("Deleted on disk").TextColor(p.danger)
			})
		}
		snap := d.buf.Snapshot()
		caret := min(d.caret, snap.Len())
		line := snap.LineAt(caret)
		s, _ := snap.LineRange(line)
		pos := fmt.Sprintf("Ln %d, Col %d", line+1, columns(snap.Slice(s, caret))+1)
		if n := abs(d.sel[1] - d.sel[0]); n > 0 {
			pos += fmt.Sprintf(" (%d selected)", n)
		}
		item("pos", pos, "Go to Line (⌃G)", nil, func() { a.openQuick(":") })
		item("lines", plural(snap.LineCount(), "line"), "", nil, nil)
		menuItem("indent", d.indent.String(), "Indentation", func(m *ui.Menu) {
			if m.Item("Indent Using Tabs").Checked(d.indent.tabs).Chosen() {
				d.indent.tabs = true
			}
			for _, n := range []int{2, 4, 8} {
				if m.Item(fmt.Sprintf("Indent Using %d Spaces", n)).Checked(!d.indent.tabs && d.indent.size == n).Chosen() {
					d.indent = indentStyle{size: n}
				}
			}
			m.Separator()
			if m.Item("Convert Indentation to Tabs").Chosen() {
				a.editActive(func(d *editor) { reindent(d, indentStyle{tabs: true, size: tabWidth}) })()
			}
			if m.Item("Convert Indentation to Spaces").Chosen() {
				size := d.indent.size
				if d.indent.tabs {
					size = tabWidth
				}
				a.editActive(func(d *editor) { reindent(d, indentStyle{size: size}) })()
			}
		})
		item("enc", "UTF-8", "", nil, nil)
		eol := "LF"
		if d.crlf {
			eol = "CRLF"
		}
		menuItem("eol", eol, "Line Endings", func(m *ui.Menu) {
			for _, crlf := range []bool{false, true} {
				label := "LF"
				if crlf {
					label = "CRLF"
				}
				if m.Item(label).Checked(d.crlf == crlf).Chosen() {
					d.crlf = crlf
				}
			}
		})
		item("lang", highlight.DisplayName(d.hl.Lexer()), "", nil, nil)
	})
}

// plural counts n things, as "1 line" or "2 lines".
func plural(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// iconArt is the app icon's art, which tools/icongen places on the
// macOS icon grid.
//
//go:embed icon-art.png
var iconArt []byte

// iconFrame is the width of the art's own pixel tile border, which the
// icon crops, in pixels of the 1254 px art; as in tools/icongen.
const iconFrame = 52

// logo is Mayoi's mark: the app icon itself, its border cropped as on
// the Dock, decoded the first time the welcome page shows.
var logo = sync.OnceValue(func() *ui.Bitmap {
	art, err := png.Decode(bytes.NewReader(iconArt))
	if err != nil {
		panic(err)
	}
	return ui.NewBitmap(art.(interface {
		SubImage(image.Rectangle) image.Image
	}).SubImage(art.Bounds().Inset(iconFrame)))
})

// welcome shows while no file is open: what to start with, and the
// folders opened last.
func (a *app) welcome(c *ui.Context) {
	p := a.pal
	// Centered in the view, and scrolled when the window is too short.
	view := a.welcomeView.Bounds(c)
	ui.Scroll(c.Key("welcome")).Bind(&a.welcomeView).Fill().Background(p.editor).Children(func() {
		ui.Column(c).FillWidth().MinHeight(view.H).Center().Padding(40, 24, 64).Children(func() {
			ui.Column(c).Width(380).MaxWidthPercent(100).Gap(32).Children(func() {
				ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
					// Clipped to the icon's squircle: 185 of its 824 DIPs.
					ui.Column(c).Size(52, 52).Radius(52.0 * 185 / 824).Clip().Children(func() {
						ui.Image(c, logo()).Size(52, 52)
					})
					ui.Column(c).Gap(3).Children(func() {
						ui.Text(c, "Mayoi").FontSize(26).FontWeight(600).LetterSpacing(-0.3).TextColor(p.text)
						ui.Text(c, "迷い · a small, quick code editor").FontSize(13).TextColor(p.muted)
					})
				})
				a.welcomeSection(c, "Start", func() {
					if a.root == "" {
						a.welcomeAction(c, iconFolderOpen, "Open Folder…", "⌘O", a.chooseFolder)
					} else {
						a.welcomeAction(c, iconSearch, "Quick Open…", "⌘P", func() { a.openQuick("") })
					}
					a.welcomeAction(c, iconFilePlus, "New File", "⌘N", a.newFile)
					a.welcomeAction(c, iconTerminal, "Toggle Terminal", "⌃`", a.togglePanel)
					a.welcomeAction(c, iconSidebar, "Toggle Sidebar", "⌘B", a.toggleSidebar)
				})
				if a.root == "" && len(a.settings.Recent) > 0 {
					a.welcomeSection(c, "Recent", func() {
						for _, dir := range a.settings.Recent[:min(6, len(a.settings.Recent))] {
							r := ui.ButtonBase(c.Key("recent:"+dir)).FillWidth().Padding(6, 10).Radius(6).Gap(10).
								Justify(ui.Start).AlignItems(ui.Center).Label("Open " + dir).Tooltip(shortenHome(dir))
							if r.Hovered() {
								r.Background(p.hover)
							}
							r.Children(func() {
								ui.Icon(c, iconFolder).FontSize(15).TextColor(p.accent)
								ui.Text(c, filepath.Base(dir)).FontSize(13).TextColor(p.text).SingleLine().Shrink(0)
								ui.Text(c, shortenHome(filepath.Dir(dir))).FontSize(12).TextColor(p.faint).SingleLine().Shrink(1)
							})
							// With Cmd, in a window of its own.
							switch {
							case r.Focused() && c.Shortcut(ui.Cmd, ui.KeyEnter):
								a.host.newWindow(dir)
							case r.Clicked():
								if r.ClickModifiers()&ui.Cmd != 0 {
									a.host.newWindow(dir)
								} else {
									a.switchFolder(dir)
								}
							}
						}
					})
				}
			})
		})
	})
}

// welcomeSection is a titled group of the welcome page.
func (a *app) welcomeSection(c *ui.Context, title string, rows func()) {
	p := a.pal
	ui.Column(c).Gap(2).Children(func() {
		ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(p.muted).Padding(0, 10, 6)
		rows()
	})
}

// welcomeAction is a row of the welcome page that runs a command: its
// icon, its name and its keys.
func (a *app) welcomeAction(c *ui.Context, icon *ui.SVG, label, keys string, fn func()) {
	p := a.pal
	r := ui.ButtonBase(c.Key("action:"+label)).FillWidth().Padding(6, 10).Radius(6).Gap(10).
		Justify(ui.Start).AlignItems(ui.Center).Label(label)
	hovered := r.Hovered()
	if hovered {
		r.Background(p.hover)
	}
	r.Children(func() {
		col := p.muted
		if hovered {
			col = p.accent
		}
		ui.Icon(c, icon).FontSize(15).TextColor(col)
		ui.Text(c, label).FontSize(13).TextColor(p.text).Grow(1).SingleLine()
		a.keycaps(c, keys)
	})
	if r.Clicked() {
		fn()
	}
}

// keycaps shows keys as the caps of a keyboard, one a symbol, except for
// a named key as "esc".
func (a *app) keycaps(c *ui.Context, keys string) {
	p := a.pal
	var caps []string
	if isASCII(keys) && len(keys) > 1 && strings.ToLower(keys) == keys && !strings.ContainsAny(keys, "`") {
		caps = []string{keys}
	} else {
		for _, k := range keys {
			caps = append(caps, string(k))
		}
	}
	ui.Row(c).Gap(3).Shrink(0).Children(func() {
		for _, k := range caps {
			ui.Text(c, k).FontSize(11).TextColor(p.muted).TextAlign(ui.Center).
				MinWidth(18).Height(18).Padding(1, 4, 0).Radius(4).Background(p.hover).Border(1, p.border)
		}
	})
}

func (a *app) toggleSidebar() {
	a.settings.SidebarOpen = !a.settings.SidebarOpen
	a.settings.Changed()
}

// togglePanel shows or hides the terminal panel, starting a terminal the
// first time it shows.
func (a *app) togglePanel() {
	w := a.window
	if w.PanelOpen && a.focus != paneTerminal && a.panelTerm != nil {
		// Shown but not focused: focus it, as VS Code does.
		a.focusTerminal()
		return
	}
	w.PanelOpen = !w.PanelOpen
	if w.PanelOpen {
		if a.panelTerm == nil {
			a.newTerminal()
		}
		a.focusTerminal()
	} else {
		a.focusColumn()
	}
	a.settings.Changed()
}

func shortenHome(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(v, hi)) }
