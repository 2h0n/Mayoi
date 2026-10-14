package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/settings"
)

// railFixture is a window of project tabs with a project for each folder,
// the first shown, built by a tester as the window builds it.
func railFixture(t testing.TB, dirs ...string) (*frame, []*desktop, *ui.Tester) {
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

// Each project keeps its own layout: what its sidebar shows, whether it
// is closed, how wide it is, and its panel's height, as another shows
// meanwhile.
func TestProjectKeepsItsLayout(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	f, hs, tt := railFixture(t, root, other)
	a, b := hs[0].a, hs[1].a
	a.showSourceControl()
	a.window.SidebarWidth, a.window.PanelHeight = 320, 400
	tt.Frame()
	if !tt.HasText("SOURCE CONTROL") {
		t.Fatalf("A's sidebar: %v", tt.Texts())
	}
	f.active = hs[1]
	tt.Frame()
	tt.Frame()
	if tt.HasText("SOURCE CONTROL") || !tt.HasText("EXPLORER") || b.window.SidebarWidth == 320 || b.window.PanelHeight == 400 {
		t.Fatalf("B's sidebar: %+v, %v", b.window.Layout, tt.Texts())
	}
	b.toggleSidebar()
	f.active = hs[0]
	tt.Frame()
	tt.Frame()
	if !tt.HasText("SOURCE CONTROL") || a.window.SidebarClosed || a.window.SidebarWidth != 320 || a.window.PanelHeight != 400 {
		t.Fatalf("A's sidebar again: %+v, %v", a.window.Layout, tt.Texts())
	}
}

// A folder opens again with the layout it had as it closed.
func TestFolderKeepsItsLayout(t *testing.T) {
	root := workspaceFixture(t)
	s := settings.Default()
	s.AddRecent(root)
	w := s.NewWindow()
	w.Folder, w.Expanded = root, []string{"src"}
	w.SidebarView, w.SidebarWidth = "git", 320
	s.Forget(w)
	x := s.NewWindow()
	a := newApp(&fakeHost{folder: root}, s, x)
	a.openSession(root)
	if x.SidebarView != "git" || x.SidebarWidth != 320 {
		t.Fatalf("layout %+v", x.Layout)
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

// The rail of a window of project tabs, in both themes, at twice the
// scale: the project shown with unsaved changes, and another under the
// pointer.
func TestShotRail(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip()
	}
	for _, dark := range []bool{true, false} {
		var dirs []string
		for _, name := range []string{"mayoi", "my-app", "webServer", "docs", "go"} {
			dir := filepath.Join(t.TempDir(), name)
			os.MkdirAll(dir, 0o755)
			os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
			dirs = append(dirs, dir)
		}
		_, hs, tt := railFixture(t, dirs...)
		tt.SetDark(dark)
		tt.SetScale(2)
		a := hs[0].a
		a.openFile(filepath.Join(dirs[0], "main.go"), true)
		for range 10 {
			tt.Frame()
		}
		a.focusColumn()
		tt.Frame()
		tt.Type("// ")
		hs[3].a.alert = "Bell from zsh"
		r, _ := tt.Find("webServer")
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		for range 30 {
			tt.Frame()
			time.Sleep(10 * time.Millisecond)
		}
		name := "rail-light"
		if dark {
			name = "rail-dark"
		}
		(&harness{t: t, a: a, tt: tt}).shot(name)
	}
}

// A square dragged along the rail goes before the one it is dropped on,
// or at the end, and the rail's order is kept.
func TestRailReorders(t *testing.T) {
	a, b, c := workspaceFixture(t), workspaceFixture(t), workspaceFixture(t)
	f, hs, _ := railFixture(t, a, b, c)
	ids := func() []int {
		var out []int
		for _, h := range f.tabs {
			out = append(out, h.a.window.ID)
		}
		return out
	}
	id := func(i int) int { return hs[i].a.window.ID }
	f.move(hs[2], hs[0])
	if got := ids(); !slices.Equal(got, []int{id(2), id(0), id(1)}) {
		t.Fatalf("after moving the last first: %v", got)
	}
	f.move(hs[2], nil)
	if got := ids(); !slices.Equal(got, []int{id(0), id(1), id(2)}) {
		t.Fatalf("after moving it to the end: %v", got)
	}
	if !slices.Equal(f.m.s.Rail, ids()) {
		t.Errorf("rail kept as %v, shown as %v", f.m.s.Rail, ids())
	}
}

// The square's menu reveals the folder, copies its path and closes the
// project or the others.
func TestRailMenu(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	_, _, tt := railFixture(t, root, other)
	if err := tt.RightClick(filepath.Base(other)); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	for _, want := range []string{revealLabel(), "Copy Path", "Close Project", "Close Other Projects"} {
		if !slices.Contains(menu, want) {
			t.Errorf("no %q in the square's menu: %v", want, menu)
		}
	}
	if err := tt.ChooseMenuItem("Copy Path"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := tt.Clipboard(); got != other {
		t.Errorf("copied %q, want %q", got, other)
	}
}

// BenchmarkRailFrame is a frame of a window of twelve projects, drawn
// with nothing changing, as the rail is built every frame.
func BenchmarkRailFrame(b *testing.B) {
	var dirs []string
	for range 12 {
		dirs = append(dirs, workspaceFixture(b))
	}
	_, _, tt := railFixture(b, dirs...)
	for range 30 {
		tt.Frame()
	}
	b.ResetTimer()
	for range b.N {
		tt.Frame()
	}
}

// BenchmarkProjectSwitch is the frame that shows another project, each
// with a long Go file open, as a click on the rail does.
func BenchmarkProjectSwitch(b *testing.B) {
	src, _ := os.ReadFile("editor.go")
	var dirs []string
	for range 2 {
		dir := workspaceFixture(b)
		os.WriteFile(filepath.Join(dir, "editor.go"), src, 0o644)
		dirs = append(dirs, dir)
	}
	f, hs, tt := railFixture(b, dirs...)
	for i, h := range hs {
		f.active = h
		h.a.openFile(filepath.Join(dirs[i], "editor.go"), true)
		for range 30 {
			tt.Frame()
			time.Sleep(5 * time.Millisecond)
		}
	}
	b.ResetTimer()
	for i := range b.N {
		f.active = hs[i%2]
		tt.Frame()
	}
}

// A terminal's bell in a project out of sight marks its square, in its
// tip too, until the project shows.
func TestRailMarksAttention(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	f, hs, tt := railFixture(t, root, other)
	// Shown once, as a project with a terminal was.
	f.active = hs[1]
	tt.Frame()
	f.active = hs[0]
	tt.Frame()
	hs[1].a.post(func() { hs[1].a.alert = "Bell from zsh" })
	tt.Frame()
	if hs[1].a.alert == "" {
		t.Fatal("the mark of a project out of sight went")
	}
	r, _ := tt.Find(filepath.Base(other))
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	for range 5 {
		tt.Frame()
		time.Sleep(150 * time.Millisecond)
	}
	if !tt.HasText("Bell from zsh") {
		t.Errorf("the tip does not say why: %v", tt.Texts())
	}
	f.active = hs[1]
	tt.Frame()
	if hs[1].a.alert != "" {
		t.Error("the mark stays as the project shows")
	}
}
