package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDeclare(t *testing.T) {
	for _, tc := range []struct {
		line, name, kind, detail string
	}{
		{"func main() {", "main", "function", "()"},
		{"func (a *app) editorView(c *ui.Context, d *document) {", "editorView", "method", "(c *ui.Context, d *document) · app"},
		{"type palette struct {", "palette", "class", "struct"},
		{"export async function load(url: string): Promise<void> {", "load", "function", "(url: string): Promise<void>"},
		{"    def run(self):", "run", "method", "(self)"},
		{"class Foo(Base):", "Foo", "class", "(Base)"},
		{"pub(crate) fn parse<'a>(s: &'a str) -> Result<()> {", "parse", "function", "<'a>(s: &'a str) -> Result<()>"},
		{"interface Props {", "Props", "interface", ""},
	} {
		sym, ok := declare(tc.line)
		if !ok || sym.name != tc.name || sym.kind != tc.kind || sym.detail != tc.detail {
			t.Errorf("%q: %+v %v", tc.line, sym, ok)
		}
	}
	for _, line := range []string{"x := func() {}", "type (", "// func comment()", "	return function() {"} {
		if sym, ok := declare(line); ok {
			t.Errorf("%q declares %+v", line, sym)
		}
	}
}

func TestGoToSymbol(t *testing.T) {
	hs, d := editHarness(t, "a.go", "package a\n\nfunc first() {}\n\ntype second struct{}\n\nfunc (s second) third() {}\n")
	hs.a.openQuick("@thi")
	hs.settle()
	if !hs.tt.HasText("third") {
		t.Fatalf("symbols: %q", hs.tt.Texts())
	}
	hs.key(0, ui.KeyEnter)
	if d.sel != [2]int{66, 71} { // "third" on the line at 50
		t.Fatalf("selection %v", d.sel)
	}
}
