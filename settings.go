package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// settings are what Mayoi remembers between launches: the layout, the
// folder and the files open in it. The window's frame is the window's
// StateKey's.
type settings struct {
	SidebarOpen  bool     `json:"sidebarOpen"`
	SidebarWidth float32  `json:"sidebarWidth"`
	PanelOpen    bool     `json:"panelOpen"`
	PanelHeight  float32  `json:"panelHeight"`
	TerminalOpen bool     `json:"terminalOpen"`
	FontSize     float32  `json:"fontSize"`
	FontFamily   string   `json:"fontFamily"`
	Folder       string   `json:"folder"`
	Recent       []string `json:"recent"`
	OpenFiles    []string `json:"openFiles"`
	ActiveFile   string   `json:"activeFile"`

	path  string
	mu    sync.Mutex
	timer *time.Timer
}

func defaultSettings() *settings {
	return &settings{
		SidebarOpen: true, SidebarWidth: 248,
		PanelOpen: false, PanelHeight: 260,
		FontSize: 13, FontFamily: "monospace",
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
	return s
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
