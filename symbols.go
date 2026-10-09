package main

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/highlight"
)

// symbol is a declaration of a document, as go to symbol lists it: the
// line it is on, its name, which quick open matches, and its kind.
type symbol struct {
	line   int
	name   string
	detail string // the rest of the declaration, shortened
	kind   string
	depth  int // how far it is indented, in columns
	col    int // where the name starts on its line, in runes
}

// declaration finds the lines that declare something, in most languages:
// a keyword, after modifiers, then a name.
var declaration = regexp.MustCompile(`^(\s*)(?:(?:export|pub(?:\([^)]*\))?|public|private|protected|internal|static|async|abstract|final|default|override|open|sealed|data|inline|unsafe|extern|declare)\s+)*(func|def|class|type|fn|function|interface|struct|enum|trait|impl|module|object|protocol|extension|union|macro)\b\s*(.*)$`)

// mdHeading finds a heading of Markdown.
var mdHeading = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)

// maxSymbolLines is the longest document whose symbols are listed.
const maxSymbolLines = 200_000

// findSymbols lists the declarations of a document.
func findSymbols(d *document) []symbol {
	snap := d.buf.Snapshot()
	if snap.LineCount() > maxSymbolLines {
		return nil
	}
	markdown := d.hl != nil && strings.EqualFold(highlight.Name(d.hl.Lexer()), "markdown")
	var out []symbol
	fence := false
	for i := range snap.LineCount() {
		s, e := snap.LineRange(i)
		if e-s > 400 {
			continue
		}
		text := snap.Slice(s, e)
		if markdown {
			if strings.HasPrefix(text, "```") {
				fence = !fence
			}
			if m := mdHeading.FindStringSubmatch(text); m != nil && !fence {
				out = append(out, symbol{line: i, name: m[2], detail: m[1], kind: "heading", depth: 2 * (len(m[1]) - 1)})
			}
			continue
		}
		if sym, ok := declare(text); ok {
			sym.line = i
			if at := strings.Index(text, sym.name); at >= 0 {
				sym.col = utf8.RuneCountInString(text[:at])
			}
			out = append(out, sym)
		}
	}
	return out
}

// declare reads a line declaring something: its name and kind.
func declare(text string) (symbol, bool) {
	m := declaration.FindStringSubmatch(text)
	if m == nil {
		return symbol{}, false
	}
	kw, rest := m[2], m[3]
	// A method's receiver comes first in Go: func (a *app) name(…).
	receiver := ""
	if kw == "func" && strings.HasPrefix(rest, "(") {
		if end := strings.IndexByte(rest, ')'); end > 0 {
			f := strings.Fields(rest[1:end])
			receiver = strings.TrimLeft(f[len(f)-1], "*")
			rest = strings.TrimSpace(rest[end+1:])
		}
	}
	name := rest
	if end := strings.IndexFunc(rest, func(r rune) bool { return !isWordRune(r) && r != '$' && r != '.' && r != '!' }); end >= 0 {
		name = rest[:end]
	}
	if name == "" || kw == "type" && rest != "" && rest[0] == '(' {
		return symbol{}, false // an anonymous function, or a group of types
	}
	// The rest of the declaration, as a function's parameters, and the
	// type a method is of.
	detail := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(rest[len(name):]), "{:"))
	if len(detail) > 100 {
		detail = detail[:97] + "…"
	}
	if receiver != "" {
		detail = strings.TrimSpace(detail + " · " + receiver)
	}
	kind := kw
	switch kw {
	case "def", "fn", "function", "func":
		kind = "function"
		if strings.Contains(m[0], "func (") || len(m[1]) > 0 && kw == "def" {
			kind = "method"
		}
	case "struct", "union", "object", "class", "impl", "extension", "module":
		kind = "class"
	case "interface", "trait", "protocol":
		kind = "interface"
	case "type":
		// Go's types say what they are after their name.
		switch f := strings.Fields(detail); {
		case len(f) > 0 && f[0] == "struct":
			kind = "class"
		case len(f) > 0 && f[0] == "interface":
			kind = "interface"
		}
	}
	return symbol{name: name, detail: detail, kind: kind, depth: columns(m[1])}, true
}

// symbolCache keeps the symbols of the version of a document go to
// symbol listed last.
type symbolCache struct {
	doc     int
	version uint64
	symbols []symbol
	names   []string
}

// documentSymbols returns the symbols of the document shown.
func (a *app) documentSymbols() ([]symbol, []string) {
	d := a.activeEditor()
	sc := &a.quick.syms
	if d == nil {
		return nil, nil
	}
	if sc.doc == d.id && sc.version == d.buf.Version() && sc.symbols != nil {
		return sc.symbols, sc.names
	}
	sc.doc, sc.version = d.id, d.buf.Version()
	sc.symbols = findSymbols(d.document)
	if sc.symbols == nil {
		sc.symbols = []symbol{}
	}
	sc.names = make([]string, len(sc.symbols))
	for i, s := range sc.symbols {
		sc.names[i] = s.name
	}
	return sc.symbols, sc.names
}

// symbolIcon is the mark of a kind of symbol, and its color.
func (a *app) symbolIcon(kind string) (*ui.SVG, ui.Color) {
	p := a.pal
	switch kind {
	case "function", "method":
		return iconFunction, p.syntax[highlight.Function]
	case "class":
		return iconBox, p.syntax[highlight.Type]
	case "interface":
		return iconBraces, p.syntax[highlight.Type]
	case "type", "enum":
		return iconType, p.syntax[highlight.Type]
	case "heading":
		return iconHash, p.syntax[highlight.Heading]
	}
	return iconHash, p.muted
}
