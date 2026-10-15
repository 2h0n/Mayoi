package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/remote"
)

// transfer is a copy to or from a host running in the background, which
// the status bar shows with how far it got.
type transfer struct {
	label    string
	progress *remote.Progress
}

// startTransfer shows a transfer in the status bar until endTransfer.
func (a *app) startTransfer(label string) *transfer {
	x := &transfer{label: label, progress: &remote.Progress{}}
	a.transfers = append(a.transfers, x)
	return x
}

func (a *app) endTransfer(x *transfer) {
	if i := slices.Index(a.transfers, x); i >= 0 {
		a.transfers = slices.Delete(a.transfers, i, i+1)
	}
}

// transferText is what the status bar says of the transfers: the first,
// with how much of it is done, and how many more there are.
func (a *app) transferText() string {
	if len(a.transfers) == 0 {
		return ""
	}
	x := a.transfers[0]
	s := x.label
	done, total := x.progress.Done(), x.progress.Total()
	switch {
	case total > 0:
		s += fmt.Sprintf(" %d%%", min(done*100/total, 100))
	case done > 0:
		s += " " + byteSize(done)
	}
	if n := len(a.transfers) - 1; n > 0 {
		s += fmt.Sprintf(" (+%d)", n)
	}
	return s
}

// transferStatus is the part of the status bar of the transfers running,
// drawn again a few times a second while they run.
func (a *app) transferStatus(c *ui.Context, text func(string) ui.Element) {
	label := a.transferText()
	if label == "" {
		return
	}
	ui.Row(c.Key("status:transfer")).Height(statusBarH).Padding(0, 6).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Spinner(c).Size(11, 11)
		text(label)
	})
	time.AfterFunc(250*time.Millisecond, a.invalidate)
}

// byteSize is a count of bytes as people read it: 12 KB, 3.4 MB.
func byteSize(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%d KB", n/1000)
	case n < 1000*1000*1000:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	}
	return fmt.Sprintf("%.2f GB", float64(n)/1e9)
}

// A terminal of a host runs its programs there, which cannot read the
// files of this machine: a paste of files, of the Finder or of the
// explorer, or of an image alone, as a screenshot for Claude Code,
// uploads them to a temporary folder of the host and pastes their paths
// there. Files of the host itself paste their paths on it.

// sessionConn returns the connection of a terminal of a host, nil for
// one of this machine.
func sessionConn(s *session) *remote.Conn {
	if c, _, ok := remote.Lookup(s.dir); ok {
		return c
	}
	return nil
}

// terminalPaste takes a paste into a terminal of a host, as OnPaste does,
// when the clipboard has files or an image alone.
func (a *app) terminalPaste(s *session, text string) bool {
	if sessionConn(s) == nil {
		return false
	}
	if srcs := a.clipboardSources(); len(srcs) > 0 {
		a.pastePaths(s, srcs)
		return true
	}
	if text != "" {
		return false
	}
	img := a.host.clipboardImage()
	if len(img) == 0 {
		return false
	}
	// A file of its own, named as a screenshot, which programs that take
	// images, as Claude Code, read when its path is pasted.
	dir, err := os.MkdirTemp("", "mayoi-image-")
	if err != nil {
		a.fail("Could not paste the image: %v", err)
		return true
	}
	p := filepath.Join(dir, "image-"+time.Now().Format("20060102-150405")+".png")
	if err := os.WriteFile(p, img, 0o644); err != nil {
		os.RemoveAll(dir)
		a.fail("Could not paste the image: %v", err)
		return true
	}
	a.uploadPaths(s, []string{p}, func() { os.RemoveAll(dir) })
	return true
}

// uploadPaths copies files to a temporary folder of the host of a
// terminal, in the background, and pastes their paths on the host into
// it, then runs done.
func (a *app) uploadPaths(s *session, srcs []string, done func()) {
	c := sessionConn(s)
	if !c.Connected() {
		a.fail("Could not paste: %s is not connected", c.Host)
		if done != nil {
			done()
		}
		return
	}
	label := "Uploading " + filepath.Base(srcs[0]) + " to " + c.Host + "…"
	if len(srcs) > 1 {
		label = fmt.Sprintf("Uploading %d items to %s…", len(srcs), c.Host)
	}
	x := a.startTransfer(label)
	go func() {
		got, err := toHost(c, srcs, "", x.progress)
		if done != nil {
			done()
		}
		a.post(func() {
			a.endTransfer(x)
			if err != nil {
				a.fail("Could not paste: %v", err)
				return
			}
			if slices.Contains(a.terms, s) {
				s.term.Paste(quotePaths(got))
			}
		})
	}()
}

// quotePaths are paths as a terminal takes them typed: quoted for the
// shell, with a space after each.
func quotePaths(paths []string) string {
	quoted := make([]string, len(paths))
	for i, p := range paths {
		quoted[i] = shellQuote(p)
	}
	return strings.Join(quoted, " ") + " "
}
