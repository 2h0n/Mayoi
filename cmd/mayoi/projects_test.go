package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/settings"
)

// railFixture is a window of project tabs with a project for each folder,
// the first shown, built by a tester as the window builds it.
func railFixture(t *testing.T, dirs ...string) (*frame, []*desktop, *ui.Tester) {
	t.Helper()
	s := settings.Default()
	s.ProjectTabs = true
	m := &mayoi{s: s}
	f := &frame{m: m}
	var hs []*desktop
	for _, dir := range dirs {
		w := s.NewWindow()
		w.Folder = dir
		// The app asks a fake host for what a window does: the tests have
		// none.
		h := &desktop{m: m}
		h.a = newApp(&fakeHost{}, s, w)
		f.add(h)
		hs = append(hs, h)
	}
	f.active = hs[0]
	m.wins = slices.Clone(hs)
	tt := ui.NewTester(f.view, 1200, 760)
	tt.Frame()
	return f, hs, tt
}

// The rail has a square for each project, named by its folder, which a
// window of one project has none of.
func TestRailShowsTheProjects(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	f, hs, tt := railFixture(t, root, other)
	for _, dir := range []string{root, other} {
		if _, ok := tt.Find(filepath.Base(dir)); !ok {
			t.Fatalf("no square for %s on the rail: %v", dir, tt.Texts())
		}
	}
	if !slices.Equal(f.m.s.Rail, []int{hs[0].a.window.ID, hs[1].a.window.ID}) {
		t.Errorf("rail order %v", f.m.s.Rail)
	}
	if err := tt.RightClick(filepath.Base(other)); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Contains(menu, "Close Project") {
		t.Errorf("the square's menu: %v", menu)
	}
	tt.CloseMenu()
	f.tabs = f.tabs[:1]
	tt.Frame()
	if _, ok := tt.Find(filepath.Base(other)); ok {
		t.Error("a window of one project has a rail")
	}
}

// A project out of sight keeps its caret and where it was scrolled, and
// gets them back as it shows again, although its text area is a new one.
func TestProjectKeepsItsPlace(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	path := filepath.Join(root, "long.go")
	writeLines(t, path, 400)
	f, hs, tt := railFixture(t, root, other)
	a := hs[0].a
	a.openFile(path, true)
	tt.Frame()
	ed := a.activeEditor()
	if ed == nil {
		t.Fatal("no editor")
	}
	at := strings.Index(ed.buf.String(), "line 200")
	ed.pending = &[2]int{at, at}
	tt.Frame()
	tt.Frame()
	sel, y := ed.sel, ed.vscroll.Y
	if sel != [2]int{at, at} || y <= 0 {
		t.Fatalf("before: sel %v, scrolled %v", sel, y)
	}
	f.active = hs[1]
	tt.Frame()
	tt.Frame()
	if !ed.hidden {
		t.Fatal("the editor out of sight is not parked")
	}
	f.active = hs[0]
	tt.Frame()
	tt.Frame()
	if ed.sel != sel || ed.vscroll.Y != y {
		t.Fatalf("back: sel %v (was %v), scrolled %v (was %v)", ed.sel, sel, ed.vscroll.Y, y)
	}
}

// What is posted to a project out of sight runs, as the window draws.
func TestHiddenProjectRunsItsPosts(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	f, hs, tt := railFixture(t, root, other)
	f.active = hs[1]
	tt.Frame()
	f.active = hs[0]
	tt.Frame()
	ran := false
	hs[1].a.post(func() { ran = true })
	tt.Frame()
	if !ran {
		t.Error("a hidden project's post never ran")
	}
}

// A project of the rail not shown since the app started keeps the session
// the settings have of it as the app quits.
func TestUnshownProjectKeepsItsSession(t *testing.T) {
	s := settings.Default()
	w := s.NewWindow()
	w.Columns = []settings.Column{{Tabs: []settings.Tab{{File: "/tmp/a.go"}}, Weight: 1}}
	a := newApp(&fakeHost{}, s, w)
	a.keepSession()
	a.park()
	if len(w.Columns) != 1 || len(w.Columns[0].Tabs) != 1 {
		t.Fatalf("session lost: %+v", w.Columns)
	}
}

// With project tabs, a folder chosen opens in a tab of its own, and a
// window with no folder takes it.
func TestProjectTabsOpenFoldersInTabs(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	s := settings.Default()
	s.ProjectTabs = true
	h := &fakeHost{}
	a := newApp(h, s, s.NewWindow())
	a.switchFolder(root)
	if a.root != root || len(h.windows) != 0 {
		t.Fatalf("no folder: folder %q, windows %v", a.root, h.windows)
	}
	a.switchFolder(other)
	if a.root != root || !slices.Equal(h.windows, []string{other}) {
		t.Fatalf("with a folder: folder %q, windows %v", a.root, h.windows)
	}
}

// writeLines writes a file of n numbered lines.
func writeLines(t *testing.T, path string, n int) {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "// line %d\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectWords(t *testing.T) {
	for name, want := range map[string]string{
		"mayoi": "M", "my-app": "MA", "mayoi_editor": "ME", "mayoiEditor": "ME", "go.dev": "GD", "": "?",
	} {
		if got := initials(projectWords(name)); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
}
