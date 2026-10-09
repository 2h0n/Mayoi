package main

import (
	"path/filepath"

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
	doc := func(fn func(d *editor)) func() {
		return func() {
			if d := a.textEditor(); d != nil {
				fn(d)
			}
		}
	}
	return []command{
		{"file.new", "New File", "CmdOrCtrl+N", a.newFile},
		{"file.openFolder", "Open Folder…", "CmdOrCtrl+O", a.chooseFolder},
		{"file.openFolderWindow", "Open Folder in New Window…", "Alt+CmdOrCtrl+O", a.chooseFolderWindow},
		{"file.save", "Save", "CmdOrCtrl+S", a.saveActive},
		{"file.saveAs", "Save As…", "Shift+CmdOrCtrl+S", a.saveActiveAs},
		{"file.saveAll", "Save All", "Alt+CmdOrCtrl+S", a.saveAll},
		{"file.autoSave", "Auto Save", "", a.toggleAutoSave},
		{"file.close", "Close Tab", "CmdOrCtrl+W", a.closeActive},
		{"file.closeFolder", "Close Folder", "", a.closeFolder},
		{"file.installCLI", "Install 'mayoi' Command", "", func() { a.host.installCLI() }},
		{"file.uninstallCLI", "Uninstall 'mayoi' Command", "", func() { a.host.uninstallCLI() }},
		// Undo and Redo take no keys in the menu: ⌘Z goes to the focused
		// control, the editor's history or a text field's.
		{"edit.undo", "Undo", "", a.editActive(func(d *editor) { d.undo() })},
		{"edit.redo", "Redo", "", a.editActive(func(d *editor) { d.redo() })},
		{"edit.find", "Find", "CmdOrCtrl+F", func() { a.openFind(false) }},
		{"edit.replace", "Replace", "Alt+CmdOrCtrl+F", func() { a.openFind(true) }},
		{"edit.findNext", "Find Next", "CmdOrCtrl+G", doc(func(d *editor) { a.find.open = true; a.step(d, 1) })},
		{"edit.findPrev", "Find Previous", "Shift+CmdOrCtrl+G", doc(func(d *editor) { a.find.open = true; a.step(d, -1) })},
		{"sel.comment", "Toggle Line Comment", "CmdOrCtrl+/", a.editActive(toggleComment)},
		{"sel.indent", "Indent Lines", "CmdOrCtrl+]", a.editActive(func(d *editor) { indentLines(d, false) })},
		{"sel.outdent", "Outdent Lines", "CmdOrCtrl+[", a.editActive(func(d *editor) { indentLines(d, true) })},
		// The arrows with Alt are the editor's own keys, which the terminal
		// takes too.
		{"sel.moveUp", "Move Line Up", "", a.editActive(func(d *editor) { moveLines(d, false) })},
		{"sel.moveDown", "Move Line Down", "", a.editActive(func(d *editor) { moveLines(d, true) })},
		{"sel.copyUp", "Copy Line Up", "", a.editActive(func(d *editor) { copyLines(d, false) })},
		{"sel.copyDown", "Copy Line Down", "", a.editActive(func(d *editor) { copyLines(d, true) })},
		{"sel.deleteLine", "Delete Line", "Shift+CmdOrCtrl+K", a.editActive(deleteLines)},
		{"sel.lineBelow", "Insert Line Below", "", a.editActive(func(d *editor) { insertLine(d, false) })},
		{"sel.lineAbove", "Insert Line Above", "", a.editActive(func(d *editor) { insertLine(d, true) })},
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
		{"view.terminalTab", "New Terminal in Editor Area", "", func() { a.newTerminalTab(a.column(), a.workDir(), true) }},
		{"view.terminalMove", "Move Terminal to Editor Area or Panel", "", a.moveTerminal},
		{"view.splitRight", "Split Right", "CmdOrCtrl+\\", a.splitRight},
		{"view.nextColumn", "Focus Next Column", "Alt+CmdOrCtrl+Right", func() { a.focusColumnBy(1) }},
		{"view.prevColumn", "Focus Previous Column", "Alt+CmdOrCtrl+Left", func() { a.focusColumnBy(-1) }},
		{"view.moveTabNext", "Move Tab to Next Column", "", func() { a.moveTabBy(1) }},
		{"view.moveTabPrev", "Move Tab to Previous Column", "", func() { a.moveTabBy(-1) }},
		{"view.togglePreview", "Toggle SVG Preview", "Shift+CmdOrCtrl+V", a.togglePreview},
		// The text, or the picture shown.
		{"view.zoomIn", "Zoom In", "CmdOrCtrl+=", func() { a.zoom(1) }},
		{"view.zoomOut", "Zoom Out", "CmdOrCtrl+-", func() { a.zoom(-1) }},
		{"view.zoomReset", "Reset Zoom", "CmdOrCtrl+0", func() { a.zoom(0) }},
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

// chooseFolder asks for a folder to open in the window, or shows the
// window that has it open.
func (a *app) chooseFolder() {
	a.host.openFolder(a.switchFolder)
}

// chooseFolderWindow asks for a folder to open in a window of its own.
func (a *app) chooseFolderWindow() {
	a.host.openFolder(func(dir string) { a.host.newWindow(dir) })
}

// switchFolder opens dir in the window, unless another window has it
// open: then that one comes to the front.
func (a *app) switchFolder(dir string) {
	if !a.host.showFolder(filepath.Clean(dir)) {
		a.openFolder(dir)
	}
}

// closeActive closes the tab shown in the column that has the keyboard,
// or the window when there is none.
func (a *app) closeActive() {
	t := a.activeTab()
	if t == nil {
		a.host.closeWindow()
		return
	}
	a.closeTab(t, nil)
}

// moveTerminal moves the terminal that has the keyboard, or had it last,
// from the panel to a tab of the column, or from its tab to the panel.
func (a *app) moveTerminal() {
	switch t := a.activeTab(); {
	case a.focus == paneTerminal && a.panelTerm != nil:
		a.terminalToColumn(a.panelTerm, a.column(), a.column().active+1)
	case t != nil && t.term != nil:
		a.terminalToPanel(t.term)
	case a.panelTerm != nil:
		a.terminalToColumn(a.panelTerm, a.column(), a.column().active+1)
	}
}

// setAppearance makes the window light, dark, or follow the system for
// "".
func (a *app) setAppearance(appearance string) {
	a.settings.Appearance = appearance
	a.settings.changed()
	a.host.setAppearance(appearance)
}

// zoom makes the picture shown bigger or smaller, or fits it for 0, and
// otherwise the text.
func (a *app) zoom(by float32) {
	if v := a.activeViewer(); v != nil {
		v.zoomBy(int(by))
		return
	}
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

// menu is the menu bar, which the windows share: its items run the
// command of the window they are chosen in, by run. cli adds the items
// that install the mayoi command.
func (a *app) menu(cli bool, run func(id string, win *mygo.Window)) *mygo.Menu {
	byID := map[string]command{}
	for _, cmd := range a.commands() {
		byID[cmd.id] = cmd
	}
	item := func(id string) *mygo.MenuItem {
		cmd := byID[id]
		return &mygo.MenuItem{Label: cmd.label, Accelerator: cmd.accel, Click: func(_ *mygo.MenuItem, win *mygo.Window) {
			run(id, win)
		}}
	}
	checkbox := func(id string, checked bool) *mygo.MenuItem {
		it := item(id)
		it.Type, it.Checked = mygo.MenuItemCheckbox, checked
		return it
	}
	radio := func(id, appearance string) *mygo.MenuItem {
		it := item(id)
		it.Type, it.Checked = mygo.MenuItemRadio, a.settings.Appearance == appearance
		return it
	}
	file := []*mygo.MenuItem{
		item("file.new"), item("file.openFolder"), item("file.openFolderWindow"), mygo.Separator(),
		item("file.save"), item("file.saveAs"), item("file.saveAll"), checkbox("file.autoSave", a.settings.AutoSave), mygo.Separator(),
		item("file.close"), item("file.closeFolder"),
	}
	if cli {
		file = append(file, mygo.Separator(), item("file.installCLI"), item("file.uninstallCLI"))
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: file},
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
			item("view.sidebar"), mygo.Separator(),
			item("view.togglePreview"), mygo.Separator(),
			item("view.splitRight"), item("view.nextColumn"), item("view.prevColumn"),
			item("view.moveTabNext"), item("view.moveTabPrev"), mygo.Separator(),
			item("view.terminal"), item("view.newTerminal"), item("view.terminalTab"), item("view.terminalMove"), mygo.Separator(),
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
	// Cmd+1…9 shows a tab of the column, as in browsers.
	for i, k := range []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9} {
		if c.Shortcut(ui.Cmd, k) {
			col := a.column()
			if k == ui.Key9 {
				i = len(col.tabs) - 1
			}
			a.activate(col, i, true)
		}
	}
}
