package main

import (
	"github.com/egoist/mygo/ui"

	"mayoi/internal/highlight"
)

// palette is Mayoi's look: the colors of its chrome and of the code, for
// a light or a dark window. Widgets take theirs from the ui.Theme that
// apply derives from it.
type palette struct {
	dark bool

	editor    ui.Color // the editor and the terminal
	chrome    ui.Color // the sidebar, the tab strip and the panel's header
	panel     ui.Color // the bottom panel
	border    ui.Color
	hover     ui.Color
	active    ui.Color // a chosen row
	text      ui.Color
	muted     ui.Color
	faint     ui.Color
	accent    ui.Color
	accentDim ui.Color
	line      ui.Color // the caret's line
	selection ui.Color
	gutter    ui.Color // line numbers
	gutterNow ui.Color // the caret's line number
	status    ui.Color
	statusFg  ui.Color
	modified  ui.Color
	danger    ui.Color
	match     ui.Color // a match of find
	matchNow  ui.Color // the current match
	matchRing ui.Color
	guide     ui.Color // indent guides
	guideNow  ui.Color // the guide of the caret's block
	bracket   ui.Color // the brackets paired at the caret
	word      ui.Color // the other occurrences of the word at the caret
	shadow    ui.Color
	added     ui.Color // lines changed since Git's last commit
	changed   ui.Color
	removed   ui.Color

	syntax [highlight.Kinds]ui.Color
}

func darkPalette() *palette {
	p := &palette{
		dark:      true,
		editor:    ui.Hex("#18181d"),
		chrome:    ui.Hex("#131317"),
		panel:     ui.Hex("#151519"),
		border:    ui.Hex("#26262e"),
		hover:     ui.RGBA(255, 255, 255, 0.045),
		active:    ui.RGBA(139, 124, 246, 0.16),
		text:      ui.Hex("#d6d6de"),
		muted:     ui.Hex("#8c8c99"),
		faint:     ui.Hex("#55555f"),
		accent:    ui.Hex("#8b7cf6"),
		accentDim: ui.RGBA(139, 124, 246, 0.55),
		line:      ui.RGBA(255, 255, 255, 0.035),
		selection: ui.RGBA(139, 124, 246, 0.30),
		gutter:    ui.Hex("#4a4a55"),
		gutterNow: ui.Hex("#b9b9c6"),
		status:    ui.Hex("#131317"),
		statusFg:  ui.Hex("#8c8c99"),
		modified:  ui.Hex("#e8b86d"),
		danger:    ui.Hex("#f0718b"),
		match:     ui.RGBA(232, 184, 109, 0.20),
		matchNow:  ui.RGBA(232, 184, 109, 0.38),
		matchRing: ui.RGBA(232, 184, 109, 0.85),
		guide:     ui.RGBA(255, 255, 255, 0.065),
		guideNow:  ui.RGBA(255, 255, 255, 0.22),
		bracket:   ui.RGBA(255, 255, 255, 0.30),
		word:      ui.RGBA(255, 255, 255, 0.075),
		shadow:    ui.RGBA(0, 0, 0, 0.35),
		added:     ui.Hex("#4fa86d"),
		changed:   ui.Hex("#5a8ee0"),
		removed:   ui.Hex("#e0607e"),
	}
	s := &p.syntax
	s[highlight.Plain] = p.text
	s[highlight.Keyword] = ui.Hex("#b99cf7")
	s[highlight.Type] = ui.Hex("#7dcfe0")
	s[highlight.Function] = ui.Hex("#8fb4ff")
	s[highlight.String] = ui.Hex("#a9d48f")
	s[highlight.Number] = ui.Hex("#f2a679")
	s[highlight.Comment] = ui.Hex("#686876")
	s[highlight.Operator] = ui.Hex("#a0a5b8")
	s[highlight.Punctuation] = ui.Hex("#8a8e9e")
	s[highlight.Constant] = ui.Hex("#f2a679")
	s[highlight.Builtin] = ui.Hex("#e8c482")
	s[highlight.Tag] = ui.Hex("#f590a8")
	s[highlight.Attribute] = ui.Hex("#e8c482")
	s[highlight.Variable] = ui.Hex("#e2e2ea")
	s[highlight.Heading] = ui.Hex("#8fb4ff")
	s[highlight.Deleted] = ui.Hex("#f0718b")
	s[highlight.Inserted] = ui.Hex("#a9d48f")
	return p
}

func lightPalette() *palette {
	p := &palette{
		editor:    ui.Hex("#fcfcfd"),
		chrome:    ui.Hex("#f4f4f7"),
		panel:     ui.Hex("#f8f8fa"),
		border:    ui.Hex("#e4e4ea"),
		hover:     ui.RGBA(0, 0, 0, 0.04),
		active:    ui.RGBA(109, 91, 208, 0.12),
		text:      ui.Hex("#2a2a33"),
		muted:     ui.Hex("#6c6c79"),
		faint:     ui.Hex("#a3a3ae"),
		accent:    ui.Hex("#6d5bd0"),
		accentDim: ui.RGBA(109, 91, 208, 0.5),
		line:      ui.RGBA(0, 0, 0, 0.03),
		selection: ui.RGBA(109, 91, 208, 0.20),
		gutter:    ui.Hex("#b4b4be"),
		gutterNow: ui.Hex("#4a4a55"),
		status:    ui.Hex("#f4f4f7"),
		statusFg:  ui.Hex("#6c6c79"),
		modified:  ui.Hex("#c4862a"),
		danger:    ui.Hex("#d0365a"),
		match:     ui.RGBA(230, 160, 40, 0.22),
		matchNow:  ui.RGBA(230, 160, 40, 0.42),
		matchRing: ui.RGBA(196, 120, 20, 0.9),
		guide:     ui.RGBA(0, 0, 0, 0.07),
		guideNow:  ui.RGBA(0, 0, 0, 0.24),
		bracket:   ui.RGBA(0, 0, 0, 0.28),
		word:      ui.RGBA(0, 0, 0, 0.065),
		shadow:    ui.RGBA(0, 0, 0, 0.12),
		added:     ui.Hex("#3fa363"),
		changed:   ui.Hex("#4a7fd4"),
		removed:   ui.Hex("#d6456a"),
	}
	s := &p.syntax
	s[highlight.Plain] = p.text
	s[highlight.Keyword] = ui.Hex("#7c3bd6")
	s[highlight.Type] = ui.Hex("#0b7c99")
	s[highlight.Function] = ui.Hex("#2a5bd7")
	s[highlight.String] = ui.Hex("#3d8a28")
	s[highlight.Number] = ui.Hex("#c2571a")
	s[highlight.Comment] = ui.Hex("#9898a5")
	s[highlight.Operator] = ui.Hex("#555a6b")
	s[highlight.Punctuation] = ui.Hex("#6c6f80")
	s[highlight.Constant] = ui.Hex("#c2571a")
	s[highlight.Builtin] = ui.Hex("#a86a0c")
	s[highlight.Tag] = ui.Hex("#c8264f")
	s[highlight.Attribute] = ui.Hex("#a86a0c")
	s[highlight.Variable] = ui.Hex("#2a2a33")
	s[highlight.Heading] = ui.Hex("#2a5bd7")
	s[highlight.Deleted] = ui.Hex("#c8264f")
	s[highlight.Inserted] = ui.Hex("#3d8a28")
	return p
}

var palettes = [2]*palette{lightPalette(), darkPalette()}

// apply sets the window's theme from the palette of its appearance, and
// returns that palette.
func applyTheme(c *ui.Context) *palette {
	t := *c.Theme()
	p := palettes[0]
	if t.Dark {
		p = palettes[1]
	}
	t.Background = p.editor
	t.Accent = p.accent
	t.Selection = p.selection
	t.Border = p.border
	t.Text, t.TextMuted = p.text, p.muted
	t.Focus = p.accentDim
	t.Radius = 6
	t.Spacing = 3.5
	t.FontSize = 13
	t.ScrollbarWidth = 8
	if p.dark {
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#202027"), ui.Hex("#272730"), ui.Hex("#2e2e38")
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.16)
		t.Inverse, t.InverseText = ui.Hex("#2b2b34"), ui.Hex("#e6e6ee")
	} else {
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#ffffff"), ui.Hex("#f2f2f6"), ui.Hex("#eaeaf0")
		t.Scrollbar = ui.RGBA(0, 0, 0, 0.18)
	}
	c.SetTheme(&t)
	return p
}
