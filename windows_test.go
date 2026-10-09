package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The window of a version with one, and the tabs of one without columns,
// become the first window.
func TestSettingsMigrateTheWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	old := map[string]any{
		"sidebarWidth": 300, "folder": "/tmp/x", "panelOpen": true, "terminalOpen": true,
		"openFiles": []string{"/tmp/x/a.go", "/tmp/x/b.go"}, "activeFile": "/tmp/x/b.go",
	}
	data, _ := json.Marshal(old)
	os.WriteFile(path, data, 0o644)
	s := loadSettings(path)
	if len(s.Windows) != 1 || s.Folder != "" || s.OpenFiles != nil || s.PanelOpen {
		t.Fatalf("windows %d, left behind %q %v %v", len(s.Windows), s.Folder, s.OpenFiles, s.PanelOpen)
	}
	w := s.Windows[0]
	if w.ID != 0 || w.Folder != "/tmp/x" || !w.PanelOpen || !w.TerminalOpen || len(w.Columns) != 1 ||
		len(w.Columns[0].Tabs) != 2 || w.Columns[0].Active != 1 {
		t.Fatalf("window %+v", w)
	}
	s.flush()
	data, _ = os.ReadFile(path)
	for _, key := range []string{`"folder": "/tmp/x"`, `"openFiles"`} {
		if strings.Contains(string(data), "\n  "+key) {
			t.Errorf("%s still kept at the top", key)
		}
	}
}

// The windows are kept the one focused last first; a new one takes the
// first ID that is free, and the one that closes lets its own go.
func TestSettingsKeepTheWindowsInOrder(t *testing.T) {
	s := defaultSettings()
	a, b, c := s.newWindow(), s.newWindow(), s.newWindow()
	if ids := []int{a.ID, b.ID, c.ID}; !slices.Equal(ids, []int{0, 1, 2}) || s.Windows[0] != c {
		t.Fatalf("IDs %v", ids)
	}
	s.focused(a)
	if !slices.Equal(s.Windows, []*windowState{a, c, b}) {
		t.Fatalf("order after focusing the first")
	}
	s.forget(c)
	if d := s.newWindow(); d.ID != 2 || s.Windows[0] != d || len(s.Windows) != 3 {
		t.Fatalf("new window %d of %d", d.ID, len(s.Windows))
	}
}

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
	s := defaultSettings()
	w := s.newWindow()
	a := newApp(&fakeHost{}, s, w)
	if !a.blank() {
		t.Error("an empty window is not blank")
	}
	w.Columns = []columnState{{Tabs: []tabState{{File: "/tmp/a.go"}}}}
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
	s := defaultSettings()
	s.Recent = []string{root, other}
	h := &fakeHost{others: []string{other}}
	a := newApp(h, s, s.newWindow())
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
	s := defaultSettings()
	s.Recent = []string{root}
	h := &fakeHost{}
	a := newApp(h, s, s.newWindow())
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

// The mayoi command hands absolute paths to the app.
func TestCLIScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := writeCLI(bin, "/Apps/My Mayoi.app"); err != nil {
		t.Fatal(err)
	}
	// open says what it was asked, and knows no app by its identifier
	// when $NO_ID is set.
	os.WriteFile(filepath.Join(bin, "open"), []byte("#!/bin/sh\n[ \"$1\" = -b ] && [ -n \"$NO_ID\" ] && exit 1\nfor a in \"$@\"; do echo \"$a\"; done\n"), 0o755)
	run := func(env string, args ...string) []string {
		cmd := exec.Command(filepath.Join(bin, "mayoi"), args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin", "PWD="+dir, env)
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	}
	if got := run(""); !slices.Equal(got, []string{"-b", bundleID}) {
		t.Errorf("no args: %v", got)
	}
	got := run("", ".", "a b.go", "/abs/x")
	want := []string{"-b", bundleID, dir + "/.", dir + "/a b.go", "/abs/x"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Where the system knows no app by the identifier, the one it was
	// installed from.
	got = run("NO_ID=1", "x")
	want = []string{"-a", "/Apps/My Mayoi.app", dir + "/x"}
	if !slices.Equal(got, want) {
		t.Errorf("fallback: got %v, want %v", got, want)
	}
}

func TestInstallAndUninstallCLI(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "second")
	p, err := installCLI([]string{first, second}, "")
	if err != nil || p != filepath.Join(first, "mayoi") {
		t.Fatalf("installed at %q: %v", p, err)
	}
	if info, err := os.Stat(p); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("not executable: %v", err)
	}
	// Again, over its own.
	if _, err := installCLI([]string{first}, ""); err != nil {
		t.Fatal(err)
	}
	// Another program's command stays.
	os.MkdirAll(second, 0o755)
	foreign := filepath.Join(second, "mayoi")
	os.WriteFile(foreign, []byte("#!/bin/sh\necho other\n"), 0o755)
	if _, err := installCLI([]string{second}, ""); err == nil {
		t.Error("overwrote another program's mayoi")
	}
	removed, err := uninstallCLI([]string{first, second})
	if err != nil || !slices.Equal(removed, []string{p}) {
		t.Fatalf("removed %v: %v", removed, err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Error("removed another program's mayoi")
	}
}

func TestCLIDirs(t *testing.T) {
	local := filepath.Join("/home/u", ".local", "bin")
	if got := cliDirs("/home/u", []string{"/usr/bin", local}); !slices.Equal(got, []string{local, "/usr/local/bin"}) {
		t.Errorf("on the path: %v", got)
	}
	if got := cliDirs("/home/u", []string{"/usr/bin"}); !slices.Equal(got, []string{"/usr/local/bin"}) {
		t.Errorf("off the path: %v", got)
	}
}
