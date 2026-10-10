package main

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"mayoi/internal/linediff"
)

// gitRepo makes the harness's folder a work tree with all its files
// committed, and returns a function that runs git there.
func (hs *harness) gitRepo() func(args ...string) string {
	t := hs.t
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", hs.root, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git("init", "-q", "-b", "main")
	git("config", "user.email", "t@t")
	git("config", "user.name", "t")
	os.WriteFile(hs.path(".gitignore"), []byte("node_modules/\n"), 0o644)
	git("add", ".")
	git("commit", "-q", "-m", "first")
	hs.a.openFolder(hs.root)
	hs.settleGit()
	return git
}

// settleGit draws frames until the status read in the background is in.
func (hs *harness) settleGit() {
	hs.a.gitChanged()
	for range 200 {
		hs.settle()
		if g := &hs.a.git; g.looked && !g.running && !g.stale && g.busy == "" {
			return
		}
	}
	hs.t.Fatal("git status never settled")
}

func TestExplorerShowsGitState(t *testing.T) {
	hs := newHarness(t, true)
	git := hs.gitRepo()
	os.WriteFile(hs.path("main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(hs.path("internal/a/new.go"), []byte("package a\n"), 0o644)
	hs.a.files.Open.Add(hs.path("internal"))
	hs.settleGit()
	a := hs.a
	if b := a.branch(); b != "main" {
		t.Errorf("branch %q", b)
	}
	if e := a.git.entries[hs.path("main.go")]; e.Letter() != 'M' {
		t.Errorf("main.go: %q", e.Letter())
	}
	if e := a.git.entries[hs.path("internal/a/new.go")]; e.Letter() != 'U' {
		t.Errorf("new.go: %q", e.Letter())
	}
	if l := a.git.dirs[hs.path("internal")]; l != 'U' {
		t.Errorf("internal: %q", l)
	}
	if !hs.tt.HasText("M") {
		t.Errorf("no M badge: %q", hs.tt.Texts())
	}
	hs.shot("git-explorer")
	git("add", ".")
	git("commit", "-q", "-m", "second")
	hs.settleGit()
	if len(a.git.entries) != 0 {
		t.Errorf("after the commit: %v", a.git.entries)
	}
	if strings.Contains(strings.Join(hs.tt.Texts(), " "), " U ") {
		t.Errorf("a badge stays")
	}
}

func TestSourceControlStagesAndCommits(t *testing.T) {
	hs := newHarness(t, true)
	git := hs.gitRepo()
	a := hs.a
	os.WriteFile(hs.path("main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	os.WriteFile(hs.path("notes.txt"), []byte("hi\n"), 0o644)
	os.Remove(hs.path("go.mod"))
	a.run("view.scm")
	hs.settleGit()
	for _, s := range []string{"SOURCE CONTROL", "CHANGES", "UNTRACKED", "main.go", "notes.txt", "go.mod"} {
		if !hs.tt.HasText(s) {
			t.Errorf("no %q: %q", s, hs.tt.Texts())
		}
	}
	hs.shot("git-scm")
	a.stage(hs.path("main.go"))
	hs.settleGit()
	if g := a.groupEntries(); len(g[groupStaged]) != 1 || len(g[groupChanges]) != 1 {
		t.Fatalf("groups %v", g)
	}
	// Committing what is staged.
	a.git.message = "change main"
	a.commit(false)
	hs.settleGit()
	if log := git("log", "--format=%s", "-1"); strings.TrimSpace(log) != "change main" {
		t.Fatalf("log %q", log)
	}
	if a.git.message != "" {
		t.Errorf("message %q stays", a.git.message)
	}
	// Nothing staged: committing asks to stage everything.
	a.git.message = "the rest"
	a.commit(false)
	if a.confirm == nil {
		t.Fatal("no question")
	}
	a.confirm.run()
	a.confirm = nil
	hs.settleGit()
	if n := a.changeCount(); n != 0 {
		t.Fatalf("%d changes after committing all: %v", n, a.git.status.Entries)
	}
	// Amending takes the last message.
	a.startAmend()
	hs.settleGit()
	if a.git.message != "the rest" || !a.git.amend {
		t.Fatalf("amending: %q %v", a.git.message, a.git.amend)
	}
	a.git.message = "the rest, amended"
	a.commit(false)
	hs.settleGit()
	if log := git("log", "--format=%s"); log != "the rest, amended\nchange main\nfirst\n" {
		t.Fatalf("log %q", log)
	}
}

func TestDiscardChanges(t *testing.T) {
	hs := newHarness(t, true)
	hs.gitRepo()
	a := hs.a
	os.WriteFile(hs.path("main.go"), []byte("broken\n"), 0o644)
	os.WriteFile(hs.path("junk.txt"), []byte("junk\n"), 0o644)
	hs.settleGit()
	g := a.groupEntries()
	a.discard(append(g[groupChanges], g[groupUntracked]...)...)
	if a.confirm == nil {
		t.Fatal("no question")
	}
	a.confirm.run()
	hs.settleGit()
	if data, _ := os.ReadFile(hs.path("main.go")); !strings.HasPrefix(string(data), "package main") {
		t.Errorf("main.go: %q", data)
	}
	if _, err := os.Stat(hs.path("junk.txt")); err == nil {
		t.Error("junk.txt stays")
	}
	if n := a.changeCount(); n != 0 {
		t.Errorf("%d changes", n)
	}
}

func TestDiffTab(t *testing.T) {
	hs := newHarness(t, true)
	hs.gitRepo()
	a := hs.a
	os.WriteFile(hs.path("main.go"), []byte("package main\n\nimport \"fmt\"\n\n// main says hi.\nfunc main() {\n\tfmt.Println(\"hi, mayoi\")\n\tfmt.Println(\"bye\")\n}\n"), 0o644)
	hs.settleGit()
	a.run("view.scm")
	hs.settle()
	if err := hs.tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	hs.settleGit()
	tab := a.activeTab()
	if tab == nil || tab.diff == nil || tab.diff.base != diffIndex {
		t.Fatalf("tab %+v", tab)
	}
	v := tab.diff
	want := []linediff.Hunk{hunk(4, 1, 4, 1), hunk(6, 1, 6, 2)}
	if !slices.Equal(v.hunks, want) {
		t.Fatalf("hunks %v, want %v", v.hunks, want)
	}
	if s := v.diffStatus(); s != "2 changes · +3 −2" {
		t.Errorf("status %q", s)
	}
	if !hs.tt.HasText("Index ↔ Working Tree") {
		t.Errorf("no sides: %q", hs.tt.Texts())
	}
	hs.shot("git-diff-unified")
	a.setDiffSplit(v, true)
	hs.settle()
	if len(v.pairs) != 9 || len(v.unified) != 11 {
		t.Errorf("rows %d %d", len(v.pairs), len(v.unified))
	}
	hs.shot("git-diff-split")
	// Typing in the file changes the diff.
	a.openFile(hs.path("main.go"), true)
	d := a.activeEditor()
	d.edit(d.sel, textEdit{0, 0, "// top\n"})
	a.activate(a.column(), a.column().showingDiff(hs.path("main.go"), diffIndex), true)
	time.Sleep(200 * time.Millisecond)
	hs.settle()
	if len(v.hunks) != 3 || v.hunks[0] != hunk(0, 0, 0, 1) {
		t.Errorf("after typing: %v", v.hunks)
	}
	// It opened at its first change; two more go to the third.
	v.stepHunk(1)
	v.stepHunk(1)
	if v.current != 2 || v.jump != v.pairsHunks[2] {
		t.Errorf("current %d jump %d", v.current, v.jump)
	}
	// It is kept, and comes back.
	a.rememberTabs()
	found := false
	for _, ts := range a.window.Columns[0].Tabs {
		found = found || ts.Diff == "index" && ts.Split
	}
	if !found {
		t.Errorf("tabs kept %+v", a.window.Columns)
	}
	a.closeActive()
	hs.settle()
	a.reopenClosed()
	hs.settle()
	if tab := a.activeTab(); tab == nil || tab.diff == nil || !tab.diff.split {
		t.Errorf("reopened %+v", tab)
	}
}

func hunk(a, alen, b, blen int) linediff.Hunk {
	return linediff.Hunk{A: a, ALen: alen, B: b, BLen: blen}
}

func TestGutterHunkActions(t *testing.T) {
	hs := newHarness(t, true)
	git := hs.gitRepo()
	a := hs.a
	before, _ := os.ReadFile(hs.path("main.go"))
	os.WriteFile(hs.path("main.go"), []byte(strings.Replace(string(before), "hello, mayoi", "hi", 1)+"// more\n"), 0o644)
	a.openFile(hs.path("main.go"), true)
	hs.settleGit()
	d := a.activeEditor()
	if hs := editorHunks(d); len(hs) != 2 {
		t.Fatalf("hunks %v", hs)
	}
	// A click on the mark of line 7 shows what it was.
	x := a.settings.SidebarWidth + d.gutterW - 5
	y := titleBarH + tabStripH + crumbsH + editorPadY + 6.5*a.font.lineH
	hs.tt.ClickAt(x, y)
	hs.settle()
	if h, k := d.peekedHunk(); k != 0 || h.B != 6 {
		t.Fatalf("peeked %v %d (peek %d)", h, k, d.peek)
	}
	if !hs.tt.HasText("\tfmt.Println(\"hello, mayoi\")") && !hs.tt.HasText("    fmt.Println(\"hello, mayoi\")") {
		t.Errorf("the old line does not show: %q", hs.tt.Texts())
	}
	hs.shot("git-hunk-peek")
	// Staging it leaves the other in the work tree only.
	h, _ := d.peekedHunk()
	a.stageHunk(d, h)
	hs.settleGit()
	if staged := git("diff", "--cached", "--numstat"); strings.TrimSpace(staged) != "1\t1\tmain.go" {
		t.Errorf("staged %q", staged)
	}
	if unstaged := git("diff", "--numstat"); strings.TrimSpace(unstaged) != "1\t0\tmain.go" {
		t.Errorf("not staged %q", unstaged)
	}
	// Reverting the other takes the line added out, as a step of undo.
	a.queueEdit(d, func(d *editor) { a.onCaretHunk(d, a.revertHunk) })
	d.selectRange(d.buf.Len()-2, d.buf.Len()-2, false)
	hs.settle()
	a.queueEdit(d, func(d *editor) { a.onCaretHunk(d, a.revertHunk) })
	hs.settle()
	if strings.Contains(d.buf.String(), "// more") {
		t.Errorf("not reverted: %q", d.buf.String())
	}
	d.undo()
	if !strings.Contains(d.buf.String(), "// more") {
		t.Errorf("undo: %q", d.buf.String())
	}
	// Going from change to change.
	d.selectRange(0, 0, false)
	hs.settle()
	a.stepChange(1)
	hs.settle()
	if l := caretLine(d); l != 6 {
		t.Errorf("next change at line %d", l)
	}
	a.stepChange(1)
	hs.settle()
	if l := caretLine(d); l != 8 {
		t.Errorf("next change at line %d", l)
	}
}

func TestBranchesAndBlame(t *testing.T) {
	hs := newHarness(t, true)
	git := hs.gitRepo()
	a := hs.a
	a.askNewBranch()
	if a.prompt == nil {
		t.Fatal("no prompt")
	}
	if msg := a.prompt.done("main"); msg == "" {
		t.Error("made a branch that exists")
	}
	if msg := a.prompt.done("topic"); msg != "" {
		t.Fatal(msg)
	}
	a.prompt = nil
	hs.settleGit()
	if b := a.branch(); b != "topic" || !slices.Equal(a.git.branches, []string{"main", "topic"}) {
		t.Fatalf("branch %q of %v", b, a.git.branches)
	}
	if !hs.tt.HasText("topic") {
		t.Errorf("the status bar lacks the branch: %q", hs.tt.Texts())
	}
	// A commit on the branch, then back to main, which has the old text.
	os.WriteFile(hs.path("README.md"), []byte("# Topic\n"), 0o644)
	git("commit", "-qam", "topic work")
	a.openFile(hs.path("README.md"), true)
	hs.settleGit()
	d := a.activeEditor()
	if d.buf.String() != "# Topic\n" {
		t.Fatalf("text %q", d.buf.String())
	}
	a.switchBranch("main", false)
	hs.settleGit()
	hs.until("the file reloads", func() bool { return d.buf.String() == "# Mayoi\n\nA small editor.\n" })
	if b := a.branch(); b != "main" {
		t.Fatalf("branch %q", b)
	}
	// The caret's line, at rest, says who changed it last.
	d.selectRange(2, 2, false)
	hs.settle()
	hs.until("the line is blamed", func() bool {
		hs.tt.Frame()
		return d.blame.done
	})
	if !strings.HasPrefix(d.blame.text, "t, just now · first") {
		t.Errorf("blame %q", d.blame.text)
	}
	hs.shot("git-blame")
	// A line typed is not committed.
	a.queueEdit(d, func(d *editor) { d.edit(d.sel, textEdit{0, 0, "new\n"}) })
	hs.settle()
	hs.until("the new line is blamed", func() bool {
		hs.tt.Frame()
		return d.blame.done
	})
	if d.blame.text != "" {
		t.Errorf("blame of a new line %q", d.blame.text)
	}
}

func TestPushPullFetch(t *testing.T) {
	hs := newHarness(t, true)
	git := hs.gitRepo()
	a := hs.a
	remote := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	git("remote", "add", "origin", remote)
	// The first push makes the upstream.
	a.push()
	hs.settleGit()
	if st := a.git.status; st.Upstream != "origin/main" {
		t.Fatalf("upstream %q", st.Upstream)
	}
	// Commit & Push.
	os.WriteFile(hs.path("README.md"), []byte("# Changed\n"), 0o644)
	hs.settleGit()
	a.stage(hs.path("README.md"))
	hs.settleGit()
	a.git.message = "change the readme"
	a.commit(true)
	hs.settleGit()
	hs.settleGit()
	if st := a.git.status; st.Ahead != 0 {
		t.Fatalf("ahead %d after commit & push", st.Ahead)
	}
	if out, _ := exec.Command("git", "-C", remote, "log", "--format=%s", "-1", "main").Output(); strings.TrimSpace(string(out)) != "change the readme" {
		t.Fatalf("remote has %q", out)
	}
	// A commit elsewhere: fetching tells, pulling brings it.
	other := t.TempDir()
	if out, err := exec.Command("git", "clone", "-q", "-b", "main", remote, other).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	os.WriteFile(other+"/go.mod", []byte("module other\n"), 0o644)
	for _, args := range [][]string{{"commit", "-qam", "other"}, {"push", "-q"}} {
		cmd := exec.Command("git", append([]string{"-C", other, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
	a.fetch()
	hs.settleGit()
	if st := a.git.status; st.Behind != 1 || a.branchLabel() != "main ↓1" {
		t.Fatalf("behind %d, label %q", st.Behind, a.branchLabel())
	}
	a.pull()
	hs.settleGit()
	if st := a.git.status; st.Behind != 0 {
		t.Fatalf("behind %d after pulling", st.Behind)
	}
	if data, _ := os.ReadFile(hs.path("go.mod")); string(data) != "module other\n" {
		t.Fatalf("go.mod %q", data)
	}
	// A remote that cannot be reached says so.
	git("remote", "set-url", "origin", "/nonexistent/repo.git")
	a.errors = nil
	a.fetch()
	for range 100 {
		hs.settle()
		if a.git.busy == "" {
			break
		}
	}
	hs.tt.Frame()
	if !slices.ContainsFunc(hs.tt.Texts(), func(s string) bool { return strings.Contains(s, "nonexistent") }) {
		t.Errorf("no toast: %q", hs.tt.Texts())
	}
}

func TestRenamesAndDiscardingOpenFiles(t *testing.T) {
	hs := newHarness(t, true)
	git := hs.gitRepo()
	a := hs.a
	git("mv", "go.mod", "go2.mod")
	os.WriteFile(hs.path("go2.mod"), []byte("module example\n\ngo 1.27\n\n// renamed\n"), 0o644)
	hs.settleGit()
	g := a.groupEntries()
	if len(g[groupStaged]) != 1 || len(g[groupChanges]) != 1 {
		t.Fatalf("groups %+v", g)
	}
	a.stageAll()
	hs.settleGit()
	if g = a.groupEntries(); len(g[groupChanges]) != 0 {
		t.Fatalf("after staging all: %+v", g)
	}
	// Unstaging a file renamed puts back where it was.
	a.toggleStaged(scmRow{group: groupStaged, e: g[groupStaged][0]})
	hs.settleGit()
	if st := strings.TrimSpace(git("status", "--short")); st != "D go.mod\n?? go2.mod" {
		t.Errorf("status %q", st)
	}
	// Discarding the changes of a file open with unsaved edits shows the
	// file as it is.
	git("checkout", "-q", "--", "go.mod")
	a.openFile(hs.path("main.go"), true)
	os.WriteFile(hs.path("main.go"), []byte("package main // on disk\n"), 0o644)
	hs.settleGit()
	d := a.activeEditor()
	d.edit(d.sel, textEdit{0, 0, "// typed\n"})
	if !d.dirty() {
		t.Fatal("not dirty")
	}
	for _, e := range a.groupEntries()[groupChanges] {
		if e.Path == hs.path("main.go") {
			a.discard(e)
		}
	}
	a.confirm.run()
	hs.settleGit()
	if d.dirty() || !strings.HasPrefix(d.buf.String(), "package main\n") {
		t.Errorf("dirty %v, text %q", d.dirty(), d.buf.String())
	}
	d.undo()
	if !strings.HasPrefix(d.buf.String(), "// typed") {
		t.Errorf("undo: %q", d.buf.String())
	}
}

func TestDiffScrollsToItsFirstChange(t *testing.T) {
	hs := newHarness(t, true)
	git := hs.gitRepo()
	a := hs.a
	long := hs.path("long.txt")
	var b strings.Builder
	for i := range 400 {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	os.WriteFile(long, []byte(b.String()), 0o644)
	git("add", ".")
	git("commit", "-qm", "long")
	data, _ := os.ReadFile(long)
	os.WriteFile(long, []byte(strings.Replace(string(data), "line 300", "LINE 300", 1)), 0o644)
	hs.settleGit()
	a.openDiff(long, diffIndex, true)
	hs.settleGit()
	v := a.activeTab().diff
	top := int((v.scroll.Y - editorPadY) / a.font.lineH)
	if v.current != 0 || top > 299 || top+20 < 299 {
		t.Fatalf("current %d, top row %d", v.current, top)
	}
	// A change in view needs no scrolling.
	os.WriteFile(long, []byte(strings.Replace(string(data), "line 3\n", "LINE 3\n", 1)), 0o644)
	a.closeActive()
	hs.settleGit()
	a.openDiff(long, diffIndex, true)
	hs.settleGit()
	if v := a.activeTab().diff; v.scroll.Y != 0 {
		t.Errorf("scrolled to %v", v.scroll.Y)
	}
}
