// Package settings keeps what Mayoi remembers between launches, in a JSON
// file.
package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Settings are what Mayoi remembers between launches: the layout, and
// the windows with the folder and the files open in each. A window's
// frame is its StateKey's.
type Settings struct {
	Prefs
	SidebarOpen bool `json:"sidebarOpen"`
	// SidebarView is what the sidebar shows: "" the explorer, "git" the
	// changes for source control.
	SidebarView  string  `json:"sidebarView,omitempty"`
	SidebarWidth float32 `json:"sidebarWidth"`
	PanelHeight  float32 `json:"panelHeight"`
	// DiffSplit is what an older version kept for DiffLayout "split".
	DiffSplit bool `json:"diffSplit,omitempty"`
	// WrapGen counts the changes of WordWrap.
	WrapGen int      `json:"-"`
	Recent  []string `json:"recent"`
	// Windows are the windows open, the one focused last first.
	Windows []*Window `json:"windows,omitempty"`

	// Folder, Columns, Column, PanelOpen and TerminalOpen are the window
	// as a version with one window kept it, and OpenFiles and ActiveFile
	// its tabs as a version without columns kept them: Load
	// makes a window of them.
	Folder       string   `json:"folder,omitempty"`
	Columns      []Column `json:"columns,omitempty"`
	Column       int      `json:"column,omitempty"`
	PanelOpen    bool     `json:"panelOpen,omitempty"`
	TerminalOpen bool     `json:"terminalOpen,omitempty"`
	OpenFiles    []string `json:"openFiles,omitempty"`
	ActiveFile   string   `json:"activeFile,omitempty"`

	path  string
	mu    sync.Mutex
	timer *time.Timer
	// written is what the settings last wrote to their file, which a
	// change of the file that is not theirs differs from.
	written []byte
}

// Prefs are the settings a person chooses, which an edit of the file
// applies as Mayoi runs: not the windows and the layout it keeps itself.
type Prefs struct {
	FontSize   float32 `json:"fontSize"`
	FontFamily string  `json:"fontFamily"`
	// FontWeight is how heavy the code's text is, from 100 to 900.
	FontWeight int `json:"fontWeight"`
	// FontFeatures turn OpenType features of the code's font on and off,
	// as "calt=0" turns off most coding ligatures and "ss01" a stylistic
	// set on.
	FontFeatures []string `json:"fontFeatures,omitempty"`
	// LineHeight is the height of a line of code, a multiple of FontSize.
	LineHeight float32 `json:"lineHeight"`
	// TerminalFontFamily and TerminalFontSize are the terminal's, or
	// those of the code when empty and zero.
	TerminalFontFamily string  `json:"terminalFontFamily,omitempty"`
	TerminalFontSize   float32 `json:"terminalFontSize,omitempty"`
	// AutoSave saves changed files a moment after an edit, and as the
	// keyboard leaves them.
	AutoSave bool `json:"autoSave"`
	// WordWrap wraps long lines at the edge of the editor, rather than
	// scrolling sideways to them. A tab may choose otherwise, until this
	// changes: Settings.WrapGen counts its changes.
	WordWrap bool `json:"wordWrap,omitempty"`
	// DiffLayout is how diffs show: "split" side by side, "inline" in
	// one column, or "" side by side when they are wide enough to.
	DiffLayout string `json:"diffLayout,omitempty"`
	// ScmTree lists the changes in source control in a tree of their
	// folders, rather than by path.
	ScmTree bool `json:"scmTree,omitempty"`
	// Blame shows who last changed the caret's line, and when, after it.
	Blame bool `json:"blame"`
	// Vim edits with vim's keys: the editors start in normal mode.
	Vim bool `json:"vim,omitempty"`
	// Appearance is "light", "dark", or "" to follow the system.
	Appearance string `json:"appearance,omitempty"`
	// Theme is "matrix" for The Matrix, which is always dark, "claude" for
	// Claude, light or dark as Appearance is, or "" for Mayoi's own.
	Theme string `json:"theme,omitempty"`
	// SidebarRight puts the sidebar right of the editors, rather than left.
	SidebarRight bool `json:"sidebarRight,omitempty"`
}

// Window is a window as the settings keep it: its folder, the
// columns of its editor area and what their tabs show, and Column the one
// that had the keyboard.
type Window struct {
	// ID names the window's frame, which the next start gives it back.
	ID           int      `json:"id"`
	Folder       string   `json:"folder,omitempty"`
	Columns      []Column `json:"columns,omitempty"`
	Column       int      `json:"column,omitempty"`
	PanelOpen    bool     `json:"panelOpen,omitempty"`
	TerminalOpen bool     `json:"terminalOpen,omitempty"`
}

// Column is a column of the editor area, as the settings keep it.
type Column struct {
	Tabs   []Tab   `json:"tabs"`
	Active int     `json:"active"`
	Weight float32 `json:"weight"`
}

// Tab is a tab: the file it shows, or a terminal, which the next
// start opens anew.
type Tab struct {
	File     string `json:"file,omitempty"`
	Terminal bool   `json:"terminal,omitempty"`
	// Preview is set for an SVG shown as a picture rather than as text.
	Preview bool `json:"preview,omitempty"`
	// Diff is set for the changes of File rather than the file: "head",
	// "index" or "staged", what they are against. With no File, it is
	// the changes of every file, against that. Layout is the diff's, as
	// the settings' DiffLayout; Split is what an older version kept for
	// "split". "commit:" and a hash is the changes of that commit.
	Diff   string `json:"diff,omitempty"`
	Layout string `json:"layout,omitempty"`
	Split  bool   `json:"split,omitempty"`
}

// Default is the settings of a first launch.
func Default() *Settings {
	return &Settings{
		Prefs: Prefs{
			FontSize: 13, FontFamily: "monospace", FontWeight: 400, LineHeight: 1.55,
			AutoSave: true, Blame: true,
		},
		SidebarOpen: true, SidebarWidth: 248, PanelHeight: 260,
	}
}

// Load reads the settings at path, the defaults where it has none.
func Load(path string) *Settings {
	s := Default()
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, s)
	}
	s.path = path
	s.SidebarWidth = clamp(s.SidebarWidth, 160, 600)
	s.PanelHeight = clamp(s.PanelHeight, 100, 2000)
	s.normalize()
	s.migrate()
	return s
}

// normalize keeps the preferences in their bounds, the defaults where the
// file leaves them out or makes no sense.
func (s *Settings) normalize() {
	s.FontSize = clamp(s.FontSize, 9, 28)
	if strings.TrimSpace(s.FontFamily) == "" {
		s.FontFamily = "monospace"
	}
	if s.FontWeight == 0 {
		s.FontWeight = 400
	}
	s.FontWeight = min(max(s.FontWeight, 100), 900)
	if s.LineHeight == 0 {
		s.LineHeight = 1.55
	}
	s.LineHeight = clamp(s.LineHeight, 1, 3)
	s.FontFeatures = slices.DeleteFunc(s.FontFeatures, func(f string) bool { return strings.TrimSpace(f) == "" })
	if s.TerminalFontSize != 0 {
		s.TerminalFontSize = clamp(s.TerminalFontSize, 9, 28)
	}
	if s.DiffSplit {
		s.DiffLayout, s.DiffSplit = "split", false
	}
	// The themes were appearances once.
	if s.Appearance == "matrix" || s.Appearance == "claude" {
		s.Theme, s.Appearance = s.Appearance, ""
	}
}

// migrate makes a window of the one an older version kept.
func (s *Settings) migrate() {
	if len(s.Windows) > 0 || s.Folder == "" && len(s.Columns) == 0 && len(s.OpenFiles) == 0 {
		return
	}
	w := &Window{Folder: s.Folder, Columns: s.Columns, Column: s.Column, PanelOpen: s.PanelOpen, TerminalOpen: s.TerminalOpen}
	if len(w.Columns) == 0 && len(s.OpenFiles) > 0 {
		// The files of one column.
		cs := Column{Active: -1, Weight: 1}
		for _, p := range s.OpenFiles {
			if p == s.ActiveFile {
				cs.Active = len(cs.Tabs)
			}
			cs.Tabs = append(cs.Tabs, Tab{File: p})
		}
		w.Columns, w.Column = []Column{cs}, 0
	}
	s.Windows = []*Window{w}
	s.Folder, s.Columns, s.Column, s.PanelOpen, s.TerminalOpen = "", nil, 0, false, false
	s.OpenFiles, s.ActiveFile = nil, ""
}

// Path is the file the settings are kept in, "" for none.
func (s *Settings) Path() string { return s.path }

// NewWindow keeps a new window, focused first, and names its frame by
// the first ID no other window has.
func (s *Settings) NewWindow() *Window {
	id := 0
	for slices.ContainsFunc(s.Windows, func(w *Window) bool { return w.ID == id }) {
		id++
	}
	w := &Window{ID: id}
	s.Windows = slices.Insert(s.Windows, 0, w)
	s.Changed()
	return w
}

// Focused puts w first, as the window Focused last.
func (s *Settings) Focused(w *Window) {
	i := slices.Index(s.Windows, w)
	if i <= 0 {
		return
	}
	s.Windows = slices.Insert(slices.Delete(s.Windows, i, i+1), 0, w)
	s.Changed()
}

// Forget lets go of a window that closed.
func (s *Settings) Forget(w *Window) {
	s.Windows = slices.DeleteFunc(s.Windows, func(o *Window) bool { return o == w })
	s.Changed()
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(v, hi)) }

// Changed saves the settings a moment after the last change.
func (s *Settings) Changed() {
	if s.path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	data := s.Encode()
	s.timer = time.AfterFunc(400*time.Millisecond, func() { s.write(data) })
}

// Flush saves the settings now, as the app quits.
func (s *Settings) Flush() {
	if s.path == "" {
		return
	}
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
	}
	data := s.Encode()
	s.mu.Unlock()
	s.write(data)
}

// Encode is the settings as their file keeps them.
func (s *Settings) Encode() []byte {
	data, _ := json.MarshalIndent(s, "", "  ")
	return data
}

func (s *Settings) write(data []byte) {
	s.mu.Lock()
	s.written = data
	s.mu.Unlock()
	os.MkdirAll(filepath.Dir(s.path), 0o755)
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		os.Rename(tmp, s.path)
	}
}

// AddRecent puts dir first of the recent folders, of which it keeps ten.
func (s *Settings) AddRecent(dir string) {
	s.Recent = slices.DeleteFunc(s.Recent, func(d string) bool { return d == dir })
	s.Recent = slices.Insert(s.Recent, 0, dir)
	if len(s.Recent) > 10 {
		s.Recent = s.Recent[:10]
	}
}

// RecentExcept returns the recent folders but dir, and those that are not
// on the disk now, which stay in the list for when they come back, as a
// drive plugged in again.
func (s *Settings) RecentExcept(dir string) []string {
	var out []string
	for _, d := range s.Recent {
		if d == dir {
			continue
		}
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			out = append(out, d)
		}
	}
	return out
}

// ClearRecent forgets the recent folders.
func (s *Settings) ClearRecent() {
	s.Recent = nil
	s.Changed()
}

// SetPrefs makes p the preferences, as a change of the file has them,
// and returns those it replaces. A change of WordWrap counts in WrapGen.
func (s *Settings) SetPrefs(p Prefs) (old Prefs) {
	old = s.Prefs
	s.Prefs = p
	if p.WordWrap != old.WordWrap {
		s.WrapGen++
	}
	return old
}

// Watch calls changed, on a goroutine of its own, with the preferences
// of the file each time something other than the settings writes it, as
// a person saving it in an editor. A file that does not parse is let be
// until it does. stop stops watching.
func (s *Settings) Watch(changed func(Prefs)) (stop func(), err error) {
	if s.path == "" {
		return func() {}, nil
	}
	dir := filepath.Dir(s.path)
	os.MkdirAll(dir, 0o755)
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	// The folder rather than the file, which saves replace: the settings'
	// own, and most editors'.
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, err
	}
	go func() {
		var settle <-chan time.Time
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if filepath.Clean(ev.Name) == filepath.Clean(s.path) && ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
					// Once a save that writes the file in pieces is done.
					settle = time.After(80 * time.Millisecond)
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			case <-settle:
				settle = nil
				if p, ok := s.edited(); ok {
					changed(p)
				}
			}
		}
	}()
	return func() { w.Close() }, nil
}

// edited reads the preferences of the file, unless the settings wrote it
// as it is or it does not parse.
func (s *Settings) edited() (Prefs, bool) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return Prefs{}, false
	}
	s.mu.Lock()
	ours := bytes.Equal(data, s.written)
	s.mu.Unlock()
	if ours {
		return Prefs{}, false
	}
	f := Default()
	if json.Unmarshal(data, f) != nil {
		return Prefs{}, false
	}
	f.normalize()
	return f.Prefs, true
}
