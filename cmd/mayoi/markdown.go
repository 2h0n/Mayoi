package main

import (
	"html"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"mayoi/internal/highlight"

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
	bitmap                          *ui.Bitmap
	w, h                            int
	kind                            highlight.Kind
}

// markdownBlock is a paragraph-sized row, including its enclosing lists and quotes.
type markdownBlock struct {
	kind                 string
	tight                bool
	level, indent, quote int
	marker, language     string
	runs                 []markdownInline
	cells                [][]markdownInline
	aligns               []east.Alignment
}

func isMarkdown(d *document) bool {
	ext := filepath.Ext(d.path)
	return strings.EqualFold(ext, ".md") || strings.EqualFold(ext, ".markdown")
}

func canPreview(d *document) bool { return isSVG(d) || isMarkdown(d) }

// markdownPath resolves local destinations without fetching remote resources.
func markdownPath(path, dest string) string {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" {
		return ""
	}
	if filepath.IsAbs(u.Path) {
		return filepath.Clean(u.Path)
	}
	return filepath.Join(filepath.Dir(path), filepath.FromSlash(u.Path))
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
func parseMarkdown(source []byte, path string, current func() bool) []markdownBlock {
	root := markdownParser().Parse(text.NewReader(source))
	var blocks []markdownBlock
	var inline func(ast.Node, markdownInline) []markdownInline
	inline = func(n ast.Node, style markdownInline) []markdownInline {
		var out []markdownInline
		for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
			s := style
			switch x := ch.(type) {
			case *ast.Text:
				s.text = string(x.Segment.Value(source))
				if !x.IsRaw() {
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
				if local := markdownPath(path, s.image); local != "" && (current == nil || current()) {
					s.bitmap, s.w, s.h, _, _ = decodeImage(local)
				}
			case *ast.RawHTML:
				s.raw = true
				s.text = string(x.Segments.Value(source))
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
	var walk func(ast.Node, int, int, string, bool)
	walk = func(n ast.Node, indent, quote int, marker string, tight bool) {
		b := markdownBlock{indent: indent, quote: quote, marker: marker, tight: tight}
		switch x := n.(type) {
		case *ast.Heading:
			b.kind, b.level = "heading", x.Level
			b.runs = inline(n, markdownInline{bold: true})
		case *ast.Paragraph, *ast.TextBlock:
			b.kind = "paragraph"
			b.runs = inline(n, markdownInline{})
			for _, r := range b.runs {
				if strings.HasPrefix(r.text, "☐") || strings.HasPrefix(r.text, "☑") {
					b.marker = ""
					break
				}
			}
		case *ast.ThematicBreak:
			b.kind = "rule"
		case *ast.FencedCodeBlock:
			b.kind, b.language = "code", string(x.Language(source))
			b.runs = markdownCode(string(x.Lines().Value(source)), b.language)
		case *ast.CodeBlock:
			b.kind = "code"
			b.runs = markdownCode(string(x.Lines().Value(source)), "")
		case *ast.HTMLBlock:
			raw := string(x.Lines().Value(source))
			if x.HasClosure() {
				raw += string(x.ClosureLine.Value(source))
			}
			b.kind = "html"
			b.runs = []markdownInline{{text: raw, raw: true}}
		case *ast.Blockquote:
			for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
				walk(ch, indent, quote+1, marker, tight)
				marker = ""
			}
			return
		case *ast.List:
			i := x.Start
			for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
				m := "•"
				if x.IsOrdered() {
					m = strconv.Itoa(i) + "."
					i++
				}
				walk(ch, indent+1, quote, m, x.IsTight)
			}
			return
		case *east.Table:
			for row := n.FirstChild(); row != nil; row = row.NextSibling() {
				rb := b
				rb.kind = "table"
				rb.aligns = x.Alignments
				_, head := row.(*east.TableHeader)
				if head {
					rb.kind = "tableHeader"
				}
				for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
					rb.cells = append(rb.cells, inline(cell, markdownInline{bold: head}))
				}
				blocks = append(blocks, rb)
			}
			return
		default:
			for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
				walk(ch, indent, quote, marker, tight)
				marker = ""
			}
			return
		}
		// Images get their own rows so that paragraphs retain native text wrapping.
		if b.kind == "paragraph" || b.kind == "heading" {
			runs := b.runs
			b.runs = nil
			for _, r := range runs {
				if r.image != "" {
					if markdownHasText(b.runs) {
						blocks = append(blocks, b)
						b.runs = nil
						b.marker = ""
					}
					ib := b
					ib.kind = "image"
					ib.runs = []markdownInline{r}
					blocks = append(blocks, ib)
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
	walk(root, 0, 0, "", false)
	return blocks
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
		lines = highlight.Lex(lexers.Get(language), strings.TrimSuffix(code, "\n"))
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
	d, m := v.svg, v.markdown
	ver := d.buf.Version()
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
		source, path := []byte(d.buf.String()), d.path
		go func() {
			blocks := parseMarkdown(source, path, func() bool { return m.generation.Load() == generation })
			a.post(func() {
				m.running, m.pending = false, false
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

// markdownSpan resolves every inline style before either a span or link is built.
func (a *app) markdownSpan(r markdownInline, color ui.Color, blockCode bool) ui.Span {
	p, settings := a.pal, a.settings
	span := ui.Span{Text: r.text, Color: color, Italic: r.italic, Strikethrough: r.strike}
	if p.mono || r.code || blockCode {
		span.Font, span.Weight, span.Features = settings.FontFamily, settings.FontWeight, strings.Join(settings.FontFeatures, ",")
	}
	if r.bold {
		span.Weight = 700
	}
	if r.raw {
		span.Color = p.muted
	}
	if r.code {
		if blockCode {
			if col := p.syntax[r.kind]; col.A > 0 {
				span.Color = col
			}
		} else {
			span.Background = p.surfaceHover
			span.Text = "\u2009" + span.Text + "\u2009"
		}
	}
	if r.link != "" {
		span.Color = p.accent
	}
	if r.strike {
		span.DecorationColor = span.Color
	}
	return span
}

func (a *app) markdownSize(b markdownBlock) float32 {
	size := a.settings.FontSize
	if b.kind == "heading" {
		size *= []float32{1.6, 1.35, 1.15, 1, 1, 1}[b.level-1]
	}
	return size
}

func (a *app) markdownSpacing(b markdownBlock) (top, bottom float32) {
	gap := a.settings.FontSize * a.settings.LineHeight * 0.75
	bottom = gap
	if b.tight || b.kind == "table" || b.kind == "tableHeader" {
		bottom = 0
	}
	if b.kind == "heading" {
		top, bottom = gap, gap*0.5
	}
	return
}

func (a *app) markdownText(c *ui.Context, runs []markdownInline, size float32, muted bool, path string, blockCode bool) {
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
			span := a.markdownSpan(r, color, blockCode)
			if r.link != "" {
				if e := a.markdownLink(c, r.link, path, false); e.Valid() {
					e.Children(func() { ui.RichText(c, span) })
					continue
				}
			}
			ui.RichText(c, span)
		}
	})
}

func (a *app) markdownLink(c *ui.Context, dest, path string, image bool) ui.Element {
	u, err := url.Parse(dest)
	remote := err == nil && (u.Scheme == "http" || u.Scheme == "https")
	local := markdownPath(path, dest)
	if !remote && local == "" {
		return ui.Element{}
	}
	e := ui.Element{}
	if image {
		e = ui.Box(c).Cursor(ui.CursorPointer).Focusable()
	} else {
		e = ui.Link(c, "", "")
	}
	e = e.Label("Open link " + dest)
	if remote {
		return e.OnClick(func() { c.OpenURL(dest) })
	}
	return e.OnClick(func() { a.openFile(local, true) })
}

func (a *app) markdownImage(c *ui.Context, r markdownInline, size, width float32, path string) {
	draw := func() {
		if r.bitmap != nil {
			w := min(float32(r.w), max(1, width))
			ui.Image(c, r.bitmap).Width(w).Height(w * float32(r.h) / float32(r.w)).Fit(ui.Contain)
		} else {
			a.markdownText(c, []markdownInline{{text: "[Image: " + r.text + "]"}}, size, true, path, false)
		}
	}
	if e := a.markdownLink(c, r.link, path, true); e.Valid() {
		e.Children(draw)
	} else {
		draw()
	}
}

func (a *app) markdownView(c *ui.Context, v *viewer, name string) {
	a.followMarkdown(v)
	m, p := v.markdown, a.pal
	size := a.settings.FontSize
	// Measure the actual body font, including the Matrix UI font.
	font := ui.Font{Family: c.Theme().Font, Size: size}
	if p.mono {
		font.Weight, font.Features = a.settings.FontWeight, strings.Join(a.settings.FontFeatures, ",")
	}
	width, _ := ui.ShapeText(strings.Repeat("0", 80), font, 0).Size()
	ui.List(c.Key("markdown"), &m.list, len(m.blocks)).Bind(&v.pane).TrackScroll(&m.scroll).Fill().Background(p.editor).Label("Preview "+name).Padding(editorPadY, textPadR).Rows(func(row ui.ListRow) {
		c := row.Context
		b := m.blocks[row.Index]
		ui.Row(c).WidthPercent(100).Justify(ui.Center).Children(func() {
			ui.Row(c).WidthPercent(100).MaxWidth(width).AlignItems(ui.Start).Padding(0, 0, 0, float32(max(0, b.indent-1))*size*1.5).Gap(size * 0.5).Children(func() {
				for range b.quote {
					ui.Box(c).Width(3).AlignSelf(ui.Stretch).Background(p.guideNow)
				}
				if b.marker != "" {
					if b.marker == "•" {
						ui.Box(c).Width(size * 1.5).Height(size * a.settings.LineHeight).Children(func() {
							ui.Box(c).Width(size*0.35).Height(size*0.35).Radius(size*0.175).Background(p.text).Margin((size*a.settings.LineHeight-size*0.35)/2, 0)
						})
					} else {
						ui.Text(c, b.marker).Width(size * 1.5).TextColor(p.text).FontSize(size).LineHeight(a.settings.LineHeight)
					}
				}
				top, bottom := a.markdownSpacing(b)
				if (b.tight && (row.Index+1 == len(m.blocks) || !m.blocks[row.Index+1].tight)) || (b.kind == "table" && (row.Index+1 == len(m.blocks) || m.blocks[row.Index+1].kind != "table")) {
					bottom = size * a.settings.LineHeight * 0.75
				}

				ui.Column(c).Grow(1).MinWidth(0).Padding(top, 0, bottom, 0).Children(func() {
					switch b.kind {
					case "rule":
						ui.Box(c).Height(1).WidthPercent(100).Background(p.border).Margin(size*a.settings.LineHeight*0.375, 0)
					case "image":
						imageWidth := max(1, min(width, v.pane.Bounds(c).W-2*textPadR)-float32(b.indent)*size*1.5-float32(b.quote)*size)
						a.markdownImage(c, b.runs[0], size, imageWidth, v.svg.path)
					case "code", "html":
						ui.Column(c).WidthPercent(100).Padding(size*0.5, size*0.75).Background(p.chrome).Radius(6).Children(func() {
							a.markdownText(c, b.runs, size, b.kind == "html", v.svg.path, true)
						})
					case "table", "tableHeader":
						ui.Row(c).WidthPercent(100).Children(func() {
							for i, cell := range b.cells {
								cellBox := ui.Column(c).WidthPercent(100/float32(len(b.cells))).Padding(size*0.35, size*0.5).Border(1, p.border)
								if b.kind == "tableHeader" {
									cellBox.Background(p.chrome)
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
											for n < len(cell) && cell[n].image == "" {
												n++
											}
											if n > 0 {
												a.markdownText(c, cell[:n], size, false, v.svg.path, false)
											}
											if n < len(cell) {
												a.markdownImage(c, cell[n], size, width/float32(len(b.cells))-size, v.svg.path)
												n++
											}
											cell = cell[n:]
										}
									})
								})
							}
						})
					default:
						a.markdownText(c, b.runs, a.markdownSize(b), b.kind == "heading" && b.level == 6, v.svg.path, false)
					}
				})
			})
		})
	})
}
