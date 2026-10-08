package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// command is something the menus, their shortcuts and the command
// palette do.
type command struct {
	id, label, accel string
	run              func()
}

func (a *app) commands() []command {
	doc := func(fn func(d *document)) func() {
		return func() {
			if d := a.activeDoc(); d != nil {
				fn(d)
			}
		}
	}
	return []command{
		{"file.new", "New File", "CmdOrCtrl+N", a.newFile},
		{"file.openFolder", "Open Folder…", "CmdOrCtrl+O", a.chooseFolder},
		{"file.save", "Save", "CmdOrCtrl+S", a.saveActive},
		{"file.saveAs", "Save As…", "Shift+CmdOrCtrl+S", a.saveActiveAs},
		{"file.saveAll", "Save All", "Alt+CmdOrCtrl+S", a.saveAll},
		{"file.close", "Close Tab", "CmdOrCtrl+W", a.closeActive},
		{"file.closeFolder", "Close Folder", "", a.closeFolder},
		{"edit.find", "Find", "CmdOrCtrl+F", func() { a.openFind(false) }},
		{"edit.replace", "Replace", "Alt+CmdOrCtrl+F", func() { a.openFind(true) }},
		{"edit.findNext", "Find Next", "CmdOrCtrl+G", doc(func(d *document) { a.find.open = true; a.step(d, 1) })},
		{"edit.findPrev", "Find Previous", "Shift+CmdOrCtrl+G", doc(func(d *document) { a.find.open = true; a.step(d, -1) })},
		{"go.quickOpen", "Quick Open…", "CmdOrCtrl+P", func() { a.openQuick("") }},
		{"go.line", "Go to Line…", "Ctrl+G", func() { a.openQuick(":") }},
		{"go.nextTab", "Next Tab", "Ctrl+Tab", func() { a.cycleTab(1) }},
		{"go.prevTab", "Previous Tab", "Ctrl+Shift+Tab", func() { a.cycleTab(-1) }},
		{"view.sidebar", "Toggle Sidebar", "CmdOrCtrl+B", a.toggleSidebar},
		{"view.terminal", "Toggle Terminal", "Ctrl+`", a.togglePanel},
		{"view.newTerminal", "New Terminal", "Ctrl+Shift+`", a.newTerminal},
		{"view.zoomIn", "Bigger Text", "CmdOrCtrl+=", func() { a.zoom(1) }},
		{"view.zoomOut", "Smaller Text", "CmdOrCtrl+-", func() { a.zoom(-1) }},
		{"view.zoomReset", "Actual Size", "CmdOrCtrl+0", func() { a.zoom(0) }},
	}
}

func (a *app) run(id string) {
	for _, cmd := range a.commands() {
		if cmd.id == id {
			cmd.run()
			return
		}
	}
}

func (a *app) chooseFolder() {
	a.host.openFolder(func(dir string) { a.openFolder(dir) })
}

// closeActive closes the tab shown, or the window when there is none.
func (a *app) closeActive() {
	if a.active < 0 {
		a.host.closeWindow()
		return
	}
	a.closeDoc(a.active, nil)
}

func (a *app) zoom(by float32) {
	s := a.settings
	if by == 0 {
		s.FontSize = 13
	} else {
		s.FontSize = clamp(s.FontSize+by, 9, 28)
	}
	s.changed()
	for _, t := range a.terms {
		t.term.SetFont(termFont(a.settings))
	}
}

// menu is the menu bar: its items run commands, then draw a frame.
func (a *app) menu(win *mygo.Window) *mygo.Menu {
	byID := map[string]command{}
	for _, cmd := range a.commands() {
		byID[cmd.id] = cmd
	}
	item := func(id string) *mygo.MenuItem {
		cmd := byID[id]
		return &mygo.MenuItem{Label: cmd.label, Accelerator: cmd.accel, Click: func(*mygo.MenuItem, *mygo.Window) {
			a.post(cmd.run)
			win.Invalidate()
		}}
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			item("file.new"), item("file.openFolder"), mygo.Separator(),
			item("file.save"), item("file.saveAs"), item("file.saveAll"), mygo.Separator(),
			item("file.close"), item("file.closeFolder"),
		}},
		{Label: "Edit", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleUndo}, {Role: mygo.RoleRedo}, mygo.Separator(),
			{Role: mygo.RoleCut}, {Role: mygo.RoleCopy}, {Role: mygo.RolePaste}, {Role: mygo.RoleSelectAll}, mygo.Separator(),
			item("edit.find"), item("edit.replace"), item("edit.findNext"), item("edit.findPrev"),
		}},
		{Label: "View", Submenu: []*mygo.MenuItem{
			item("view.sidebar"), item("view.terminal"), item("view.newTerminal"), mygo.Separator(),
			item("view.zoomIn"), item("view.zoomOut"), item("view.zoomReset"), mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen}, {Role: mygo.RoleToggleDevTools},
		}},
		{Label: "Go", Submenu: []*mygo.MenuItem{
			item("go.quickOpen"), item("go.line"), mygo.Separator(),
			item("go.nextTab"), item("go.prevTab"),
		}},
		{Role: mygo.RoleWindowMenu},
	})
}

// shortcuts are the keys the view takes itself.
func (a *app) shortcuts(c *ui.Context) {
	// Cmd+1…9 shows a tab, as in browsers.
	for i, k := range []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9} {
		if c.Shortcut(ui.Cmd, k) {
			if k == ui.Key9 {
				i = len(a.docs) - 1
			}
			a.activate(i, true)
		}
	}
}
