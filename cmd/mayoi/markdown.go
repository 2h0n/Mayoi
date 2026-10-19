package main

import (
	"cmp"
	"html"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"mayoi/internal/highlight"
	"mayoi/internal/remote"
	"mayoi/internal/vfs"
	"mayoi/locales"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/egoist/mygo/ui"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// markdownPreview keeps parsed blocks and the pane's place between edits.
type markdownPreview struct {
	blocks     []markdownBlock
	cache      markdownCache
	root       string
	measure    float32
	measured   ui.Font
	list       ui.ListState
	scroll     ui.ScrollState
	observed   uint64
	path       string
	pending    bool
	running    bool
	generation atomic.Uint64
}

type markdownInline struct {
	text, link, image               string
	bold, italic, strike, code, raw bool
	bitmap                          ui.ImageSource
	w, h                            float32
	kind                            highlight.Kind
}

type markdownKind uint8

const (
	markdownParagraph markdownKind = iota
	markdownHeading
	markdownRule
	markdownCodeBlock
	markdownHTML
	markdownImageBlock
	markdownTable
	markdownTableHeader
	// markdownAlertTitle is the first row of a GitHub alert, its icon and
	// its kind, as "Note".
	markdownAlertTitle
)

// markdownAlert is the kind of a GitHub alert, a quote that opens with
// [!NOTE] or another of them, 0 for a plain quote.
type markdownAlert uint8

const (
	alertNone markdownAlert = iota
	alertNote
	alertTip
	alertImportant
	alertWarning
	alertCaution
)

var markdownAlerts = map[string]markdownAlert{
	"NOTE": alertNote, "TIP": alertTip, "IMPORTANT": alertImportant, "WARNING": alertWarning, "CAUTION": alertCaution,
}

type markdownImageKey struct {
	path  string
	mtime time.Time
	size  int64
}

// markdownCache keeps the images a parse decoded and the code it
// highlighted, by language and text, for the next parse of the document,
// which drops those it no longer has.
type markdownCache struct {
	images map[markdownImageKey]markdownInline
	code   map[[2]string][]markdownInline
}

// markdownBlock is a paragraph-sized row, including its enclosing lists and quotes.
type markdownBlock struct {
	kind                 markdownKind
	offset, markerWidth  float32
	tight                bool
	level, indent, quote int
	marker, language     string
	runs                 []markdownInline
	cells                [][]markdownInline
	aligns               []east.Alignment
	// widths are the shares of a table's columns, by the longest text of
	// each, and row is the place of a row of its body, from 1.
	widths []float32
	row    int
	// alert is the kind of the alert the block is in, and alertAt the
	// depth of its quote.
	alert   markdownAlert
	alertAt int
	// slug is a heading's anchor, as GitHub makes it, which a link to
	// #slug scrolls to.
	slug string
}

func isMarkdown(d *document) bool {
	ext := filepath.Ext(d.path)
	return strings.EqualFold(ext, ".md") || strings.EqualFold(ext, ".markdown")
}

func canPreview(d *document) bool { return isSVG(d) || isMarkdown(d) }

// markdownPath resolves local destinations without fetching remote resources.
// A leading / means root, as it means the repository's on GitHub, or the
// filesystem's when there is no root.
func markdownPath(root, path, dest string) string {
	// A whole path of Windows, C:\ or C:/, which a URL would take for one
	// of scheme c. Not a share, as //host/x is a URL without its scheme.
	if len(dest) > 2 && filepath.VolumeName(dest) == dest[:2] && (dest[2] == '\\' || dest[2] == '/') {
		return filepath.Clean(dest)
	}
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" {
		return ""
	}
	// A URL's slash, which Windows does not take for a whole path.
	if strings.HasPrefix(u.Path, "/") {
		return filepath.Join(cmp.Or(root, "/"), filepath.FromSlash(u.Path))
	}
	return filepath.Join(filepath.Dir(path), filepath.FromSlash(u.Path))
}

// markdownRoot is the Git repository holding path, or else the workspace
// when it holds path, or "".
func markdownRoot(path, workspace string) string {
	// Of a remote file, the workspace, rather than a round trip to its
	// host for each folder above it.
	if remote.Is(path) {
		if rel, err := filepath.Rel(workspace, path); workspace != "" && err == nil && filepath.IsLocal(rel) {
			return workspace
		}
		return ""
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	if rel, err := filepath.Rel(workspace, path); workspace != "" && err == nil && filepath.IsLocal(rel) {
		return workspace
	}
	return ""
}

// markdownFrontMatter reports whether lines, those between front matter's
// delimiters, look like YAML, so a document opening with a rule keeps the
// text up to its next rule.
func markdownFrontMatter(lines []string) bool {
	keys := false
	for _, line := range lines {
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || line == "-" || strings.HasPrefix(line, "- "):
			if !keys {
				return false
			}
		default:
			key, rest, ok := strings.Cut(line, ":")
			if !ok || strings.TrimSpace(key) == "" || rest != "" && rest[0] != ' ' && rest[0] != '\t' {
				return false
			}
			keys = true
		}
	}
	return keys
}

// markdownParser parses GitHub's Markdown: goldmark's parser with the
// parts of its GFM extension that parse, without its HTML renderer, which
// the preview has no use for and would make Mayoi larger.
func markdownParser() parser.Parser {
	return parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(append(parser.DefaultInlineParsers(),
			util.Prioritized(extension.NewStrikethroughParser(), 500),
			util.Prioritized(extension.NewTaskCheckBoxParser(), 0),
			util.Prioritized(extension.NewLinkifyParser(), 999))...),
		parser.WithParagraphTransformers(append(parser.DefaultParagraphTransformers(),
			util.Prioritized(extension.NewTableParagraphTransformer(), 200))...),
		parser.WithASTTransformers(util.Prioritized(extension.NewTableASTTransformer(), 0)),
	)
}

// parseMarkdown converts the AST once, away from the UI thread.
// A parse that current reports superseded keeps the images it skipped cached.
func parseMarkdown(source []byte, path, root string, current func() bool, caches ...*markdownCache) []markdownBlock {
	c := &markdownCache{}
	if len(caches) > 0 && caches[0] != nil {
		c = caches[0]
	}
	if c.images == nil {
		c.images = map[markdownImageKey]markdownInline{}
	}
	if c.code == nil {
		c.code = map[[2]string][]markdownInline{}
	}
	cache := c.images
	used := map[markdownImageKey]bool{}
	usedCode := map[[2]string]bool{}
	defer func() {
		if current != nil && !current() {
			return
		}
		for k := range cache {
			if !used[k] {
				delete(cache, k)
			}
		}
		for k := range c.code {
			if !usedCode[k] {
				delete(c.code, k)
			}
		}
	}()
	// code highlights a block of code, as the last parse did if it had it.
	code := func(text, language string) []markdownInline {
		key := [2]string{language, text}
		usedCode[key] = true
		runs, ok := c.code[key]
		if !ok {
			runs = markdownCode(text, language)
			runs = runs[:len(runs):len(runs)]
			c.code[key] = runs
		}
		return runs
	}
	// Front matter is recognized only at the start, with exact delimiter lines
	// around what looks like YAML.
	if lines := strings.SplitAfter(string(source), "\n"); strings.TrimSuffix(strings.TrimSuffix(lines[0], "\n"), "\r") == "---" {
		for i := 1; i < len(lines); i++ {
			line := strings.TrimSuffix(strings.TrimSuffix(lines[i], "\n"), "\r")
			if line == "---" || line == "..." {
				if markdownFrontMatter(lines[1:i]) {
					source = []byte(strings.Join(lines[i+1:], ""))
				}
				break
			}
		}
	}

	tree := markdownParser().Parse(text.NewReader(source))
	var blocks []markdownBlock
	var inline func(ast.Node, markdownInline) []markdownInline
	inline = func(n ast.Node, style markdownInline) []markdownInline {
		var out []markdownInline
		for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
			s := style
			switch x := ch.(type) {
			case *ast.Text:
				s.text = string(x.Segment.Value(source))
				if !x.IsRaw() && strings.ContainsAny(s.text, "\\&") {
					s.text = html.UnescapeString(string(util.UnescapePunctuations([]byte(s.text))))
				}
				if x.HardLineBreak() {
					s.text += "\n"
				} else if x.SoftLineBreak() {
					s.text += " "
				}
			case *ast.String:
				s.text = string(x.Value)
			case *ast.Emphasis:
				if x.Level == 2 {
					s.bold = true
				} else {
					s.italic = true
				}
				out = append(out, inline(ch, s)...)
				continue
			case *east.Strikethrough:
				s.strike = true
				out = append(out, inline(ch, s)...)
				continue
			case *ast.CodeSpan:
				s.code = true
				out = append(out, inline(ch, s)...)
				continue
			case *ast.Link:
				s.link = string(x.Destination)
				out = append(out, inline(ch, s)...)
				continue
			case *ast.AutoLink:
				s.text, s.link = string(x.Label(source)), string(x.URL(source))
			case *ast.Image:
				s.text = string(x.Text(source))
				s.image = string(x.Destination)
				if local := markdownPath(root, path, s.image); local != "" && (current == nil || current()) {
					if info, err := vfs.Stat(local); err == nil && info.Mode().IsRegular() {
						key := markdownImageKey{local, info.ModTime(), info.Size()}
						used[key] = true
						image, ok := cache[key]
						if !ok {
							if strings.EqualFold(filepath.Ext(local), ".svg") {
								if data, err := readImage(local); err == nil {
									if svg, err := ui.ParseSVG(data); err == nil {
										image.bitmap = svg
										image.w, image.h = svg.Size()
										if image.w <= 0 || image.h <= 0 {
											image.bitmap = nil
										}
									}
								}
							} else {
								bm, w, h, _, _ := decodeImage(local)
								image.bitmap, image.w, image.h = bm, float32(w), float32(h)
								if bm == nil {
									image.bitmap = nil
								}
							}
							cache[key] = image
						}
						s.bitmap, s.w, s.h = image.bitmap, image.w, image.h
					}
				}
				if s.text == "" {
					s.text = filepath.Base(s.image)
					if s.text == "." || s.text == "" {
						s.text = "image"
					}
				}
				s.raw = s.bitmap == nil
			case *ast.RawHTML:
				s.raw = true
				s.text = markdownHTMLText(string(x.Segments.Value(source)))
				if tag := strings.ToLower(strings.TrimSpace(s.text)); tag == "<br>" || tag == "<br/>" || tag == "<br />" {
					s.text, s.raw = "\n", false
				}
			case *east.TaskCheckBox:
				s.text = "☐ "
				if x.IsChecked {
					s.text = "☑ "
				}
			default:
				out = append(out, inline(ch, s)...)
				continue
			}
			out = append(out, s)
		}
		return out
	}
	// alert and alertAt are those of the alert being walked; strip is the
	// length of its marker, which its first paragraph drops.
	alert, alertAt, strip := alertNone, 0, 0
	slugs := map[string]int{}
	var walk func(ast.Node, int, int, string, bool, float32, float32)
	walk = func(n ast.Node, indent, quote int, marker string, tight bool, offset, markerWidth float32) {
		b := markdownBlock{indent: indent, quote: quote, marker: marker, tight: tight, offset: offset, markerWidth: markerWidth, alert: alert, alertAt: alertAt}
		switch x := n.(type) {
		case *ast.Heading:
			b.kind, b.level = markdownHeading, x.Level
			b.runs = inline(n, markdownInline{bold: true})
			b.slug = markdownSlug(markdownRunsText(b.runs))
			if k := slugs[b.slug]; k > 0 {
				slugs[b.slug] = k + 1
				b.slug += "-" + strconv.Itoa(k)
			} else {
				slugs[b.slug] = 1
			}
		case *ast.Paragraph, *ast.TextBlock:
			b.kind = markdownParagraph
			b.runs = inline(n, markdownInline{})
			if strip > 0 {
				b.runs = markdownTrimRuns(b.runs, strip)
				strip = 0
			}
			if _, task := n.FirstChild().(*east.TaskCheckBox); task && marker != "" {
				b.marker = strings.TrimSpace(b.runs[0].text)
				b.runs = b.runs[1:]
			}

		case *ast.ThematicBreak:
			b.kind = markdownRule
		case *ast.FencedCodeBlock:
			b.kind, b.language = markdownCodeBlock, string(x.Language(source))
			b.runs = code(string(x.Lines().Value(source)), b.language)
		case *ast.CodeBlock:
			b.kind = markdownCodeBlock
			b.runs = code(string(x.Lines().Value(source)), "")
		case *ast.HTMLBlock:
			raw := string(x.Lines().Value(source))
			if x.HasClosure() {
				raw += string(x.ClosureLine.Value(source))
			}
			b.kind = markdownHTML
			raw = markdownHTMLText(raw)
			if strings.TrimSpace(raw) == "" {
				return
			}
			b.runs = []markdownInline{{text: raw, raw: true}}
		case *ast.Blockquote:
			outer, outerAt := alert, alertAt
			if k, n := markdownAlertOf(x, source); k != alertNone {
				alert, alertAt, strip = k, quote+1, n
				blocks = append(blocks, markdownBlock{kind: markdownAlertTitle, indent: indent, quote: quote + 1, marker: marker, tight: tight,
					offset: offset, markerWidth: markerWidth, alert: k, alertAt: quote + 1})
				if marker != "" {
					offset += markerWidth + 0.5
					markerWidth = 0
				}
				marker = ""
			}
			for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
				walk(ch, indent, quote+1, marker, tight, offset, markerWidth)
				if marker != "" {
					offset += markerWidth + 0.5
					markerWidth = 0
				}
				marker = ""
			}
			alert, alertAt, strip = outer, outerAt, 0
			return
		case *ast.List:
			i := x.Start
			width := float32(1.5)
			if x.IsOrdered() {
				width = max(width, float32(len(strconv.Itoa(x.Start+x.ChildCount()-1))+1)*0.65)
			}
			if marker != "" {
				offset += markerWidth + 0.5
			}
			for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
				m := "•"
				if x.IsOrdered() {
					m = strconv.Itoa(i) + "."
					i++
				}
				walk(ch, indent+1, quote, m, x.IsTight, offset, width)
			}
			return
		case *east.Table:
			first, body := len(blocks), 0
			for row := n.FirstChild(); row != nil; row = row.NextSibling() {
				rb := b
				rb.kind = markdownTable
				rb.aligns = x.Alignments
				_, head := row.(*east.TableHeader)
				if head {
					rb.kind = markdownTableHeader
				} else {
					body++
					rb.row = body
				}
				for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
					rb.cells = append(rb.cells, markdownMerge(inline(cell, markdownInline{bold: head})))
				}
				blocks = append(blocks, rb)
			}
			widths := markdownWidths(blocks[first:])
			for i := first; i < len(blocks); i++ {
				blocks[i].widths = widths
			}
			return
		default:
			for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
				walk(ch, indent, quote, marker, tight, offset, markerWidth)
				if marker != "" {
					offset += markerWidth + 0.5
					markerWidth = 0
				}
				marker = ""
			}
			return
		}
		// Decoded images get their own rows so that paragraphs retain native text wrapping.
		if b.kind == markdownParagraph || b.kind == markdownHeading {
			b.runs = markdownMerge(b.runs)
			if !markdownHasText(b.runs) && !slices.ContainsFunc(b.runs, func(r markdownInline) bool { return r.bitmap != nil }) {
				return
			}
		}
		if (b.kind == markdownParagraph || b.kind == markdownHeading) && slices.ContainsFunc(b.runs, func(r markdownInline) bool { return r.bitmap != nil }) {
			runs := b.runs
			b.runs = nil
			for _, r := range runs {
				if r.bitmap != nil {
					if markdownHasText(b.runs) {
						blocks = append(blocks, b)
						b.runs = nil
						if b.marker != "" {
							b.offset += b.markerWidth + 0.5
						}
						b.markerWidth = 0
						b.marker = ""
					}
					ib := b
					ib.kind = markdownImageBlock
					ib.runs = []markdownInline{r}
					blocks = append(blocks, ib)
					if b.marker != "" {
						b.offset += b.markerWidth + 0.5
					}
					b.markerWidth = 0
					b.marker = ""
				} else {
					b.runs = append(b.runs, r)
				}
			}
			if !markdownHasText(b.runs) {
				return
			}
		}
		blocks = append(blocks, b)
	}
	walk(tree, 0, 0, "", false, 0, 0)
	return blocks
}

// markdownAlertOf returns the kind of the alert a quote is, by the marker
// alone on its first line, as [!NOTE], and the marker's length in runes.
func markdownAlertOf(q *ast.Blockquote, source []byte) (markdownAlert, int) {
	p, ok := q.FirstChild().(*ast.Paragraph)
	if !ok || p.Lines().Len() == 0 {
		return alertNone, 0
	}
	seg := p.Lines().At(0)
	line := strings.TrimSpace(string(seg.Value(source)))
	if len(line) < 4 || !strings.HasPrefix(line, "[!") || !strings.HasSuffix(line, "]") {
		return alertNone, 0
	}
	return markdownAlerts[strings.ToUpper(line[2:len(line)-1])], utf8.RuneCountInString(line)
}

// markdownTrimRuns drops the first n runes of the text of runs, and the
// break after them.
func markdownTrimRuns(runs []markdownInline, n int) []markdownInline {
	for len(runs) > 0 && n > 0 {
		rs := []rune(runs[0].text)
		if len(rs) > n {
			runs[0].text = string(rs[n:])
			break
		}
		n -= len(rs)
		runs = runs[1:]
	}
	for len(runs) > 0 {
		runs[0].text = strings.TrimLeft(runs[0].text, " \t\n")
		if runs[0].text != "" || runs[0].bitmap != nil {
			break
		}
		runs = runs[1:]
	}
	return runs
}

// markdownSlug is the anchor GitHub gives a heading: its text in lower
// case, without punctuation, its spaces hyphens.
func markdownSlug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// markdownWidths shares a table's width between its columns by the
// longest text of each, in columns of a fixed font, a wide character two,
// within bounds that keep a short column readable and a long one from
// taking all the room.
func markdownWidths(rows []markdownBlock) []float32 {
	var cols []float32
	for _, r := range rows {
		for i, cell := range r.cells {
			w := float32(0)
			for _, run := range cell {
				if run.bitmap != nil {
					w += 12
					continue
				}
				for _, ch := range run.text {
					w++
					if ch >= 0x1100 && unicode.In(ch, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) || ch >= 0xff00 && ch <= 0xffef {
						w++
					}
				}
			}
			if i >= len(cols) {
				cols = append(cols, 0)
			}
			cols[i] = max(cols[i], w)
		}
	}
	total := float32(0)
	for i, w := range cols {
		cols[i] = clamp(w, 6, 48)
		total += cols[i]
	}
	for i := range cols {
		cols[i] /= total
	}
	return cols
}

// markdownMerge joins the runs of a style that follow each other, as
// goldmark ends a run at each line and at much punctuation, into fewer
// runs for the preview to lay out.
func markdownMerge(runs []markdownInline) []markdownInline {
	out := runs[:0]
	for _, r := range runs {
		if n := len(out); n > 0 {
			l := &out[n-1]
			if r.bitmap == nil && l.bitmap == nil && r.image == "" && l.image == "" && r.link == l.link && r.kind == l.kind &&
				r.bold == l.bold && r.italic == l.italic && r.strike == l.strike && r.code == l.code && r.raw == l.raw {
				l.text += r.text
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// markdownRunsText is the text of runs.
func markdownRunsText(runs []markdownInline) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.text)
	}
	return b.String()
}

// HTML comments may span lines; other HTML remains literal.
func markdownHTMLText(s string) string {
	for {
		start := strings.Index(s, "<!--")
		if start < 0 {
			return s
		}
		end := strings.Index(s[start+4:], "-->")
		if end < 0 {
			return s[:start]
		}
		s = s[:start] + s[start+4+end+3:]
	}
}

// Soft breaks between standalone images must not become empty paragraph rows.
func markdownHasText(runs []markdownInline) bool {
	for _, r := range runs {
		if strings.TrimSpace(r.text) != "" {
			return true
		}
	}
	return false
}

func markdownCode(code, language string) []markdownInline {
	var out []markdownInline
	var lines []highlight.Line
	if language != "" {
		lexer := lexers.Get(language)
		if lexer != nil {
			lexer = chroma.Coalesce(lexer)
		}
		lines = highlight.Lex(lexer, strings.TrimSuffix(code, "\n"))
	} else {
		return []markdownInline{{text: strings.TrimSuffix(code, "\n"), code: true}}
	}
	for i, line := range lines {
		rs := []rune(line.Text)
		at := 0
		for _, tok := range line.Tokens {
			if tok.Start > at {
				out = append(out, markdownInline{text: string(rs[at:tok.Start]), code: true})
			}
			out = append(out, markdownInline{text: string(rs[tok.Start:tok.End]), code: true, kind: tok.Kind})
			at = tok.End
		}
		tail := string(rs[at:])
		if i < len(lines)-1 {
			tail += "\n"
		}
		out = append(out, markdownInline{text: tail, code: true})
	}
	return out
}

// followMarkdown debounces edits and drops results superseded or closed while parsing.
func (a *app) followMarkdown(v *viewer) {
	d, m := v.doc, v.markdown
	ver := d.buf.Version()
	// Idle keeps the loaded version; debouncing keeps its timer; running keeps one
	// worker; running+dirty advances the generation and schedules after that worker.
	if (m.pending || v.loaded && v.version == ver) && m.observed == ver && m.path == d.path {
		return
	}
	first := m.generation.Load() == 0
	generation := m.generation.Add(1)
	m.observed, m.path = ver, d.path
	m.pending = true
	if v.wait != nil {
		v.wait.Stop()
		v.wait = nil
	}
	if m.running {
		return
	}
	loading := v.loading
	start := func() {
		if m.generation.Load() != generation || v.loading != loading || !slices.Contains(a.docs, d) {
			return
		}
		v.wait = nil
		m.running = true
		source, path, workspace := []byte(d.buf.String()), d.path, a.folder()
		// Give the worker its own cache snapshot; publish it on the UI thread.
		cache := &markdownCache{maps.Clone(m.cache.images), maps.Clone(m.cache.code)}
		go func() {
			root := markdownRoot(path, workspace)
			blocks := parseMarkdown(source, path, root, func() bool { return m.generation.Load() == generation }, cache)
			a.post(func() {
				m.running, m.pending = false, false
				m.cache = *cache
				m.root = root
				if v.loading != loading || !slices.Contains(a.docs, d) {
					return
				}
				if m.generation.Load() != generation || d.buf.Version() != ver || d.path != path {
					a.followMarkdown(v)
					return
				}
				m.blocks = blocks
				v.version = ver
				v.loaded = true
			})
		}()
	}
	if first {
		start()
	} else {
		v.wait = time.AfterFunc(svgDelay, func() { a.post(start) })
	}
}

// markdownSpan resolves every inline style before either a span or link is
// built, of text size big.
func (a *app) markdownSpan(r markdownInline, color ui.Color, size float32, blockCode bool) ui.Span {
	p, settings := a.pal, a.settings
	span := ui.Span{Text: r.text, Color: color, Italic: r.italic, Strikethrough: r.strike}
	if p.mono || r.code || blockCode {
		span.Font, span.Weight, span.Features = settings.FontFamily, settings.FontWeight, strings.Join(settings.FontFeatures, ",")
	}
	if r.bold {
		span.Weight = 700
	}
	if r.raw || r.image != "" && r.bitmap == nil {
		span.Color = p.muted
	}
	if r.image != "" && r.bitmap == nil && !blockCode {
		// A picture not shown is its alt text on a chip, as a badge.
		span.Background, span.Radius = p.surfaceHover, 4
		span.Text = "\u2009" + span.Text + "\u2009"
	}
	if r.code {
		if blockCode {
			if col := p.syntax[r.kind]; col.A > 0 {
				span.Color = col
			}
		} else {
			// Code within text is a size smaller, on a chip hugging it.
			span.Background, span.Radius = p.surfaceHover, 4
			span.Size = size * 0.9
			span.Text = "\u2009" + span.Text + "\u2009"
		}
	}
	if r.link != "" && r.image == "" {
		span.Color = p.accent
	}
	if r.strike {
		span.DecorationColor = span.Color
	}
	return span
}

// markdownSize is the size of a block's text, the headings' as GitHub
// scales them.
func (a *app) markdownSize(b markdownBlock) float32 {
	size := a.settings.FontSize
	if b.kind == markdownHeading {
		size *= []float32{2, 1.5, 1.25, 1, 0.875, 0.85}[b.level-1]
	}
	return size
}

func (a *app) markdownSpacing(b markdownBlock) (top, bottom float32) {
	gap := a.settings.FontSize * a.settings.LineHeight * 0.75
	bottom = gap
	if b.tight || b.kind == markdownTable || b.kind == markdownTableHeader {
		bottom = 0
	}
	switch b.kind {
	case markdownHeading:
		// A heading is nearer the text it heads than the text before it.
		top, bottom = gap, gap*0.5
		if b.level <= 2 {
			top = gap * 1.4
		}
	case markdownAlertTitle:
		bottom = gap * 0.3
	}
	return
}

// markdownColor is the color of an alert of a kind, GitHub's.
func (a *app) markdownColor(k markdownAlert) ui.Color {
	p := a.pal
	if p.mono {
		return p.accent
	}
	dark := [...]string{"", "#4493f8", "#3fb950", "#ab7df8", "#d29922", "#f85149"}
	light := [...]string{"", "#0969da", "#1a7f37", "#8250df", "#9a6700", "#d1242f"}
	if p.dark {
		return ui.Hex(dark[k])
	}
	return ui.Hex(light[k])
}

// markdownAlertHead is the icon and the word that open an alert.
func markdownAlertHead(k markdownAlert) (*ui.SVG, string) {
	switch k {
	case alertTip:
		return iconLightbulb, locales.AlertTip()
	case alertImportant:
		return iconMessageAlert, locales.AlertImportant()
	case alertWarning:
		return iconTriangleAlert, locales.AlertWarning()
	case alertCaution:
		return iconOctagonAlert, locales.AlertCaution()
	}
	return iconInfo, locales.AlertNote()
}

func (a *app) markdownText(c *ui.Context, m *markdownPreview, path string, runs []markdownInline, size float32, muted, blockCode bool) {
	p := a.pal
	color := p.text
	if muted {
		color = p.muted
	}
	text := ui.RichText(c).FontSize(size).LineHeight(a.settings.LineHeight).TextColor(color).DecorationColor(color).WidthPercent(100)
	if blockCode || p.mono {
		text.Font(a.settings.FontFamily).FontWeight(a.settings.FontWeight).FontFeatures(a.settings.FontFeatures...)
	}
	text.Children(func() {
		for _, r := range runs {
			span := a.markdownSpan(r, color, size, blockCode)
			if r.link != "" {
				if e := a.markdownLink(c, m, path, r.link, false); e.Valid() {
					e.Children(func() { ui.RichText(c, span) })
					continue
				}
			}
			ui.RichText(c, span)
		}
	})
}

// markdownLink is the element a link opens its destination from: a page
// in the browser, a file in Mayoi, or a heading of the document, which
// the preview scrolls to. Invalid for a destination that is none of them.
func (a *app) markdownLink(c *ui.Context, m *markdownPreview, path, dest string, image bool) ui.Element {
	u, err := url.Parse(dest)
	remote := err == nil && (u.Scheme == "http" || u.Scheme == "https")
	local, heading := "", -1
	if m != nil {
		local = markdownPath(m.root, path, dest)
		if anchor, ok := strings.CutPrefix(dest, "#"); ok && anchor != "" {
			if s, err := url.PathUnescape(anchor); err == nil {
				anchor = s
			}
			anchor = strings.ToLower(anchor)
			heading = slices.IndexFunc(m.blocks, func(b markdownBlock) bool { return b.slug == anchor })
		}
	} else {
		local = markdownPath("", path, dest)
	}
	if !remote && local == "" && heading < 0 {
		return ui.Element{}
	}
	e := ui.Element{}
	if image {
		e = ui.Box(c).Cursor(ui.CursorPointer).Focusable()
	} else {
		e = ui.Link(c, "", "")
	}
	e = e.Label(locales.OpenLink(dest))
	switch {
	case remote:
		return e.OnClick(func() { c.OpenURL(dest) })
	case heading >= 0:
		return e.OnClick(func() { m.list.ScrollTo(heading, ui.Start) })
	}
	return e.OnClick(func() { a.openFile(local, true) })
}

func (a *app) markdownImage(c *ui.Context, m *markdownPreview, path string, r markdownInline, width float32) {
	draw := func() {
		w := min(r.w, max(1, width))
		ui.Image(c, r.bitmap).Width(w).Height(w * r.h / r.w).Fit(ui.Contain)
	}

	if e := a.markdownLink(c, m, path, r.link, true); e.Valid() {
		e.Children(draw)
	} else {
		draw()
	}
}

// markdownCodeCard is a block of code on its own card, its language in the
// corner, where a copy button shows as the pointer comes over it.
func (a *app) markdownCodeCard(c *ui.Context, m *markdownPreview, path string, b markdownBlock, size float32) {
	p := a.pal
	card := ui.Column(c.Key("code")).WidthPercent(100).Padding(size*0.85, size).Background(p.chrome).Border(1, p.border).Radius(8)
	over := card.Hovered()
	card.Children(func() {
		a.markdownText(c, m, path, b.runs, size*0.93, b.kind == markdownHTML, true)
		if b.kind != markdownCodeBlock || !over && b.language == "" {
			return
		}
		ui.Row(c).Absolute().Top(size * 0.4).Right(size * 0.4).Background(p.chrome).Radius(4).Children(func() {
			if !over {
				ui.Text(c, b.language).FontSize(size*0.8).TextColor(p.faint).SingleLine().Padding(2, 6)
				return
			}
			if a.smallButton(c, iconCopy, locales.CopyCode()) {
				a.services.WriteClipboard(markdownRunsText(b.runs))
				a.tell(note{message: locales.CopiedCode()})
			}
		})
	})
}

// markdownTableRow is a row of a table, its columns as wide as their
// text asks, its body's rows striped.
func (a *app) markdownTableRow(c *ui.Context, m *markdownPreview, path string, b markdownBlock, size, width float32) {
	p := a.pal
	ui.Row(c).WidthPercent(100).Children(func() {
		for i, cell := range b.cells {
			share := 1 / float32(len(b.cells))
			if i < len(b.widths) {
				share = b.widths[i]
			}
			cellBox := ui.Column(c).WidthPercent(100*share).Padding(size*0.4, size*0.75).Border(1, p.border).BorderWidth(0, 1, 1, 0)
			if i == 0 {
				cellBox.BorderWidth(0, 1, 1, 1)
			}
			switch {
			case b.kind == markdownTableHeader:
				cellBox.Background(p.chrome)
				if i == 0 {
					cellBox.BorderWidth(1)
				} else {
					cellBox.BorderWidth(1, 1, 1, 0)
				}
			case b.row%2 == 0:
				cellBox.Background(p.hover)
			}
			cellBox.Children(func() {
				align := ui.Start
				if i < len(b.aligns) {
					switch b.aligns[i] {
					case east.AlignCenter:
						align = ui.Center
					case east.AlignRight:
						align = ui.End
					}
				}
				ui.Column(c).TextAlign(align).WidthPercent(100).Children(func() {
					for len(cell) > 0 {
						n := 0
						for n < len(cell) && cell[n].bitmap == nil {
							n++
						}
						if n > 0 {
							a.markdownText(c, m, path, cell[:n], size, false, false)
						}
						if n < len(cell) {
							a.markdownImage(c, m, path, cell[n], width*share-size*1.5)
							n++
						}
						cell = cell[n:]
					}
				})
			})
		}
	})
}

func (a *app) markdownView(c *ui.Context, v *viewer, name string) {
	a.followMarkdown(v)
	m, p := v.markdown, a.pal
	size, lh := a.settings.FontSize, a.settings.LineHeight
	path := v.doc.path
	// Measure the actual body font, including the Matrix UI font. The
	// column is 120 digits wide, room for tables and images while a line
	// of prose stays short enough to read.
	font := ui.Font{Family: c.Theme().Font, Size: size}
	if p.mono {
		font.Weight, font.Features = a.settings.FontWeight, strings.Join(a.settings.FontFeatures, ",")
	}
	if m.measured != font || m.measure == 0 {
		m.measured = font
		m.measure, _ = ui.ShapeText(strings.Repeat("0", 120), font, 0).Size()
	}
	if v.loaded && len(m.blocks) == 0 {
		ui.Column(c.Key("markdown-empty")).Fill().Center().Background(p.editor).Label(locales.PreviewLabel(name)).Children(func() {
			ui.Icon(c, iconFileText).FontSize(28).TextColor(p.faint)
			ui.Text(c, locales.PreviewEmpty()).FontSize(13).TextColor(p.muted).Margin(10, 0, 0, 0)
		})
		return
	}
	width := min(m.measure, max(1, v.pane.Bounds(c).W-2*textPadR))
	ui.List(c.Key("markdown"), &m.list, len(m.blocks)).Bind(&v.pane).TrackScroll(&m.scroll).Fill().Background(p.editor).Label(locales.PreviewLabel(name)).Padding(editorPadY*2, textPadR, editorPadY*4, textPadR).Selectable().Rows(func(row ui.ListRow) {
		c := row.Context
		b := m.blocks[row.Index]
		top, bottom := a.markdownSpacing(b)
		if row.Index == 0 {
			top = 0
		}
		if (b.tight && (row.Index+1 == len(m.blocks) || !m.blocks[row.Index+1].tight)) || (b.kind == markdownTable && (row.Index+1 == len(m.blocks) || m.blocks[row.Index+1].kind != markdownTable)) {
			bottom = size * lh * 0.75
		}
		outside := float32(0)
		if b.quote > 0 && (row.Index+1 == len(m.blocks) || m.blocks[row.Index+1].quote < b.quote) {
			outside, bottom = max(bottom, size*lh*0.75), 0
		}
		ui.Row(c).WidthPercent(100).Justify(ui.Center).Margin(0, 0, outside, 0).Children(func() {
			ui.Row(c).WidthPercent(100).MaxWidth(m.measure).AlignItems(ui.Start).Padding(0, 0, 0, b.offset*size).Gap(size * 0.5).Children(func() {
				for i := range b.quote {
					bar := p.guideNow
					if b.alert != alertNone && i == b.alertAt-1 {
						bar = a.markdownColor(b.alert)
					}
					ui.Box(c).Width(3).AlignSelf(ui.Stretch).Background(bar).Margin(0, size*0.25, 0, 0)
				}
				if b.marker != "" {
					a.markdownMarker(c, b, size)
				}

				ui.Column(c).Grow(1).MinWidth(0).Padding(top, 0, bottom, 0).Children(func() {
					switch b.kind {
					case markdownRule:
						ui.Box(c).Height(2).WidthPercent(100).Radius(1).Background(p.border).Margin(size*lh*0.375, 0)
					case markdownImageBlock:
						imageWidth := max(1, width-(b.offset+b.markerWidth)*size-float32(b.quote)*size)
						a.markdownImage(c, m, path, b.runs[0], imageWidth)
					case markdownCodeBlock, markdownHTML:
						a.markdownCodeCard(c, m, path, b, size)
					case markdownTable, markdownTableHeader:
						a.markdownTableRow(c, m, path, b, size, width)
					case markdownAlertTitle:
						icon, title := markdownAlertHead(b.alert)
						col := a.markdownColor(b.alert)
						ui.Row(c).AlignItems(ui.Center).Gap(size * 0.45).Height(size * lh).Children(func() {
							ui.Icon(c, icon).FontSize(size * 1.1).TextColor(col)
							ui.Text(c, title).FontSize(size).FontWeight(600).TextColor(col).SingleLine()
						})
					default:
						// Quotes and the smallest headings are muted, as on
						// GitHub; the text of an alert is not.
						muted := b.quote > b.alertAt || b.kind == markdownHeading && b.level == 6
						a.markdownText(c, m, path, b.runs, a.markdownSize(b), muted, false)
						if b.kind == markdownHeading && b.level <= 2 {
							// A hairline under the largest headings, as GitHub draws it.
							ui.Box(c).Height(1).WidthPercent(100).Background(p.border).Margin(size*0.4, 0, 0, 0)
						}
					}
				})
			})
		})
	})
}

// markdownMarker is the marker of a list's item: a dot, a ring for a list
// inside another, a task's box, or a number.
func (a *app) markdownMarker(c *ui.Context, b markdownBlock, size float32) {
	p, lh := a.pal, a.settings.LineHeight
	switch b.marker {
	case "•":
		// A dot in the middle of the marker's room, a ring for a list
		// inside another, as GitHub draws them.
		d := size * 0.32
		ui.Box(c).Width(size * b.markerWidth).Height(size * lh).AlignItems(ui.Center).Children(func() {
			dot := ui.Box(c).Width(d).Height(d).Radius(d/2).Margin((size*lh-d)/2, 0)
			if b.indent > 1 {
				dot.Border(1.25, p.muted)
			} else {
				dot.Background(p.muted)
			}
		})
	case "☐", "☑":
		// A task's box, in the middle of the marker's room as a dot is,
		// filled with the accent once done.
		d := size * 0.9
		ui.Box(c).Width(size * b.markerWidth).Height(size * lh).AlignItems(ui.Center).Children(func() {
			box := ui.Box(c).Width(d).Height(d).Radius(3).Margin((size*lh-d)/2, 0).
				Justify(ui.Center).AlignItems(ui.Center)
			if b.marker == "☑" {
				box.Background(p.fill).Children(func() {
					ui.Icon(c, iconCheck).FontSize(d * 0.8).TextColor(p.fillText)
				})
			} else {
				box.Border(1.25, p.muted)
			}
		})
	default:
		// A list's number stays out of what is selected and copied.
		ui.Text(c, b.marker).Width(size * b.markerWidth).TextColor(p.muted).FontSize(size).LineHeight(lh).TextAlign(ui.End).Unselectable()
	}
}
