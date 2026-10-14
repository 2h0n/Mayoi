package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/settings"
)

// railW is the width of the rail of project tabs.
const railW = 48

// frame is a native window of the app. It shows one project, or with
// project tabs several, one at a time, which the rail on its left
// chooses among.
type frame struct {
	m   *mayoi
	win *mygo.Window
	// tabs are the window's projects in the rail's order, and active the
	// one it shows.
	tabs   []*desktop
	active *desktop
	// services are the window's, which the projects it does not show post
	// to, once it has drawn.
	services    ui.Services
	hasServices bool
	// merged is set as the window closes once its projects moved to
	// another, and closing once the changes of its projects are settled.
	merged, closing bool
	// shown is the project the window built last.
	shown *desktop
}

// newFrame opens a window that shows h.
func (m *mayoi) newFrame(h *desktop) *frame {
	f := &frame{m: m}
	st := h.a.window
	key := "main"
	switch {
	case m.s.ProjectTabs:
		key = "projects"
	case st.ID > 0:
		key = fmt.Sprintf("main-%d", st.ID)
	}
	opts := mygo.WindowOptions{
		Title: "Mayoi", Width: 1280, Height: 820, MinWidth: 640, MinHeight: 420,
		StateKey:             key,
		TitleBarStyle:        mygo.TitleBarHidden,
		TrafficLightPosition: &mygo.Point{X: 14, Y: 13},
		BackgroundColor:      "#131317",
		Content:              ui.View(f.view),
	}
	if len(m.wins) > 0 && m.wins[0].f != nil {
		// Below and right of the window in front, unless the frame of
		// st's window is remembered.
		opts.X, opts.Y = m.wins[0].f.win.Position()
		opts.X, opts.Y = opts.X+28, opts.Y+28
	}
	win := mygo.NewWindow(opts)
	f.win = win
	m.frames = append(m.frames, f)
	f.add(h)
	f.active = h

	// Closing the window saves the files that save themselves, and with
	// other unsaved changes asks about each first.
	win.OnClose(func(e *mygo.CloseEvent) {
		if f.merged || f.closing || m.quitting || !slices.ContainsFunc(f.tabs, func(d *desktop) bool { return d.a.anyDirty() }) {
			return
		}
		e.PreventDefault()
		m.settle(slices.Clone(f.tabs), func() {
			f.closing = true
			win.Close()
		})
	})
	// Back from another app, which may have committed.
	win.OnFocus(func() {
		h := f.active
		if h == nil {
			return
		}
		m.front(h)
		a := h.a
		a.post(func() { a.blurred = false })
		a.post(a.gitChanged)
		// The File menu's recent folders leave out this one's.
		m.refreshMenu()
	})
	win.OnBlur(func() {
		if h := f.active; h != nil {
			a := h.a
			a.post(func() { a.blurred = true })
			a.post(a.autoSaveAll)
		}
	})
	// Files dropped where nothing takes them, as on the explorer, open in
	// the column that has the keyboard.
	win.OnFileDrop(func(e *mygo.FileDropEvent) {
		h := f.active
		if h == nil {
			return
		}
		paths, a := e.Paths, h.a
		a.post(func() {
			col := a.column()
			a.dropFiles(paths, col, col.active+1)
		})
	})
	// The last window to close quits the app, and stays in the settings
	// to open again, as do those the quitting closes. The others keep
	// their folders' sessions.
	win.OnClosed(func() {
		m.frames = slices.DeleteFunc(m.frames, func(o *frame) bool { return o == f })
		if f.merged {
			return
		}
		for _, h := range f.tabs {
			h.a.keepSession()
			h.a.closeTerminals()
			m.wins = slices.DeleteFunc(m.wins, func(w *desktop) bool { return w == h })
		}
		if !m.quitting && len(m.wins) > 0 {
			for _, h := range f.tabs {
				m.s.Forget(h.a.window)
			}
		}
		if len(m.wins) == 0 {
			m.s.Flush()
		}
	})
	return f
}

// view builds the project the window shows. Those it does not show run
// what was posted to them, as a terminal's new title, and are built
// anew as they show again.
func (f *frame) view(c *ui.Context) {
	f.services, f.hasServices = c.Services(), true
	// The project shown before goes out of sight now, rather than as
	// another was chosen: the rest of its frame built it still.
	if old := f.shown; old != f.active {
		if old != nil && slices.Contains(f.tabs, old) {
			old.a.park()
		}
		f.shown = f.active
	}
	for _, h := range slices.Clone(f.tabs) {
		if h != f.active && h.a.started {
			h.a.setServices(f.services)
			h.a.drain()
		}
	}
	h := f.active
	if h == nil {
		return
	}
	// Each project's elements are its own, as its tabs are numbered as
	// another's are.
	ui.Column(c.Key(fmt.Sprintf("project-%d", h.a.window.ID))).Fill().Children(func() { h.a.view(c) })
}

// add puts h at the end of the rail, without showing it.
func (f *frame) add(h *desktop) {
	h.f = f
	f.tabs = append(f.tabs, h)
	h.a.rail = func(c *ui.Context) { f.railView(c, h.a) }
	if f.hasServices {
		h.a.setServices(f.services)
	}
	f.keepRail()
}

// keepRail keeps the order of the rail in the settings.
func (f *frame) keepRail() {
	if !f.m.s.ProjectTabs {
		return
	}
	ws := make([]*settings.Window, len(f.tabs))
	for i, h := range f.tabs {
		ws[i] = h.a.window
	}
	f.m.s.SetRail(ws)
}

// activate shows h in the window, in place of the project it showed,
// which saves the files that save themselves.
func (f *frame) activate(h *desktop) {
	if f.active == h || h.f != f {
		return
	}
	if old := f.active; old != nil {
		old.a.autoSaveAll()
		old.a.blurred = true
	}
	f.active = h
	a := h.a
	a.blurred = false
	// Its files may have changed meanwhile, and the keyboard goes back to
	// its column.
	a.post(a.gitChanged)
	a.post(a.focusColumn)
	f.m.front(h)
	title := a.title
	if title == "" {
		title = "Mayoi"
	}
	f.win.SetTitle(title)
	f.win.Invalidate()
	f.m.refreshMenu()
}

// show brings the window to the front.
func (f *frame) show() {
	if f.win.IsMinimized() {
		f.win.Restore()
	}
	f.win.Focus()
}

// front makes h the project focused last.
func (m *mayoi) front(h *desktop) {
	if i := slices.Index(m.wins, h); i > 0 {
		m.wins = slices.Insert(slices.Delete(m.wins, i, i+1), 0, h)
	}
	m.s.Focused(h.a.window)
}

// stepProject shows the project by steps along the rail, around its ends.
func (h *desktop) stepProject(by int) {
	f := h.f
	if n := len(f.tabs); n > 1 {
		i := slices.Index(f.tabs, f.active)
		f.activate(f.tabs[((i+by)%n+n)%n])
	}
}

// showProject shows the project at i of the rail, the last for -1.
func (h *desktop) showProject(i int) {
	f := h.f
	if i < 0 {
		i = len(f.tabs) - 1
	}
	if i >= 0 && i < len(f.tabs) {
		f.activate(f.tabs[i])
	}
}

// closeProject closes the project, once its changes are settled, or the
// window when it is the window's only one.
func (h *desktop) closeProject() {
	m, f, a := h.m, h.f, h.a
	if len(f.tabs) < 2 {
		f.win.Close()
		return
	}
	if !a.started {
		m.dropProject(h)
		return
	}
	h.do(func() {
		if a.autoSaveAll(); !a.anyDirty() {
			m.dropProject(h)
			return
		}
		h.show()
		a.closeKeeping(func() { m.dropProject(h) })
	})
}

func (h *desktop) projectTabsChanged() { h.m.projectTabsChanged() }

// dropProject takes h off its window's rail, keeping its session for
// the next time its folder opens, and shows its neighbor.
func (m *mayoi) dropProject(h *desktop) {
	f, a := h.f, h.a
	i := slices.Index(f.tabs, h)
	if i < 0 {
		return
	}
	a.keepSession()
	a.closeTerminals()
	if a.tree != nil {
		a.tree.Close()
	}
	a.resetSearch()
	m.wins = slices.DeleteFunc(m.wins, func(w *desktop) bool { return w == h })
	m.s.Forget(a.window)
	f.tabs = slices.Delete(f.tabs, i, i+1)
	if f.active == h {
		f.active = nil
		f.activate(f.tabs[min(i, len(f.tabs)-1)])
	}
	f.keepRail()
	f.win.Invalidate()
}

// projectTabsChanged gathers the windows into the one in front, as project
// tabs, or parts the projects of each window into a window each, as the
// settings now have it.
func (m *mayoi) projectTabsChanged() {
	if len(m.wins) == 0 || m.wins[0].f == nil {
		return
	}
	front := m.wins[0]
	if m.s.ProjectTabs {
		into := front.f
		// The others after the one in front, the one focused last first.
		for _, h := range slices.Clone(m.wins) {
			from := h.f
			if from == into {
				continue
			}
			from.tabs = slices.DeleteFunc(from.tabs, func(d *desktop) bool { return d == h })
			if from.active == h {
				from.active = nil
			}
			h.a.park()
			into.add(h)
			if len(from.tabs) == 0 {
				from.merged = true
				from.win.Close()
			}
		}
		into.keepRail()
		into.win.Invalidate()
	} else {
		for _, f := range slices.Clone(m.frames) {
			for _, h := range slices.Clone(f.tabs) {
				if h == f.active {
					continue
				}
				f.tabs = slices.DeleteFunc(f.tabs, func(d *desktop) bool { return d == h })
				h.a.park()
				m.newFrame(h)
			}
			f.win.Invalidate()
		}
		// Once the new windows have come forward.
		front.do(front.show)
	}
	m.refreshMenu()
}

// railView is the rail of project tabs, left of the sidebar: a square of
// each project's initials, the one shown marked, and a button that opens
// another folder. A window of one project has none. a is the project it
// is built in, which the frame goes on building as a click shows another.
func (f *frame) railView(c *ui.Context, a *app) {
	if !f.m.s.ProjectTabs || len(f.tabs) < 2 {
		return
	}
	p := a.pal
	ui.Column(c.Key("rail")).Width(railW).Shrink(0).BorderWidth(0, 1, 0, 0).BorderColor(p.border).Children(func() {
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(8, 0).Gap(6).Children(func() {
				for i, h := range slices.Clone(f.tabs) {
					f.railTab(c, a, h, i)
				}
			})
		})
		ui.Row(c).FillWidth().Justify(ui.Center).Padding(6, 0, 8, 0).Children(func() {
			if a.iconButton(c, iconPlus, a.withKey("Open Folder…", "file.openFolder"), false).Clicked() {
				a.chooseFolder()
			}
		})
	})
}

// railTab is a project's square on the rail, which shows it.
func (f *frame) railTab(c *ui.Context, a *app, h *desktop, i int) {
	p := a.pal
	dir := h.a.folder()
	name, tip := "Untitled", "No folder"
	if dir != "" {
		name, tip = filepath.Base(dir), shortenHome(dir)
	}
	if i < 9 {
		mods := "Ctrl+Alt+"
		if macKeys() {
			mods = "Ctrl+CmdOrCtrl+"
		}
		tip += " (" + keyText(fmt.Sprintf("%s%d", mods, i+1)) + ")"
	}
	on := h == f.active
	dirty := h.a.anyDirty()
	col := avatarColor(p, dir)
	b := ui.ButtonBase(c.Key(fmt.Sprintf("project-%d", h.a.window.ID))).FillWidth().Height(34).Shrink(0).Label(name).Tooltip(tip)
	hovered := b.Hovered()
	b.Draw(func(pt *ui.Painter, r ui.Rect) {
		sq := ui.Rect{X: r.X + (r.W-32)/2, Y: r.Y + (r.H-32)/2, W: 32, H: 32}
		if on {
			// A bar along the rail's edge, as the activity bar marks the
			// view shown.
			pt.Fill(ui.Rect{X: r.X, Y: sq.Y + 6, W: 3, H: sq.H - 12}, p.accent, 1.5)
		}
		alpha := float32(0.16)
		switch {
		case on:
			alpha = 0.32
		case hovered:
			alpha = 0.24
		}
		pt.Fill(sq, col.Alpha(alpha), 8)
		if on {
			pt.Stroke(sq, col.Alpha(0.7), 8, 1)
		}
		paintInitials(pt, sq, projectWords(name), 12, col)
		if dirty {
			pt.Fill(ui.Rect{X: sq.X + sq.W - 7, Y: sq.Y - 1, W: 8, H: 8}, p.text, 4)
		}
	})
	b.ContextMenu(func(mn *ui.Menu) {
		if mn.Item("Close Project").Chosen() {
			h.closeProject()
		}
	})
	if b.Clicked() {
		f.activate(h)
	}
}

// projectWords spells a folder's name as words, for its initials: "my-app"
// as "my app", "mayoiEditor" as "mayoi Editor".
func projectWords(name string) string {
	var b strings.Builder
	prev := rune(0)
	for _, r := range name {
		switch {
		case r == '-' || r == '_' || r == '.':
			r = ' '
		case r >= 'A' && r <= 'Z' && prev >= 'a' && prev <= 'z':
			b.WriteRune(' ')
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}
