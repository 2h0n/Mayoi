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
		// Undo and Redo take no keys in the menu: ⌘Z goes to the focused
		// control, the editor's history or a text field's.
		{"edit.undo", "Undo", "", a.editActive(func(d *document) { d.undo() })},
		{"edit.redo", "Redo", "", a.editActive(func(d *document) { d.redo() })},
		{"edit.find", "Find", "CmdOrCtrl+F", func() { a.openFind(false) }},
		{"edit.replace", "Replace", "Alt+CmdOrCtrl+F", func() { a.openFind(true) }},
		{"edit.findNext", "Find Next", "CmdOrCtrl+G", doc(func(d *document) { a.find.open = true; a.step(d, 1) })},
		{"edit.findPrev", "Find Previous", "Shift+CmdOrCtrl+G", doc(func(d *document) { a.find.open = true; a.step(d, -1) })},
		{"sel.comment", "Toggle Line Comment", "CmdOrCtrl+/", a.editActive(toggleComment)},
		{"sel.indent", "Indent Lines", "CmdOrCtrl+]", a.editActive(func(d *document) { indentLines(d, false) })},
		{"sel.outdent", "Outdent Lines", "CmdOrCtrl+[", a.editActive(func(d *document) { indentLines(d, true) })},
		// The arrows with Alt are the editor's own keys, which the terminal
		// takes too.
		{"sel.moveUp", "Move Line Up", "", a.editActive(func(d *document) { moveLines(d, false) })},
		{"sel.moveDown", "Move Line Down", "", a.editActive(func(d *document) { moveLines(d, true) })},
		{"sel.copyUp", "Copy Line Up", "", a.editActive(func(d *document) { copyLines(d, false) })},
		{"sel.copyDown", "Copy Line Down", "", a.editActive(func(d *document) { copyLines(d, true) })},
		{"sel.deleteLine", "Delete Line", "Shift+CmdOrCtrl+K", a.editActive(deleteLines)},
		{"sel.lineBelow", "Insert Line Below", "", a.editActive(func(d *document) { insertLine(d, false) })},
		{"sel.lineAbove", "Insert Line Above", "", a.editActive(func(d *document) { insertLine(d, true) })},
		{"sel.line", "Select Line", "CmdOrCtrl+L", a.editActive(selectLines)},
		{"go.bracket", "Go to Bracket", "Shift+CmdOrCtrl+\\", a.editActive(goToBracket)},
		{"go.quickOpen", "Quick Open…", "CmdOrCtrl+P", func() { a.openQuick("") }},
		{"go.line", "Go to Line…", "Ctrl+G", func() { a.openQuick(":") }},
		{"go.symbol", "Go to Symbol…", "Shift+CmdOrCtrl+O", func() { a.openQuick("@") }},
		{"go.nextTab", "Next Tab", "Ctrl+Tab", func() { a.cycleTab(1) }},
		{"go.prevTab", "Previous Tab", "Ctrl+Shift+Tab", func() { a.cycleTab(-1) }},
		{"view.sidebar", "Toggle Sidebar", "CmdOrCtrl+B", a.toggleSidebar},
		{"view.terminal", "Toggle Terminal", "Ctrl+`", a.togglePanel},
		{"view.newTerminal", "New Terminal", "Ctrl+Shift+`", a.newTerminal},
		{"view.zoomIn", "Bigger Text", "CmdOrCtrl+=", func() { a.zoom(1) }},
		{"view.zoomOut", "Smaller Text", "CmdOrCtrl+-", func() { a.zoom(-1) }},
		{"view.zoomReset", "Actual Size", "CmdOrCtrl+0", func() { a.zoom(0) }},
		{"view.appearance.system", "System", "", func() { a.setAppearance("") }},
		{"view.appearance.light", "Light", "", func() { a.setAppearance("light") }},
		{"view.appearance.dark", "Dark", "", func() { a.setAppearance("dark") }},
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

// setAppearance makes the window light, dark, or follow the system for
// "".
func (a *app) setAppearance(appearance string) {
	a.settings.Appearance = appearance
	a.settings.changed()
	a.host.setAppearance(appearance)
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
	radio := func(id, appearance string) *mygo.MenuItem {
		it := item(id)
		it.Type, it.Checked = mygo.MenuItemRadio, a.settings.Appearance == appearance
		return it
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			item("file.new"), item("file.openFolder"), mygo.Separator(),
			item("file.save"), item("file.saveAs"), item("file.saveAll"), mygo.Separator(),
			item("file.close"), item("file.closeFolder"),
		}},
		{Label: "Edit", Submenu: []*mygo.MenuItem{
			item("edit.undo"), item("edit.redo"), mygo.Separator(),
			{Role: mygo.RoleCut}, {Role: mygo.RoleCopy}, {Role: mygo.RolePaste}, {Role: mygo.RoleSelectAll}, mygo.Separator(),
			item("edit.find"), item("edit.replace"), item("edit.findNext"), item("edit.findPrev"),
		}},
		{Label: "Selection", Submenu: []*mygo.MenuItem{
			item("sel.line"), mygo.Separator(),
			item("sel.comment"), item("sel.indent"), item("sel.outdent"), mygo.Separator(),
			item("sel.moveUp"), item("sel.moveDown"), item("sel.copyUp"), item("sel.copyDown"), mygo.Separator(),
			item("sel.lineBelow"), item("sel.lineAbove"), item("sel.deleteLine"),
		}},
		{Label: "View", Submenu: []*mygo.MenuItem{
			item("view.sidebar"), item("view.terminal"), item("view.newTerminal"), mygo.Separator(),
			item("view.zoomIn"), item("view.zoomOut"), item("view.zoomReset"), mygo.Separator(),
			{Label: "Appearance", Submenu: []*mygo.MenuItem{
				radio("view.appearance.system", ""), radio("view.appearance.light", "light"), radio("view.appearance.dark", "dark"),
			}}, mygo.Separator(),
			{Role: mygo.RoleToggleFullScreen}, {Role: mygo.RoleToggleDevTools},
		}},
		{Label: "Go", Submenu: []*mygo.MenuItem{
			item("go.quickOpen"), item("go.symbol"), item("go.line"), item("go.bracket"), mygo.Separator(),
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
