package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
