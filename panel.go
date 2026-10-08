package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// session is a terminal of the panel.
type session struct {
	id    int
	term  *terminal.Terminal
	title string
	dir   string
}

// termTheme is the terminal's colors for a palette: its background the
// editor's, so that the panel and the editor are of a piece.
func termTheme(p *palette) *terminal.Theme {
	th := &terminal.Theme{
		Foreground: p.text, Background: p.panel,
		Cursor: p.accent, Selection: p.selection,
	}
	if p.dark {
		th.Palette = [16]ui.Color{
			ui.Hex("#2a2a33"), ui.Hex("#f0718b"), ui.Hex("#a9d48f"), ui.Hex("#e8c482"),
			ui.Hex("#8fb4ff"), ui.Hex("#b99cf7"), ui.Hex("#7dcfe0"), ui.Hex("#d6d6de"),
			ui.Hex("#5a5a66"), ui.Hex("#ff8fa5"), ui.Hex("#bfe6a5"), ui.Hex("#f5d699"),
			ui.Hex("#a9c6ff"), ui.Hex("#cdb5fa"), ui.Hex("#9ee0ee"), ui.Hex("#ffffff"),
		}
	} else {
		th.Palette = [16]ui.Color{
			ui.Hex("#2a2a33"), ui.Hex("#c8264f"), ui.Hex("#3d8a28"), ui.Hex("#a86a0c"),
			ui.Hex("#2a5bd7"), ui.Hex("#7c3bd6"), ui.Hex("#0b7c99"), ui.Hex("#a3a3ae"),
			ui.Hex("#6c6c79"), ui.Hex("#e0405f"), ui.Hex("#4fa63a"), ui.Hex("#c4862a"),
			ui.Hex("#4a78e8"), ui.Hex("#9558e8"), ui.Hex("#1a9cbd"), ui.Hex("#d6d6de"),
		}
	}
	return th
}

// newTerminal starts a terminal in the workspace, or the home directory.
func (a *app) newTerminal() {
	dir := a.root
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	a.newTerminalIn(dir)
}

// newTerminalIn starts a terminal in dir and shows it.
func (a *app) newTerminalIn(dir string) {
	a.nextTerm++
	s := &session{id: a.nextTerm, dir: dir, title: shellName()}
	term, err := terminal.New(terminal.Options{
		Dir:       dir,
		Font:      termFont(a.settings),
		Theme:     termTheme(palettes[0]),
		DarkTheme: termTheme(palettes[1]),
		Cursor:    terminal.CursorBar,
		OnTitle: func(title string) {
			a.post(func() { s.title = title })
		},
		OnExit: func(int) {
			a.post(func() { a.removeTerminal(s) })
		},
	})
	if err != nil {
		a.fail("Could not start a terminal: %v", err)
		return
	}
	s.term = term
	a.terms = append(a.terms, s)
	a.term = len(a.terms) - 1
	a.settings.PanelOpen, a.settings.TerminalOpen = true, true
	a.settings.changed()
	a.focusTerminal()
}

func shellName() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return filepath.Base(sh)
	}
	return "shell"
}

// removeTerminal drops a terminal whose program ended, or that the user
// killed, hiding the panel after the last.
func (a *app) removeTerminal(s *session) {
	i := slices.Index(a.terms, s)
	if i < 0 {
		return
	}
	s.term.Close()
	a.terms = slices.Delete(a.terms, i, i+1)
	if a.term >= len(a.terms) {
		a.term = len(a.terms) - 1
	}
	if len(a.terms) == 0 {
		a.settings.PanelOpen, a.settings.TerminalOpen = false, false
		a.settings.changed()
		if d := a.activeDoc(); d != nil {
			d.vscroll.Focus()
		}
	} else {
		a.focusTerminal()
	}
}

func (a *app) activeTerminal() *session {
	if a.term < 0 || a.term >= len(a.terms) {
		return nil
	}
	return a.terms[a.term]
}

func (a *app) focusTerminal() {
	a.termView.Focus()
	a.focus = paneTerminal
}

// closeTerminals hangs up every terminal, as the window closes.
func (a *app) closeTerminals() {
	for _, s := range a.terms {
		s.term.Close()
	}
	a.terms = nil
}

// panel is the terminal panel: a header of the terminals and its tools,
// and the terminal shown.
func (a *app) panel(c *ui.Context) {
	p := a.pal
	ui.Column(c).Fill().Background(p.panel).Children(func() {
		ui.Row(c).Height(panelHeadH).Padding(0, 8, 0, 14).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Height(panelHeadH).Justify(ui.Center).Children(func() {
				ui.Text(c, "TERMINAL").FontSize(11).FontWeight(600).LetterSpacing(0.7).TextColor(p.text)
				ui.Box(c).Absolute().Bottom(0).Left(0).Right(0).Height(1.5).Background(p.accent)
			})
			ui.Box(c).Width(10)
			// The terminals, as chips, when there are several.
			ui.ScrollHorizontal(c.Key("terms")).Grow(1).Children(func() {
				ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
					if len(a.terms) < 2 {
						return
					}
					for i, s := range a.terms {
						chip := ui.ButtonBase(c.Key(s.id)).Padding(3, 8).Gap(6).Radius(5).AlignItems(ui.Center).Label(s.title)
						on := i == a.term
						if on {
							chip.Background(p.active)
						} else if chip.Hovered() {
							chip.Background(p.hover)
						}
						chip.Children(func() {
							ui.Icon(c, iconTerminal).FontSize(12).TextColor(p.muted)
							col := p.muted
							if on {
								col = p.text
							}
							ui.Text(c, fmt.Sprintf("%d: %s", i+1, s.title)).FontSize(12).TextColor(col).SingleLine().MaxWidth(180)
						})
						if chip.Clicked() {
							a.term = i
							a.focusTerminal()
						}
					}
				})
			})
			if a.iconButton(c, iconPlus, "New Terminal (⌃⇧`)", false).Clicked() {
				a.newTerminal()
			}
			if s := a.activeTerminal(); s != nil && a.iconButton(c, iconTrash, "Kill Terminal", false).Clicked() {
				a.removeTerminal(s)
			}
			icon, label := iconMaximize, "Maximize Panel"
			if a.panelMax {
				icon, label = iconMinimize, "Restore Panel"
			}
			if a.iconButton(c, icon, label, false).Clicked() {
				a.panelMax = !a.panelMax
			}
			if a.iconButton(c, iconX, "Hide Panel", false).Clicked() {
				a.settings.PanelOpen = false
				a.settings.changed()
				if d := a.activeDoc(); d != nil {
					d.vscroll.Focus()
				}
			}
		})
		ui.Box(c).Grow(1).Padding(2, 4, 4, 14).Children(func() {
			s := a.activeTerminal()
			if s == nil {
				ui.Column(c).Fill().Center().Children(func() {
					if ui.Button(c, "New Terminal").Clicked() {
						a.newTerminal()
					}
				})
				return
			}
			terminal.View(c.Key(s.id), s.term).Fill().Bind(&a.termView)
		})
	})
}

func termFont(s *settings) terminal.Font {
	return terminal.Font{Family: s.FontFamily, Size: max(s.FontSize-0.5, 9)}
}
