package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/git"
)

// scmGroup is a group of the changes the source control view lists.
type scmGroup int

const (
	groupMerge scmGroup = iota // in conflict
	groupStaged
	groupChanges
	groupUntracked
	groupCount
)

var groupNames = [groupCount]string{"Merge Changes", "Staged Changes", "Changes", "Untracked"}

// scmRow is a row of the source control view: a group's header, or a
// file of it.
type scmRow struct {
	group  scmGroup
	header bool
	count  int // the files of a header's group
	e      git.Entry
}

// scmState is the source control view's: its list, the row chosen, and
// the groups closed.
type scmState struct {
	list      ui.ListState
	row       int
	collapsed [groupCount]bool
	rows      []scmRow // as the last frame built them
}

// groupEntries returns the files of each group, by path.
func (a *app) groupEntries() [groupCount][]git.Entry {
	var out [groupCount][]git.Entry
	st := a.git.status
	if st == nil {
		return out
	}
	for _, e := range st.Entries {
		switch {
		case e.Conflicted():
			out[groupMerge] = append(out[groupMerge], e)
		case e.Untracked():
			out[groupUntracked] = append(out[groupUntracked], e)
		default:
			if e.Staged() {
				out[groupStaged] = append(out[groupStaged], e)
			}
			if e.Unstaged() {
				out[groupChanges] = append(out[groupChanges], e)
			}
		}
	}
	for _, es := range out {
		slices.SortFunc(es, func(x, y git.Entry) int { return strings.Compare(x.Path, y.Path) })
	}
	return out
}

// scmRows returns the rows of the view: the groups that have files, and
// the files of those open.
func (a *app) scmRows() []scmRow {
	var rows []scmRow
	groups := a.groupEntries()
	for g, es := range groups {
		if len(es) == 0 {
			continue
		}
		rows = append(rows, scmRow{group: scmGroup(g), header: true, count: len(es)})
		if a.scm.collapsed[g] {
			continue
		}
		for _, e := range es {
			rows = append(rows, scmRow{group: scmGroup(g), e: e})
		}
	}
	return rows
}

// changeCount is how many files differ from HEAD, or Git does not track.
func (a *app) changeCount() int {
	if a.git.status == nil {
		return 0
	}
	return len(a.git.status.Entries)
}

// scmView is the sidebar's source control view: the commit message, and
// the files changed, staged and not.
func (a *app) scmView(c *ui.Context) {
	p := a.pal
	g := &a.git
	if g.repo == nil {
		ui.Column(c).Padding(12, 16).Gap(12).Children(func() {
			if !g.looked {
				return
			}
			ui.Text(c, "The folder is not in a Git repository.").FontSize(12.5).TextColor(p.muted)
			if ui.PrimaryButton(c, "Initialize Repository").FillWidth().Clicked() {
				a.initRepo()
			}
		})
		return
	}
	a.scmBranchRow(c)
	ui.Column(c).Padding(2, 12, 8, 12).Gap(6).Children(func() {
		if g.amend {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Amending the last commit").FontSize(11.5).TextColor(p.muted).Grow(1).SingleLine()
				b := ui.ButtonBase(c).Padding(1, 4).Radius(4).Label("Stop Amending")
				if b.Hovered() {
					b.Background(p.hover)
				}
				b.Children(func() { ui.Text(c, "Cancel").FontSize(11.5).TextColor(p.accent) })
				if b.Clicked() {
					g.amend, g.message = false, ""
				}
			})
		}
		branch := a.branch()
		hint := "Message (⌘⏎ to commit)"
		if branch != "" {
			hint = fmt.Sprintf("Message (⌘⏎ to commit on %s)", branch)
		}
		box := ui.TextArea(c.Key("commit-message"), &g.message).Placeholder(hint).FontSize(12.5).
			MinHeight(56).MaxHeight(160).Label("Commit Message")
		if box.Shortcut(ui.Cmd, ui.KeyEnter) {
			a.commit(false)
		}
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			label := "Commit"
			if g.amend {
				label = "Amend"
			}
			if ui.PrimaryButton(c, label).Grow(1).Disabled(g.busy != "").Clicked() {
				a.commit(false)
			}
			more := ui.ButtonBase(c.Key("commit-more")).Size(28, 28).Radius(6).Justify(ui.Center).AlignItems(ui.Center).
				Border(1, p.border).Label("More Actions").Tooltip("More Actions")
			if more.Hovered() {
				more.Background(p.hover)
			}
			more.Children(func() { ui.Icon(c, iconMore).FontSize(15).TextColor(p.muted) })
			more.Menu(a.scmMenu)
		})
		if g.busy != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(12, 12)
				ui.Text(c, g.busy).FontSize(11.5).TextColor(p.muted)
			})
		}
	})
	rows := a.scmRows()
	s := &a.scm
	s.rows = rows
	if len(rows) == 0 {
		if g.status != nil {
			ui.Text(c, "No changes.").FontSize(12.5).TextColor(p.faint).Padding(8, 16)
		}
		return
	}
	s.list.Selected = &s.row
	s.list.Header = func(i int) bool { return i < len(rows) && rows[i].header }
	s.list.Key = func(i int) any {
		if i >= len(rows) {
			return i
		}
		if r := rows[i]; r.header {
			return fmt.Sprintf("group:%d", r.group)
		}
		return fmt.Sprintf("%d:%s", rows[i].group, rows[i].e.Path)
	}
	s.list.Label = func(i int) string {
		if i >= len(rows) || rows[i].header {
			return ""
		}
		return filepath.Base(rows[i].e.Path)
	}
	base := c.Theme()
	quiet := *base
	quiet.Accent, quiet.AccentText, quiet.SurfaceHover = p.active, p.text, p.hover
	c.SetTheme(&quiet)
	var action func()
	out := ui.List(c.Key("scm"), &s.list, len(rows), func(i int) {
		if r := rows[i]; r.header {
			a.scmHeader(c, r, &action)
		} else {
			a.scmFile(c, r, &action)
		}
	}).Grow(1).Padding(0, 6, 8, 6)
	c.SetTheme(base)
	if action != nil {
		action()
	}
	chosen := func() (scmRow, bool) {
		if s.row >= 0 && s.row < len(rows) && !rows[s.row].header {
			return rows[s.row], true
		}
		return scmRow{}, false
	}
	if r, ok := chosen(); ok && out.Submitted() {
		a.openChange(r, true)
	}
	if r, ok := chosen(); ok && out.Shortcut(0, ui.KeySpace) {
		a.toggleStaged(r)
	}
}

// scmHeader is the row heading a group: a click opens or closes it.
func (a *app) scmHeader(c *ui.Context, r scmRow, action *func()) {
	p := a.pal
	row := ui.Row(c).Grow(1).Gap(4).AlignItems(ui.Center).Height(22).Background(p.chrome)
	hovered := row.Hovered()
	clicked := row.Clicked()
	row.Children(func() {
		icon := iconChevronDown
		if a.scm.collapsed[r.group] {
			icon = iconChevronRight
		}
		ui.Icon(c, icon).FontSize(12).TextColor(p.muted)
		ui.Text(c, strings.ToUpper(groupNames[r.group])).FontSize(11).FontWeight(600).LetterSpacing(0.4).
			TextColor(p.muted).SingleLine().Shrink(1)
		ui.Spacer(c)
		if hovered {
			paths := a.groupPaths(r.group)
			switch r.group {
			case groupStaged:
				if a.smallButton(c, iconFiles, "Open Staged Changes") {
					*action = func() { a.showChanges(diffTarget{base: diffStaged}, true) }
				}
				if a.smallButton(c, iconMinus, "Unstage All Changes") {
					*action = func() { a.unstage(paths...) }
				}
			case groupChanges, groupUntracked:
				if a.smallButton(c, iconFiles, "Open Changes") {
					*action = func() { a.showChanges(diffTarget{base: diffIndex}, true) }
				}
				if a.smallButton(c, iconUndo, "Discard All Changes…") {
					*action = func() { a.discard(a.groupEntries()[r.group]...) }
				}
				if a.smallButton(c, iconPlus, "Stage All Changes") {
					*action = func() { a.stage(paths...) }
				}
			case groupMerge:
				if a.smallButton(c, iconPlus, "Stage All Merge Changes") {
					*action = func() { a.stage(paths...) }
				}
			}
		}
		ui.Text(c, fmt.Sprint(r.count)).FontSize(10.5).TextColor(p.muted).Padding(0, 5).Radius(8).Background(p.hover)
	})
	if clicked && *action == nil {
		*action = func() { a.scm.collapsed[r.group] = !a.scm.collapsed[r.group] }
	}
}

// scmFile is the row of a changed file: a click shows its changes.
func (a *app) scmFile(c *ui.Context, r scmRow, action *func()) {
	p := a.pal
	e := r.e
	row := ui.Row(c).Grow(1).Gap(6).AlignItems(ui.Center).Height(22).Padding(0, 0, 0, 14)
	hovered := row.Hovered()
	clicked := row.Clicked()
	row.ContextMenu(func(m *ui.Menu) { a.scmFileMenu(m, r, action) })
	letter := e.Letter()
	if r.group == groupStaged {
		letter = e.X
	}
	row.Children(func() {
		name := filepath.Base(e.Path)
		icon, col := fileIcon(p, name)
		ui.Icon(c, icon).FontSize(14).TextColor(col)
		label := ui.Text(c, name).FontSize(13).SingleLine().Shrink(0).TextColor(gitColor(p, letter))
		if letter == 'D' {
			label.Strikethrough()
		}
		if dir := filepath.Dir(a.git.repo.Rel(e.Path)); dir != "." {
			ui.Text(c, dir).FontSize(11.5).TextColor(p.faint).SingleLine().Shrink(1)
		}
		ui.Spacer(c)
		if hovered {
			if letter != 'D' && a.smallButton(c, iconOpenFile, "Open File") {
				*action = func() { a.openFile(e.Path, true) }
			}
			switch r.group {
			case groupStaged:
				if a.smallButton(c, iconMinus, "Unstage Changes") {
					*action = func() { a.unstage(entryPaths(e, true)...) }
				}
			default:
				if r.group != groupMerge && a.smallButton(c, iconUndo, "Discard Changes…") {
					*action = func() { a.discard(e) }
				}
				if a.smallButton(c, iconPlus, "Stage Changes") {
					*action = func() { a.stage(e.Path) }
				}
			}
		}
		ui.Text(c, string(letter)).FontSize(11).FontWeight(600).Font(a.font.family).Padding(0, 4, 0, 2).
			TextColor(gitColor(p, letter)).Shrink(0)
	})
	if clicked && *action == nil {
		*action = func() { a.openChange(r, false) }
	}
}

// scmFileMenu is the menu of a changed file's row.
func (a *app) scmFileMenu(m *ui.Menu, r scmRow, action *func()) {
	e := r.e
	if m.Item("Open Changes").Chosen() {
		*action = func() { a.openChange(r, true) }
	}
	if m.Item("Open File").Disabled(e.Letter() == 'D' && r.group != groupStaged).Chosen() {
		*action = func() { a.openFile(e.Path, true) }
	}
	m.Separator()
	if r.group == groupStaged {
		if m.Item("Unstage Changes").Chosen() {
			*action = func() { a.unstage(entryPaths(e, true)...) }
		}
	} else {
		if m.Item("Stage Changes").Chosen() {
			*action = func() { a.stage(e.Path) }
		}
		if r.group != groupMerge && m.Item("Discard Changes…").Chosen() {
			*action = func() { a.discard(e) }
		}
	}
	m.Separator()
	if m.Item("Reveal in Explorer").Chosen() {
		*action = func() { a.showExplorer(); a.revealInTree(e.Path) }
	}
	if m.Item("Copy Path").Chosen() {
		a.services.WriteClipboard(e.Path)
	}
}

// scmMenu is the menu of the view's actions.
func (a *app) scmMenu(m *ui.Menu) {
	g := &a.git
	st := g.status
	if m.Item("Commit").Chosen() {
		a.post(func() { a.commit(false) })
	}
	if m.Item("Commit & Push").Chosen() {
		a.post(func() { a.commit(true) })
	}
	if m.Item("Amend Last Commit").Disabled(st == nil || st.Head == "").Chosen() {
		a.post(a.startAmend)
	}
	m.Separator()
	groups := a.groupEntries()
	if m.Item("Open All Changes").Disabled(len(groups[groupStaged])+len(groups[groupChanges])+len(groups[groupUntracked]) == 0).Chosen() {
		a.post(func() { a.openChangesTab(true) })
	}
	m.Separator()
	if m.Item("Stage All Changes").Disabled(len(groups[groupChanges])+len(groups[groupUntracked])+len(groups[groupMerge]) == 0).Chosen() {
		a.post(a.stageAll)
	}
	if m.Item("Unstage All Changes").Disabled(len(groups[groupStaged]) == 0).Chosen() {
		a.post(func() { a.unstage(a.groupPaths(groupStaged)...) })
	}
	if m.Item("Discard All Changes…").Disabled(len(groups[groupChanges])+len(groups[groupUntracked]) == 0).Chosen() {
		a.post(func() { a.discard(append(groups[groupChanges], groups[groupUntracked]...)...) })
	}
	m.Separator()
	if m.Item("Fetch").Disabled(g.busy != "").Chosen() {
		a.post(a.fetch)
	}
	if m.Item("Pull").Disabled(g.busy != "").Chosen() {
		a.post(a.pull)
	}
	if m.Item("Push").Disabled(g.busy != "").Chosen() {
		a.post(a.push)
	}
	m.Separator()
	if m.Item("Refresh").Chosen() {
		a.post(a.gitChanged)
	}
}

// smallButton is a small button of a row, showing an icon, which reports
// whether it was clicked.
func (a *app) smallButton(c *ui.Context, icon *ui.SVG, label string) bool {
	p := a.pal
	b := ui.ButtonBase(c.Key(label)).Size(20, 20).Radius(4).Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(label)
	col := p.muted
	if b.Hovered() {
		b.Background(p.hover)
		col = p.text
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(13).TextColor(col) })
	return b.Clicked()
}

// groupPaths returns the paths of the files of a group: of a file
// renamed and staged, where it was too, which unstaging puts back.
func (a *app) groupPaths(g scmGroup) []string {
	var out []string
	for _, e := range a.groupEntries()[g] {
		out = append(out, entryPaths(e, g == groupStaged)...)
	}
	return out
}

// entryPaths returns the path of a file, and where it was renamed from
// when staged.
func entryPaths(e git.Entry, staged bool) []string {
	if staged && e.Orig != "" {
		return []string{e.Path, e.Orig}
	}
	return []string{e.Path}
}

// chooseScmFile chooses the row of a file of a diff in the view, as the
// diff scrolls to it.
func (a *app) chooseScmFile(f *diffFile) {
	for i, r := range a.scm.rows {
		if !r.header && r.e.Path == f.path && (r.group == groupStaged) == (f.base == diffStaged) {
			a.scm.row = i
			return
		}
	}
}

// openChange shows the changes of a file of the view: those staged, or
// those not, or the file itself while it is in conflict.
func (a *app) openChange(r scmRow, focus bool) {
	switch r.group {
	case groupMerge:
		a.openFile(r.e.Path, focus)
	case groupStaged:
		a.showChanges(diffTarget{path: r.e.Path, base: diffStaged}, focus)
	default:
		a.showChanges(diffTarget{path: r.e.Path, base: diffIndex}, focus)
	}
}

// toggleStaged stages a file of the view, or unstages it when staged.
func (a *app) toggleStaged(r scmRow) {
	if r.group == groupStaged {
		a.unstage(entryPaths(r.e, true)...)
	} else {
		a.stage(r.e.Path)
	}
}

func (a *app) stage(paths ...string) {
	if len(paths) == 0 {
		return
	}
	a.gitDo("", func(r *git.Repo) error { return r.Stage(paths...) }, nil)
}

func (a *app) unstage(paths ...string) {
	if len(paths) == 0 {
		return
	}
	a.gitDo("", func(r *git.Repo) error { return r.Unstage(paths...) }, nil)
}

// stageAll stages every change, and the files Git does not track.
func (a *app) stageAll() {
	groups := a.groupEntries()
	var paths []string
	for _, g := range []scmGroup{groupMerge, groupChanges, groupUntracked} {
		for _, e := range groups[g] {
			paths = append(paths, e.Path)
		}
	}
	a.stage(paths...)
}

// discard asks, then throws away the changes of files in the work tree
// that the index does not have: a tracked file is as the index has it
// again, and one Git does not track goes to the Trash.
func (a *app) discard(es ...git.Entry) {
	if len(es) == 0 {
		return
	}
	var tracked, untracked []string
	for _, e := range es {
		if e.Untracked() {
			untracked = append(untracked, e.Path)
		} else {
			tracked = append(tracked, e.Path)
		}
	}
	title := fmt.Sprintf("Discard the changes of %d files?", len(es))
	if len(es) == 1 {
		title = fmt.Sprintf("Discard the changes of %s?", filepath.Base(es[0].Path))
		if es[0].Untracked() {
			title = fmt.Sprintf("Move %s to the Trash?", filepath.Base(es[0].Path))
		}
	}
	msg := "The changes cannot be undone."
	if len(untracked) > 0 {
		msg = "The changes cannot be undone. Files Git does not track go to the Trash."
	}
	a.confirm = &confirm{title: title, message: msg, ok: "Discard", danger: true, run: func() {
		for _, p := range untracked {
			if err := a.host.trash(p); err != nil {
				a.fail("Could not move %s to the Trash: %v", filepath.Base(p), err)
			}
		}
		if a.tree != nil && len(untracked) > 0 {
			a.tree.Refresh()
			a.indexStale = true
		}
		if len(tracked) > 0 {
			a.gitDo("", func(r *git.Repo) error { return r.Discard(tracked...) }, func() {
				// Open documents show the files as they are again, unsaved
				// changes too, as a step of undo.
				for _, p := range tracked {
					if i := a.docIndex(p); i >= 0 {
						a.reloadDoc(a.docs[i])
					}
				}
			})
		} else {
			a.gitChanged()
		}
	}}
}

// startAmend has the next commit amend the last, its message the last
// one's unless one is written.
func (a *app) startAmend() {
	g := &a.git
	g.amend = true
	if strings.TrimSpace(g.message) != "" || g.repo == nil {
		return
	}
	r := g.repo
	go func() {
		msg := r.LastMessage()
		a.post(func() {
			if g.amend && strings.TrimSpace(g.message) == "" {
				g.message = msg
			}
		})
	}()
}

// commit commits what is staged with the message written, then pushes
// when push is set. With nothing staged it offers to stage every change
// first. Changed files are saved before.
func (a *app) commit(push bool) {
	g := &a.git
	if g.repo == nil || g.busy != "" {
		return
	}
	msg := strings.TrimSpace(g.message)
	if msg == "" {
		a.fail("Write a commit message first.")
		return
	}
	for _, d := range a.docs {
		if d.path != "" && d.dirty() && g.repo.Contains(d.path) {
			a.saveDoc(d, nil)
		}
	}
	amend := g.amend
	run := func(stageAll bool) {
		a.gitDo("Committing…", func(r *git.Repo) error {
			if stageAll {
				if err := r.Stage(r.Root); err != nil {
					return err
				}
			}
			return r.Commit(msg, amend)
		}, func() {
			g.message, g.amend = "", false
			if push {
				a.push()
			}
		})
	}
	groups := a.groupEntries()
	switch {
	case amend || len(groups[groupStaged]) > 0:
		run(false)
	case len(groups[groupMerge]) > 0:
		a.fail("Resolve the merge conflicts and stage the files first.")
	case len(groups[groupChanges])+len(groups[groupUntracked]) > 0:
		a.confirm = &confirm{title: "Stage all changes and commit them?", ok: "Stage All & Commit",
			message: "There are no staged changes to commit.", run: func() { run(true) }}
	default:
		a.fail("There are no changes to commit.")
	}
}

// initRepo makes the folder a Git repository.
func (a *app) initRepo() {
	if a.root == "" {
		return
	}
	root := a.root
	go func() {
		err := git.Init(root)
		a.post(func() {
			if err != nil {
				a.gitFailed(err)
				return
			}
			if a.root == root {
				a.git = gitState{}
				a.ignoredStale = true
			}
		})
	}()
}

// showExplorer shows the files in the sidebar.
func (a *app) showExplorer() {
	a.settings.SidebarOpen, a.settings.SidebarView = true, ""
	a.settings.Changed()
}

// showSourceControl shows the changes in the sidebar.
func (a *app) showSourceControl() {
	a.settings.SidebarOpen, a.settings.SidebarView = true, "git"
	a.settings.Changed()
}

// activePaths returns the file of the tab shown, to stage or unstage.
func (a *app) activePaths() []string {
	if t := a.activeTab(); t != nil && t.path() != "" {
		return []string{t.path()}
	}
	return nil
}

// scmBranchRow is the row at the top of the source control view: the
// branch, how far it is ahead and behind, and the buttons of the remotes
// and to refresh.
func (a *app) scmBranchRow(c *ui.Context) {
	p := a.pal
	ui.Row(c).Height(28).Padding(0, 8, 0, 14).Gap(2).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconBranch).FontSize(12).TextColor(p.muted)
		ui.Text(c, a.branchLabel()).FontSize(12).TextColor(p.muted).SingleLine().Shrink(1).Grow(1).Padding(0, 0, 0, 4)
		a.scmActions(c)
	})
}

// scmActions are the buttons of the remotes, which say how far the
// branch is ahead and behind, and the one to refresh.
func (a *app) scmActions(c *ui.Context) {
	g := &a.git
	st := g.status
	pull, push := "Pull", "Push"
	if st != nil && st.Behind > 0 {
		pull = fmt.Sprintf("Pull (%d behind)", st.Behind)
	}
	if st != nil && st.Ahead > 0 {
		push = fmt.Sprintf("Push (%d ahead)", st.Ahead)
	}
	if a.iconButton(c, iconFetch, "Fetch", false).Disabled(g.busy != "").Clicked() {
		a.fetch()
	}
	if a.iconButton(c, iconPull, pull, st != nil && st.Behind > 0).Disabled(g.busy != "").Clicked() {
		a.pull()
	}
	if a.iconButton(c, iconPush, push, st != nil && st.Ahead > 0).Disabled(g.busy != "").Clicked() {
		a.push()
	}
	if a.iconButton(c, iconRefresh, "Refresh", false).Clicked() {
		a.gitChanged()
	}
}
