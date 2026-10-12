package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

// matrixHarness is a dark window in The Matrix.
func matrixHarness(t testing.TB) *harness {
	hs := newHarness(t, true)
	hs.a.setAppearance(appearanceMatrix)
	hs.settle()
	return hs
}

func TestMatrixAppearance(t *testing.T) {
	if paletteFor(appearanceMatrix, false) != matrixPal || paletteFor(appearanceMatrix, true) != matrixPal {
		t.Error("The Matrix does not follow the window's light or dark")
	}
	if paletteFor("", false) != lightPal || paletteFor("dark", true) != darkPal {
		t.Error("light and dark windows have the wrong palettes")
	}
	if themeSource(appearanceMatrix) != mygo.ThemeDark || themeSource("light") != mygo.ThemeLight {
		t.Error("the native window is not dark under The Matrix")
	}
	hs := matrixHarness(t)
	a := hs.a
	if a.pal != matrixPal {
		t.Fatal("the window is not in The Matrix")
	}
	if a.termAppearance != appearanceMatrix {
		t.Error("the terminals do not have The Matrix's colors")
	}
	if th := termTheme(matrixPal); th.Background != matrixPal.panel || th.Palette != matrixPal.ansi {
		t.Error("the terminal's colors are not the palette's")
	}
	// The icons of files are greens, not their languages' colors.
	if _, c := fileIcon(a.pal, "main.go"); c != langColor(matrixPal, "#00add8", "") || c.G < c.R || c.G < c.B {
		t.Errorf("a Go file's icon is %v, not a green", c)
	}
	a.setAppearance("dark")
	hs.settle()
	if a.pal != darkPal || a.termAppearance != "" {
		t.Error("the window does not leave The Matrix")
	}
}

// The rain falls while the window has the keyboard, and holds still
// without it, asking for no frames.
func TestMatrixRainStopsWhenBlurred(t *testing.T) {
	hs := matrixHarness(t)
	a := hs.a
	if len(a.cols) != 1 || len(a.cols[0].tabs) != 0 {
		t.Fatal("the welcome page does not show")
	}
	if !a.rain.moving {
		t.Fatal("the rain does not fall on the welcome page")
	}
	a.post(func() { a.blurred = true })
	hs.settle()
	step := a.rain.step
	if a.rain.moving {
		t.Error("the rain falls in a window without the keyboard")
	}
	hs.settle()
	if a.rain.step != step {
		t.Error("the rain moved while still")
	}
	a.post(func() { a.blurred = false })
	hs.settle()
	if !a.rain.moving {
		t.Error("the rain does not fall again with the keyboard back")
	}
	// Out of The Matrix, there is none.
	a.setAppearance("dark")
	a.rain.moving = false
	hs.settle()
	if a.rain.moving {
		t.Error("the rain falls in a dark window")
	}
}

func TestShotMatrix(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip()
	}
	hs := matrixHarness(t)
	a := hs.a
	hs.shot("matrix-welcome")
	src, _ := os.ReadFile("editor.go")
	os.WriteFile(hs.path("editor.go"), src, 0o644)
	a.openFile(hs.path("editor.go"), true)
	hs.settle()
	d := a.activeEditor()
	i := strings.Index(string(src), "func newCodeFont")
	at := len([]rune(string(src)[:i]))
	hs.place(d, at, at+20)
	hs.shot("matrix-editor")
	a.openQuick("")
	hs.tt.Type("ed")
	hs.settle()
	hs.shot("matrix-quick")

	hs = matrixHarness(t)
	scmFixture(hs)
	hs.a.run("view.scm")
	hs.settleGit()
	hs.a.showChanges(diffTarget{path: hs.path("main.go"), base: diffIndex}, true)
	hs.settleGit()
	hs.shot("matrix-scm")
}

// claudeHarness is a window, dark or not, in Claude.
func claudeHarness(t testing.TB, dark bool) *harness {
	hs := newHarness(t, dark)
	hs.a.setAppearance(appearanceClaude)
	hs.settle()
	return hs
}

func TestClaudeAppearance(t *testing.T) {
	if paletteFor(appearanceClaude, false) != claudeLightPal || paletteFor(appearanceClaude, true) != claudeDarkPal {
		t.Error("Claude does not follow the window's light or dark")
	}
	if themeSource(appearanceClaude) != mygo.ThemeSystem {
		t.Error("the native window does not follow the system under Claude")
	}
	if !ownTheme(appearanceClaude) || !ownTheme(appearanceMatrix) || ownTheme("dark") || ownTheme("") {
		t.Error("ownTheme is wrong")
	}
	for _, dark := range []bool{false, true} {
		hs := claudeHarness(t, dark)
		a := hs.a
		if a.pal != paletteFor(appearanceClaude, dark) || a.pal.dark != dark {
			t.Fatalf("the window (dark %v) is not in Claude", dark)
		}
		if a.termAppearance != appearanceClaude {
			t.Error("the terminals do not have Claude's colors")
		}
		light, darkTh := a.termThemes()
		if light.Background != claudeLightPal.panel || darkTh.Palette != claudeDarkPal.ansi {
			t.Error("the terminal's colors are not Claude's")
		}
		// The icons of files keep their languages' colors.
		if _, c := fileIcon(a.pal, "main.go"); c == langColor(matrixPal, "#00add8", "") {
			t.Error("a Go file's icon is a mono shade")
		}
		a.setAppearance("")
		hs.settle()
		if a.termAppearance != "" || a.pal == claudeLightPal || a.pal == claudeDarkPal {
			t.Error("the window does not leave Claude")
		}
	}
}

// Clawd blinks for a moment, bobs once in a few periods, and hops while
// there is work, the hop changing every step.
func TestClawdPoses(t *testing.T) {
	at := func(ms int64) time.Time { return time.UnixMilli(ms) }
	period := clawdPeriod.Milliseconds()
	if p, hold := clawdAt(at(3*period+10), false); !p.blink || hold != clawdBlink-10*time.Millisecond {
		t.Errorf("no blink at a period's start: %+v for %v", p, hold)
	}
	if p, hold := clawdAt(at(period+500), false); p.blink || p.lift != 0 || hold != clawdPeriod-500*time.Millisecond {
		t.Errorf("Clawd does not rest until the next blink: %+v for %v", p, hold)
	}
	bob := clawdBobAt.Milliseconds()
	if p, _ := clawdAt(at(3*period+bob+10), false); p.lift == 0 {
		t.Error("Clawd does not bob")
	}
	if p, hold := clawdAt(at(3*period+500), false); p.lift != 0 || hold != clawdBobAt-500*time.Millisecond {
		t.Errorf("Clawd does not wait for its bob: %+v for %v", p, hold)
	}
	a, _ := clawdAt(at(0), true)
	b, hold := clawdAt(at(clawdHop.Milliseconds()), true)
	if a == b || hold != clawdHop {
		t.Errorf("Clawd does not hop: %+v then %+v for %v", a, b, hold)
	}
}

// Clawd moves on the welcome page and in the status bar while the window
// has the keyboard, hops while Git works, and holds still without the
// keyboard.
func TestClawdMoves(t *testing.T) {
	hs := claudeHarness(t, true)
	a := hs.a
	if !a.clawd.moving || a.clawd.hopping {
		t.Fatal("Clawd does not idle in Claude")
	}
	a.post(func() { a.git.busy = "Pushing…" })
	hs.settle()
	if !a.clawd.hopping {
		t.Error("Clawd does not hop while Git pushes")
	}
	if a.clawdTip() != "Claude · Pushing…" {
		t.Errorf("Clawd's tip is %q", a.clawdTip())
	}
	a.post(func() { a.git.busy, a.blurred = "", true })
	hs.settle()
	if a.clawd.moving || a.clawd.hopping {
		t.Error("Clawd moves in a window without the keyboard")
	}
	a.post(func() { a.blurred = false })
	hs.settle()
	if !a.clawd.moving {
		t.Error("Clawd does not move again with the keyboard back")
	}
	// Out of Claude, there is none.
	a.setAppearance("dark")
	a.clawd.moving = false
	hs.settle()
	if a.clawd.moving {
		t.Error("Clawd shows in a dark window")
	}
}

func TestShotClaude(t *testing.T) {
	if os.Getenv("SHOTS") == "" {
		t.Skip()
	}
	for _, dark := range []bool{false, true} {
		name := "claude-light"
		if dark {
			name = "claude-dark"
		}
		hs := claudeHarness(t, dark)
		a := hs.a
		hs.shot(name + "-welcome")
		src, _ := os.ReadFile("editor.go")
		os.WriteFile(hs.path("editor.go"), src, 0o644)
		a.openFile(hs.path("editor.go"), true)
		hs.settle()
		d := a.activeEditor()
		i := strings.Index(string(src), "func newCodeFont")
		at := len([]rune(string(src)[:i]))
		hs.place(d, at, at+20)
		a.togglePanel()
		hs.settle()
		hs.shot(name + "-editor")
	}
}
