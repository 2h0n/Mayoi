package main

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/workspace"
)

// quickOpen is the state of the quick open box: files by fuzzy name, or
// ":n" for a line of the document shown.
type quickOpen struct {
	open    bool
	query   string
	results []workspace.Match
	sel     int
	shown   string // the query the results are of
	stale   bool
	moved   bool // the choice moved by keys, to scroll into view
}

func (a *app) openQuick(query string) {
	q := &a.quick
	q.open, q.query, q.sel, q.stale = true, query, 0, true
	a.indexStale = a.indexStale || a.index == nil
	a.refreshIndex()
}

func (a *app) closeQuick() {
	a.quick.open = false
	if d := a.activeDoc(); d != nil && a.focus != paneTerminal {
		d.vscroll.Focus()
	}
}

// gotoLine parses ":n" or ":n:c", 1-based.
func gotoLine(query string) (line, col int, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(query), ":")
	if !found {
		return 0, 0, false
	}
	parts := strings.SplitN(rest, ":", 2)
	n, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, rest == ""
	}
	if len(parts) == 2 {
		col, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	}
	return n, col, true
}

func (a *app) quickOpenView(c *ui.Context) {
	q := &a.quick
	if !q.open {
		return
	}
	p := a.pal
	t := c.Theme()
	line, col, isLine := gotoLine(q.query)
	if !isLine && (q.stale || q.shown != q.query) {
		q.results = workspace.Fuzzy(a.index, q.query, 60)
		if strings.TrimSpace(q.query) == "" {
			q.results = a.recentFirst(q.results)
		}
		q.shown, q.stale = q.query, false
		q.sel = 0
	}
	choose := func(i int) {
		if i < 0 || i >= len(q.results) {
			return
		}
		rel := a.index[q.results[i].Index]
		q.open = false
		a.openFile(a.root+"/"+rel, true)
	}
	ui.Overlay(c, func() {
		back := ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).AlignItems(ui.Center).Modal()
		back.Children(func() {
			box := ui.Column(c).Width(600).MaxWidthPercent(92).Margin(titleBarH+8, 0, 0, 0).Radius(10).
				Background(t.Surface).Border(1, p.border).Shadow(0, 16, 40, 0, ui.RGBA(0, 0, 0, 0.32)).Clip().
				Role(ui.RoleDialog).Label("Quick Open")
			box.Children(func() {
				ui.Row(c).Padding(10, 14).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
					ui.Icon(c, iconSearch).FontSize(15).TextColor(p.muted)
					in := ui.TextInputBase(c.Key("quick-input"), &q.query).Grow(1).FontSize(14).AutoFocus().
						Placeholder("Search files by name, or : to go to a line").Label("Quick Open")
					if in.Shortcut(0, ui.KeyDown) {
						q.sel, q.moved = min(q.sel+1, len(q.results)-1), true
					}
					if in.Shortcut(0, ui.KeyUp) {
						q.sel, q.moved = max(q.sel-1, 0), true
					}
					if in.Submitted() {
						if isLine {
							if line > 0 {
								q.open = false
								a.goToLine(line, col)
							}
						} else {
							choose(q.sel)
						}
					}
				})
				a.quickBody(c, isLine, line, col, choose)
				a.quickFooter(c, isLine)
			})
			if box.PressedOutside() || back.OverlayShortcut(0, ui.KeyEscape) {
				a.closeQuick()
			}
		})
	})
}

// quickBody is what quick open lists: the files found, or what going to
// a line does.
func (a *app) quickBody(c *ui.Context, isLine bool, line, col int, choose func(int)) {
	q := &a.quick
	p := a.pal
	if isLine {
		hint := "Type a line number to go to"
		if d := a.activeDoc(); d == nil {
			hint = "Open a file to go to a line"
		} else if line > 0 {
			hint = fmt.Sprintf("Go to line %d", line)
			if col > 0 {
				hint += fmt.Sprintf(", column %d", col)
			}
		} else {
			hint = fmt.Sprintf("Type a line number between 1 and %d", d.buf.LineCount())
		}
		ui.Text(c, hint).FontSize(13).TextColor(p.muted).Padding(12, 16)
		return
	}
	if len(q.results) == 0 {
		msg := "No matching files"
		switch {
		case a.root == "":
			msg = "Open a folder to search its files"
		case a.indexing && a.index == nil:
			msg = "Indexing files…"
		}
		ui.Text(c, msg).FontSize(13).TextColor(p.muted).Padding(12, 16)
		return
	}
	ui.Scroll(c.Key("quick-results")).MaxHeight(380).Padding(6).Children(func() {
		for i, m := range q.results {
			rel := a.index[m.Index]
			row := ui.ButtonBase(c.Key(rel)).Padding(5, 10).Gap(10).Radius(6).Justify(ui.Start).AlignItems(ui.Center).Label(rel)
			switch {
			case i == q.sel:
				row.Background(p.active)
			case row.Hovered():
				row.Background(p.hover)
			}
			if i == q.sel && q.moved {
				row.ScrollIntoView()
			}
			if row.Clicked() {
				choose(i)
			}
			row.Children(func() { a.quickRow(c, rel, m.Positions) })
		}
	})
	q.moved = false
}

// quickFooter shows the keys of quick open.
func (a *app) quickFooter(c *ui.Context, isLine bool) {
	p := a.pal
	ui.Row(c).Height(30).Padding(0, 14).Gap(14).AlignItems(ui.Center).Background(p.hover).
		BorderWidth(1, 0, 0, 0).BorderColor(p.border).Children(func() {
		hint := func(keys, label string) {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				a.keycaps(c, keys)
				ui.Text(c, label).FontSize(11.5).TextColor(p.muted).SingleLine()
			})
		}
		if isLine {
			hint("⏎", "go to line")
		} else {
			hint("↑↓", "navigate")
			hint("⏎", "open")
		}
		hint("esc", "close")
		ui.Spacer(c)
		if !isLine && a.root != "" {
			ui.Text(c, "Type : to go to a line").FontSize(11.5).TextColor(p.faint).SingleLine()
		}
	})
}

// quickRow shows a file of the results: its name with the characters the
// query matched in the accent color, and its folder.
func (a *app) quickRow(c *ui.Context, rel string, positions []int) {
	p := a.pal
	base := strings.LastIndexByte(rel, '/') + 1
	icon, col := fileIcon(p, rel[base:])
	ui.Icon(c, icon).FontSize(15).TextColor(col)
	// The positions are byte offsets, in order: walk them along the text.
	spans := func(s string, from int, normal ui.Color) []ui.Span {
		out := make([]ui.Span, 0, 4)
		k := 0
		for i, r := range s {
			for k < len(positions) && positions[k] < from+i {
				k++
			}
			on := k < len(positions) && positions[k] == from+i
			if n := len(out); n > 0 && (out[n-1].Weight == 650) == on {
				out[n-1].Text += string(r)
				continue
			}
			sp := ui.Span{Text: string(r), Color: normal}
			if on {
				sp.Color, sp.Weight = p.accent, 650
			}
			out = append(out, sp)
		}
		return out
	}
	ui.RichText(c, spans(rel[base:], base, p.text)...).FontSize(13).SingleLine().Shrink(0)
	if dir := path.Dir(rel); dir != "." {
		ui.RichText(c, spans(dir, 0, p.faint)...).FontSize(12).SingleLine().Shrink(1)
	}
}

// goToLine moves the caret to a line (and column) of the document shown,
// 1-based.
func (a *app) goToLine(line, col int) {
	d := a.activeDoc()
	if d == nil {
		return
	}
	snap := d.buf.Snapshot()
	l := max(0, min(line-1, snap.LineCount()-1))
	s, e := snap.LineRange(l)
	at := min(s+max(col-1, 0), e)
	d.selectRange(at, at, true)
	d.vscroll.Focus()
}

// recentFirst puts the files shown last before the others, as quick open
// lists them before anything is typed, the one shown now after the rest of
// them, as it is the one the user is leaving.
func (a *app) recentFirst(results []workspace.Match) []workspace.Match {
	// Find the recent files in the index, without a map of all of it.
	at := make(map[string]int, len(a.recent))
	for _, p := range a.recent {
		at[a.tree.Rel(p)] = -1
	}
	for i, f := range a.index {
		if j, ok := at[f]; ok && j < 0 {
			at[f] = i
		}
	}
	for f, i := range at {
		if i < 0 {
			delete(at, f)
		}
	}
	var out []workspace.Match
	seen := map[int]bool{}
	for n, p := range a.recent {
		if n == 0 && len(a.recent) > 1 {
			continue
		}
		if i, ok := at[a.tree.Rel(p)]; ok && !seen[i] {
			out = append(out, workspace.Match{Index: i})
			seen[i] = true
		}
	}
	if len(a.recent) > 1 {
		if i, ok := at[a.tree.Rel(a.recent[0])]; ok {
			out = append(out, workspace.Match{Index: i})
			seen[i] = true
		}
	}
	for _, m := range results {
		if !seen[m.Index] {
			out = append(out, m)
		}
	}
	return out[:min(len(out), 60)]
}
