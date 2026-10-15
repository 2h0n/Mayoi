package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRelocate(t *testing.T) {
	text := []string{"a", "b", "c", "d", "b", "c"}
	for _, c := range []struct {
		lines []string
		from  int
		at    int
		ok    bool
	}{
		{[]string{"b", "c"}, 1, 1, true},
		{[]string{"b", "c"}, 3, 4, true}, // the nearest
		{[]string{"b", "c"}, 0, 1, true},
		{[]string{"c", "d"}, 5, 2, true},
		{[]string{"x"}, 2, 2, false},
		{[]string{"c", "d", "b", "c", "e"}, 2, 2, false},
	} {
		at, ok := relocate(c.lines, text, c.from)
		if at != c.at || ok != c.ok {
			t.Errorf("relocate(%v, %d) = %d, %v; want %d, %v", c.lines, c.from, at, ok, c.at, c.ok)
		}
	}
}

func TestFormatReview(t *testing.T) {
	rel := func(p string) string { return strings.TrimPrefix(p, "/repo/") }
	one := &reviewNote{path: "/repo/a.go", from: 11, to: 13, diff: true, snippet: []string{"-x := 1", "+x := 2"}, text: " Not here, the previous section. \n"}
	if got, want := formatReview([]*reviewNote{one}, rel), "a.go:12-13\n```diff\n-x := 1\n+x := 2\n```\nNot here, the previous section."; got != want {
		t.Errorf("one:\n%s\nwant:\n%s", got, want)
	}
	two := &reviewNote{path: "/repo/README.md", from: 0, to: 1, snippet: []string{"```go"}, text: "Fix the fence", outdated: true}
	removed := &reviewNote{path: "/elsewhere/b.go", commit: "0123456789abcdef", oldSide: true, from: 4, to: 5, diff: true, snippet: []string{"-y"}}
	got := formatReview([]*reviewNote{one, two, removed}, rel)
	for _, want := range []string{
		"1. a.go:12-13\n",
		"\n\n2. README.md:1 (outdated: the lines have changed since)\n````md\n```go\n````\nFix the fence\n",
		"\n3. /elsewhere/b.go:5 (removed lines, numbered as before the change; in commit 0123456)\n```diff\n-y\n```",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	staged := &reviewNote{path: "/repo/a.go", staged: true, from: 2, to: 3, snippet: []string{"z"}}
	if got := staged.ref(rel); got != "a.go:3 (staged)" {
		t.Errorf("staged %q", got)
	}
}

func TestCommentRows(t *testing.T) {
	if got := commentRows("short\n\nthe quick brown fox jumps", 10); !slices.Equal(got, []string{"short", "", "the quick ", "brown fox ", "jumps"}) {
		t.Errorf("rows %q", got)
	}
	n := &reviewNote{text: "one\ntwo"}
	if h := noteHeight(n, false, "", 40); h != 4 {
		t.Errorf("height %d", h)
	}
	if h := noteHeight(n, true, "", 40); h != 6 {
		t.Errorf("editing height %d", h)
	}
}

// reviewRepo makes a repository with t.txt of 30 lines committed, then
// changes line 3 and line 26 of it.
func reviewRepo(hs *harness) (orig, changed string) {
	git := hs.gitRepo()
	var b strings.Builder
	for i := range 30 {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	orig = b.String()
	os.WriteFile(hs.path("t.txt"), []byte(orig), 0o644)
	git("add", ".")
	git("commit", "-qm", "t")
	changed = strings.Replace(strings.Replace(orig, "line 2\n", "line two\n", 1), "line 25\n", "line twenty-five\n", 1)
	os.WriteFile(hs.path("t.txt"), []byte(changed), 0o644)
	hs.settleGit()
	return orig, changed
}

func TestReviewDiff(t *testing.T) {
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	_, changed := reviewRepo(hs)
	a.showChanges(diffTarget{path: hs.path("t.txt"), base: diffIndex}, true)
	hs.settleGit()
	v := a.activeTab().diff
	a.setDiffLayout(v, layoutInline)
	hs.settle()
	removed := slices.IndexFunc(v.rows, func(r diffRow) bool { return r.kind == rowLine && r.old == 2 && r.new < 0 })
	if removed < 0 || v.rows[removed+1].new != 2 {
		t.Fatalf("rows %s", kinds(v))
	}
	// Shift+J chooses the removed line and the line added.
	v.pane.Focus()
	v.setCursor(removed)
	hs.settle()
	hs.key(ui.Shift, ui.KeyJ)
	if lo, hi := v.selection(); lo != removed || hi != removed+1 || !v.selected(removed) {
		t.Fatalf("selection %d-%d, cursor %d anchor %d", lo, hi, v.cursor, v.anchor)
	}
	a.copyExcerpt(v)
	hs.settle()
	if got, want := tt.Clipboard(), "t.txt:3\n```diff\n-line 2\n+line two\n```"; got != want {
		t.Errorf("copied %q, want %q", got, want)
	}
	// c writes a comment under the lines.
	hs.key(0, ui.KeyC)
	if a.review.draft == nil || v.anchor >= 0 {
		t.Fatalf("no draft, or still chosen: %v", v.anchor)
	}
	at := strings.Index(kinds(v), "c")
	if at != removed+2 {
		t.Fatalf("card at %d: %s", at, kinds(v))
	}
	tt.Type("Not here, the previous section.")
	hs.settle()
	// The keys of the diff are the box's while it is typed in.
	if a.review.text != "Not here, the previous section." {
		t.Fatalf("typed %q", a.review.text)
	}
	hs.shot("review-draft")
	hs.key(ui.Cmd, ui.KeyEnter)
	if len(a.review.notes) != 1 || a.review.draft != nil {
		t.Fatalf("notes %d, draft %v", len(a.review.notes), a.review.draft)
	}
	n := a.review.notes[0]
	if n.from != 2 || n.to != 3 || n.oldSide || n.text != "Not here, the previous section." {
		t.Errorf("note %+v", n)
	}
	if !strings.Contains(kinds(v), "c") {
		t.Errorf("no card: %s", kinds(v))
	}
	hs.shot("review-saved")
	// The hunks are as they were, the card among the lines of the first.
	if len(v.starts) != 2 {
		t.Errorf("starts %v: %s", v.starts, kinds(v))
	}
	// Lines added above move the note with its lines.
	os.WriteFile(hs.path("t.txt"), []byte("new 1\nnew 2\n"+changed), 0o644)
	hs.settleGit()
	hs.settle()
	text := a.reviewText()
	if n.from != 4 || n.outdated || !strings.HasPrefix(text, "t.txt:5\n```diff\n-line 2\n+line two\n```\nNot here") {
		t.Errorf("moved: %d %v\n%s", n.from, n.outdated, text)
	}
	// Changing the lines marks it outdated.
	os.WriteFile(hs.path("t.txt"), []byte(strings.Replace(changed, "line two", "line 2!", 1)), 0o644)
	hs.settleGit()
	if text := a.reviewText(); !n.outdated || !strings.Contains(text, "(outdated") {
		t.Errorf("outdated %v:\n%s", n.outdated, text)
	}
	// With no terminal, sending copies them, and keeps them.
	a.sendReview()
	hs.settle()
	if got := tt.Clipboard(); !strings.Contains(got, "Not here") || len(a.review.notes) != 1 {
		t.Errorf("sent to the clipboard %q, %d notes", got, len(a.review.notes))
	}
	a.clearReview()
	hs.settle()
	if strings.Contains(kinds(v), "c") {
		t.Errorf("cleared: %s", kinds(v))
	}
	// A drag chooses the rows from where it started.
	pane, ok := tt.Find("Diff of the Changes")
	if !ok {
		t.Fatal("no diff")
	}
	v.scroll.Y = 0
	hs.settle()
	rowY := func(i int) float32 { return pane.Y + editorPadY + (float32(v.tops[i])+0.5)*a.font.lineH }
	first := slices.IndexFunc(v.rows, func(r diffRow) bool { return r.kind == rowLine })
	hs.drag(pane.X+120, rowY(first), pane.X+120, rowY(first+3))
	if lo, hi := v.selection(); lo != first || hi != first+3 {
		t.Errorf("dragged %d-%d, want %d-%d", lo, hi, first, first+3)
	}
	// A click chooses one row again, as Escape does.
	tt.ClickAt(pane.X+120, rowY(first+1))
	hs.settle()
	if lo, hi := v.selection(); lo != first+1 || hi != first+1 {
		t.Errorf("clicked %d-%d", lo, hi)
	}
	hs.key(ui.Shift, ui.KeyDown)
	hs.key(0, ui.KeyEscape)
	if v.anchor >= 0 {
		t.Errorf("Escape keeps %d", v.anchor)
	}
}

func TestReviewEditorToTerminal(t *testing.T) {
	featShell(t)
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	t.Cleanup(a.closeTerminals)
	reviewRepo(hs)
	a.newTerminal()
	hs.settle()
	s := a.panelTerm
	hs.until("the prompt", func() bool { return strings.Contains(s.term.Text(), "❯") })
	a.openFile(hs.path("t.txt"), true)
	hs.settle()
	d := a.activeEditor()
	snap := d.buf.Snapshot()
	from, _ := snap.LineRange(2)
	to, _ := snap.LineRange(4) // the start of a line ends before it
	hs.place(d, from, to)
	a.run("review.comment")
	hs.settle()
	if a.review.draft == nil || a.review.ed != d {
		t.Fatal("no box")
	}
	if _, ok := tt.Find("Review Comment"); !ok {
		t.Error("the box does not show")
	}
	tt.Type("Use a loop")
	hs.settle()
	hs.shot("review-editor")
	hs.key(ui.Cmd, ui.KeyEnter)
	if len(a.review.notes) != 1 {
		t.Fatalf("notes %d", len(a.review.notes))
	}
	n := a.review.notes[0]
	if n.diff || n.from != 2 || n.to != 4 || !slices.Equal(n.snippet, []string{"line two", "line 3"}) {
		t.Errorf("note %+v", n)
	}
	// The command on lines of a note edits it.
	hs.place(d, from, from)
	a.run("review.comment")
	hs.settle()
	if a.review.draft != n || a.review.text != "Use a loop" {
		t.Fatalf("editing %v %q", a.review.draft == n, a.review.text)
	}
	hs.key(0, ui.KeyEscape)
	if a.review.draft != nil || n.text != "Use a loop" {
		t.Fatalf("cancel: %v %q", a.review.draft, n.text)
	}
	a.sendReview()
	hs.settle()
	hs.until("the comment pasted", func() bool {
		return strings.Contains(s.term.Text(), "t.txt:3-4") && strings.Contains(s.term.Text(), "Use a loop")
	})
	if len(a.review.notes) != 0 || a.focus != paneTerminal {
		t.Errorf("after sending: %d notes, focus %v", len(a.review.notes), a.focus)
	}
}
