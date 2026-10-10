package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/git"
)

// gitState is what the window knows of the Git work tree its folder is
// in: the files that differ from HEAD, the branch, and the commands
// running.
type gitState struct {
	repo *git.Repo // nil outside a work tree
	// looked is set once the folder was looked up, looking while it is.
	looked, looking bool

	status   *git.Status
	branches []string // the local branches, read with the status
	// entries are the files that differ, by path; dirs the folders above
	// them, with the letter of the one that matters most.
	entries map[string]git.Entry
	dirs    map[string]byte
	// stale has the next frame read the status again, unless running;
	// read is when it was read last.
	stale, running bool
	read           time.Time

	// meta is when the files Git changes as HEAD moves last changed, and
	// checked when the app last looked.
	meta, checked time.Time

	// busy names the command running in the background, as "Pushing…",
	// which the status bar shows; "" while none.
	busy string
	// message is the commit message being written, and amend set while
	// it is to amend the last commit.
	message string
	amend   bool
}

// statusEvery is how often the status is read again on its own, for the
// files changed in folders the tree does not watch.
const statusEvery = 5 * time.Second

// lookUpRepo finds the work tree of the folder in the background.
func (a *app) lookUpRepo() {
	g := &a.git
	if a.root == "" || g.looked || g.looking {
		return
	}
	g.looking = true
	root := a.root
	go func() {
		r, err := git.Open(root)
		a.post(func() {
			if a.root != root {
				return
			}
			g.looking, g.looked = false, true
			if err == nil {
				g.repo, g.stale = r, true
			}
		})
	}()
}

// refreshGit runs once a frame: it looks up the work tree, notices HEAD
// moving, and reads the status again when it may have changed.
func (a *app) refreshGit() {
	a.lookUpRepo()
	g := &a.git
	if g.repo == nil {
		return
	}
	if time.Since(g.checked) >= 1500*time.Millisecond {
		g.checked = time.Now()
		if t := gitMeta(g.repo); !t.Equal(g.meta) {
			if !g.meta.IsZero() {
				a.gitChanged()
			}
			g.meta = t
		}
	}
	if g.running || !g.stale && time.Since(g.read) < statusEvery {
		return
	}
	g.running, g.stale = true, false
	r := g.repo
	go func() {
		st, err := r.Status()
		branches, _ := r.Branches()
		a.post(func() {
			g.running, g.read = false, time.Now()
			if g.repo != r {
				return
			}
			if err != nil {
				return
			}
			a.setStatus(st)
			g.branches = branches
		})
	}()
}

// gitMeta returns when the files that HEAD moving changes last changed:
// the index, HEAD and its log, and what a fetch writes.
func gitMeta(r *git.Repo) time.Time {
	var t time.Time
	for _, p := range []string{
		filepath.Join(r.Dir, "index"), filepath.Join(r.Dir, "HEAD"), filepath.Join(r.Dir, "logs", "HEAD"),
		filepath.Join(r.Common, "FETCH_HEAD"),
	} {
		if info, err := os.Stat(p); err == nil && info.ModTime().After(t) {
			t = info.ModTime()
		}
	}
	return t
}

// setStatus takes a status read, and the folders above its files.
func (a *app) setStatus(st *git.Status) {
	g := &a.git
	g.status = st
	g.entries = make(map[string]git.Entry, len(st.Entries))
	g.dirs = map[string]byte{}
	for _, e := range st.Entries {
		g.entries[e.Path] = e
		l := e.Letter()
		for d := filepath.Dir(e.Path); len(d) >= len(g.repo.Root); d = filepath.Dir(d) {
			if old, ok := g.dirs[d]; ok && letterRank(old) >= letterRank(l) {
				break
			}
			g.dirs[d] = l
			if d == filepath.Dir(d) {
				break
			}
		}
	}
}

// letterRank orders the letters of files by how much a folder above them
// shows it: a conflict before a change before a file added.
func letterRank(l byte) int {
	switch l {
	case 'C':
		return 4
	case 'M', 'R', 'T', 'D':
		return 3
	case 'A':
		return 2
	case 'U':
		return 1
	}
	return 0
}

// branch is the branch checked out, or the commit when HEAD is detached,
// "" outside a work tree.
func (a *app) branch() string {
	st := a.git.status
	switch {
	case st == nil:
		return ""
	case st.Branch != "":
		return st.Branch
	case len(st.Head) >= 7:
		return st.Head[:7]
	}
	return ""
}

// gitChanged has the documents read their bases again, and the status be
// read again, as when a commit may have been made.
func (a *app) gitChanged() {
	for _, d := range a.docs {
		d.git.HeadMoved()
	}
	a.diffsChanged()
	a.git.stale = true
}

// gitDo runs a command on the work tree in the background, naming it
// busy while it runs when busy is set, then reads the status again and
// calls then, unless it failed: then it says why.
func (a *app) gitDo(busy string, fn func(r *git.Repo) error, then func()) {
	g := &a.git
	r := g.repo
	if r == nil {
		return
	}
	if busy != "" {
		if g.busy != "" {
			return
		}
		g.busy = busy
	}
	go func() {
		err := fn(r)
		a.post(func() {
			if busy != "" {
				g.busy = ""
			}
			a.gitChanged()
			if err != nil {
				a.gitFailed(err)
				return
			}
			if then != nil {
				then()
			}
		})
	}()
}

// gitFailed says why a git command failed.
func (a *app) gitFailed(err error) {
	msg := err.Error()
	if len(msg) > 400 {
		msg = msg[:400] + "…"
	}
	if git.NeedsAuth(err) {
		msg += "\nGit needs to sign in: run the command in the terminal once."
	}
	a.fail("%s", msg)
}

// fetch fetches from the remotes, which tells how far behind the branch
// is.
func (a *app) fetch() {
	a.gitDo("Fetching…", func(r *git.Repo) error { return r.Fetch() }, nil)
}

// pull brings in the upstream's commits, when the branch has none of its
// own that it lacks.
func (a *app) pull() {
	a.gitDo("Pulling…", func(r *git.Repo) error { return r.Pull() }, nil)
}

// push pushes the branch to its upstream, or to a remote, which becomes
// its upstream.
func (a *app) push() {
	st := a.git.status
	if st == nil || st.Branch == "" {
		a.fail("Check out a branch to push.")
		return
	}
	branch, upstream := st.Branch, st.Upstream != ""
	a.gitDo("Pushing…", func(r *git.Repo) error { return r.Push(branch, upstream) }, nil)
}

// switchBranch checks out a branch, or makes it first when create is set,
// once the documents are saved: Git keeps the changes of the work tree
// when they do not clash with the branch's.
func (a *app) switchBranch(branch string, create bool) {
	for _, d := range a.docs {
		if d.dirty() && d.path != "" && a.git.repo != nil && a.git.repo.Contains(d.path) {
			a.fail("Save the changes of %s before switching branches.", d.name())
			return
		}
	}
	a.gitDo("Switching to "+branch+"…", func(r *git.Repo) error { return r.Switch(branch, create) }, nil)
}

// askNewBranch asks for the name of a branch to make and check out.
func (a *app) askNewBranch() {
	if a.git.repo == nil {
		return
	}
	a.prompt = &prompt{title: "Create Branch…", hint: "from " + a.branch(), done: func(name string) string {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, " ~^:?*[\\") {
			return "Enter a branch name without spaces."
		}
		if slices.Contains(a.git.branches, name) {
			return "A branch with that name exists."
		}
		a.switchBranch(name, true)
		return ""
	}}
}

// branchMenu is the menu of the branch in the status bar: the branches to
// switch to, and the commands of the remotes.
func (a *app) branchMenu(m *ui.Menu) {
	g := &a.git
	cur := a.branch()
	for _, b := range g.branches {
		if m.Item(b).Checked(b == cur).Disabled(g.busy != "").Chosen() && b != cur {
			a.post(func() { a.switchBranch(b, false) })
		}
	}
	if len(g.branches) > 0 {
		m.Separator()
	}
	if m.Item("Create Branch…").Disabled(g.busy != "").Chosen() {
		a.post(a.askNewBranch)
	}
	m.Separator()
	if m.Item("Fetch").Disabled(g.busy != "").Chosen() {
		a.post(a.fetch)
	}
	if m.Item("Pull").Disabled(g.busy != "").Chosen() {
		a.post(a.pull)
	}
	if m.Item("Push").Disabled(g.busy != "").Chosen() {
		a.post(a.push)
	}
}

// branchLabel is what the status bar says of the branch: its name, and
// how far it is ahead of its upstream and behind it.
func (a *app) branchLabel() string {
	label := a.branch()
	if st := a.git.status; st != nil && st.Upstream != "" {
		if st.Behind > 0 {
			label += fmt.Sprintf(" ↓%d", st.Behind)
		}
		if st.Ahead > 0 {
			label += fmt.Sprintf(" ↑%d", st.Ahead)
		}
	}
	return label
}
