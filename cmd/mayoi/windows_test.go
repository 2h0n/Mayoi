package main

import (
	"mayoi/internal/settings"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSplitPaths(t *testing.T) {
	root := workspaceFixture(t)
	got := argPaths([]string{"-psn_0_1", ".", "main.go", "internal/", root, "gone.go", ""}, root)
	folders, files := splitPaths(got)
	if !slices.Equal(folders, []string{root, filepath.Join(root, "internal")}) {
		t.Errorf("folders %v", folders)
	}
	if !slices.Equal(files, []string{filepath.Join(root, "main.go")}) {
		t.Errorf("files %v", files)
	}
}

// A window that shows nothing takes a folder, also before it starts; one
// with tabs to restore does not.
func TestBlankWindow(t *testing.T) {
	s := settings.Default()
	w := s.NewWindow()
	a := newApp(&fakeHost{}, s, w)
	if !a.blank() {
		t.Error("an empty window is not blank")
	}
	w.Columns = []settings.Column{{Tabs: []settings.Tab{{File: "/tmp/a.go"}}}}
	if a.blank() {
		t.Error("a window with tabs to restore is blank")
	}
	w.Columns, w.Folder = nil, "/tmp/x/"
	if a.blank() || a.folder() != "/tmp/x" {
		t.Errorf("a window of a folder: blank %v, folder %q", a.blank(), a.folder())
	}
}

// Recent folders open in the window, or with Cmd in a window of their
// own; a folder another window has open brings that one to the front.
func TestRecentFolderOpensInWhichWindow(t *testing.T) {
	root, other := workspaceFixture(t), workspaceFixture(t)
	s := settings.Default()
	s.Recent = []string{root, other}
	h := &fakeHost{others: []string{other}}
	a := newApp(h, s, s.NewWindow())
	tt := ui.NewTester(a.view, 1200, 760)
	tt.Frame()
	if err := tt.ClickWith(ui.Cmd, "Open "+root); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !slices.Equal(h.windows, []string{root}) || a.root != "" {
		t.Fatalf("Cmd-click: windows %v, folder %q", h.windows, a.root)
	}
	tt.Click("Open " + other)
	tt.Frame()
	if a.root != "" || len(h.windows) != 1 {
		t.Fatalf("a folder of another window opened here: %q", a.root)
	}
	tt.Click("Open " + root)
	tt.Frame()
	if a.root != root || a.window.Folder != root {
		t.Fatalf("click: folder %q", a.root)
	}
}

func TestRecentFolderCmdEnter(t *testing.T) {
	root := workspaceFixture(t)
	s := settings.Default()
	s.Recent = []string{root}
	h := &fakeHost{}
	a := newApp(h, s, s.NewWindow())
	tt := ui.NewTester(a.view, 1200, 760)
	tt.Frame()
	label := "Open " + root
	for range 12 {
		if tt.Focused(label) {
			break
		}
		tt.Key(0, ui.KeyTab)
		tt.Frame()
	}
	if !tt.Focused(label) {
		t.Fatal("Tab never reached the recent folder")
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	if !slices.Equal(h.windows, []string{root}) || a.root != "" {
		t.Fatalf("Cmd+Enter: windows %v, folder %q", h.windows, a.root)
	}
}
