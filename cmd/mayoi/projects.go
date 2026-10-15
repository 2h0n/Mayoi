package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/remote"
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
			h.a.shutdown()
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
func (h *desktop) closeProject() { h.closeProjectThen(nil) }

// closeProjectThen closes the project as closeProject does, then runs
// done, unless its changes were kept open.
func (h *desktop) closeProjectThen(done func()) {
	m, f, a := h.m, h.f, h.a
	if len(f.tabs) < 2 {
		f.win.Close()
		return
	}
	drop := func() {
		m.dropProject(h)
		if done != nil {
			done()
		}
	}
	if !a.started {
		drop()
		return
	}
	h.do(func() {
		if a.autoSaveAll(); !a.anyDirty() {
			drop()
			return
		}
		h.show()
		a.closeKeeping(drop)
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
	a.shutdown()
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
// each project's initials, the one shown marked, and after them a button
// that opens another folder. A window of one project has none. a is the
// project it is built in, which the frame goes on building as a click
// shows another. A square is dragged along the rail to reorder it, and a
// folder dropped on the rail opens as a project.
func (f *frame) railView(c *ui.Context, a *app) {
	if !f.m.s.ProjectTabs || len(f.tabs) < 2 {
		return
	}
	p := a.pal
	rail := ui.Column(c.Key("rail")).Width(railW).Shrink(0).Background(railGround(p)).BorderWidth(0, 1, 0, 0).BorderColor(p.border)
	// Past the last square: at the end.
	f.railDrop(rail, a, nil)
	_, atEnd := ui.DragOver[railDrag](rail)
	folders := rail.FileDragOver()
	rail.Children(func() {
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(8, 0, 10, 0).Gap(6).AlignItems(ui.Center).Children(func() {
				for i, h := range slices.Clone(f.tabs) {
					f.railTab(c, a, h, i)
				}
				if atEnd {
					ui.Box(c).Size(24, 2).Radius(1).Background(p.accent)
				}
				ui.Box(c).Size(20, 1).Margin(4, 0, 2, 0).Background(p.border)
				f.railAdd(c, a, folders)
			})
		})
	})
}

// railGround is the rail's background: a shade off the sidebar's, as an
// activity bar is.
func railGround(p *palette) ui.Color {
	if p.dark {
		return mix(p.chrome, ui.RGBA(0, 0, 0, 1), 0.18)
	}
	return mix(p.chrome, p.border, 0.22)
}

// railDrag is a project's square dragged along the rail.
type railDrag struct{ h *desktop }

// railDrop moves the square dropped on e before the project before, at
// the end for nil, and opens the folders dropped there from another app.
func (f *frame) railDrop(e ui.Element, a *app, before *desktop) {
	if it, ok := ui.Drop[railDrag](e); ok {
		a.post(func() { f.move(it.h, before) })
	}
	if paths := e.DroppedFiles(); paths != nil {
		var dirs []string
		for _, path := range paths {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				dirs = append(dirs, path)
			}
		}
		if len(dirs) > 0 {
			a.post(func() { a.host.openFolders(dirs) })
		}
	}
}

// move puts h before the project before on the rail, at its end for nil.
func (f *frame) move(h, before *desktop) {
	if h == before || !slices.Contains(f.tabs, h) {
		return
	}
	f.tabs = slices.DeleteFunc(f.tabs, func(d *desktop) bool { return d == h })
	at := len(f.tabs)
	if i := slices.Index(f.tabs, before); i >= 0 {
		at = i
	}
	f.tabs = slices.Insert(f.tabs, at, h)
	f.keepRail()
	if f.win != nil {
		f.win.Invalidate()
	}
}

// railSquare is the side of a project's square, and railRadius its corners.
const (
	railSquare = 32
	railRadius = 9
)

// railTab is a project's square on the rail, which shows it, with a pill
// along the rail's edge that grows from under the pointer to the project
// shown, a dot of unsaved changes, and one of a terminal's bell or
// notification while the project was out of sight.
func (f *frame) railTab(c *ui.Context, a *app, h *desktop, i int) {
	p := a.pal
	dir := h.a.folder()
	name, where := "Untitled", "No folder"
	if dir != "" {
		name, where = filepath.Base(dir), shortenHome(dir)
	}
	var keys []string
	if i < 9 {
		mods := "Ctrl+Alt+"
		if macKeys() {
			mods = "Ctrl+CmdOrCtrl+"
		}
		keys = accelCaps(platformAccel("", fmt.Sprintf("%s%d", mods, i+1)), macKeys())
	}
	on := h == f.active
	dirty := h.a.anyDirty()
	if on {
		h.a.alert = ""
	}
	alert := h.a.alert
	col := avatarColor(p, dir)
	b := ui.ButtonBase(c.Key(fmt.Sprintf("project-%d", h.a.window.ID))).Width(railW).Height(railSquare + 4).Shrink(0).
		Label(name).Description(where).Drag(railDrag{h})
	f.railTip(c, a, b, name, where, keys, dirty, alert)
	f.railDrop(b, a, h)
	_, over := ui.DragOver[railDrag](b)
	hovered, pressed, dragged := b.Hovered(), b.Pressed(), b.Dragging()
	pill, fill := float32(0), float32(0.14)
	switch {
	case on:
		pill, fill = 20, 0.30
	case hovered:
		pill, fill = 8, 0.22
	}
	pill = b.Animate("pill", pill, 160*time.Millisecond)
	fill = b.Animate("fill", fill, 120*time.Millisecond)
	ring := b.Animate("ring", unit(on), 160*time.Millisecond)
	sink := b.Animate("sink", unit(pressed && !dragged), 80*time.Millisecond)
	ground := railGround(p)
	b.Draw(func(pt *ui.Painter, r ui.Rect) {
		if over {
			// Where the square dragged goes: before this one.
			pt.Fill(ui.Rect{X: r.X + (r.W-24)/2, Y: r.Y - 4, W: 24, H: 2}, p.accent, 1)
		}
		if pill > 0.5 {
			// Along the rail's edge, as the activity bar marks the view
			// shown.
			pc := p.muted
			if on {
				pc = p.accent
			}
			pt.Fill(ui.Rect{X: r.X - 2, Y: r.Y + (r.H-pill)/2, W: 5, H: pill}, pc, 2.5)
		}
		s := float32(railSquare) - 2*sink
		sq := ui.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}
		alpha := float32(1)
		if dragged {
			alpha = 0.4
		}
		pt.Fill(sq, col.Alpha(fill*alpha), railRadius)
		if ring > 0.01 {
			pt.Stroke(sq, col.Alpha(0.75*ring*alpha), railRadius, 1.25)
		}
		ink := col
		if !on && !hovered {
			ink = col.Mix(p.muted, 0.25)
		}
		paintInitials(pt, sq, projectWords(name), 12, ink.Alpha(alpha))
		if alert != "" {
			// Below the dot of unsaved changes, in the accent.
			d := ui.Rect{X: sq.X + sq.W - 7, Y: sq.Y + sq.H - 7, W: 10, H: 10}
			pt.Fill(ui.Rect{X: d.X - 2, Y: d.Y - 2, W: d.W + 4, H: d.H + 4}, ground, 7)
			pt.Fill(d, p.accent, 5)
		}
		if dirty {
			// Cut out of the square by a ring of the rail's ground.
			d := ui.Rect{X: sq.X + sq.W - 7, Y: sq.Y - 3, W: 10, H: 10}
			pt.Fill(ui.Rect{X: d.X - 2, Y: d.Y - 2, W: d.W + 4, H: d.H + 4}, ground, 7)
			pt.Fill(d, p.text.Alpha(0.85), 5)
		}
	})
	b.ContextMenu(func(mn *ui.Menu) {
		if mn.Item(revealLabel()).Disabled(dir == "" || remote.Is(dir)).Chosen() {
			a.host.reveal(dir)
		}
		if mn.Item("Copy Path").Disabled(dir == "").Chosen() {
			a.services.WriteClipboard(hostPath(dir))
		}
		mn.Separator()
		if mn.Item("Close Project").Chosen() {
			h.closeProject()
		}
		if mn.Item("Close Other Projects").Chosen() {
			a.post(func() { f.closeOthers(h) })
		}
	})
	if b.Clicked() {
		f.activate(h)
	}
}

// railTip is the tip of a project's square, to its right, where the
// window's buttons above it leave it room: the project's name, its folder,
// the keys that show it and whether it has unsaved changes.
func (f *frame) railTip(c *ui.Context, a *app, b ui.Element, name, where string, keys []string, dirty bool, alert string) {
	p := a.pal
	ui.TooltipBase(c, b, func(tip ui.Element) {
		tip.AttachTo(b, ui.AnchorRight, ui.AnchorLeft).Margin(0, 0, 0, 4)
		a.floating(c, tip, false).Padding(8, 10).Radius(8).Gap(3).MaxWidth(360)
		tip.Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).FontSize(12.5).FontWeight(600).TextColor(p.text).SingleLine().Grow(1).MinWidth(0)
				if keys != nil {
					a.caps(c, keys)
				}
			})
			ui.Text(c, where).FontSize(11.5).TextColor(p.muted)
			note := func(dot ui.Color, s string) {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Margin(2, 0, 0, 0).Children(func() {
					ui.Box(c).Size(6, 6).Radius(3).Background(dot).Shrink(0)
					ui.Text(c, s).FontSize(11.5).TextColor(p.muted)
				})
			}
			if alert != "" {
				note(p.accent, alert)
			}
			if dirty {
				note(p.text.Alpha(0.85), "Unsaved changes")
			}
		})
	})
}

// railAdd is the button after the squares that opens another folder: a
// square drawn dashed, which fills under the pointer, or as folders are
// dragged over the rail.
func (f *frame) railAdd(c *ui.Context, a *app, folders bool) {
	p := a.pal
	b := ui.ButtonBase(c.Key("rail-add")).Size(railW, railSquare+4).Shrink(0).Label("Open Folder…").
		Tooltip(a.withKey("Open Folder…", "file.openFolder"))
	hot := b.Hovered() || folders
	k := b.Animate("hot", unit(hot), 120*time.Millisecond)
	b.Draw(func(pt *ui.Painter, r ui.Rect) {
		sq := ui.Rect{X: r.X + (r.W-railSquare)/2, Y: r.Y + (r.H-railSquare)/2, W: railSquare, H: railSquare}
		if k > 0.01 {
			pt.Fill(sq, p.accent.Alpha(0.14*k), railRadius)
		}
		pt.StrokeDashed(sq, p.faint.Mix(p.accent, k), railRadius, 1)
	})
	b.Children(func() {
		ui.Box(c).Absolute().Fill().Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconPlus).FontSize(16).TextColor(p.muted.Mix(p.accent, k))
		})
	})
	if b.Clicked() {
		a.chooseFolder()
	}
}

// closeOthers closes the projects of the window but h, one after the
// other, each once its changes are settled, and shows h. Canceling one's
// changes stops there.
func (f *frame) closeOthers(h *desktop) {
	others := slices.DeleteFunc(slices.Clone(f.tabs), func(o *desktop) bool { return o == h })
	var next func()
	next = func() {
		others = slices.DeleteFunc(others, func(o *desktop) bool { return !slices.Contains(f.tabs, o) })
		if len(others) == 0 {
			f.activate(h)
			return
		}
		others[0].closeProjectThen(next)
	}
	next()
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

// unit is 1 for true and 0 for false, as an animation's target.
func unit(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
