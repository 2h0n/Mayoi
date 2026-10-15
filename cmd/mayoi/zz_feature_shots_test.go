package main

// Temporary: screenshots of every feature, for a gallery. Run with
// FSHOTS=<dir> go test -run TestFeat ./cmd/mayoi

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/settings"
)

const featW, featH = 1440, 900

func featDir(t *testing.T) string {
	dir := os.Getenv("FSHOTS")
	if dir == "" {
		t.Skip("FSHOTS is not set")
	}
	return dir
}

// featShell makes terminals start a zsh with a short, colorful prompt.
func featShell(t *testing.T) {
	z := t.TempDir()
	os.WriteFile(filepath.Join(z, ".zshrc"), []byte(`
PROMPT='%F{cyan}%1~%f %F{magenta}main%f %F{green}❯%f '
RPROMPT=''
export CLICOLOR=1 LSCOLORS=ExGxFxdaCxDaDahbadacec
alias ls='ls -G'
`), 0o644)
	t.Setenv("ZDOTDIR", z)
	t.Setenv("SHELL", "/bin/zsh")
}

// featRepo clones Mayoi into a folder named mayoi.
func featRepo(t *testing.T) string {
	root := filepath.Join(t.TempDir(), "mayoi")
	out, err := exec.Command("git", "clone", "-q", "--no-hardlinks", "../..", root).CombinedOutput()
	if err != nil {
		t.Fatalf("clone: %v %s", err, out)
	}
	return root
}

type featOpts struct {
	theme    string
	expanded []string
	w, h     int
	root     string
}

func featHarness(t *testing.T, dark bool, o featOpts) *harness {
	root := o.root
	if root == "" {
		root = featRepo(t)
	}
	s := settings.Default()
	w := s.NewWindow()
	w.Folder = root
	w.Expanded = o.expanded
	h := &fakeHost{folder: root}
	a := newApp(h, s, w)
	if o.w == 0 {
		o.w, o.h = featW, featH
	}
	tt := ui.NewTester(a.view, o.w, o.h)
	tt.SetDark(dark)
	tt.SetScale(2)
	hs := &harness{t: t, a: a, h: h, tt: tt, root: root}
	t.Cleanup(a.closeTerminals)
	hs.settle()
	if o.theme != "" {
		a.setTheme(o.theme)
	}
	hs.settleGit()
	return hs
}

func (hs *harness) fshot(name string, dark bool) {
	dir := os.Getenv("FSHOTS")
	suffix := "-light"
	if dark {
		suffix = "-dark"
	}
	old := os.Getenv("SHOTS")
	os.Setenv("SHOTS", dir)
	hs.shot(name + suffix)
	os.Setenv("SHOTS", old)
}

// caretAt puts the caret, or a selection, at the first of what in d.
func (hs *harness) caretAt(d *editor, what string, off, n int) {
	s := d.buf.String()
	i := strings.Index(s, what)
	if i < 0 {
		hs.t.Fatalf("no %q", what)
	}
	at := len([]rune(s[:i])) + off
	hs.place(d, at, at+n)
}

// scrollTo puts line of at, less above, at the top of d's view.
func (hs *harness) scrollTo(d *editor, what string, above int) {
	s := d.buf.String()
	i := strings.Index(s, what)
	line := d.buf.Snapshot().LineAt(len([]rune(s[:i])))
	d.vscroll.Y = editorPadY + float32(max(line-above, 0))*hs.a.font.lineH
	hs.settle()
}

func (hs *harness) termRun(s *session, cmd, want string) {
	hs.tt.Type(cmd)
	hs.tt.Key(0, ui.KeyEnter)
	deadline := time.Now().Add(20 * time.Second)
	for !strings.Contains(s.term.Text(), want) && time.Now().Before(deadline) {
		hs.settle()
	}
	hs.settle()
}

func featBoth(t *testing.T, f func(t *testing.T, dark bool)) {
	featDir(t)
	for _, dark := range []bool{true, false} {
		f(t, dark)
	}
}

// 01: the hero, two columns and a terminal.
func TestFeatHero(t *testing.T) {
	featShell(t)
	featBoth(t, func(t *testing.T, dark bool) {
		hs := featHarness(t, dark, featOpts{expanded: []string{"cmd", "cmd/mayoi"}})
		a := hs.a
		a.openFile(hs.path("cmd/mayoi/editor.go"), true)
		hs.settle()
		d := a.activeEditor()
		hs.scrollTo(d, "func newCodeFont", 4)
		hs.caretAt(d, "func newCodeFont", 5, 11)
		a.run("view.splitRight")
		a.openFile(hs.path("cmd/mayoi/wrap.go"), true)
		hs.settle()
		a.run("view.terminal")
		hs.settle()
		hs.termRun(a.terms[0], "git log --oneline -6", "Open the folders")
		hs.fshot("01-hero", dark)
		a.closeTerminals()
	})
}

// gitIn runs git in root, as a test user.
func gitIn(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.email=yunqian@example.com", "-c", "user.name=yunqian"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func editFile(t *testing.T, path string, f func(string) string) {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte(f(string(b))), 0o644)
}

// featChanges changes the work tree in each way source control lists.
func featChanges(t *testing.T, root string) {
	p := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
	editFile(t, p("cmd/mayoi/wrap.go"), func(s string) string {
		s = strings.Replace(s, "// wraps reports whether an editor wraps its lines: as its tab chose, or\n// else as the settings say.\n",
			"// wraps reports whether an editor wraps its lines.\n//\n// A tab's own choice, made with ⌥Z, wins over the settings until\n// they change again.\n", 1)
		s = strings.Replace(s, "\ts.WordWrap = !s.WordWrap\n", "\ts.WordWrap = !s.WordWrap\n\ta.toast(\"Word wrap \" + onOff(s.WordWrap))\n", 1)
		return s
	})
	editFile(t, p("README.md"), func(s string) string {
		return strings.Replace(s, "a small, fast code editor written in Go", "a small, fast and friendly code editor written in Go", 1)
	})
	editFile(t, p("internal/linediff/linediff.go"), func(s string) string {
		return strings.Replace(s, "package linediff", "// Package linediff diffs texts line by line, as Git does.\npackage linediff", 1)
	})
	gitIn(t, root, "add", "internal/linediff/linediff.go")
	os.Remove(p("scripts/dev.sh"))
	os.WriteFile(p("docs/ROADMAP.md"), []byte("# Roadmap\n\n- [ ] Multiple cursors\n- [ ] Language servers\n"), 0o644)
	os.MkdirAll(p("build/darwin-arm64"), 0o755)
	os.WriteFile(p("build/darwin-arm64/Mayoi.dmg"), []byte("x"), 0o644)
}

// 02: the explorer, with Git's colors and ignored files faint.
func TestFeatExplorer(t *testing.T) {
	featBoth(t, func(t *testing.T, dark bool) {
		root := featRepo(t)
		featChanges(t, root)
		hs := featHarness(t, dark, featOpts{root: root, expanded: []string{"build", "build/darwin-arm64", "docs", "internal", "internal/linediff", "scripts"}})
		a := hs.a
		a.openFile(hs.path("README.md"), true)
		hs.settleGit()
		hs.tt.Move(120, 400)
		hs.settle()
		hs.fshot("02-explorer", dark)
	})
}

// 03: reading aids: indent guides, the bracket, the word at the caret.
func TestFeatReadingAids(t *testing.T) {
	featBoth(t, func(t *testing.T, dark bool) {
		hs := featHarness(t, dark, featOpts{})
		a := hs.a
		a.openFile(hs.path("cmd/mayoi/editor.go"), true)
		hs.settle()
		d := a.activeEditor()
		hs.scrollTo(d, "func newCodeFont", 6)
		hs.caretAt(d, "f.charW = w / 64", 2, 0)
		hs.fshot("03-reading-aids", dark)
		hs.caretAt(d, "w, _ := ui.ShapeText", 0, 120)
		hs.fshot("03-selection", dark)
	})
}

// 04: sticky scroll and breadcrumbs.
func TestFeatSticky(t *testing.T) {
	featBoth(t, func(t *testing.T, dark bool) {
		hs := featHarness(t, dark, featOpts{})
		a := hs.a
		a.openFile(hs.path("cmd/mayoi/editor.go"), true)
		hs.settle()
		d := a.activeEditor()
		hs.caretAt(d, "for _, w := range deco.words", 0, 0)
		hs.scrollTo(d, "for _, w := range deco.words", 10)
		hs.fshot("04-sticky-scroll", dark)
	})
}

// 05: word wrap.
func TestFeatWrap(t *testing.T) {
	featBoth(t, func(t *testing.T, dark bool) {
		hs := featHarness(t, dark, featOpts{expanded: []string{"docs"}})
		a := hs.a
		a.openFile(hs.path("docs/FEATURES.md"), true)
		hs.settle()
		a.run("view.wordWrap")
		hs.settle()
		d := a.activeEditor()
		hs.scrollTo(d, "## Source control", 0)
		hs.fshot("05-word-wrap", dark)
		a.run("view.wordWrap")
	})
}

// 06: vim mode, in visual line mode.
func TestFeatVim(t *testing.T) {
	featBoth(t, func(t *testing.T, dark bool) {
		hs := featHarness(t, dark, featOpts{})
		a := hs.a
		a.toggleVim()
		a.openFile(hs.path("cmd/mayoi/wrap.go"), true)
		hs.settle()
		d := a.activeEditor()
		hs.caretAt(d, "func (a *app) toggleWordWrap", 0, 0)
		a.focusColumn()
		hs.vim("Vjjjj")
		hs.settle()
		hs.fshot("06-vim-visual", dark)
		hs.vim("<Esc>")
		hs.caretAt(d, "s := a.settings", 0, 0)
		hs.vim("w")
		hs.settle()
		hs.fshot("06-vim-normal", dark)
	})
}

// 07: the gutter's marks, a change peeked, and blame.
func TestFeatGutter(t *testing.T) {
	featBoth(t, func(t *testing.T, dark bool) {
		root := featRepo(t)
		featChanges(t, root)
		hs := featHarness(t, dark, featOpts{root: root})
		a := hs.a
		a.openFile(hs.path("cmd/mayoi/wrap.go"), true)
		hs.settleGit()
		d := a.activeEditor()
		hs.caretAt(d, "return a.settings.WordWrap", 0, 0)
		hs.until("blame", func() bool { hs.tt.Frame(); return d.blame.done })
		hs.settle()
		hs.fshot("07-blame-gutter", dark)
		s := d.buf.String()
		line := d.buf.Snapshot().LineAt(len([]rune(s[:strings.Index(s, "A tab's own")])))
		d.peekHunk(line)
		hs.settle()
		hs.fshot("07-change-peek", dark)
	})
}

// 08: source control and the diffs.
func TestFeatSourceControl(t *testing.T) {
	featBoth(t, func(t *testing.T, dark bool) {
		root := featRepo(t)
		featChanges(t, root)
		hs := featHarness(t, dark, featOpts{root: root})
		a := hs.a
		a.run("view.scm")
		hs.settleGit()
		a.git.message = "Say what word wrap does as it toggles"
		a.showChanges(diffTarget{path: hs.path("cmd/mayoi/wrap.go")}, true)
		hs.settleGit()
		hs.settle()
		a.setDiffLayout(a.activeTab().diff, layoutInline)
		hs.settle()
		hs.fshot("08-scm-diff-inline", dark)
		a.setDiffLayout(a.activeTab().diff, layoutSplit)
		hs.settle()
		hs.fshot("08-scm-diff-split", dark)
		a.settings.ScmTree = true
		hs.tt.Move(150, 300)
		hs.settle()
		hs.fshot("08-scm-tree", dark)
		a.settings.ScmTree = false
	})
}

// featHistory adds a merged branch, tags and unpushed commits.
func featHistory(t *testing.T, root string) {
	p := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
	gitIn(t, root, "tag", "v1.0.0")
	gitIn(t, root, "switch", "-q", "-c", "feature/minimap")
	editFile(t, p("cmd/mayoi/decorations.go"), func(s string) string { return s + "\n// minimap draws the file small, along the right edge.\nfunc minimap() {}\n" })
	gitIn(t, root, "commit", "-qam", "Draw a minimap along the right edge")
	editFile(t, p("cmd/mayoi/decorations.go"), func(s string) string { return s + "\n// minimapWidth is the minimap's width, in points.\nconst minimapWidth = 80\n" })
	gitIn(t, root, "commit", "-qam", "Make the minimap 80 points wide")
	gitIn(t, root, "switch", "-q", "main")
	editFile(t, p("README.md"), func(s string) string {
		return strings.Replace(s, "a small, fast code editor", "a small, fast and friendly code editor", 1)
	})
	gitIn(t, root, "commit", "-qam", "Call Mayoi friendly in the README")
	gitIn(t, root, "merge", "-q", "--no-ff", "-m", "Merge branch 'feature/minimap'", "feature/minimap")
	gitIn(t, root, "tag", "v1.1.0")
	for _, b := range []string{"fix/scroll-jump", "feature/lsp", "release/1.1"} {
		gitIn(t, root, "branch", b, "HEAD~2")
	}
	editFile(t, p("cmd/mayoi/wrap.go"), func(s string) string {
		return strings.Replace(s, "// else as the settings say.", "// else as the settings say, in every column.", 1)
	})
	gitIn(t, root, "commit", "-qam", "Wrap the lines of every column alike\n\nA tab's own choice still wins until the setting changes.")
}

// 09: the commits on their graph, a commit's changes, and the branches.
func TestFeatCommits(t *testing.T) {
	featBoth(t, func(t *testing.T, dark bool) {
		root := featRepo(t)
		featHistory(t, root)
		hs := featHarness(t, dark, featOpts{root: root})
		a := hs.a
		a.run("view.scm")
		hs.settleGit()
		a.showCommit(a.git.history[0], false)
		hs.settleGit()
		hs.settle()
		hs.fshot("09-commits", dark)
		a.settings.ScmAllBranches = true
		a.toggleAllBranches()
		a.toggleAllBranches()
		hs.settleGit()
		a.openBranchPicker()
		hs.settle()
		hs.fshot("09-branch-picker", dark)
	})
}
