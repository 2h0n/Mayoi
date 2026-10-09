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
		ui.Row(c).Grow(1).MinWidth(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			a.commandCenter(c)
		})
		ui.Row(c).Width(120).Justify(ui.End).Gap(2).Children(func() {
			if a.iconButton(c, iconPanel, "Toggle Terminal (⌃`)", a.settings.PanelOpen).Clicked() {
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
		if d := a.activeDoc(); d != nil {
			name = d.name()
			if d.dirty() {
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
	// The blocks the caret is in, after the file: the innermost two.
	snap := d.buf.Snapshot()
	var scope []int
	if isCode(d) {
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
				a.editActive(func(d *document) { a.jumpToScope(d, line) })()
			}
			if i < len(scope)-1 {
				ui.Icon(c, iconChevronRight).FontSize(11).TextColor(p.faint)
			}
		}
	})
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
		if a.branch != "" {
			item("branch", a.branch, "Branch "+a.branch, iconBranch, nil)
		}
		if a.root != "" {
			item("folder", filepath.Base(a.root), "Reveal "+shortenHome(a.root)+" in Finder", iconFolder, func() { a.host.reveal(a.root) })
		}
		ui.Spacer(c)
		d := a.activeDoc()
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
		item("lines", fmt.Sprintf("%d lines", snap.LineCount()), "", nil, nil)
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
				a.editActive(func(d *document) { reindent(d, indentStyle{tabs: true, size: tabWidth}) })()
			}
			if m.Item("Convert Indentation to Spaces").Chosen() {
				size := d.indent.size
				if d.indent.tabs {
					size = tabWidth
				}
				a.editActive(func(d *document) { reindent(d, indentStyle{size: size}) })()
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
		item("lang", highlight.Name(d.hl.Lexer()), "", nil, nil)
	})
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// logo is Mayoi's mark, as on its icon: an M in a gradient of the accent,
// and the dot of a moment's hesitation below it.
var logo = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64">
<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#c2b5ff"/><stop offset="1" stop-color="#8b7cf6"/></linearGradient></defs>
<path d="M14 42V14L32 32L50 14V42" fill="none" stroke="url(#g)" stroke-width="7" stroke-linecap="round" stroke-linejoin="round"/>
<circle cx="32" cy="51" r="4" fill="#e8b86d"/></svg>`))

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
					ui.Image(c, logo).Size(52, 52)
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
							if r.Clicked() {
								a.openFolder(dir)
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
