package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mayoi/internal/linediff"
)

// gitMarks are the lines of a document changed since Git's last commit,
// which the gutter marks. The text of the file at HEAD is read, and the
// lines compared with it, in the background.
type gitMarks struct {
	base    []string // the lines at HEAD; nil for a file Git has none of
	loaded  bool
	loading bool
	stale   bool // HEAD may have moved: read the base again

	version uint64 // the version of the text the marks are of
	diffed  bool   // the marks are of the base read last
	running bool
	started time.Time // when the last comparison started
	waiting bool      // a comparison waits for the pause between them
	marks   []linediff.Kind
}

// diffPause is the least time between two comparisons of a document's
// lines, so that typing in a long one does not keep a core comparing.
const diffPause = 150 * time.Millisecond

// refreshGit reads a document's base again, when it may have changed, and
// compares the lines once its text changed.
func (a *app) refreshGit(d *document) {
	g := &d.git
	if d.path == "" {
		return
	}
	if (!g.loaded || g.stale) && !g.loading {
		g.loading, g.stale = true, false
		dir, name := filepath.Dir(d.path), filepath.Base(d.path)
		go func() {
			out, err := exec.Command("git", "-C", dir, "show", "HEAD:./"+name).Output()
			a.post(func() {
				g.loading, g.loaded, g.diffed = false, true, false
				// The marks of the old base show until those of the new.
				g.base = nil
				if err == nil {
					text := strings.ReplaceAll(string(out), "\r\n", "\n")
					g.base = strings.Split(text, "\n")
				} else {
					g.marks = nil
				}
			})
		}()
	}
	snap := d.buf.Snapshot()
	if !g.loaded || g.base == nil || g.running || g.waiting || g.diffed && g.version == snap.Version() {
		return
	}
	if wait := diffPause - time.Since(g.started); g.diffed && wait > 0 {
		g.waiting = true
		time.AfterFunc(wait, func() { a.post(func() { g.waiting = false }) })
		return
	}
	g.running, g.started = true, time.Now()
	base := g.base
	go func() {
		marks := linediff.Lines(base, strings.Split(snap.String(), "\n"))
		a.post(func() {
			g.running = false
			if g.loaded && sameLines(g.base, base) {
				g.marks, g.version, g.diffed = marks, snap.Version(), true
			}
		})
	}()
}

// sameLines reports whether two bases are one.
func sameLines(a, b []string) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// gitChanged has the documents read their bases again, as when a commit
// may have been made.
func (a *app) gitChanged() {
	for _, d := range a.docs {
		d.git.stale = true
	}
}
