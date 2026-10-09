package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/highlight"
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
}

// app is the state of Mayoi's window.
type app struct {
	host     host
	settings *settings

	pal      *palette
	font     codeFont
	ranges   []ui.TextRange
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
	branch                   string

	// ignored is what Git ignores in the workspace, which the tree shows
	// faint.
	ignored                map[string]bool
	ignoring, ignoredStale bool

	index      []string
	recent     []string // the files shown last, latest first
	indexing   bool
	indexStale bool

	docs        []*document
	active      int // the document shown, -1 for none
	nextID      int
	tabRevealed int
	untitled    int

	terms    []*session
	term     int
	nextTerm int

	panelMax    bool // the panel takes the editor's room
	termView    ui.Handle
	welcomeView ui.Handle

	quick  quickOpen
	find   findState
	prompt *prompt

	errors  []string // to show as toasts
	title   string
	focus   pane
	started bool
}

type pane int

const (
	paneNone pane = iota
	paneFiles
	paneEditor
	paneTerminal
)

func newApp(h host, s *settings) *app {
	a := &app{host: h, settings: s, active: -1, fileRow: -1}
	a.files.List.Selected = &a.fileRow
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

// start restores the last session: its folder and the files it had open.
func (a *app) start() {
	s := a.settings
	if s.Folder != "" {
		if info, err := os.Stat(s.Folder); err == nil && info.IsDir() {
			a.openFolder(s.Folder)
		}
	}
	// Opening each file makes it the active one: keep the one to show.
	activeFile := s.ActiveFile
	for _, p := range s.OpenFiles {
		if _, err := os.Stat(p); err == nil {
			a.openFile(p, false)
		}
	}
	if i := a.docIndex(activeFile); i >= 0 {
		a.activate(i, false)
	}
	if s.TerminalOpen && s.PanelOpen && a.root != "" {
		a.newTerminal()
	}
}

// openFolder makes dir the workspace.
func (a *app) openFolder(dir string) {
	dir = filepath.Clean(dir)
	if a.tree != nil {
		a.tree.Close()
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
	a.branch = gitBranch(dir)
	a.settings.Folder = dir
	a.settings.addRecent(dir)
	for _, d := range a.docs {
		a.tree.Watch(d.path)
	}
	a.settings.changed()
}

// closeFolder closes the workspace, once its documents are closed.
func (a *app) closeFolder() {
	a.closeAll(func() {
		if a.tree != nil {
			a.tree.Close()
		}
		a.root, a.tree, a.index, a.branch = "", nil, nil, ""
		a.settings.Folder = ""
		a.settings.changed()
	})
}

func (a *app) docIndex(path string) int {
	if path == "" {
		return -1
	}
	return slices.IndexFunc(a.docs, func(d *document) bool { return d.path == path })
}

func (a *app) activeDoc() *document {
	if a.active < 0 || a.active >= len(a.docs) {
		return nil
	}
	return a.docs[a.active]
}

// openFile shows a file in a tab, opening one unless it has one.
func (a *app) openFile(path string, focus bool) {
	if i := a.docIndex(path); i >= 0 {
		a.activate(i, focus)
		return
	}
	a.nextID++
	d, err := readDocument(a.nextID, path, a.invalidate)
	if errors.Is(err, errBinary) {
		a.fail("%s is a binary file", filepath.Base(path))
		return
	}
	if err != nil {
		a.fail("Could not open %s: %v", filepath.Base(path), err)
		return
	}
	// A new tab goes after the one shown.
	at := len(a.docs)
	if a.active >= 0 {
		at = a.active + 1
	}
	a.docs = slices.Insert(a.docs, at, d)
	if a.tree != nil {
		a.tree.Watch(path)
	}
	a.activate(at, focus)
	a.rememberTabs()
}

// newFile opens an untitled document.
func (a *app) newFile() {
	a.nextID++
	a.untitled++
	buf := ui.NewTextBuffer("")
	d := &document{id: a.nextID, buf: buf, saved: buf.Version(), hl: highlight.New(nil, nil), title: fmt.Sprintf("Untitled-%d", a.untitled)}
	a.docs = append(a.docs, d)
	a.activate(len(a.docs)-1, true)
}

func (a *app) activate(i int, focus bool) {
	if i < 0 || i >= len(a.docs) {
		return
	}
	a.active = i
	if p := a.docs[i].path; p != "" && a.tree != nil {
		a.recent = slices.Insert(slices.DeleteFunc(a.recent, func(r string) bool { return r == p }), 0, p)
		if len(a.recent) > 30 {
			a.recent = a.recent[:30]
		}
	}
	if focus {
		a.docs[i].vscroll.Focus()
		a.focus = paneEditor
	}
	a.find.stale = true
	a.rememberTabs()
}

func (a *app) cycleTab(step int) {
	if len(a.docs) == 0 {
		return
	}
	a.activate((a.active+step+len(a.docs))%len(a.docs), true)
}

// closeDoc closes tab i, asking first to save changes.
func (a *app) closeDoc(i int, then func()) {
	if i < 0 || i >= len(a.docs) {
		return
	}
	d := a.docs[i]
	remove := func() {
		j := slices.Index(a.docs, d)
		if j < 0 {
			return
		}
		d.hl.Close()
		a.docs = slices.Delete(a.docs, j, j+1)
		switch {
		case len(a.docs) == 0:
			a.active = -1
		case a.active > j || a.active == len(a.docs):
			a.active--
		}
		if d := a.activeDoc(); d != nil {
			d.vscroll.Focus()
		}
		a.rememberTabs()
		if then != nil {
			then()
		}
	}
	if !d.dirty() {
		remove()
		return
	}
	a.host.confirmClose(d.name(), func(choice int) {
		switch choice {
		case 0:
			a.saveDoc(d, remove)
		case 1:
			remove()
		}
	})
}

// closeAll closes every tab, asking about each changed one, then calls
// then unless one stayed open.
func (a *app) closeAll(then func()) {
	if len(a.docs) == 0 {
		then()
		return
	}
	a.closeDoc(len(a.docs)-1, func() { a.closeAll(then) })
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
	d.diskGone = false
	a.rememberTabs()
	if then != nil {
		then()
	}
}

func (a *app) saveActive() {
	if d := a.activeDoc(); d != nil {
		a.saveDoc(d, nil)
	}
}

func (a *app) saveActiveAs() {
	d := a.activeDoc()
	if d == nil {
		return
	}
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
		data, err := os.ReadFile(p)
		if err != nil || isBinary(data) {
			continue
		}
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		if string(data) == d.buf.String() {
			continue // as when the editor saved it
		}
		d.buf.Set(string(data))
		d.saved = d.buf.Version()
		d.measureWidest()
	}
	for _, p := range removed {
		if i := a.docIndex(p); i >= 0 {
			if _, err := os.Stat(p); err != nil {
				a.docs[i].diskGone = true
			}
		}
	}
	a.branch = gitBranch(a.root)
}

// rememberTabs keeps the open files in the settings.
func (a *app) rememberTabs() {
	var open []string
	for _, d := range a.docs {
		if d.path != "" {
			open = append(open, d.path)
		}
	}
	a.settings.OpenFiles = open
	a.settings.ActiveFile = ""
	if d := a.activeDoc(); d != nil {
		a.settings.ActiveFile = d.path
	}
	a.settings.changed()
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

// gitBranch returns the branch checked out in dir's repository, "" when it
// is none.
func gitBranch(dir string) string {
	if dir == "" {
		return ""
	}
	for d := dir; ; d = filepath.Dir(d) {
		head := filepath.Join(d, ".git", "HEAD")
		if data, err := os.ReadFile(head); err == nil {
			s := strings.TrimSpace(string(data))
			if ref, ok := strings.CutPrefix(s, "ref: refs/heads/"); ok {
				return ref
			}
			if len(s) >= 7 {
				return s[:7] // detached
			}
			return ""
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

// windowTitle is the title the window shows.
func (a *app) windowTitle() string {
	parts := []string{}
	if d := a.activeDoc(); d != nil {
		name := d.name()
		if d.dirty() {
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
	if t := a.windowTitle(); t != a.title {
		a.title = t
		a.host.setTitle(t)
	}
}
