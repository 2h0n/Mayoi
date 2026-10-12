package main

import (
	"os"
	"strings"
	"testing"

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
