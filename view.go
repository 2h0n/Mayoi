package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	a.drain()
	a.pal = applyTheme(c)
	p := a.pal
	if f := a.settings; a.font.charW == 0 || a.font.size != f.FontSize || a.font.family != f.FontFamily {
		a.font = newCodeFont(f.FontFamily, f.FontSize)
	}
	if !a.started {
		a.started = true
		a.start()
	}
	a.shortcuts(c)
	// Which pane has the keyboard focus, as the frame before left it.
	switch d := a.activeDoc(); {
	case a.termView.FocusWithin(c):
		a.focus = paneTerminal
	case d != nil && d.vscroll.FocusWithin(c):
		a.focus = paneEditor
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
					a.splitter(c, false, func(d float32) {
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
	for _, msg := range a.errors {
		c.Toast(msg)
	}
	a.errors = a.errors[:0]
	a.tick()
}

// main is the editor and, below it, the panel.
func (a *app) main(c *ui.Context) {
	p, s := a.pal, a.settings
	panel := s.PanelOpen
	editor := ui.Column(c.Key("editor")).Grow(1).MinHeight(0).Background(p.editor).Clip()
	if panel && a.panelMax {
		// Out of sight, but built, so that the editors keep their state.
		editor.Grow(0).Height(0).Invisible()
	}
	editor.Children(func() { a.editorArea(c) })
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
			a.splitter(c, true, func(d float32) {
				s.PanelHeight = clamp(height-d, 80, max(room-80, 80))
			})
		}
	})
}

// splitter is the handle along the edge of a pane that resizes it: the
// right edge of the sidebar, or the top of the panel. drag gets how far
// the pointer moved.
func (a *app) splitter(c *ui.Context, vertical bool, drag func(d float32)) {
	h := ui.Box(c).Absolute()
	if vertical {
		h.Top(0).Left(0).Right(0).Height(5).Cursor(ui.CursorResizeNS)
	} else {
		h.Top(0).Bottom(0).Right(0).Width(5).Cursor(ui.CursorResizeEW)
	}
	lit := h.Hovered() || h.Dragging()
	h.Children(func() {
		if !lit {
			return
		}
		line := ui.Box(c).Absolute().Background(a.pal.accentDim)
		if vertical {
			line.Top(0).Left(0).Right(0).Height(2)
		} else {
			line.Top(0).Bottom(0).Right(0).Width(2)
		}
	})
	dx, dy, held := h.Dragged()
	if held && (dx != 0 || dy != 0) {
		if vertical {
			drag(dy)
		} else {
			drag(dx)
		}
		a.settings.changed()
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
		ui.Row(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, a.windowTitle()).FontSize(12.5).FontWeight(500).TextColor(p.muted).SingleLine()
		})
		ui.Row(c).Width(120).Justify(ui.End).Gap(2).Children(func() {
			if a.iconButton(c, iconSearch, "Quick Open (⌘P)", false).Clicked() {
				a.openQuick("")
			}
			if a.iconButton(c, iconPanel, "Toggle Terminal (⌃`)", a.settings.PanelOpen).Clicked() {
				a.togglePanel()
			}
		})
	})
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

// editorArea is the tab strip, the breadcrumbs, and the editor of the
// document shown, or the welcome page.
func (a *app) editorArea(c *ui.Context) {
	if len(a.docs) == 0 {
		a.welcome(c)
		return
	}
	a.tabStrip(c)
	a.breadcrumbs(c)
	// Every open document keeps its editor, so that its undo history, its
	// selection and its place stay as tabs change: those not shown are
	// built invisible, out of the way.
	ui.Box(c).Grow(1).Children(func() {
		for i, d := range a.docs {
			box := ui.Column(c.Key(d.id)).Absolute().Top(0).Left(0).Right(0).Bottom(0)
			if i != a.active {
				box.Invisible()
			}
			box.Children(func() {
				if i == a.active {
					a.editorView(c, d)
					a.findView(c, d)
				} else {
					a.hiddenEditor(c, d)
				}
			})
		}
	})
}

// hiddenEditor keeps the state of a document's text area while another
// is shown.
func (a *app) hiddenEditor(c *ui.Context, d *document) {
	ui.ScrollHorizontal(c).TrackScroll(&d.hscroll).Fill().Children(func() {
		ui.TextAreaBufferBase(c, d.buf).NoWrap().Font(a.font.family).FontSize(a.font.size).FixedLineHeight(a.font.lineH).
			Fill().TrackScroll(&d.vscroll)
	})
}

func (a *app) breadcrumbs(c *ui.Context) {
	d := a.activeDoc()
	p := a.pal
	var parts []string
	if d.path != "" {
		rel := d.path
		if a.root != "" && strings.HasPrefix(d.path, a.root+string(filepath.Separator)) {
			rel = d.path[len(a.root)+1:]
		}
		parts = strings.Split(filepath.ToSlash(rel), "/")
	} else {
		parts = []string{d.name()}
	}
	ui.Row(c).Height(crumbsH).Padding(0, 14).Gap(4).AlignItems(ui.Center).Background(p.editor).Clip().Children(func() {
		for i, part := range parts {
			last := i == len(parts)-1
			if last {
				icon, col := fileIcon(p, part)
				ui.Icon(c, icon).FontSize(13).TextColor(col)
			}
			t := ui.Text(c, part).FontSize(12).SingleLine().TextColor(p.muted)
			if last {
				t.TextColor(p.text)
			}
			if !last {
				ui.Icon(c, iconChevronRight).FontSize(11).TextColor(p.faint)
			}
		}
	})
}

func (a *app) statusBar(c *ui.Context) {
	p := a.pal
	ui.Row(c).Height(statusBarH).Padding(0, 10).Gap(14).AlignItems(ui.Center).Background(p.status).
		BorderWidth(1, 0, 0, 0).BorderColor(p.border).Children(func() {
		item := func(text string) ui.Element {
			return ui.Text(c, text).FontSize(11.5).TextColor(p.statusFg).SingleLine()
		}
		if a.branch != "" {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconBranch).FontSize(12).TextColor(p.statusFg)
				item(a.branch)
			})
		}
		if a.root != "" {
			item(filepath.Base(a.root))
		}
		ui.Spacer(c)
		d := a.activeDoc()
		if d == nil {
			return
		}
		if d.diskGone {
			item("Deleted on disk").TextColor(p.danger)
		}
		snap := d.buf.Snapshot()
		caret := min(d.caret, snap.Len())
		line := snap.LineAt(caret)
		s, _ := snap.LineRange(line)
		col := columns(snap.Slice(s, caret)) + 1
		item(fmt.Sprintf("Ln %d, Col %d", line+1, col))
		if a.find.open && len(a.find.matches) > 0 {
			item(fmt.Sprintf("%d matches", len(a.find.matches)))
		}
		item(fmt.Sprintf("%d lines", snap.LineCount()))
		item("UTF-8")
		if d.crlf {
			item("CRLF")
		} else {
			item("LF")
		}
		item(highlight.Name(d.hl.Lexer()))
	})
}

// welcome shows while no file is open.
func (a *app) welcome(c *ui.Context) {
	p := a.pal
	ui.Column(c).Fill().Center().Gap(28).Background(p.editor).Children(func() {
		ui.Column(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "Mayoi").FontSize(40).FontWeight(300).LetterSpacing(1).TextColor(p.text)
			ui.Text(c, "迷い — a small, quick editor").FontSize(13).TextColor(p.muted)
		})
		ui.Column(c).Gap(2).Width(300).Children(func() {
			row := func(label, keys string, fn func()) {
				r := ui.ButtonBase(c).Padding(7, 12).Radius(6).AlignItems(ui.Center).Label(label)
				if r.Hovered() {
					r.Background(p.hover)
				}
				r.Children(func() {
					ui.Text(c, label).FontSize(13).TextColor(p.text).Grow(1)
					ui.Text(c, keys).FontSize(12).TextColor(p.faint).Font("monospace")
				})
				if r.Clicked() {
					fn()
				}
			}
			if a.root == "" {
				row("Open Folder…", "⌘O", a.chooseFolder)
			} else {
				row("Quick Open", "⌘P", func() { a.openQuick("") })
			}
			row("New File", "⌘N", a.newFile)
			row("Toggle Terminal", "⌃`", a.togglePanel)
			row("Toggle Sidebar", "⌘B", a.toggleSidebar)
		})
		if a.root == "" && len(a.settings.Recent) > 0 {
			ui.Column(c).Gap(2).Width(300).Children(func() {
				ui.Text(c, "RECENT").FontSize(11).FontWeight(600).LetterSpacing(0.6).TextColor(p.faint).Padding(0, 12, 4)
				for _, dir := range a.settings.Recent[:min(5, len(a.settings.Recent))] {
					r := ui.ButtonBase(c.Key("recent:"+dir)).Padding(6, 12).Radius(6).Gap(8).AlignItems(ui.Center).Label(dir)
					if r.Hovered() {
						r.Background(p.hover)
					}
					r.Children(func() {
						ui.Icon(c, iconFolder).FontSize(13).TextColor(p.accent)
						ui.Text(c, filepath.Base(dir)).FontSize(13).TextColor(p.text).SingleLine()
						ui.Text(c, shortenHome(filepath.Dir(dir))).FontSize(12).TextColor(p.faint).SingleLine().Shrink(1)
					})
					if r.Clicked() {
						a.openFolder(dir)
					}
				}
			})
		}
	})
}

func (a *app) toggleSidebar() {
	a.settings.SidebarOpen = !a.settings.SidebarOpen
	a.settings.changed()
}

// togglePanel shows or hides the terminal panel, starting a terminal the
// first time it shows.
func (a *app) togglePanel() {
	s := a.settings
	if s.PanelOpen && a.focus != paneTerminal && len(a.terms) > 0 {
		// Shown but not focused: focus it, as VS Code does.
		a.focusTerminal()
		return
	}
	s.PanelOpen = !s.PanelOpen
	if s.PanelOpen {
		if len(a.terms) == 0 {
			a.newTerminal()
		}
		a.focusTerminal()
	} else if d := a.activeDoc(); d != nil {
		d.vscroll.Focus()
		a.focus = paneEditor
	}
	s.changed()
}

func shortenHome(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}
