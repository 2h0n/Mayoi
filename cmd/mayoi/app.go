package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/highlight"
	"mayoi/internal/settings"
	"mayoi/internal/workspace"
)

// host is what the app asks of the desktop: native dialogs and the trash.
// Its functions return at once and call back on the main thread.
type host interface {
	openFolder(done func(path string))
	saveAs(name string, done func(path string))
	// confirmClose asks whether to save a changed document before closing
	// it: 0 saves, 1 discards the changes, 2 keeps it open.
	confirmClose(name string, done func(choice int))
	trash(path string) error
	reveal(path string)
	setTitle(title string)
	closeWindow()
	// setAppearance makes the window light, dark, or follow the system
	// for "".
	setAppearance(appearance string)
	// showFolder brings the other window that has dir open to the front,
	// and reports whether there is one.
	showFolder(dir string) bool
	// newWindow opens dir in a window of its own, or brings the one that
	// has it open to the front.
	newWindow(dir string)
	// recentChanged shows the recent folders anew, in the menus, once
	// they change.
	recentChanged()
	// installCLI and uninstallCLI put the mayoi command on the shell's
	// path, and take it away, saying how it went.
	installCLI()
	uninstallCLI()
}

// app is the state of a window of Mayoi.
type app struct {
	host     host
	settings *settings.Settings
	// window is what the settings keep of this window.
	window *settings.Window

	pal      *palette
	font     codeFont
	services ui.Services

	// queue holds the functions other goroutines post for the main thread,
	// which the next frame runs.
	mu          sync.Mutex
	queue       []func()
	hasServices bool

	root    string
	tree    *workspace.Tree
	files   ui.OutlineState[string]
	fileRow int
	// treeSynced is the document the tree last followed, selectInTree the
	// file it should choose once its row shows.
	treeSynced, selectInTree string
	git                      gitState
	scm                      scmState

	// ignored is what Git ignores in the workspace, which the tree shows
	// faint.
	ignored                map[string]bool
	ignoring, ignoredStale bool

	index      []string
	recent     []string // the files shown last, latest first
	indexing   bool
	indexStale bool

	// docs are the documents open, each in a tab or several; cols the
	// columns of the editor area, and col the one that has the keyboard,
	// or had it last, which commands are for.
	docs     []*document
	cols     []*column
	col      int
	nextID   int
	nextTab  int
	nextCol  int
	untitled int
	// closed are the tabs closed, the last last, which ⇧⌘T opens again.
	closed []closedTab
	// shownDoc is the document the column with the keyboard showed last
	// frame, which saves itself once it shows another.
	shownDoc *document

	// terms are the terminals, those of the panel and those in tabs, and
	// panelTerm the one the panel shows.
	terms     []*session
	panelTerm *session
	nextTerm  int

	panelMax    bool // the panel takes the editor's room
	welcomeView ui.Handle
	colsView    ui.Handle // the columns, whose width their weights share
	// frame counts the frames, and frameAt is when the last started: a
	// frame may build the view several times.
	frame   int
	frameAt time.Time
	// dragging is set while a tab is dragged, as the frame before found.
	dragging, dragNow bool

	quick    quickOpen
	branches branchPicker // the box that switches branches
	recents  recentPicker // the box of the recent folders
	find     findState
	prompt   *prompt
	confirm  *confirm

	errors  []string // to show as toasts
	notes   []note   // what went well, to show as toasts
	title   string
	focus   pane
	started bool
}

type pane int

const (
	paneNone pane = iota
	paneFiles
	paneEditor   // a column: its editor, or its terminal
	paneTerminal // the panel
)

func newApp(h host, s *settings.Settings, w *settings.Window) *app {
	a := &app{host: h, settings: s, window: w, fileRow: -1}
	a.scm.row = -1
	a.files.List.Selected = &a.fileRow
	a.cols = []*column{a.newColumn()}
	return a
}

// post runs fn on the main thread before the next frame. Safe from any
// goroutine.
func (a *app) post(fn func()) {
	a.mu.Lock()
	a.queue = append(a.queue, fn)
	sv := a.services
	a.mu.Unlock()
	sv.Invalidate()
}

// invalidate draws a new frame, as when the highlighter has new tokens.
// Safe from any goroutine.
func (a *app) invalidate() {
	a.mu.Lock()
	sv := a.services
	a.mu.Unlock()
	sv.Invalidate()
}

// drain runs the functions posted since the last frame.
func (a *app) drain() {
	for {
		a.mu.Lock()
		q := a.queue
		a.queue = nil
		a.mu.Unlock()
		if len(q) == 0 {
			return
		}
		for _, fn := range q {
			fn()
		}
	}
}

func (a *app) fail(format string, args ...any) {
	a.errors = append(a.errors, fmt.Sprintf(format, args...))
}

// note is a toast of what a command did, with a button of label that runs
// action, as Undo, when action is set.
type note struct {
	message, label string
	action         func()
}

// tell shows a toast of what a command did.
func (a *app) tell(n note) { a.notes = append(a.notes, n) }

// start restores the window's last session: its folder and the files it
// had open.
func (a *app) start() {
	w := a.window
	if w.Folder != "" {
		if info, err := os.Stat(w.Folder); err == nil && info.IsDir() {
			a.openFolder(w.Folder)
		}
	}
	a.restoreTabs()
	if w.TerminalOpen && w.PanelOpen && a.root != "" {
		a.newTerminal()
	}
}

// openFolder makes dir the workspace.
func (a *app) openFolder(dir string) {
	dir = filepath.Clean(dir)
	if a.tree != nil {
		a.tree.Close()
	}
	if a.root != dir {
		a.closed = nil
	}
	a.root = dir
	a.tree = workspace.NewTree(dir, a.post)
	a.tree.OnChange = a.diskChanged
	a.tree.Load(dir)
	a.files = ui.OutlineState[string]{}
	a.fileRow = -1
	a.files.List.Selected = &a.fileRow
	a.index, a.indexStale = nil, true
	a.ignored, a.ignoredStale = nil, true
	a.git = gitState{}
	a.window.Folder = dir
	a.settings.AddRecent(dir)
	a.host.recentChanged()
	for _, d := range a.docs {
		a.tree.Watch(d.path)
	}
	for _, v := range a.imageViewers() {
		a.tree.Watch(v.path)
	}
	a.settings.Changed()
}

// folder is the window's folder, also before the window starts and
// opens it.
func (a *app) folder() string {
	if a.root != "" || a.window.Folder == "" {
		return a.root
	}
	return filepath.Clean(a.window.Folder)
}

// blank reports whether the window shows nothing: no folder, no tabs and
// no terminal, also before it starts.
func (a *app) blank() bool {
	if a.folder() != "" || len(a.docs) > 0 || len(a.terms) > 0 {
		return false
	}
	for _, cs := range a.window.Columns {
		if len(cs.Tabs) > 0 {
			return false
		}
	}
	return true
}

// closeFolder closes the workspace, once its documents are closed.
func (a *app) closeFolder() {
	a.closeAll(func() {
		if a.tree != nil {
			a.tree.Close()
		}
		a.root, a.tree, a.index, a.git = "", nil, nil, gitState{}
		a.closed = nil
		a.window.Folder = ""
		a.settings.Changed()
	})
}

// saveDoc saves a document, asking where an untitled one goes.
func (a *app) saveDoc(d *document, then func()) {
	if d.path == "" {
		a.host.saveAs(d.name(), func(path string) {
			if path == "" {
				return
			}
			d.path = path
			d.hl.Close()
			d.hl = highlight.New(highlight.Lexer(path), a.invalidate)
			a.saveDoc(d, then)
			if a.tree != nil {
				a.tree.Watch(path)
			}
		})
		return
	}
	if err := d.save(); err != nil {
		a.fail("Could not save %s: %v", d.name(), err)
		return
	}
	d.diskGone, d.autoSave.failed = false, false
	a.git.stale = true
	a.rememberTabs()
	if then != nil {
		then()
	}
}

func (a *app) saveActive() {
	if d := a.activeEditor(); d != nil {
		a.saveDoc(d.document, nil)
	}
}

func (a *app) saveActiveAs() {
	ed := a.activeEditor()
	if ed == nil {
		return
	}
	d := ed.document
	a.host.saveAs(d.name(), func(path string) {
		if path == "" {
			return
		}
		d.path = path
		d.hl.Close()
		d.hl = highlight.New(highlight.Lexer(path), a.invalidate)
		a.saveDoc(d, nil)
	})
}

func (a *app) saveAll() {
	for _, d := range a.docs {
		if d.dirty() {
			a.saveDoc(d, nil)
		}
	}
}

// diskChanged follows files changed on disk: an open document without
// changes of its own shows the file's new text.
func (a *app) diskChanged(changed, removed []string) {
	a.indexStale, a.ignoredStale = true, true
	a.diffsOnDisk(append(changed, removed...))
	// An image shows the file's new picture, or that it is gone.
	for _, v := range a.imageViewers() {
		switch {
		case slices.Contains(changed, v.path):
			v.diskGone = false
			a.loadImage(v)
		case slices.Contains(removed, v.path):
			if _, err := os.Stat(v.path); err != nil {
				v.diskGone = true
			}
		}
	}
	for _, p := range changed {
		i := a.docIndex(p)
		if i < 0 {
			continue
		}
		d := a.docs[i]
		d.diskGone = false
		if d.dirty() {
			continue
		}
		a.reloadDoc(d)
	}
	for _, p := range removed {
		if i := a.docIndex(p); i >= 0 {
			if _, err := os.Stat(p); err != nil {
				a.docs[i].diskGone = true
			}
		}
	}
	a.gitChanged()
}

// reloadDoc makes a document's text the file's on disk, as a step of
// undo, as in other editors.
func (a *app) reloadDoc(d *document) {
	data, err := os.ReadFile(d.path)
	if err != nil || isBinary(data) {
		return
	}
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if string(data) != d.buf.String() {
		if ed := d.main(); ed != nil {
			ed.edit(ed.sel, textEdit{0, d.buf.Len(), string(data)})
		}
		d.measureWidest()
	}
	d.markSaved()
}

// imageViewers returns the viewers of the image files open in tabs.
func (a *app) imageViewers() []*viewer {
	var out []*viewer
	for _, col := range a.cols {
		for _, t := range col.tabs {
			if t.view != nil {
				out = append(out, t.view)
			}
		}
	}
	return out
}

// refreshIndex lists the workspace's files again in the background, for
// quick open, when they may have changed.
func (a *app) refreshIndex() {
	if a.root == "" || a.indexing || !a.indexStale {
		return
	}
	a.indexing, a.indexStale = true, false
	root := a.root
	go func() {
		files := workspace.Index(root)
		a.post(func() {
			a.indexing = false
			if a.root == root {
				a.index = files
				a.quick.stale = true
			}
		})
	}()
}

// refreshIgnored asks Git again in the background what it ignores, when
// the files may have changed.
func (a *app) refreshIgnored() {
	if a.root == "" || a.ignoring || !a.ignoredStale {
		return
	}
	a.ignoring, a.ignoredStale = true, false
	root := a.root
	go func() {
		ignored := workspace.Ignored(root)
		a.post(func() {
			a.ignoring = false
			if a.root == root {
				a.ignored = ignored
			}
		})
	}()
}

// isIgnored reports whether Git ignores path, or a folder above it.
func (a *app) isIgnored(path string) bool {
	if len(a.ignored) == 0 {
		return false
	}
	for p := path; len(p) > len(a.root); p = filepath.Dir(p) {
		if a.ignored[p] {
			return true
		}
	}
	return false
}

// windowTitle is the title the window shows.
func (a *app) windowTitle() string {
	parts := []string{}
	if t := a.activeTab(); t != nil {
		name := t.name()
		if t.ed != nil && t.ed.dirty() {
			name = "● " + name
		}
		parts = append(parts, name)
	}
	if a.root != "" {
		parts = append(parts, filepath.Base(a.root))
	}
	if len(parts) == 0 {
		return "Mayoi"
	}
	return strings.Join(parts, " — ")
}

// tick runs once a frame, after the posted functions.
func (a *app) tick() {
	a.refreshIgnored()
	a.refreshGit()
	if t := a.windowTitle(); t != a.title {
		a.title = t
		a.host.setTitle(t)
	}
}
