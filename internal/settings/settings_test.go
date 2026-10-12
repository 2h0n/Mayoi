package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	s := Load(path)
	s.SidebarWidth = 300
	w := s.NewWindow()
	w.Folder, w.Columns = "/tmp/x", []Column{{Tabs: []Tab{{File: "/tmp/x/a.go"}}}}
	s.NewWindow().Folder = "/tmp/y"
	s.Flush()
	got := Load(path)
	if got.SidebarWidth != 300 || len(got.Windows) != 2 || got.Windows[0].Folder != "/tmp/y" || got.Windows[0].ID != 1 ||
		got.Windows[1].Folder != "/tmp/x" || len(got.Windows[1].Columns) != 1 {
		t.Errorf("loaded %+v", got)
	}
}

// The window of a version with one, and the tabs of one without columns,
// become the first window.
func TestMigrateTheWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	old := map[string]any{
		"sidebarWidth": 300, "folder": "/tmp/x", "panelOpen": true, "terminalOpen": true,
		"openFiles": []string{"/tmp/x/a.go", "/tmp/x/b.go"}, "activeFile": "/tmp/x/b.go",
	}
	data, _ := json.Marshal(old)
	os.WriteFile(path, data, 0o644)
	s := Load(path)
	if len(s.Windows) != 1 || s.Folder != "" || s.OpenFiles != nil || s.PanelOpen {
		t.Fatalf("windows %d, left behind %q %v %v", len(s.Windows), s.Folder, s.OpenFiles, s.PanelOpen)
	}
	w := s.Windows[0]
	if w.ID != 0 || w.Folder != "/tmp/x" || !w.PanelOpen || !w.TerminalOpen || len(w.Columns) != 1 ||
		len(w.Columns[0].Tabs) != 2 || w.Columns[0].Active != 1 {
		t.Fatalf("window %+v", w)
	}
	s.Flush()
	data, _ = os.ReadFile(path)
	for _, key := range []string{`"folder": "/tmp/x"`, `"openFiles"`} {
		if strings.Contains(string(data), "\n  "+key) {
			t.Errorf("%s still kept at the top", key)
		}
	}
}

// The windows are kept the one focused last first; a new one takes the
// first ID that is free, and the one that closes lets its own go.
func TestKeepTheWindowsInOrder(t *testing.T) {
	s := Default()
	a, b, c := s.NewWindow(), s.NewWindow(), s.NewWindow()
	if ids := []int{a.ID, b.ID, c.ID}; !slices.Equal(ids, []int{0, 1, 2}) || s.Windows[0] != c {
		t.Fatalf("IDs %v", ids)
	}
	s.Focused(a)
	if !slices.Equal(s.Windows, []*Window{a, c, b}) {
		t.Fatalf("order after focusing the first")
	}
	s.Forget(c)
	if d := s.NewWindow(); d.ID != 2 || s.Windows[0] != d || len(s.Windows) != 3 {
		t.Fatalf("new window %d of %d", d.ID, len(s.Windows))
	}
}

func TestMigrateDiffSplit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"diffSplit": true}`), 0o644)
	if s := Load(path); s.DiffLayout != "split" || s.DiffSplit {
		t.Errorf("layout %q, split %v", s.DiffLayout, s.DiffSplit)
	}
	os.WriteFile(path, []byte(`{}`), 0o644)
	if s := Load(path); s.DiffLayout != "" {
		t.Errorf("layout %q", s.DiffLayout)
	}
}

// The recent folders leave out the one given and those gone from the
// disk, which they keep; Clear forgets them all.
func TestRecentExcept(t *testing.T) {
	a, b, gone := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "gone")
	s := Default()
	for _, d := range []string{gone, b, a} {
		s.AddRecent(d)
	}
	if got := s.RecentExcept(a); !slices.Equal(got, []string{b}) {
		t.Errorf("except %s: %v", a, got)
	}
	if got := s.RecentExcept(""); !slices.Equal(got, []string{a, b}) {
		t.Errorf("all: %v", got)
	}
	if len(s.Recent) != 3 {
		t.Errorf("a folder gone was forgotten: %v", s.Recent)
	}
	s.ClearRecent()
	if len(s.Recent) != 0 || len(s.RecentExcept("")) != 0 {
		t.Errorf("cleared: %v", s.Recent)
	}
}

// The font preferences keep in their bounds, and take their defaults
// where the file leaves them out.
func TestFontPrefs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if s := Load(path); s.FontWeight != 400 || s.LineHeight != 1.55 || s.TerminalFontSize != 0 || s.FontFeatures != nil {
		t.Fatalf("defaults %+v", s.Prefs)
	}
	os.WriteFile(path, []byte(`{"fontFamily": " ", "fontWeight": 1000, "lineHeight": 9, "terminalFontSize": 4,
		"fontFeatures": ["calt=0", "", "ss01"]}`), 0o644)
	s := Load(path)
	if s.FontFamily != "monospace" || s.FontWeight != 900 || s.LineHeight != 3 || s.TerminalFontSize != 9 ||
		!slices.Equal(s.FontFeatures, []string{"calt=0", "ss01"}) {
		t.Fatalf("loaded %+v", s.Prefs)
	}
}

// An edit of the file changes the preferences it watches, but not the
// windows; the settings' own writes and a file that does not parse
// change nothing.
func TestWatchTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Load(path)
	s.NewWindow().Folder = "/tmp/x"
	s.Flush()
	got := make(chan Prefs, 4)
	stop, err := s.Watch(func(p Prefs) { got <- p })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	s.FontSize = 15
	s.Flush()
	os.WriteFile(path, []byte(`{"fontSize": 1`), 0o644)
	select {
	case p := <-got:
		t.Fatalf("changed by its own write or a broken file: %+v", p)
	case <-time.After(300 * time.Millisecond):
	}
	os.WriteFile(path, []byte(`{"fontSize": 16, "fontWeight": 500, "vim": true, "wordWrap": true, "windows": []}`), 0o644)
	select {
	case p := <-got:
		if p.FontSize != 16 || p.FontWeight != 500 || !p.Vim || p.FontFamily != "monospace" {
			t.Fatalf("prefs %+v", p)
		}
		gen := s.WrapGen
		if old := s.SetPrefs(p); old.FontSize != 15 || s.WrapGen != gen+1 || len(s.Windows) != 1 {
			t.Fatalf("old %+v, wrap gen %d, windows %d", old, s.WrapGen, len(s.Windows))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no change seen")
	}
}

// The themes were appearances once: a file of then keeps its theme.
func TestMigrateTheme(t *testing.T) {
	for _, theme := range []string{"matrix", "claude"} {
		path := filepath.Join(t.TempDir(), "settings.json")
		os.WriteFile(path, []byte(`{"appearance":"`+theme+`"}`), 0o644)
		if s := Load(path); s.Theme != theme || s.Appearance != "" {
			t.Errorf("theme %q, appearance %q", s.Theme, s.Appearance)
		}
	}
}
