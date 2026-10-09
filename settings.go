package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// settings are what Mayoi remembers between launches: the layout, and
// the windows with the folder and the files open in each. A window's
// frame is its StateKey's.
type settings struct {
	SidebarOpen  bool    `json:"sidebarOpen"`
	SidebarWidth float32 `json:"sidebarWidth"`
	PanelHeight  float32 `json:"panelHeight"`
	FontSize     float32 `json:"fontSize"`
	FontFamily   string  `json:"fontFamily"`
	// AutoSave saves changed files a moment after an edit, and as the
	// keyboard leaves them.
	AutoSave bool `json:"autoSave"`
	// Appearance is "light", "dark", or "" to follow the system.
	Appearance string   `json:"appearance,omitempty"`
	Recent     []string `json:"recent"`
	// Windows are the windows open, the one focused last first.
	Windows []*windowState `json:"windows,omitempty"`

	// Folder, Columns, Column, PanelOpen and TerminalOpen are the window
	// as a version with one window kept it, and OpenFiles and ActiveFile
	// its tabs as a version without columns kept them: loadSettings
	// makes a window of them.
	Folder       string        `json:"folder,omitempty"`
	Columns      []columnState `json:"columns,omitempty"`
	Column       int           `json:"column,omitempty"`
	PanelOpen    bool          `json:"panelOpen,omitempty"`
	TerminalOpen bool          `json:"terminalOpen,omitempty"`
	OpenFiles    []string      `json:"openFiles,omitempty"`
	ActiveFile   string        `json:"activeFile,omitempty"`

	path  string
	mu    sync.Mutex
	timer *time.Timer
}

// windowState is a window as the settings keep it: its folder, the
// columns of its editor area and what their tabs show, and Column the one
// that had the keyboard.
type windowState struct {
	// ID names the window's frame, which the next start gives it back.
	ID           int           `json:"id"`
	Folder       string        `json:"folder,omitempty"`
	Columns      []columnState `json:"columns,omitempty"`
	Column       int           `json:"column,omitempty"`
	PanelOpen    bool          `json:"panelOpen,omitempty"`
	TerminalOpen bool          `json:"terminalOpen,omitempty"`
}

// columnState is a column of the editor area, as the settings keep it.
type columnState struct {
	Tabs   []tabState `json:"tabs"`
	Active int        `json:"active"`
	Weight float32    `json:"weight"`
}

// tabState is a tab: the file it shows, or a terminal, which the next
// start opens anew.
type tabState struct {
	File     string `json:"file,omitempty"`
	Terminal bool   `json:"terminal,omitempty"`
	// Preview is set for an SVG shown as a picture rather than as text.
	Preview bool `json:"preview,omitempty"`
}

func defaultSettings() *settings {
	return &settings{
		SidebarOpen: true, SidebarWidth: 248, PanelHeight: 260,
		FontSize: 13, FontFamily: "monospace",
		AutoSave: true,
	}
}

// loadSettings reads the settings at path, the defaults where it has none.
func loadSettings(path string) *settings {
	s := defaultSettings()
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, s)
	}
	s.path = path
	s.SidebarWidth = clamp(s.SidebarWidth, 160, 600)
	s.PanelHeight = clamp(s.PanelHeight, 100, 2000)
	s.FontSize = clamp(s.FontSize, 9, 28)
	if s.FontFamily == "" {
		s.FontFamily = "monospace"
	}
	s.migrate()
	return s
}

// migrate makes a window of the one an older version kept.
func (s *settings) migrate() {
	if len(s.Windows) > 0 || s.Folder == "" && len(s.Columns) == 0 && len(s.OpenFiles) == 0 {
		return
	}
	w := &windowState{Folder: s.Folder, Columns: s.Columns, Column: s.Column, PanelOpen: s.PanelOpen, TerminalOpen: s.TerminalOpen}
	if len(w.Columns) == 0 && len(s.OpenFiles) > 0 {
		// The files of one column.
		cs := columnState{Active: -1, Weight: 1}
		for _, p := range s.OpenFiles {
			if p == s.ActiveFile {
				cs.Active = len(cs.Tabs)
			}
			cs.Tabs = append(cs.Tabs, tabState{File: p})
		}
		w.Columns, w.Column = []columnState{cs}, 0
	}
	s.Windows = []*windowState{w}
	s.Folder, s.Columns, s.Column, s.PanelOpen, s.TerminalOpen = "", nil, 0, false, false
	s.OpenFiles, s.ActiveFile = nil, ""
}

// newWindow keeps a new window, focused first, and names its frame by
// the first ID no other window has.
func (s *settings) newWindow() *windowState {
	id := 0
	for slices.ContainsFunc(s.Windows, func(w *windowState) bool { return w.ID == id }) {
		id++
	}
	w := &windowState{ID: id}
	s.Windows = slices.Insert(s.Windows, 0, w)
	s.changed()
	return w
}

// focused puts w first, as the window focused last.
func (s *settings) focused(w *windowState) {
	i := slices.Index(s.Windows, w)
	if i <= 0 {
		return
	}
	s.Windows = slices.Insert(slices.Delete(s.Windows, i, i+1), 0, w)
	s.changed()
}

// forget lets go of a window that closed.
func (s *settings) forget(w *windowState) {
	s.Windows = slices.DeleteFunc(s.Windows, func(o *windowState) bool { return o == w })
	s.changed()
}

func clamp(v, lo, hi float32) float32 { return max(lo, min(v, hi)) }

// changed saves the settings a moment after the last change.
func (s *settings) changed() {
	if s.path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	data := s.encode()
	s.timer = time.AfterFunc(400*time.Millisecond, func() { s.write(data) })
}

// flush saves the settings now, as the app quits.
func (s *settings) flush() {
	if s.path == "" {
		return
	}
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
	}
	data := s.encode()
	s.mu.Unlock()
	s.write(data)
}

func (s *settings) encode() []byte {
	data, _ := json.MarshalIndent(s, "", "  ")
	return data
}

func (s *settings) write(data []byte) {
	os.MkdirAll(filepath.Dir(s.path), 0o755)
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		os.Rename(tmp, s.path)
	}
}

func (s *settings) addRecent(dir string) {
	s.Recent = slices.DeleteFunc(s.Recent, func(d string) bool { return d == dir })
	s.Recent = slices.Insert(s.Recent, 0, dir)
	if len(s.Recent) > 10 {
		s.Recent = s.Recent[:10]
	}
}
