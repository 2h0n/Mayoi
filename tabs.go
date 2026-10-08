package main

import (
	"github.com/egoist/mygo/ui"
)

// tabStrip shows a tab for each open document: a click shows it, the
// cross closes it, and a dot marks unsaved changes.
func (a *app) tabStrip(c *ui.Context) {
	p := a.pal
	closeAt := -1
	ui.Row(c).Height(tabStripH).Background(p.chrome).Children(func() {
		ui.ScrollHorizontal(c.Key("tabs")).Grow(1).Height(tabStripH).ScrollbarInsets(0, 0, 0, 0).Children(func() {
			ui.Row(c).Height(tabStripH).AlignItems(ui.Stretch).Children(func() {
				for i, d := range a.docs {
					active := i == a.active
					tab := ui.ButtonBase(c.Key(d.id)).Padding(0, 6, 0, 12).Gap(7).AlignItems(ui.Center).
						BorderWidth(0, 1, 0, 0).BorderColor(p.border).Label(d.name()).Tooltip(a.relOrPath(d.path))
					hovered := tab.Hovered()
					switch {
					case active:
						tab.Background(p.editor)
					case hovered:
						tab.Background(mix(p.chrome, p.editor, 0.5))
					}
					if tab.Pressed() && !active {
						a.activate(i, true)
					}
					if active && a.tabRevealed != d.id {
						// Bring the tab shown into view, once as it is chosen.
						tab.ScrollIntoView()
						a.tabRevealed = d.id
					}
					tab.ContextMenu(func(m *ui.Menu) {
						if m.Item("Close").Chosen() {
							closeAt = i
						}
						if m.Item("Close Others").Disabled(len(a.docs) < 2).Chosen() {
							a.post(func() { a.closeOthers(d) })
						}
						if m.Item("Close All").Chosen() {
							a.post(func() { a.closeAll(func() {}) })
						}
						m.Separator()
						if m.Item("Reveal in Explorer").Disabled(d.path == "" || a.root == "").Chosen() {
							a.post(func() { a.revealInTree(d.path); a.settings.SidebarOpen = true })
						}
						if m.Item("Copy Path").Disabled(d.path == "").Chosen() {
							a.services.WriteClipboard(d.path)
						}
					})
					tab.Children(func() {
						if active {
							// The accent line along the top of the tab shown.
							ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(1.5).Background(p.accent)
						}
						icon, col := fileIcon(p, d.name())
						ui.Icon(c, icon).FontSize(14).TextColor(col)
						name := ui.Text(c, d.name()).FontSize(12.5).SingleLine()
						switch {
						case d.diskGone:
							name.TextColor(p.danger).Strikethrough()
						case active:
							name.TextColor(p.text)
						default:
							name.TextColor(p.muted)
						}
						// The dot of unsaved changes turns into the cross under the
						// pointer.
						x := ui.ButtonBase(c).Size(18, 18).Radius(4).Justify(ui.Center).AlignItems(ui.Center).Label("Close " + d.name())
						if x.Hovered() {
							x.Background(p.hover)
						}
						x.Children(func() {
							switch {
							case d.dirty() && !x.Hovered():
								ui.Box(c).Size(8, 8).Radius(4).Background(p.text.Alpha(0.7))
							case active || hovered || x.Hovered():
								ui.Icon(c, iconX).FontSize(13).TextColor(p.muted)
							}
						})
						if x.Clicked() {
							closeAt = i
						}
					})
				}
			})
		})
	})
	if closeAt >= 0 {
		a.closeDoc(closeAt, nil)
	}
}

func (a *app) closeOthers(keep *document) {
	for i := len(a.docs) - 1; i >= 0; i-- {
		if a.docs[i] != keep && !a.docs[i].dirty() {
			a.closeDoc(i, nil)
		}
	}
	for i, d := range a.docs {
		if d == keep {
			a.activate(i, true)
		}
	}
}
