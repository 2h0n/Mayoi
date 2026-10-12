package main

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mayoi/internal/highlight"
)

// palette is Mayoi's look: the colors of its chrome, of the code and of
// the terminal, for a light or a dark window, or for a theme of its own as
// The Matrix. Widgets take theirs from the ui.Theme that applyTheme
// derives from it.
type palette struct {
	dark bool
	// mono draws the whole window in the code's font, and the icons of
	// files in the palette's own colors rather than their languages'.
	mono bool

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
	// fill is the accent as a button's background, under fillText, and
	// fillHover and fillPressed the button under the pointer and pressed.
	fill, fillHover, fillPressed, fillText ui.Color
	// The faces of controls, as the theme's Surface, SurfaceHover and
	// SurfacePressed.
	surface, surfaceHover, surfacePressed ui.Color
	scrollbar                             ui.Color
	// inverse fills tooltips and toasts, under inverseText: zero for the
	// theme's own.
	inverse, inverseText ui.Color
	line                 ui.Color // the caret's line
	selection            ui.Color
	// The selection as the editor paints it, opaque over its background:
	// while it has the keyboard, and fainter while it has not.
	selected, selectedDim ui.Color
	gutter                ui.Color // line numbers
	gutterNow             ui.Color // the caret's line number
	status                ui.Color
	statusFg              ui.Color
	modified              ui.Color
	danger                ui.Color
	match                 ui.Color // a match of find
	matchNow              ui.Color // the current match
	matchRing             ui.Color
	guide                 ui.Color // indent guides
	guideNow              ui.Color // the guide of the caret's block
	bracket               ui.Color // the brackets paired at the caret
	word                  ui.Color // the other occurrences of the word at the caret
	cursor                ui.Color // the block cursor of vim's normal mode
	shadow                ui.Color
	added                 ui.Color // lines changed since Git's last commit
	changed               ui.Color
	removed               ui.Color

	syntax [highlight.Kinds]ui.Color
	// ansi is the terminal's 16 colors: black, red, green, yellow, blue,
	// magenta, cyan and white, then their bright kinds.
	ansi [16]ui.Color
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
		faint:     ui.Hex("#666672"),
		accent:    ui.Hex("#8b7cf6"),
		accentDim: ui.RGBA(139, 124, 246, 0.55),
		line:      ui.RGBA(255, 255, 255, 0.035),
		selection: ui.RGBA(139, 124, 246, 0.30),
		gutter:    ui.Hex("#62626e"),
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
		// The dark accent is light, for marks on a dark ground: white on
		// it would be hard to read, so buttons take a deeper purple, with
		// white on it at 4.6:1.
		fill:           ui.Hex("#7260e3"),
		fillText:       ui.Hex("#ffffff"),
		surface:        ui.Hex("#202027"),
		surfaceHover:   ui.Hex("#272730"),
		surfacePressed: ui.Hex("#2e2e38"),
		scrollbar:      ui.RGBA(255, 255, 255, 0.16),
		inverse:        ui.Hex("#2b2b34"),
		inverseText:    ui.Hex("#e6e6ee"),
		ansi: [16]ui.Color{
			ui.Hex("#2a2a33"), ui.Hex("#f0718b"), ui.Hex("#a9d48f"), ui.Hex("#e8c482"),
			ui.Hex("#8fb4ff"), ui.Hex("#b99cf7"), ui.Hex("#7dcfe0"), ui.Hex("#d6d6de"),
			ui.Hex("#5a5a66"), ui.Hex("#ff8fa5"), ui.Hex("#bfe6a5"), ui.Hex("#f5d699"),
			ui.Hex("#a9c6ff"), ui.Hex("#cdb5fa"), ui.Hex("#9ee0ee"), ui.Hex("#ffffff"),
		},
	}
	p.fillHover, p.fillPressed = p.fill.Mix(white, 0.1), p.fill.Mix(black, 0.15)
	s := &p.syntax
	s[highlight.Plain] = p.text
	s[highlight.Keyword] = ui.Hex("#b99cf7")
	s[highlight.Type] = ui.Hex("#7dcfe0")
	s[highlight.Function] = ui.Hex("#8fb4ff")
	s[highlight.String] = ui.Hex("#a9d48f")
	s[highlight.Number] = ui.Hex("#f2a679")
	s[highlight.Comment] = ui.Hex("#747481")
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
		editor:         ui.Hex("#fcfcfd"),
		chrome:         ui.Hex("#f4f4f7"),
		panel:          ui.Hex("#f8f8fa"),
		border:         ui.Hex("#e4e4ea"),
		hover:          ui.RGBA(0, 0, 0, 0.04),
		active:         ui.RGBA(109, 91, 208, 0.12),
		text:           ui.Hex("#2a2a33"),
		muted:          ui.Hex("#6c6c79"),
		faint:          ui.Hex("#8e8e9a"),
		accent:         ui.Hex("#6d5bd0"),
		accentDim:      ui.RGBA(109, 91, 208, 0.5),
		line:           ui.RGBA(0, 0, 0, 0.03),
		selection:      ui.RGBA(109, 91, 208, 0.20),
		gutter:         ui.Hex("#93939f"),
		gutterNow:      ui.Hex("#4a4a55"),
		status:         ui.Hex("#f4f4f7"),
		statusFg:       ui.Hex("#6c6c79"),
		modified:       ui.Hex("#c4862a"),
		danger:         ui.Hex("#d0365a"),
		match:          ui.RGBA(230, 160, 40, 0.22),
		matchNow:       ui.RGBA(230, 160, 40, 0.42),
		matchRing:      ui.RGBA(196, 120, 20, 0.9),
		guide:          ui.RGBA(0, 0, 0, 0.07),
		guideNow:       ui.RGBA(0, 0, 0, 0.24),
		bracket:        ui.RGBA(0, 0, 0, 0.28),
		word:           ui.RGBA(0, 0, 0, 0.065),
		shadow:         ui.RGBA(0, 0, 0, 0.12),
		added:          ui.Hex("#3fa363"),
		changed:        ui.Hex("#4a7fd4"),
		removed:        ui.Hex("#d6456a"),
		fill:           ui.Hex("#6d5bd0"),
		fillText:       ui.Hex("#ffffff"),
		surface:        ui.Hex("#ffffff"),
		surfaceHover:   ui.Hex("#f2f2f6"),
		surfacePressed: ui.Hex("#eaeaf0"),
		scrollbar:      ui.RGBA(0, 0, 0, 0.18),
		ansi: [16]ui.Color{
			ui.Hex("#2a2a33"), ui.Hex("#c8264f"), ui.Hex("#3d8a28"), ui.Hex("#a86a0c"),
			ui.Hex("#2a5bd7"), ui.Hex("#7c3bd6"), ui.Hex("#0b7c99"), ui.Hex("#a3a3ae"),
			ui.Hex("#6c6c79"), ui.Hex("#e0405f"), ui.Hex("#4fa63a"), ui.Hex("#c4862a"),
			ui.Hex("#4a78e8"), ui.Hex("#9558e8"), ui.Hex("#1a9cbd"), ui.Hex("#d6d6de"),
		},
	}
	p.fillHover, p.fillPressed = p.fill.Mix(black, 0.1), p.fill.Mix(black, 0.22)
	s := &p.syntax
	s[highlight.Plain] = p.text
	s[highlight.Keyword] = ui.Hex("#7c3bd6")
	s[highlight.Type] = ui.Hex("#0b7c99")
	s[highlight.Function] = ui.Hex("#2a5bd7")
	s[highlight.String] = ui.Hex("#3d8a28")
	s[highlight.Number] = ui.Hex("#c2571a")
	s[highlight.Comment] = ui.Hex("#868692")
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

// matrixPalette is The Matrix: phosphor green on black, the code in
// greens told apart by their brightness. What is removed or failed stays
// red, and what is changed or found amber, so diffs and errors still
// read.
func matrixPalette() *palette {
	p := &palette{
		dark:      true,
		mono:      true,
		editor:    ui.Hex("#050a06"),
		chrome:    ui.Hex("#020503"),
		panel:     ui.Hex("#040806"),
		border:    ui.Hex("#0f2a16"),
		hover:     ui.RGBA(0, 255, 65, 0.06),
		active:    ui.RGBA(0, 255, 65, 0.14),
		text:      ui.Hex("#4af07a"),
		muted:     ui.Hex("#2a9d4c"),
		faint:     ui.Hex("#1a5c2e"),
		accent:    ui.Hex("#00ff41"),
		accentDim: ui.RGBA(0, 255, 65, 0.5),
		line:      ui.RGBA(0, 255, 65, 0.04),
		selection: ui.RGBA(0, 255, 65, 0.22),
		gutter:    ui.Hex("#1f6b36"),
		gutterNow: ui.Hex("#6dff94"),
		status:    ui.Hex("#020503"),
		statusFg:  ui.Hex("#2a9d4c"),
		modified:  ui.Hex("#d9b44a"),
		danger:    ui.Hex("#ff4a5a"),
		match:     ui.RGBA(217, 180, 74, 0.22),
		matchNow:  ui.RGBA(217, 180, 74, 0.40),
		matchRing: ui.RGBA(217, 180, 74, 0.85),
		guide:     ui.RGBA(0, 255, 65, 0.07),
		guideNow:  ui.RGBA(0, 255, 65, 0.25),
		bracket:   ui.RGBA(0, 255, 65, 0.35),
		word:      ui.RGBA(0, 255, 65, 0.09),
		shadow:    ui.RGBA(0, 0, 0, 0.6),
		added:     ui.Hex("#00d43a"),
		changed:   ui.Hex("#3fd0b0"),
		removed:   ui.Hex("#e0505e"),
		// Black on the green, as a terminal's inverted text.
		fill:           ui.Hex("#00d43a"),
		fillText:       ui.Hex("#000000"),
		surface:        ui.Hex("#08140b"),
		surfaceHover:   ui.Hex("#0c1d10"),
		surfacePressed: ui.Hex("#102615"),
		scrollbar:      ui.RGBA(0, 255, 65, 0.2),
		inverse:        ui.Hex("#0f2a16"),
		inverseText:    ui.Hex("#8dffad"),
		ansi: [16]ui.Color{
			ui.Hex("#0a1a0e"), ui.Hex("#e0505e"), ui.Hex("#00ff41"), ui.Hex("#d9b44a"),
			ui.Hex("#2fbf8f"), ui.Hex("#8fd94a"), ui.Hex("#00ff9c"), ui.Hex("#4af07a"),
			ui.Hex("#1f7a3a"), ui.Hex("#ff6b78"), ui.Hex("#6dff94"), ui.Hex("#f0d070"),
			ui.Hex("#5fe8c4"), ui.Hex("#b0f070"), ui.Hex("#7dffcf"), ui.Hex("#c8ffd6"),
		},
	}
	p.fillHover, p.fillPressed = p.fill.Mix(white, 0.15), p.fill.Mix(black, 0.15)
	s := &p.syntax
	s[highlight.Plain] = p.text
	s[highlight.Keyword] = ui.Hex("#b6ffc8")
	s[highlight.Type] = ui.Hex("#00ff9c")
	s[highlight.Function] = ui.Hex("#66ff8a")
	s[highlight.String] = ui.Hex("#8fd94a")
	s[highlight.Number] = ui.Hex("#c8ff5a")
	s[highlight.Comment] = ui.Hex("#1f7a3a")
	s[highlight.Operator] = ui.Hex("#3fbf63")
	s[highlight.Punctuation] = ui.Hex("#2e8f4a")
	s[highlight.Constant] = ui.Hex("#c8ff5a")
	s[highlight.Builtin] = ui.Hex("#7dffcf")
	s[highlight.Tag] = ui.Hex("#00ff9c")
	s[highlight.Attribute] = ui.Hex("#8fd94a")
	s[highlight.Variable] = ui.Hex("#5af58a")
	s[highlight.Heading] = ui.Hex("#b6ffc8")
	s[highlight.Deleted] = p.removed
	s[highlight.Inserted] = p.added
	return p
}

var (
	black, white = ui.Hex("#000000"), ui.Hex("#ffffff")

	lightPal  = lightPalette().derive()
	darkPal   = darkPalette().derive()
	matrixPal = matrixPalette().derive()
)

// appearanceMatrix is the Appearance of The Matrix, which is dark.
const appearanceMatrix = "matrix"

// paletteFor returns the palette of an appearance of the settings, in a
// window that is dark or not.
func paletteFor(appearance string, dark bool) *palette {
	switch {
	case appearance == appearanceMatrix:
		return matrixPal
	case dark:
		return darkPal
	}
	return lightPal
}

// themeSource is the light or dark appearance of the native window for an
// appearance of the settings: a theme of Mayoi's own is dark.
func themeSource(appearance string) mygo.ThemeSource {
	if appearance == appearanceMatrix {
		return mygo.ThemeDark
	}
	return mygo.ThemeSource(appearance)
}

// derive fills in the colors made from the others.
func (p *palette) derive() *palette {
	p.selected = p.selection.Over(p.editor)
	p.selectedDim = p.selection.Alpha(0.5).Over(p.editor)
	p.cursor = p.accent.Alpha(0.5)
	return p
}

// applyTheme sets the window's theme from the palette of its appearance,
// and returns that palette.
func (a *app) applyTheme(c *ui.Context) *palette {
	t := *c.Theme()
	p := paletteFor(a.settings.Appearance, t.Dark)
	t.Dark = p.dark
	t.Background = p.editor
	// Every state of the accent, or a hovered button would turn the
	// default theme's blue.
	t.Accent, t.AccentText = p.fill, p.fillText
	t.AccentHover, t.AccentPressed = p.fillHover, p.fillPressed
	t.Danger, t.Warning, t.Success = p.danger, p.modified, p.added
	t.Selection = p.selection
	t.Border = p.border
	t.Text, t.TextMuted = p.text, p.muted
	t.Focus = p.accentDim
	t.Radius = 6
	t.Spacing = 3.5
	t.FontSize = 13
	t.ScrollbarWidth = 8
	t.Surface, t.SurfaceHover, t.SurfacePressed = p.surface, p.surfaceHover, p.surfacePressed
	t.Scrollbar = p.scrollbar
	t.Inverse, t.InverseText = p.inverse, p.inverseText
	if p.mono {
		t.Font = a.settings.FontFamily
	}
	c.SetTheme(&t)
	return p
}

// popIn is how a box over an editor comes in, as find's: it fades in,
// from a little above where it settles. Only its colors follow it after.
var popIn = ui.ElementTransition{Duration: 130 * time.Millisecond, Colors: true, Enter: &ui.Motion{Y: -6}}

// floating styles a box that floats over the window: a dialog, as quick
// open's, a picker's or a prompt's, or a smaller box over an editor, as
// find's. The shadow is lighter on a light window, where a dark one looks
// smudged.
func (a *app) floating(c *ui.Context, box ui.Element, dialog bool) ui.Element {
	p := a.pal
	box.Background(c.Theme().Surface).Border(1, p.border)
	switch {
	case dialog && p.dark:
		box.Shadow(0, 16, 40, 0, ui.RGBA(0, 0, 0, 0.40))
	case dialog:
		box.Shadow(0, 12, 32, 0, ui.RGBA(20, 20, 40, 0.16))
	case p.dark:
		box.Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, 0.30))
	default:
		box.Shadow(0, 4, 14, 0, ui.RGBA(20, 20, 40, 0.12))
	}
	return box
}

// dialogBack is the layer under a dialog, over the whole window, that
// the dialog is in: label names the dialog. It fades in, from a little
// above, as the dialog shows. The layer is new with the dialog, so a
// Transition would not move it: its motion starts the first time it is
// built for the dialog.
func (a *app) dialogBack(c *ui.Context, label string) ui.Element {
	back := ui.Column(c).Absolute().Left(0).Right(0).Bottom(0).AlignItems(ui.Center).Modal()
	if a.dialog != label {
		a.dialog = label
		back.Animate("in", 0, 0)
	}
	a.dialogBuilt = true
	k := back.Animate("in", 1, 130*time.Millisecond)
	return back.Top(-6 * (1 - k)).Opacity(k)
}
