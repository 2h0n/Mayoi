package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/remote"
	"mayoi/internal/vfs"
	"mayoi/internal/workspace"
)

// remoteSupported is whether Mayoi reaches folders over SSH here: the
// paths of remote files are POSIX's, as on macOS and Linux.
var remoteSupported = runtime.GOOS != "windows"

// fileStamp is what tells that a file changed: its size and the time of
// its last change.
type fileStamp struct {
	size int64
	mod  time.Time
}

func stampOf(info fs.FileInfo) *fileStamp {
	if info == nil {
		return nil
	}
	return &fileStamp{info.Size(), info.ModTime()}
}

// attachRemote follows the connection to the host of a remote folder,
// and connects it; a folder of this machine lets go of the one followed.
func (a *app) attachRemote(dir string) {
	host := remote.HostOf(dir)
	if a.conn != nil && a.conn.Host == host {
		return
	}
	a.detachRemote()
	if host == "" {
		return
	}
	c := remote.For(host)
	c.Acquire()
	a.conn = c
	a.connUnsub = c.Subscribe(func() { a.post(a.connChanged) })
	a.connState, a.connErr = c.State()
	c.Connect()
}

// detachRemote stops following the connection, which ends when no other
// project uses it.
func (a *app) detachRemote() {
	c := a.conn
	if c == nil {
		return
	}
	a.connUnsub()
	a.conn, a.connUnsub, a.connState, a.connErr = nil, nil, remote.Offline, nil
	// Ending it waits on ssh.
	go c.Release()
}

// online reports whether the folder's files can be reached: always for a
// folder of this machine.
func (a *app) online() bool { return a.conn == nil || a.connState == remote.Connected }

// connChanged follows the connection as it connects or goes: back online,
// what could not be read is read now, and what changed meanwhile again.
func (a *app) connChanged() {
	if a.conn == nil {
		return
	}
	was := a.connState
	a.connState, a.connErr = a.conn.State()
	if a.connState == remote.Connected && was != remote.Connected {
		a.reconnected()
	}
}

// reconnected reads again what the folder showed, as the connection comes
// back or first connects.
func (a *app) reconnected() {
	if a.tree != nil {
		if !a.tree.Loaded(a.root) {
			a.tree.Load(a.root)
		}
		for _, dir := range a.window.Expanded {
			if d := filepath.Join(a.root, filepath.FromSlash(dir)); !a.tree.Loaded(d) {
				a.tree.Load(d)
			}
		}
		for dir := range a.files.Open.All() {
			if !a.tree.Loaded(dir) {
				a.tree.Load(dir)
			}
		}
		a.tree.Refresh()
	}
	for _, d := range a.docs {
		if !remote.Is(d.path) {
			continue
		}
		switch {
		case d.loadErr != nil:
			a.loadRemote(d)
		case !d.loading && !d.dirty():
			a.reloadDoc(d)
		}
	}
	for _, v := range a.imageViewers() {
		if remote.Is(v.path) && v.err != "" {
			a.loadImage(v)
		}
	}
	if a.git.repo == nil {
		a.git.looked = false
	}
	a.git.meta, a.git.refsMeta = time.Time{}, time.Time{}
	a.indexStale, a.ignoredStale = true, true
	a.gitChanged()
}

// reconnect connects the folder's host again.
func (a *app) reconnect() {
	if a.conn != nil {
		a.conn.Connect()
	}
}

// disconnect ends the connection to the folder's host, which keeps the
// folder and its tabs, read-only, until it connects again.
func (a *app) disconnect() {
	if c := a.conn; c != nil {
		go c.Disconnect()
	}
}

// markOffline makes the documents of a host that is not connected
// read-only. It runs once a frame.
func (a *app) markOffline() {
	for _, d := range a.docs {
		d.offline = remote.Is(d.path) && !remote.IsConnected(d.path)
	}
}

// loadRemote reads the file of a remote document in the background: the
// document is empty and read-only until it is read, or says why it could
// not be.
func (a *app) loadRemote(d *document) {
	d.loading, d.loadErr = true, nil
	path := d.path
	if !remote.IsConnected(path) {
		d.loading, d.loadErr = false, remote.ErrOffline
		return
	}
	go func() {
		info, err := vfs.Stat(path)
		var data []byte
		switch {
		case err != nil:
		case info.IsDir():
			err = errors.New("it is a folder")
		default:
			data, err = vfs.ReadFileLimit(path, maxOpenBytes)
		}
		a.post(func() { a.remoteLoaded(d, path, info, data, err) })
	}()
}

// remoteLoaded takes the text of a remote document read, or why it could
// not be: a file that cannot be opened closes its tabs, as one of this
// machine never opens; one that could not be reached says so.
func (a *app) remoteLoaded(d *document, path string, info fs.FileInfo, data []byte, err error) {
	if !slices.Contains(a.docs, d) || d.path != path {
		return
	}
	d.loading = false
	if err == nil && isBinary(data) {
		err = errBinary
	}
	if err != nil {
		if errors.Is(err, remote.ErrOffline) || !remote.IsConnected(path) {
			d.loadErr = remote.ErrOffline
			return
		}
		var re *remote.Error
		if errors.As(err, &re) && !errors.Is(err, fs.ErrNotExist) {
			// The host said something else went wrong: it may be tried again.
			d.loadErr = err
			return
		}
		switch {
		case errors.Is(err, errBinary):
			a.fail("%s is a binary file", filepath.Base(path))
		case !errors.Is(err, fs.ErrNotExist) || !d.restored:
			a.fail("Could not open %s: %v", filepath.Base(path), err)
		}
		a.dropDocTabs(d)
		return
	}
	d.loadErr = nil
	crlf := bytes.Contains(data, []byte("\r\n"))
	if crlf {
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	}
	d.buf.Set(string(data))
	d.hist = history{}
	d.markSaved()
	d.crlf, d.diskCRLF = crlf, crlf
	d.indent = detectIndent(d.buf.Snapshot(), path)
	d.measureWidest()
	d.stamp = stampOf(info)
	for _, ed := range d.views {
		if ts := ed.restoreTab; ts != nil {
			ed.restoreTab = nil
			a.restoreEditor(ed, *ts)
		}
	}
	a.find.stale = true
}

// dropDocTabs closes the tabs that show a document.
func (a *app) dropDocTabs(d *document) {
	for _, col := range slices.Clone(a.cols) {
		for _, t := range slices.Clone(col.tabs) {
			if t.ed != nil && t.ed.document == d {
				a.dropTab(t)
			}
		}
	}
	a.rememberTabs()
}

// reloadRemote makes a remote document's text the file's, as reloadDoc
// does, unless the file is as the document last read or wrote it: that
// costs a stat, not the file.
func (a *app) reloadRemote(d *document) {
	if d.loading || d.saving || !remote.IsConnected(d.path) {
		return
	}
	if d.loadErr != nil {
		a.loadRemote(d)
		return
	}
	path, old := d.path, d.stamp
	go func() {
		info, err := vfs.Stat(path)
		if err != nil || info.IsDir() {
			return
		}
		if now := stampOf(info); old != nil && *now == *old {
			return
		}
		data, err := vfs.ReadFileLimit(path, maxOpenBytes)
		if err != nil || isBinary(data) {
			return
		}
		a.post(func() {
			if !slices.Contains(a.docs, d) || d.path != path || d.loading || d.saving || d.dirty() {
				return
			}
			d.takeText(data)
			d.stamp = stampOf(info)
		})
	}()
}

// saveRemote writes a remote document in the background, then runs done,
// or failed with why it could not. A save asked for while one runs
// follows it.
func (a *app) saveRemote(d *document, done func(), failed func(error)) {
	if d.saving {
		d.resave, d.resaveDone, d.resaveFailed = true, done, failed
		return
	}
	if !remote.IsConnected(d.path) {
		failed(fmt.Errorf("not connected to %s", remote.HostOf(d.path)))
		return
	}
	d.saving = true
	snap, data := d.encode()
	crlf, path := d.crlf, d.path
	go func() {
		info, err := vfs.WriteFile(path, data, 0o644)
		a.post(func() {
			d.saving = false
			if err != nil {
				failed(err)
			} else {
				if d.path == path {
					d.wrote(snap, crlf)
					d.stamp = stampOf(info)
				}
				if a.tree != nil {
					a.tree.Stamp(path, info)
				}
				if done != nil {
					done()
				}
			}
			if d.resave {
				next, nextFailed := d.resaveDone, d.resaveFailed
				d.resave, d.resaveDone, d.resaveFailed = false, nil, nil
				if d.dirty() || next != nil {
					a.saveRemote(d, next, nextFailed)
				}
			}
		})
	}()
}

// askSavePath asks where to save a document: in the native dialog, or,
// in a remote folder, for a path on its host.
func (a *app) askSavePath(name string, done func(path string)) {
	if a.conn == nil {
		a.host.saveAs(name, done)
		return
	}
	host, root, _ := remote.Split(a.root)
	a.prompt = &prompt{title: "Save As…", hint: "on " + host, value: path.Join(root, name), done: func(p string) string {
		p = strings.TrimSpace(p)
		if !strings.HasPrefix(p, "/") {
			return "Enter a path from /."
		}
		done(remote.Path(host, p))
		return ""
	}}
}

// hostPath is how a path is copied or shown as text: a remote one as its
// host has it.
func hostPath(p string) string {
	if _, rp, ok := remote.Split(p); ok {
		return rp
	}
	return p
}

// remoteBanner shows above the editors of a remote folder while its host
// is not connected: connecting, or why not, and a button to connect.
func (a *app) remoteBanner(c *ui.Context) {
	if a.conn == nil || a.connState == remote.Connected {
		return
	}
	p := a.pal
	host := a.conn.Host
	ui.Row(c.Key("remote-banner")).Padding(6, 12).Gap(8).AlignItems(ui.Center).Background(p.hover).
		BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
		switch {
		case a.connState == remote.Connecting:
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, "Connecting to "+host+"…").FontSize(12.5).TextColor(p.text).SingleLine().Shrink(1)
		default:
			ui.Icon(c, iconServer).FontSize(13).TextColor(p.muted)
			msg := "Disconnected from " + host + ": the files are read-only until it connects again."
			if a.connErr != nil {
				msg = "Could not reach " + host + ": " + a.connErr.Error()
			}
			ui.Text(c, msg).FontSize(12.5).TextColor(p.text).Shrink(1)
			ui.Spacer(c)
			if ui.Button(c, "Reconnect").Clicked() {
				a.reconnect()
			}
		}
	})
}

// remoteStatus is the part of the status bar of a remote folder: its host
// and how it is connected, a menu of the connection.
func (a *app) remoteStatus(c *ui.Context, text func(string) ui.Element) {
	if a.conn == nil {
		return
	}
	p := a.pal
	host := a.conn.Host
	label, tip := "SSH: "+host, "Connected to "+host
	switch a.connState {
	case remote.Connecting:
		tip = "Connecting to " + host
	case remote.Offline:
		tip = "Disconnected from " + host
	}
	b := ui.ButtonBase(c.Key("status:ssh")).Height(statusBarH-6).Padding(0, 6).Gap(5).Radius(4).
		AlignItems(ui.Center).Label(tip).Tooltip(tip)
	b.Background(p.accent.Alpha(0.16))
	if b.Hovered() {
		b.Background(p.accent.Alpha(0.26))
	}
	b.Children(func() {
		if a.connState == remote.Connecting {
			ui.Spinner(c).Size(11, 11)
		} else {
			icon := ui.Icon(c, iconServer).FontSize(12).TextColor(p.statusFg)
			if a.connState == remote.Offline {
				icon.TextColor(p.danger)
			}
		}
		text(label)
	})
	b.Menu(func(m *ui.Menu) {
		if m.Item("Reconnect").Disabled(a.connState != remote.Offline).Chosen() {
			a.reconnect()
		}
		if m.Item("Disconnect").Disabled(a.connState == remote.Offline).Chosen() {
			a.disconnect()
		}
		m.Separator()
		if m.Item("Open Remote Folder…").Chosen() {
			a.post(a.openRemoteFolder)
		}
		if m.Item("Connect to Host…").Chosen() {
			a.post(a.openRemotePicker)
		}
		m.Separator()
		if m.Item("Close Remote Folder").Chosen() {
			a.post(a.closeFolder)
		}
	})
}

// remotePicker is the box that connects to a host and chooses a folder
// on it: first the hosts, of ~/.ssh/config and those connected to before,
// then, once connected, the folders of the host.
type remotePicker struct {
	open  bool
	stage pickStage
	query string
	sel   int
	moved bool
	err   string

	// hosts are the hosts to choose from, names their rows, resolved who
	// and where each signs in as, as ssh's config says, and results those
	// the query matches.
	hosts    []string
	recent   map[string]bool
	resolved map[string]string
	results  []workspace.Match
	shown    string
	stale    bool

	// conn is the connection to the host chosen, held while the box is
	// open, and unsub stops following it.
	conn  *remote.Conn
	unsub func()
	// dir is the folder listed on the host, dirs its folders, listing set
	// while it is read; checking is set while the folder to open is
	// looked at.
	dir      string
	dirs     []string
	listing  bool
	listed   string
	checking bool
}

type pickStage int

const (
	pickHost pickStage = iota
	pickConnecting
	pickFolder
)

// recentHosts are the hosts of the recent folders, the latest first.
func (a *app) recentHosts() []string {
	var hosts []string
	for _, d := range a.settings.Recent {
		if h := remote.HostOf(d); h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

// openRemotePicker shows the box of the hosts.
func (a *app) openRemotePicker() {
	if !remoteSupported {
		return
	}
	a.closeRemotePicker()
	r := &a.remotePick
	*r = remotePicker{open: true, stale: true, recent: map[string]bool{}, resolved: map[string]string{}}
	for _, h := range a.recentHosts() {
		r.hosts = append(r.hosts, h)
		r.recent[h] = true
	}
	for _, h := range remote.ConfigHosts() {
		if !slices.Contains(r.hosts, h) {
			r.hosts = append(r.hosts, h)
		}
	}
	// Who and where each signs in as, which ssh -G tells, a few at a time.
	hosts := slices.Clone(r.hosts)
	go func() {
		for _, h := range hosts {
			if s := remote.Resolve(h); s != "" {
				a.post(func() {
					if a.remotePick.resolved != nil {
						a.remotePick.resolved[h] = s
					}
				})
			}
		}
	}()
}

// openRemoteFolder shows the box of the folders of the folder's host, or
// of the hosts for a folder of this machine.
func (a *app) openRemoteFolder() {
	if a.conn == nil || a.connState != remote.Connected {
		a.openRemotePicker()
		return
	}
	a.openRemotePicker()
	a.chooseHost(a.conn.Host)
}

// closeRemotePicker hides the box, letting go of the connection it held,
// which ends unless a folder uses it.
func (a *app) closeRemotePicker() {
	r := &a.remotePick
	if !r.open {
		return
	}
	r.open = false
	a.releasePick()
	if a.focus != paneTerminal {
		a.focusColumn()
	}
}

func (a *app) releasePick() {
	r := &a.remotePick
	if r.conn != nil {
		r.unsub()
		c := r.conn
		r.conn, r.unsub = nil, nil
		go c.Release()
	}
}

// chooseHost connects to host, then lists its folders.
func (a *app) chooseHost(host string) {
	r := &a.remotePick
	host = strings.TrimSpace(host)
	if err := remote.CheckHost(host); err != nil {
		r.err = err.Error()
		return
	}
	a.releasePick()
	c := remote.For(host)
	c.Acquire()
	r.conn, r.err = c, ""
	r.unsub = c.Subscribe(func() { a.post(a.pickConnChanged) })
	r.stage, r.query = pickConnecting, ""
	c.Connect()
	a.pickConnChanged()
}

// pickConnChanged follows the connection of the box: once connected, it
// lists the home folder, or the folder of the host opened last.
func (a *app) pickConnChanged() {
	r := &a.remotePick
	if !r.open || r.conn == nil || r.stage != pickConnecting {
		return
	}
	switch s, err := r.conn.State(); s {
	case remote.Connected:
		start := r.conn.Info().Home
		for _, d := range a.settings.Recent {
			if host, rp, ok := remote.Split(d); ok && host == r.conn.Host && d != a.root {
				start = rp
				break
			}
		}
		r.stage, r.sel = pickFolder, 0
		r.query = strings.TrimSuffix(start, "/") + "/"
		r.listed = ""
	case remote.Offline:
		r.stage = pickHost
		r.query = r.conn.Host
		if err != nil {
			r.err = err.Error()
		} else {
			r.err = "Could not connect to " + r.conn.Host
		}
		a.releasePick()
	}
}

// pickDir is the folder the query of the box lists, and the start of the
// name of the folder in it typed so far.
func pickDir(query string) (dir, prefix string) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "/", ""
	}
	if strings.HasSuffix(q, "/") {
		return path.Clean(q), ""
	}
	return path.Dir(path.Clean(q)), path.Base(q)
}

// listPick reads the folders of the folder the query names, in the
// background, when it changed.
func (a *app) listPick() {
	r := &a.remotePick
	dir, _ := pickDir(r.query)
	if r.conn == nil || r.listing || dir == r.listed || !strings.HasPrefix(dir, "/") {
		return
	}
	r.listing, r.listed = true, dir
	c := r.conn
	go func() {
		entries, err := c.ReadDir(dir)
		a.post(func() {
			r.listing = false
			if r.conn != c || r.listed != dir {
				return
			}
			r.dir, r.dirs = dir, nil
			if err != nil {
				return
			}
			for _, e := range entries {
				if e.Dir {
					r.dirs = append(r.dirs, e.Name)
				}
			}
			slices.SortFunc(r.dirs, func(x, y string) int {
				// Hidden folders last.
				if hx, hy := strings.HasPrefix(x, "."), strings.HasPrefix(y, "."); hx != hy {
					if hx {
						return 1
					}
					return -1
				}
				return strings.Compare(strings.ToLower(x), strings.ToLower(y))
			})
		})
	}()
}

// pickRows are the rows of the folders of the box: the folder typed, to
// open it, then the folders in it whose names start as the query's last
// part does.
func (a *app) pickRows() []string {
	r := &a.remotePick
	dir, prefix := pickDir(r.query)
	rows := []string{""}
	if r.dir != dir {
		return rows
	}
	for _, name := range r.dirs {
		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) &&
			(prefix != "" || !strings.HasPrefix(name, ".")) {
			rows = append(rows, name)
		}
	}
	return rows
}

// pickFolderRow goes into the folder of a row, or opens the folder typed
// for the first, as it is there.
func (a *app) pickFolderRow(i int) {
	r := &a.remotePick
	rows := a.pickRows()
	if i < 0 || i >= len(rows) {
		return
	}
	if i > 0 {
		dir, _ := pickDir(r.query)
		r.query = path.Join(dir, rows[i]) + "/"
		r.sel, r.err = 0, ""
		return
	}
	target := path.Clean(strings.TrimSpace(r.query))
	if !strings.HasPrefix(target, "/") {
		r.err = "Enter a path from /."
		return
	}
	if r.checking || r.conn == nil {
		return
	}
	r.checking, r.err = true, ""
	c := r.conn
	go func() {
		info, err := c.Stat(target)
		a.post(func() {
			r.checking = false
			if r.conn != c || !r.open {
				return
			}
			switch {
			case errors.Is(err, fs.ErrNotExist):
				r.err = "There is no folder " + target + "."
			case err != nil:
				r.err = err.Error()
			case !info.IsDir():
				r.err = target + " is not a folder."
			default:
				a.openRemote(remote.Path(c.Host, target))
			}
		})
	}()
}

// openRemote opens a remote folder as Open Folder opens one: in the
// window when it shows nothing, else in a window, or a project tab, of
// its own.
func (a *app) openRemote(dir string) {
	// The folder holds the connection before the box lets go of it.
	if a.blank() {
		a.switchFolder(dir)
	} else {
		a.host.newWindow(dir)
	}
	a.closeRemotePicker()
}

// remotePickerView shows the box of the hosts, or of the folders of the
// host chosen, while it is open.
func (a *app) remotePickerView(c *ui.Context) {
	r := &a.remotePick
	if !r.open {
		return
	}
	p := a.pal
	if r.stage == pickHost && (r.stale || r.shown != r.query) {
		r.results = workspace.Fuzzy(r.hosts, strings.TrimSpace(r.query), 60)
		if strings.TrimSpace(r.query) == "" {
			r.results = r.results[:0]
			for i := range r.hosts {
				r.results = append(r.results, workspace.Match{Index: i})
			}
		}
		r.shown, r.stale, r.sel = r.query, false, 0
	}
	if r.stage == pickFolder {
		a.listPick()
	}
	// The host typed, when it is none of those listed, is the first row.
	typed := strings.TrimSpace(r.query)
	extra := r.stage == pickHost && typed != "" && !slices.Contains(r.hosts, typed)
	var rows []string
	if r.stage == pickFolder {
		rows = a.pickRows()
	}
	count := len(r.results)
	switch {
	case r.stage == pickFolder:
		count = len(rows)
	case extra:
		count++
	}
	r.sel = min(r.sel, max(count-1, 0))
	choose := func(i int) {
		switch r.stage {
		case pickHost:
			if extra {
				if i == 0 {
					a.chooseHost(typed)
					return
				}
				i--
			}
			if i >= 0 && i < len(r.results) {
				a.chooseHost(r.hosts[r.results[i].Index])
			} else if typed != "" {
				a.chooseHost(typed)
			}
		case pickFolder:
			a.pickFolderRow(i)
		}
	}
	title, placeholder := "Connect to Host", "Choose or type a host: [user@]host[:port]"
	if r.stage != pickHost && r.conn != nil {
		title, placeholder = "Open Folder on "+r.conn.Host, "Type the path of a folder"
	}
	ui.Overlay(c, func() {
		back := a.dialogBack(c, "remote")
		back.Children(func() {
			box := a.floating(c, ui.Column(c).Width(560).MaxWidthPercent(92).Margin(titleBarH+8, 0, 0, 0).Radius(10), true).Clip().
				Role(ui.RoleDialog).Label(title)
			box.Children(func() {
				ui.Row(c).Padding(10, 14).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.border).Children(func() {
					icon := iconServer
					if r.stage == pickFolder {
						icon = iconFolderOpen
					}
					ui.Icon(c, icon).FontSize(15).TextColor(p.muted)
					if r.stage == pickConnecting {
						ui.Spinner(c).Size(13, 13)
						ui.Text(c, "Connecting to "+r.conn.Host+"…").FontSize(14).TextColor(p.text).Grow(1)
						return
					}
					in := a.textKeys(ui.TextInputBase(c.Key(fmt.Sprintf("remote-input-%d", r.stage)), &r.query)).Grow(1).FontSize(14).AutoFocus().
						Placeholder(placeholder).Label(title)
					if in.Changed() {
						r.err = ""
					}
					if in.Shortcut(0, ui.KeyDown) {
						r.sel, r.moved = min(r.sel+1, count-1), true
					}
					if in.Shortcut(0, ui.KeyUp) {
						r.sel, r.moved = max(r.sel-1, 0), true
					}
					sel := r.sel
					if r.stage == pickFolder && in.Shortcut(0, ui.KeyTab) {
						// Into the folder chosen, or the only one that matches.
						if sel == 0 && len(rows) == 2 {
							sel = 1
						}
						if sel > 0 {
							a.post(func() { a.pickFolderRow(sel) })
						}
					}
					if in.Submitted() {
						a.post(func() { choose(sel) })
					}
					if r.checking || r.listing {
						ui.Spinner(c).Size(12, 12)
					}
				})
				if r.err != "" {
					ui.Text(c, r.err).FontSize(12.5).TextColor(p.danger).Padding(8, 16, 0)
				}
				switch r.stage {
				case pickHost:
					a.pickHostRows(c, extra, typed, choose)
				case pickFolder:
					ui.Scroll(c.Key("remote-folders")).MaxHeight(380).Padding(6).Children(func() {
						dir, _ := pickDir(r.query)
						for i, name := range rows {
							label, icon := "Open "+path.Clean(strings.TrimSpace(r.query)), iconFolderOpen
							if i > 0 {
								label, icon = path.Join(dir, name), iconFolder
							}
							a.pickRow(c, i, "remote-dir:"+label, icon, label, "", choose)
						}
					})
				}
				r.moved = false
				switch r.stage {
				case pickFolder:
					a.quickFooter(c, false, "open or go in", "tab goes into a folder", false)
				case pickHost:
					a.quickFooter(c, false, "connect", "Keys and the SSH agent sign in", false)
				}
			})
			if box.PressedOutside() || back.OverlayShortcut(0, ui.KeyEscape) {
				a.closeRemotePicker()
			}
		})
	})
}

// pickHostRows are the rows of the hosts of the box: the host typed, then
// those the query matches.
func (a *app) pickHostRows(c *ui.Context, extra bool, typed string, choose func(int)) {
	r := &a.remotePick
	p := a.pal
	if !extra && len(r.results) == 0 {
		msg := "No hosts in ~/.ssh/config: type one, as user@host"
		ui.Text(c, msg).FontSize(13).TextColor(p.muted).Padding(12, 16)
		return
	}
	ui.Scroll(c.Key("remote-hosts")).MaxHeight(380).Padding(6).Children(func() {
		i := 0
		if extra {
			a.pickRow(c, 0, "remote-typed", iconPlus, "Connect to "+typed, "", choose)
			i++
		}
		for _, m := range r.results {
			h := r.hosts[m.Index]
			detail := r.resolved[h]
			if r.recent[h] {
				detail = strings.TrimSpace("recent  " + detail)
			}
			row := i
			a.pickRow(c, row, "remote-host:"+h, iconServer, h, detail, choose)
			i++
		}
	})
}

// pickRow is a row of the box, chosen by click or Enter.
func (a *app) pickRow(c *ui.Context, i int, key string, icon *ui.SVG, label, detail string, choose func(int)) {
	r := &a.remotePick
	p := a.pal
	row := ui.ButtonBase(c.Key(key)).Padding(5, 10).Gap(10).Radius(6).Justify(ui.Start).AlignItems(ui.Center).Label(label)
	switch {
	case i == r.sel:
		row.Background(p.active)
	case row.Hovered():
		row.Background(p.hover)
	}
	if i == r.sel && r.moved {
		row.ScrollIntoView()
	}
	if row.Clicked() {
		a.post(func() { choose(i) })
	}
	row.Children(func() {
		ui.Icon(c, icon).FontSize(15).TextColor(p.accent)
		ui.Text(c, label).FontSize(13).TextColor(p.text).SingleLine().Shrink(0)
		if detail != "" {
			ui.Text(c, detail).FontSize(12).TextColor(p.faint).SingleLine().Shrink(1)
		}
	})
}

// loadingView shows in place of the editor of a remote file not read yet:
// that it is being read, or why it could not be, with a button to try
// again.
func (a *app) loadingView(c *ui.Context, d *editor) {
	p := a.pal
	ui.Column(c).Fill().Center().Gap(10).Background(p.editor).Padding(24).Children(func() {
		if d.loading {
			ui.Spinner(c).Size(16, 16)
			ui.Text(c, "Reading "+d.name()+"…").FontSize(13).TextColor(p.muted)
			return
		}
		ui.Icon(c, iconServer).FontSize(20).TextColor(p.muted)
		msg := "Could not read " + d.name() + ": " + d.loadErr.Error()
		if errors.Is(d.loadErr, remote.ErrOffline) {
			msg = d.name() + " shows once " + remote.HostOf(d.path) + " is connected."
		}
		ui.Text(c, msg).FontSize(13).TextColor(p.muted).MaxWidth(420)
		if !errors.Is(d.loadErr, remote.ErrOffline) && ui.Button(c.Key("retry:"+d.path), "Try Again").Clicked() {
			a.loadRemote(d.document)
		}
	})
}

// shutdown stops watching the folder and lets go of its connection, as
// the project closes for good.
func (a *app) shutdown() {
	if a.tree != nil {
		a.tree.Close()
	}
	a.detachRemote()
	a.closeRemotePicker()
}
