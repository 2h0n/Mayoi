package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// sidebar is the explorer: the workspace's files in a tree.
func (a *app) sidebar(c *ui.Context) {
	p := a.pal
	sb := ui.Column(c).Fill().Background(p.chrome)
	hovered := sb.Hovered()
	sb.Children(func() {
		ui.Row(c).Height(34).Padding(0, 6, 0, 16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "EXPLORER").FontSize(11).FontWeight(600).LetterSpacing(0.7).TextColor(p.muted).Grow(1)
			if a.root == "" || !hovered && !a.files.List.FocusWithin(c) {
				return
			}
			if a.iconButton(c, iconFilePlus, "New File…", false).Clicked() {
				a.askNewEntry(a.targetDir(), false)
			}
			if a.iconButton(c, iconFolderPlus, "New Folder…", false).Clicked() {
				a.askNewEntry(a.targetDir(), true)
			}
			if a.iconButton(c, iconRefresh, "Refresh Explorer", false).Clicked() {
				a.tree.Refresh()
				a.indexStale = true
			}
			if a.iconButton(c, iconCollapse, "Collapse Folders", false).Clicked() {
				a.files.Open.Clear()
			}
		})
		if a.root == "" {
			a.noFolder(c)
			return
		}
		ui.Row(c).Height(24).Padding(0, 16).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, strings.ToUpper(filepath.Base(a.root))).FontSize(11).FontWeight(700).LetterSpacing(0.4).
				TextColor(p.text).SingleLine()
		})
		a.fileTree(c)
	})
}

func (a *app) noFolder(c *ui.Context) {
	p := a.pal
	ui.Column(c).Padding(12, 16).Gap(12).Children(func() {
		ui.Text(c, "You have not yet opened a folder.").FontSize(12.5).TextColor(p.muted)
		if ui.PrimaryButton(c, "Open Folder").FillWidth().Clicked() {
			a.chooseFolder()
		}
	})
}

// fileTree shows the files: a click on a folder opens or closes it, on a
// file shows it in the editor.
func (a *app) fileTree(c *ui.Context) {
	p := a.pal
	t := a.tree
	t.Load(a.root)
	children := func(path string) []string {
		kids := t.Children(path)
		if kids != nil && a.files.Open.Has(path) {
			t.Load(path)
		}
		return kids
	}
	activePath := ""
	if d := a.activeDoc(); d != nil {
		activePath = d.path
	}
	// Follow the document shown: open the folders above it and choose it.
	if activePath != a.treeSynced {
		a.treeSynced = activePath
		a.revealInTree(activePath)
		a.selectInTree = activePath
	}
	if a.selectInTree != "" {
		for i := range a.files.Rows() {
			if a.files.Item(i) == a.selectInTree {
				a.fileRow = i
				a.selectInTree = ""
			}
		}
	}
	// Rows show chosen in a tint of the accent, not in the accent itself,
	// which would hide the icons: the list takes the colors from the theme
	// as it builds.
	base := c.Theme()
	quiet := *base
	quiet.Accent, quiet.AccentText, quiet.SurfaceHover = p.active, p.text, p.hover
	c.SetTheme(&quiet)
	var clicked string
	var action func()
	out := ui.Outline(c.Key("files"), &a.files, t.Children(a.root), children, func(path string) {
		isDir := t.IsDir(path)
		name := filepath.Base(path)
		row := ui.Row(c).Grow(1).Gap(6).AlignItems(ui.Center).Height(17)
		if row.Clicked() {
			clicked = path
		}
		row.ContextMenu(func(m *ui.Menu) {
			dir := path
			if !isDir {
				dir = filepath.Dir(path)
			}
			if m.Item("New File…").Chosen() {
				action = func() { a.askNewEntry(dir, false) }
			}
			if m.Item("New Folder…").Chosen() {
				action = func() { a.askNewEntry(dir, true) }
			}
			m.Separator()
			if m.Item("Reveal in Finder").Chosen() {
				action = func() { a.host.reveal(path) }
			}
			if !isDir && m.Item("Open in Terminal").Chosen() {
				action = func() { a.newTerminalIn(dir) }
			}
			if isDir && m.Item("Open in Terminal").Chosen() {
				action = func() { a.newTerminalIn(path) }
			}
			m.Separator()
			if m.Item("Copy Path").Chosen() {
				action = func() { a.services.WriteClipboard(path) }
			}
			if m.Item("Copy Relative Path").Chosen() {
				action = func() { a.services.WriteClipboard(t.Rel(path)) }
			}
			m.Separator()
			if m.Item("Rename…").Chosen() {
				action = func() { a.askRename(path) }
			}
			if m.Item("Move to Trash").Chosen() {
				action = func() { a.trash(path) }
			}
		})
		row.Children(func() {
			ignored := a.isIgnored(path)
			icon, col := iconFolder, p.accent
			switch {
			case isDir && a.files.Open.Has(path):
				icon = iconFolderOpen
			case !isDir:
				icon, col = fileIcon(p, name)
			}
			if ignored {
				col = col.Alpha(0.5)
			}
			ui.Icon(c, icon).FontSize(14).TextColor(col)
			label := ui.Text(c, name).FontSize(13).SingleLine().Shrink(1).TextColor(p.text)
			switch i := a.docIndex(path); {
			case i >= 0 && a.docs[i].dirty():
				label.TextColor(p.modified)
			case ignored:
				label.TextColor(p.faint)
			case path != activePath && !isDir:
				label.TextColor(mix(p.text, p.muted, 0.35))
			}
		})
	}).Grow(1).Padding(0, 6, 8, 6).Gap(0)
	c.SetTheme(base)
	if action != nil {
		action()
	}
	if out.Submitted() && a.fileRow >= 0 && a.fileRow < a.files.Rows() {
		// Enter, or a double click: keep the file open with the focus.
		if path := a.files.Item(a.fileRow); !t.IsDir(path) {
			a.openFile(path, true)
		}
	}
	if clicked != "" {
		for i := range a.files.Rows() {
			if a.files.Item(i) == clicked {
				a.fileRow = i
			}
		}
		if t.IsDir(clicked) {
			if a.files.Open.Has(clicked) {
				a.files.Open.Remove(clicked)
			} else {
				a.files.Open.Add(clicked)
			}
		} else {
			a.openFile(clicked, false)
		}
	}
}

// targetDir is where new files go: the folder chosen in the tree, or the
// chosen file's, or the workspace's.
func (a *app) targetDir() string {
	if a.fileRow >= 0 && a.fileRow < a.files.Rows() {
		p := a.files.Item(a.fileRow)
		if a.tree.IsDir(p) {
			return p
		}
		return filepath.Dir(p)
	}
	return a.root
}

// revealInTree opens the folders above a file and chooses it.
func (a *app) revealInTree(path string) {
	if a.tree == nil || !strings.HasPrefix(path, a.root+string(filepath.Separator)) {
		return
	}
	for d := filepath.Dir(path); d != a.root && len(d) > len(a.root); d = filepath.Dir(d) {
		a.files.Open.Add(d)
		a.tree.Load(d)
	}
}

func (a *app) askNewEntry(dir string, folder bool) {
	title := "New File"
	if folder {
		title = "New Folder"
	}
	a.prompt = &prompt{title: title, hint: "in " + a.relOrPath(dir), done: func(name string) string {
		name = strings.TrimSpace(name)
		if name == "" {
			return "Enter a name."
		}
		path := filepath.Join(dir, filepath.FromSlash(name))
		if _, err := os.Lstat(path); err == nil {
			return "A file or folder with that name exists."
		}
		if folder {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err.Error()
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err.Error()
			}
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				return err.Error()
			}
		}
		a.files.Open.Add(dir)
		a.tree.Refresh()
		a.indexStale = true
		if !folder {
			a.openFile(path, true)
		}
		return ""
	}}
}

func (a *app) askRename(path string) {
	a.prompt = &prompt{title: "Rename", hint: a.relOrPath(path), value: filepath.Base(path), done: func(name string) string {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsRune(name, filepath.Separator) {
			return "Enter a name without slashes."
		}
		to := filepath.Join(filepath.Dir(path), name)
		if to == path {
			return ""
		}
		if _, err := os.Lstat(to); err == nil {
			return "A file or folder with that name exists."
		}
		if err := os.Rename(path, to); err != nil {
			return err.Error()
		}
		// Open documents follow the files renamed.
		for _, d := range a.docs {
			if d.path == path || strings.HasPrefix(d.path, path+string(filepath.Separator)) {
				d.path = to + d.path[len(path):]
				d.diskGone = false
			}
		}
		a.tree.Refresh()
		a.indexStale = true
		a.rememberTabs()
		return ""
	}}
}

func (a *app) trash(path string) {
	if err := a.host.trash(path); err != nil {
		a.fail("Could not move %s to the Trash: %v", filepath.Base(path), err)
		return
	}
	a.tree.Refresh()
	a.indexStale = true
}

func (a *app) relOrPath(path string) string {
	if a.root != "" {
		if path == a.root {
			return filepath.Base(a.root)
		}
		if strings.HasPrefix(path, a.root+string(filepath.Separator)) {
			return filepath.ToSlash(path[len(a.root)+1:])
		}
	}
	return shortenHome(path)
}

// mix blends from toward to by t.
func mix(from, to ui.Color, t float32) ui.Color { return from.Mix(to, t) }
