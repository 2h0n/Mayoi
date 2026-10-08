// Command icongen draws Mayoi's app icon into resources/icon.png:
//
//	go run ./tools/icongen
package main

import (
	"image/png"
	"log"
	"os"

	"github.com/egoist/mygo/ui"
)

func main() {
	view := func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Column(c).Fill().Center().Children(func() {
			// The macOS icon grid: an 824 DIP squircle on a 1024 canvas.
			ui.Column(c).Size(824, 824).Radius(185).Center().
				LinearGradient(ui.LinearGradient{From: ui.Hex("#2b2545"), To: ui.Hex("#121117"), Angle: 160, Oklab: true}).
				Border(3, ui.RGBA(255, 255, 255, 0.08)).
				Shadow(0, 18, 40, 0, ui.RGBA(0, 0, 0, 0.35)).
				Children(func() {
					// A path that wanders: the M of Mayoi, as a line that
					// loses its way and finds it.
					ui.Box(c).Size(560, 560).Draw(func(p *ui.Painter, r ui.Rect) {
						var path ui.Path
						x, y := r.X, r.Y
						path.MoveTo(x+70, y+430).
							LineTo(x+70, y+150).
							LineTo(x+280, y+360).
							LineTo(x+490, y+150).
							LineTo(x+490, y+430)
						p.StrokePathGradient(&path, 64, ui.LinearGradient{From: ui.Hex("#c4b5fd"), To: ui.Hex("#8b7cf6"), Angle: 90, Oklab: true})
						p.Fill(ui.Rect{X: x + 250, Y: y + 470, W: 60, H: 60}, ui.Hex("#e8b86d"), 30)
					})
				})
		})
	}
	img := ui.Render(view, 1024, 1024, 1)
	f, err := os.Create("resources/icon.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
