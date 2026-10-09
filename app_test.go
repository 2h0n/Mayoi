package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// fakeHost answers the app's dialogs as a test says.
type fakeHost struct {
	folder  string
	saveTo  string
	choice  int
	asked   []string
	title   string
	closed  bool
	trashed []string
	// others are the folders other windows have open, and windows those
	// asked to open in a window of their own.
	others  []string
	windows []string
}

func (h *fakeHost) openFolder(done func(string)) { done(h.folder) }
func (h *fakeHost) saveAs(name string, done func(string)) {
	h.asked = append(h.asked, "save "+name)
	done(h.saveTo)
}
func (h *fakeHost) confirmClose(name string, done func(int)) {
	h.asked = append(h.asked, "close "+name)
	done(h.choice)
}
func (h *fakeHost) trash(path string) error {
	h.trashed = append(h.trashed, path)
	return os.RemoveAll(path)
}
func (h *fakeHost) reveal(string)         {}
func (h *fakeHost) setTitle(title string) { h.title = title }
func (h *fakeHost) closeWindow()          { h.closed = true }
func (h *fakeHost) setAppearance(string)  {}
func (h *fakeHost) showFolder(dir string) bool {
	return slices.Contains(h.others, dir)
}
func (h *fakeHost) newWindow(dir string) { h.windows = append(h.windows, dir) }
func (h *fakeHost) installCLI()          {}
func (h *fakeHost) uninstallCLI()        {}

// workspaceFixture makes a folder of a few files.
func workspaceFixture(t testing.TB) string {
	root := t.TempDir()
	files := map[string]string{
		"main.go":             "package main\n\nimport \"fmt\"\n\n// main says hello.\nfunc main() {\n\tfmt.Println(\"hello, mayoi\")\n}\n",
		"README.md":           "# Mayoi\n\nA small editor.\n",
		"internal/a/a.go":     "package a\n\nconst Answer = 42\n",
		"internal/b/b.ts":     "export const greet = (name: string) => `hi ${name}`;\n",
		"web/index.html":      "<!doctype html>\n<title>Mayoi</title>\n",
		"web/style.css":       "body { color: #333; }\n",
		"node_modules/x/x.js": "module.exports = 1\n",
		"go.mod":              "module example\n\ngo 1.27\n",
	}
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

type harness struct {
	t    testing.TB
	a    *app
	h    *fakeHost
	tt   *ui.Tester
	root string
}

func newHarness(t testing.TB, dark bool) *harness {
	root := workspaceFixture(t)
	s := defaultSettings()
	w := s.newWindow()
	w.Folder = root
	h := &fakeHost{folder: root}
	a := newApp(h, s, w)
	tt := ui.NewTester(a.view, 1200, 760)
	tt.SetDark(dark)
	hs := &harness{t: t, a: a, h: h, tt: tt, root: root}
	hs.settle()
	return hs
}

// settle draws frames until the background work (reading directories,
// highlighting, indexing) is done.
func (hs *harness) settle() {
	for range 30 {
		hs.tt.Frame()
		time.Sleep(5 * time.Millisecond)
	}
	hs.tt.Frame()
}

func (hs *harness) shot(name string) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		hs.t.Fatal(err)
	}
	png.Encode(f, hs.tt.Image())
	f.Close()
}

func (hs *harness) path(rel string) string { return filepath.Join(hs.root, filepath.FromSlash(rel)) }

func TestOpenFolderShowsTreeAndOpensFiles(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	for _, name := range []string{"internal", "web", "main.go", "README.md", "go.mod"} {
		if !tt.HasText(name) {
			t.Errorf("tree lacks %q: %q", name, tt.Texts())
		}
	}
	if tt.HasText("node_modules") == false {
		t.Error("node_modules is hidden from the tree; it should show (only quick open skips it)")
	}
	hs.shot("01-welcome")

	if err := tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	d := a.activeEditor()
	if d == nil || d.path != hs.path("main.go") {
		t.Fatalf("active doc %v after clicking main.go", d)
	}
	// A folder opens with a click, and shows its children.
	if err := tt.Click("internal"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if !tt.HasText("a") || !tt.HasText("b") {
		t.Errorf("internal did not open: %q", tt.Texts())
	}
	hs.shot("02-editor-dark")
	if !strings.Contains(hs.h.title, "main.go") {
		t.Errorf("title %q", hs.h.title)
	}
}

func TestEditSaveAndDirtyClose(t *testing.T) {
	hs := newHarness(t, false)
	a, tt := hs.a, hs.tt
	a.settings.AutoSave = false // the prompts are for files that do not save themselves
	a.openFile(hs.path("README.md"), true)
	hs.settle()
	tt.Type("Hello ")
	hs.settle()
	d := a.activeEditor()
	if !d.dirty() || !strings.HasPrefix(d.buf.String(), "Hello # Mayoi") {
		t.Fatalf("typed: dirty %v text %q", d.dirty(), d.buf.String())
	}
	if !strings.Contains(hs.h.title, "●") {
		t.Errorf("title %q lacks the dot of changes", hs.h.title)
	}
	a.run("file.save")
	hs.settle()
	data, _ := os.ReadFile(hs.path("README.md"))
	if !strings.HasPrefix(string(data), "Hello # Mayoi") || d.dirty() {
		t.Fatalf("saved %q, dirty %v", data, d.dirty())
	}
	// Our own save comes back from the watcher without changing anything.
	hs.settle()
	if d.dirty() {
		t.Error("dirty after the watcher saw the save")
	}

	// Closing a changed tab asks; Cancel keeps it.
	tt.Type("more ")
	hs.settle()
	hs.h.choice = 2
	a.run("file.close")
	hs.settle()
	if len(a.docs) != 1 || len(hs.h.asked) != 1 {
		t.Fatalf("cancel: %d docs, asked %v", len(a.docs), hs.h.asked)
	}
	// Don't Save closes it, and leaves the file.
	hs.h.choice = 1
	a.run("file.close")
	hs.settle()
	if len(a.docs) != 0 {
		t.Fatalf("don't save: %d docs", len(a.docs))
	}
	data, _ = os.ReadFile(hs.path("README.md"))
	if strings.Contains(string(data), "more") {
		t.Error("discarded changes were saved")
	}
}

func TestExternalChangeReloadsCleanDocument(t *testing.T) {
	hs := newHarness(t, false)
	a := hs.a
	a.openFile(hs.path("go.mod"), true)
	hs.settle()
	os.WriteFile(hs.path("go.mod"), []byte("module changed\n"), 0o644)
	deadline := time.Now().Add(5 * time.Second)
	for a.activeEditor().buf.String() != "module changed\n" && time.Now().Before(deadline) {
		hs.settle()
	}
	if got := a.activeEditor().buf.String(); got != "module changed\n" {
		t.Errorf("not reloaded: %q", got)
	}
}

func TestQuickOpenAndGoToLine(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.run("go.quickOpen")
	deadline := time.Now().Add(5 * time.Second)
	for a.index == nil && time.Now().Before(deadline) {
		hs.settle()
	}
	for _, f := range a.index {
		if strings.HasPrefix(f, "node_modules/") {
			t.Errorf("index has %s", f)
		}
	}
	tt.Type("bts")
	hs.settle()
	hs.shot("03-quick-open")
	tt.Key(0, ui.KeyEnter)
	hs.settle()
	if d := a.activeEditor(); d == nil || filepath.Base(d.path) != "b.ts" {
		t.Fatalf("quick open opened %v", d)
	}
	if a.quick.open {
		t.Error("quick open stayed open")
	}

	a.openFile(hs.path("main.go"), true)
	hs.settle()
	a.run("go.line")
	hs.settle()
	tt.Type("6")
	tt.Key(0, ui.KeyEnter)
	hs.settle()
	d := a.activeEditor()
	if line := d.buf.Snapshot().LineAt(d.caret); line != 5 {
		t.Errorf("caret on line %d, want 5", line)
	}
}

func TestFindAndReplace(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	a.run("edit.replace")
	hs.settle()
	tt.Type("main")
	hs.settle()
	if n := len(a.find.matches); n != 3 {
		t.Fatalf("%d matches of main, want 3", n)
	}
	hs.shot("04-find")
	tt.Key(0, ui.KeyEnter)
	hs.settle()
	if a.find.cur != 1 {
		t.Errorf("current match %d after Enter", a.find.cur)
	}
	a.find.with = "entry"
	a.replaceAll(a.activeEditor())
	hs.settle()
	if got := a.activeEditor().buf.String(); strings.Contains(got, "main") || strings.Count(got, "entry") != 3 {
		t.Errorf("after replace all: %q", got)
	}
	// Regular expressions, with groups.
	a.find.regex, a.find.query, a.find.with, a.find.stale = true, `fmt\.(\w+)`, "log.$1", true
	a.replaceAll(a.activeEditor())
	if got := a.activeEditor().buf.String(); !strings.Contains(got, "log.Println") {
		t.Errorf("regexp replace: %q", got)
	}
	tt.Key(0, ui.KeyEscape)
	hs.settle()
	if a.find.open {
		t.Error("Escape left find open")
	}
}

func TestNewFileAndRenameFromTheTree(t *testing.T) {
	hs := newHarness(t, false)
	a, tt := hs.a, hs.tt
	a.askNewEntry(hs.root, false)
	hs.settle()
	tt.Type("notes/todo.md")
	tt.Key(0, ui.KeyEnter)
	hs.settle()
	if _, err := os.Stat(hs.path("notes/todo.md")); err != nil {
		t.Fatal(err)
	}
	if d := a.activeEditor(); d == nil || d.path != hs.path("notes/todo.md") {
		t.Fatalf("new file not open: %v", d)
	}
	a.askRename(hs.path("notes/todo.md"))
	hs.settle()
	hs.shot("05-rename")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("done.md")
	tt.Key(0, ui.KeyEnter)
	hs.settle()
	if _, err := os.Stat(hs.path("notes/done.md")); err != nil {
		t.Fatal(err)
	}
	if d := a.activeEditor(); d.path != hs.path("notes/done.md") {
		t.Errorf("open document did not follow the rename: %s", d.path)
	}
}

func TestTabsKeepTheirState(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	tt.Type("x")
	a.openFile(hs.path("README.md"), true)
	hs.settle()
	tt.Type("y")
	a.activate(a.column(), 0, true)
	hs.settle()
	// Undo in the first tab undoes its own edit, after the other tab's.
	tt.Key(ui.Cmd, ui.KeyZ)
	hs.settle()
	if got := a.docs[0].buf.String(); strings.HasPrefix(got, "x") {
		t.Errorf("undo in the first tab: %q", got[:10])
	}
	if !strings.HasPrefix(a.docs[1].buf.String(), "y") {
		t.Error("the second tab lost its edit")
	}
	a.cycleTab(1)
	if got := a.column().active; got != 1 {
		t.Errorf("active %d after the next tab", got)
	}
	hs.shot("06-tabs")
}

func TestPanelWithTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a shell")
	}
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.openFile(hs.path("main.go"), true)
	a.run("view.terminal")
	hs.settle()
	if len(a.terms) != 1 || !a.window.PanelOpen {
		t.Fatalf("terminal: %d sessions, panel %v", len(a.terms), a.window.PanelOpen)
	}
	tt.Type("echo mayoi-$((40+2))")
	tt.Key(0, ui.KeyEnter)
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(a.terms[0].term.Text(), "mayoi-42") && time.Now().Before(deadline) {
		hs.settle()
	}
	if !strings.Contains(a.terms[0].term.Text(), "mayoi-42") {
		t.Errorf("terminal text %q", a.terms[0].term.Text())
	}
	a.run("view.newTerminal")
	hs.settle()
	if len(a.terms) != 2 || a.panelTerm != a.terms[1] {
		t.Errorf("second terminal: %d, the panel shows the first", len(a.terms))
	}
	hs.shot("07-terminal")
	a.closeTerminals()
}

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	s := loadSettings(path)
	s.SidebarWidth = 300
	w := s.newWindow()
	w.Folder, w.Columns = "/tmp/x", []columnState{{Tabs: []tabState{{File: "/tmp/x/a.go"}}}}
	s.newWindow().Folder = "/tmp/y"
	s.flush()
	got := loadSettings(path)
	if got.SidebarWidth != 300 || len(got.Windows) != 2 || got.Windows[0].Folder != "/tmp/y" || got.Windows[0].ID != 1 ||
		got.Windows[1].Folder != "/tmp/x" || len(got.Windows[1].Columns) != 1 {
		t.Errorf("loaded %+v", got)
	}
}

func TestLargeFileStaysQuick(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	var b strings.Builder
	for i := range 100_000 {
		fmt.Fprintf(&b, "func f%d(x int) int { return x * %d } // line %d\n", i, i, i)
	}
	big := hs.path("big.go")
	os.WriteFile(big, []byte(b.String()), 0o644)
	start := time.Now()
	a.openFile(big, true)
	tt.Frame()
	t.Logf("open 100k lines: %v", time.Since(start))
	hs.settle()
	start = time.Now()
	const n = 100
	for range n {
		tt.Scroll(700, 400, 0, 120)
		tt.Frame()
	}
	t.Logf("scroll: %.2f ms/frame", float64(time.Since(start).Microseconds())/1000/n)
	start = time.Now()
	for range n {
		tt.Type("a")
		tt.Frame()
	}
	t.Logf("type: %.2f ms/frame", float64(time.Since(start).Microseconds())/1000/n)
	hs.settle()
	hs.shot("08-big-file")
}

// Showing and hiding the sidebar and the panel keeps the editor's state:
// its caret, its place and its undo history.
func TestLayoutChangesKeepEditorState(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	tt.Type("// edit\n")
	hs.settle()
	caret := a.activeEditor().caret
	a.run("view.sidebar")
	hs.settle()
	a.window.PanelOpen = true
	hs.settle()
	a.panelMax = true
	hs.settle()
	a.panelMax, a.window.PanelOpen = false, false
	a.run("view.sidebar")
	hs.settle()
	d := a.activeEditor()
	if d.caret != caret {
		t.Errorf("caret %d after layout changes, was %d", d.caret, caret)
	}
	d.vscroll.Focus()
	hs.settle()
	tt.Key(ui.Cmd, ui.KeyZ)
	hs.settle()
	if strings.Contains(d.buf.String(), "// edit") {
		t.Error("undo history lost across layout changes")
	}
}

func TestSplittersResize(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	w := a.settings.SidebarWidth
	x := w - 3 // the sidebar's right edge
	tt.Press(x, 300)
	tt.Move(x+40, 300)
	tt.Move(x+60, 300)
	tt.Release(x+60, 300)
	hs.settle()
	if got := a.settings.SidebarWidth; got < w+50 || got > w+70 {
		t.Errorf("sidebar %v after dragging 60 from %v", got, w)
	}
}

func TestRestoresTheActiveTab(t *testing.T) {
	root := workspaceFixture(t)
	s := defaultSettings()
	s.Folder = root
	// The tabs as a version without columns kept them.
	files := []string{filepath.Join(root, "main.go"), filepath.Join(root, "README.md"), filepath.Join(root, "go.mod")}
	s.OpenFiles, s.ActiveFile = files, files[1]
	s.migrate()
	a := newApp(&fakeHost{folder: root}, s, s.Windows[0])
	tt := ui.NewTester(a.view, 1200, 760)
	tt.Frame()
	tt.Frame()
	if d := a.activeEditor(); d == nil || d.path != files[1] {
		t.Fatalf("active %v, want README.md", d)
	}
	tabs := a.column().tabs
	if len(a.cols) != 1 || len(tabs) != 3 || tabs[0].ed.path != files[0] || tabs[2].ed.path != files[2] {
		t.Fatalf("tabs out of order")
	}
}
