// Mayoi is a small, quick code editor: a file tree, tabs of editors and a
// terminal panel, drawn by MyGo's native UI.
package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// desktop is the host of a window of the app: native dialogs, the trash.
type desktop struct {
	a   *app
	win *mygo.Window
}

func (h *desktop) openFolder(done func(string)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: h.win, Title: "Open Folder", ButtonLabel: "Open", Directory: true, CreateDirectories: true,
		})
		if err == nil && len(paths) > 0 {
			h.a.post(func() { done(paths[0]) })
		}
	}()
}

func (h *desktop) saveAs(name string, done func(string)) {
	dir := h.a.root
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: h.win, Title: "Save As", DefaultPath: filepath.Join(dir, name), CreateDirectories: true,
		})
		if err == nil && path != "" {
			h.a.post(func() { done(path) })
		}
	}()
}

func (h *desktop) confirmClose(name string, done func(int)) {
	go func() {
		res, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: h.win, Type: mygo.MessageWarning,
			Message: "Do you want to save the changes you made to " + name + "?",
			Detail:  "Your changes will be lost if you don't save them.",
			Buttons: []string{"Save", "Don't Save", "Cancel"}, DefaultButton: 0, CancelButton: 2,
		})
		choice := 2
		if err == nil {
			choice = res.Button
		}
		h.a.post(func() { done(choice) })
	}()
}

func (h *desktop) trash(path string) error { return mygo.Shell.TrashItem(path) }
func (h *desktop) reveal(path string)      { mygo.Shell.ShowItemInFolder(path) }
func (h *desktop) setTitle(title string)   { h.win.SetTitle(title) }
func (h *desktop) closeWindow()            { h.win.Close() }
func (h *desktop) setAppearance(appearance string) {
	mygo.Theme.SetSource(mygo.ThemeSource(appearance))
}

// settingsPath is where the settings are kept.
func settingsPath() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "settings.json")
}

func main() {
	mygo.App.SetName("Mayoi")
	mygo.App.WhenReady(func() {
		s := loadSettings(settingsPath())
		mygo.Theme.SetSource(mygo.ThemeSource(s.Appearance))
		// `mayoi <folder or file>` opens it.
		var openPath string
		if len(os.Args) > 1 {
			if p, err := filepath.Abs(os.Args[1]); err == nil {
				if info, err := os.Stat(p); err == nil {
					if info.IsDir() {
						s.Folder, s.OpenFiles, s.ActiveFile = p, nil, ""
					} else {
						openPath = p
					}
				}
			}
		}
		h := &desktop{}
		a := newApp(h, s)
		h.a = a
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "Mayoi", Width: 1280, Height: 820, MinWidth: 640, MinHeight: 420,
			StateKey:             "main",
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: 14, Y: 13},
			BackgroundColor:      "#131317",
			Content:              ui.View(a.view),
		})
		h.win = win
		if openPath != "" {
			a.post(func() { a.openFile(openPath, true) })
		}
		mygo.App.SetMenu(a.menu(win))
		if out := os.Getenv("MAYOI_SNAPSHOT"); out != "" {
			// For checking the real window: capture it once it settles, and
			// quit.
			go func() {
				// MAYOI_SNAPSHOT_RUN names commands to run first.
				for _, id := range strings.Fields(os.Getenv("MAYOI_SNAPSHOT_RUN")) {
					time.Sleep(600 * time.Millisecond)
					a.post(func() { a.run(id) })
				}
				time.Sleep(2500 * time.Millisecond)
				if png, err := win.CapturePage(); err == nil {
					os.WriteFile(out, png, 0o644)
				}
				mygo.App.Quit()
			}()
		}

		// Closing the window with unsaved changes asks about each first.
		closing := false
		win.OnClose(func(e *mygo.CloseEvent) {
			if closing || !a.anyDirty() {
				return
			}
			e.PreventDefault()
			a.post(func() {
				a.closeAll(func() {
					closing = true
					win.Close()
				})
			})
			win.Invalidate()
		})
		// Back from another app, which may have committed.
		win.OnFocus(func() { a.post(a.gitChanged) })
		win.OnClosed(func() {
			a.closeTerminals()
			s.flush()
			mygo.App.Quit()
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *app) anyDirty() bool {
	for _, d := range a.docs {
		if d.dirty() {
			return true
		}
	}
	return false
}
