package highlight

import (
	"strings"
	"testing"
	"time"
)

func TestLexSplitsTokensByLine(t *testing.T) {
	src := "package main\n\n/* a\nb */\nfunc f() string { return \"hé\" }\n"
	lines := Lex(Lexer("x.go"), src)
	if len(lines) != 6 {
		t.Fatalf("%d lines, want 6", len(lines))
	}
	if got := lines[0].Tokens; len(got) == 0 || got[0] != (Token{0, 7, Keyword}) {
		t.Errorf("line 0 tokens %v", got)
	}
	// The block comment continues on its second line.
	if got := lines[3].Tokens; len(got) == 0 || got[0].Kind != Comment || got[0].Start != 0 {
		t.Errorf("line 3 tokens %v", got)
	}
	var str *Token
	for i, tok := range lines[4].Tokens {
		if tok.Kind == String {
			str = &lines[4].Tokens[i]
		}
	}
	// Columns are runes: "hé" with its quotes is 4 runes.
	if str == nil || str.End-str.Start != 4 {
		t.Errorf("string token %v in %v", str, lines[4].Tokens)
	}
}

func TestPlainText(t *testing.T) {
	if Lexer("notes.unknownext") != nil {
		t.Error("lexer for an unknown extension")
	}
	if Lexer("Makefile") == nil || Lexer("main.go") == nil {
		t.Error("no lexer for Makefile or main.go")
	}
}

type fakeSource struct {
	lines   []string
	version uint64
}

func (s fakeSource) Version() uint64 { return s.version }
func (s fakeSource) LineCount() int  { return len(s.lines) }
func (s fakeSource) LineRange(line int) (int, int) {
	a := 0
	for i := range line {
		a += len(s.lines[i]) + 1
	}
	return a, a + len(s.lines[line])
}
func (s fakeSource) Slice(a, z int) string { return strings.Join(s.lines, "\n")[a:z] }

func TestHighlighterRejectsChangedLines(t *testing.T) {
	done := make(chan struct{}, 8)
	h := New(Lexer("x.go"), func() { done <- struct{}{} })
	src := fakeSource{lines: []string{"package main", "var x = 1"}, version: 1}
	h.Request(src, 0, 1)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("no notice")
	}
	if len(h.Line(0, "package main")) == 0 {
		t.Error("no tokens for an unchanged line")
	}
	if h.Line(0, "package mainx") != nil {
		t.Error("tokens for a changed line")
	}
}
