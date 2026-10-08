// Package highlight colors source code with Chroma's lexers, off the main
// thread, a window of lines at a time.
//
// A Highlighter keeps the tokens of the lines it lexed last, each with the
// text it was lexed from: a line whose text changed since shows no colors
// until the next pass lexes it again, so that stale tokens never color the
// wrong runes.
package highlight

import (
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Kind is the class of a token, which a palette colors.
type Kind uint8

const (
	Plain Kind = iota
	Keyword
	Type
	Function
	String
	Number
	Comment
	Operator
	Punctuation
	Constant
	Builtin
	Tag
	Attribute
	Variable
	Heading
	Deleted
	Inserted
	kinds
)

// Kinds is the number of kinds, for tables indexed by Kind.
const Kinds = int(kinds)

// Token is a run of one kind in a line, in runes from the line's start.
type Token struct {
	Start, End int
	Kind       Kind
}

// Line is a lexed line: the text it was lexed from and its tokens, Plain
// ones left out.
type Line struct {
	Text   string
	Tokens []Token
}

// MaxLineRunes is the longest line lexed: longer ones, as in minified
// files, show plain.
const MaxLineRunes = 4000

// Lexer returns the lexer of a file by its name, nil for plain text.
func Lexer(path string) chroma.Lexer {
	name := filepath.Base(path)
	l := lexers.Match(name)
	if l == nil {
		switch strings.ToLower(name) {
		case "makefile", "gnumakefile":
			l = lexers.Get("make")
		case "dockerfile":
			l = lexers.Get("docker")
		}
	}
	if l == nil {
		return nil
	}
	return chroma.Coalesce(l)
}

// Name returns the lexer's name to show, "Plain Text" for nil.
func Name(l chroma.Lexer) string {
	if l == nil {
		return "Plain Text"
	}
	return l.Config().Name
}

// Lex lexes text, whole lines, into a Line for each line of it.
func Lex(l chroma.Lexer, text string) []Line {
	lines := make([]Line, 0, strings.Count(text, "\n")+1)
	for s := range strings.SplitSeq(text, "\n") {
		lines = append(lines, Line{Text: s})
	}
	if l == nil {
		return lines
	}
	it, err := l.Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, text)
	if err != nil {
		return lines
	}
	row, col := 0, 0
	for tok := it(); tok != chroma.EOF; tok = it() {
		kind := classify(tok.Type)
		v := tok.Value
		for v != "" {
			nl := strings.IndexByte(v, '\n')
			part := v
			if nl >= 0 {
				part = v[:nl]
			}
			n := utf8.RuneCountInString(part)
			if kind != Plain && n > 0 && row < len(lines) {
				ln := &lines[row]
				if k := len(ln.Tokens); k > 0 && ln.Tokens[k-1].Kind == kind && ln.Tokens[k-1].End == col {
					ln.Tokens[k-1].End += n
				} else {
					ln.Tokens = append(ln.Tokens, Token{Start: col, End: col + n, Kind: kind})
				}
			}
			col += n
			if nl < 0 {
				break
			}
			row, col = row+1, 0
			v = v[nl+1:]
		}
	}
	return lines
}

func classify(t chroma.TokenType) Kind {
	switch {
	case t == chroma.KeywordType:
		return Type
	case t == chroma.KeywordConstant:
		return Constant
	case t.InCategory(chroma.Keyword):
		return Keyword
	case t == chroma.NameFunction || t == chroma.NameFunctionMagic:
		return Function
	case t == chroma.NameClass || t == chroma.NameNamespace:
		return Type
	case t == chroma.NameBuiltin || t == chroma.NameBuiltinPseudo:
		return Builtin
	case t == chroma.NameTag:
		return Tag
	case t == chroma.NameAttribute || t == chroma.NameDecorator:
		return Attribute
	case t == chroma.NameConstant || t == chroma.NameEntity:
		return Constant
	case t == chroma.NameVariable || t == chroma.NameVariableInstance || t == chroma.NameVariableGlobal || t == chroma.NameVariableClass:
		return Variable
	case t == chroma.NameOther || t == chroma.NameProperty:
		return Plain
	case t.InSubCategory(chroma.LiteralString):
		return String
	case t.InSubCategory(chroma.LiteralNumber):
		return Number
	case t == chroma.Literal || t == chroma.LiteralDate:
		return Constant
	case t.InCategory(chroma.Comment):
		return Comment
	case t.InCategory(chroma.Operator):
		return Operator
	case t == chroma.Punctuation:
		return Punctuation
	case t == chroma.GenericHeading || t == chroma.GenericSubheading:
		return Heading
	case t == chroma.GenericDeleted:
		return Deleted
	case t == chroma.GenericInserted:
		return Inserted
	case t == chroma.GenericStrong || t == chroma.GenericEmph:
		return Keyword
	}
	return Plain
}

// Source is what a Highlighter lexes: the text of lines first up to last
// of a document at a version. A ui.TextSnapshot satisfies it.
type Source interface {
	Version() uint64
	LineCount() int
	LineRange(line int) (int, int)
	Slice(start, end int) string
}

// WholeFile is the most lines lexed in one pass from the start: longer
// documents are lexed around the lines in view, with Context lines above,
// which multi-line comments and strings mostly start within.
const (
	WholeFile = 6000
	Context   = 400
	Below     = 200
)

// Highlighter lexes a document in the background and keeps the tokens of
// the lines it lexed last. Its methods are safe from any goroutine.
type Highlighter struct {
	lexer  chroma.Lexer
	notify func()

	mu      sync.Mutex
	version uint64 // the version lexed
	first   int    // the line Lines[0] is
	lines   []Line
	running bool
	want    request
	closed  bool
}

type request struct {
	src         Source
	first, last int
}

// New makes a highlighter of lexer, which calls notify, from another
// goroutine, when it has new tokens.
func New(lexer chroma.Lexer, notify func()) *Highlighter {
	return &Highlighter{lexer: lexer, notify: notify}
}

// Lexer returns the highlighter's lexer.
func (h *Highlighter) Lexer() chroma.Lexer { return h.lexer }

// Close stops the highlighter's work.
func (h *Highlighter) Close() {
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
}

// Request asks for the tokens of lines first up to last of src, lexed in
// the background unless the last pass has them. It returns at once.
func (h *Highlighter) Request(src Source, first, last int) {
	if h == nil || h.lexer == nil {
		return
	}
	n := src.LineCount()
	from, to := 0, n-1
	if n > WholeFile {
		from, to = max(0, first-Context), min(n-1, last+Below)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	if h.version == src.Version() && h.lines != nil && h.first <= max(0, first) && h.first+len(h.lines)-1 >= min(n-1, last) {
		return
	}
	h.want = request{src, from, to}
	if !h.running {
		h.running = true
		go h.run()
	}
}

func (h *Highlighter) run() {
	for {
		h.mu.Lock()
		r := h.want
		h.want = request{}
		if r.src == nil || h.closed {
			h.running = false
			h.mu.Unlock()
			return
		}
		h.mu.Unlock()

		start, _ := r.src.LineRange(r.first)
		_, end := r.src.LineRange(r.last)
		lines := Lex(h.lexer, r.src.Slice(start, end))
		for i := range lines {
			if utf8.RuneCountInString(lines[i].Text) > MaxLineRunes {
				lines[i].Tokens = nil
			}
		}

		h.mu.Lock()
		h.version, h.first, h.lines = r.src.Version(), r.first, lines
		if w := h.want; w.src != nil && w.src.Version() == h.version && w.first >= h.first && w.last < h.first+len(lines) {
			h.want = request{} // asked again for what this pass lexed
		}
		h.mu.Unlock()
		if h.notify != nil {
			h.notify()
		}
	}
}

// Line returns the tokens of line i, if the last pass lexed it from text.
func (h *Highlighter) Line(i int, text string) []Token {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	j := i - h.first
	if j < 0 || j >= len(h.lines) || h.lines[j].Text != text {
		return nil
	}
	return h.lines[j].Tokens
}
