package main

import (
	"github.com/egoist/mygo/ui"
)

// Qoduck, Qoder's duck, as its icon draws it: a yellow body under a dome
// of a head, a tuft of three feathers, a frowning brow, two eyes and an
// orange beak. It is Qoder's character, drawn from shapes rather than
// cells, so that it reads as small as the status bar, and in its own
// colors rather than the accent. In the air, its tuft trails behind it.

// The duck is drawn in units of half a cell, in a square of qoduckSpan of
// them, as its icon is: the coordinates below are the icon's, a 32nd of
// its 1024 pixels, moved right a little so that the beak fits.
const qoduckSpan = 30

var (
	qoduckBody = ui.Hex("#ffd966")
	qoduckBeak = ui.Hex("#f87825")
	qoduckInk  = ui.Hex("#0f0d0c")
	// On a light window, where the yellow fades into the white, the body
	// and the beak are edged in deeper shades of theirs.
	qoduckBodyEdge = ui.Hex("#e3b13a")
	qoduckBeakEdge = ui.Hex("#d6611a")
)

// qoduckSize is the size of the duck drawn in cells of a width, with two
// cells of headroom over it for the hop.
func qoduckSize(cell float32) (w, h float32) {
	u := cell / 2
	return qoduckSpan * u, qoduckSpan*u + 2*cell
}

// drawQoduck paints the duck in r, centered, in cells of a width, in a
// pose.
func (a *app) drawQoduck(pt *ui.Painter, r ui.Rect, cell float32, _ ui.Color, p mascotPose) {
	u := cell / 2
	w, h := qoduckSize(cell)
	x0, y0 := r.X+(r.W-w)/2, r.Y+(r.H-h)/2+2*cell*(1-p.lift)
	// at moves a point in units to the window.
	at := func(x, y float32) (float32, float32) { return x0 + x*u, y0 + y*u }
	move := func(path *ui.Path, x, y float32) *ui.Path { return path.MoveTo(at(x, y)) }
	quad := func(path *ui.Path, cx, cy, x, y float32) *ui.Path {
		cx, cy = at(cx, cy)
		x, y = at(x, y)
		return path.QuadTo(cx, cy, x, y)
	}
	cube := func(path *ui.Path, c1x, c1y, c2x, c2y, x, y float32) *ui.Path {
		c1x, c1y = at(c1x, c1y)
		c2x, c2y = at(c2x, c2y)
		x, y = at(x, y)
		return path.CubeTo(c1x, c1y, c2x, c2y, x, y)
	}

	// The head's dome, the sides, and the rounded corners of the bottom.
	body := move(new(ui.Path), 3.5, 14)
	cube(body, 3.5, 8.6, 9.2, 5.6, 16, 5.6)
	cube(body, 22.8, 5.6, 29.3, 8.6, 29.3, 14)
	body.LineTo(at(29.3, 22.6))
	cube(body, 29.3, 26.1, 26.4, 29, 22.9, 29)
	body.LineTo(at(9.9, 29))
	cube(body, 6.4, 29, 3.5, 26.1, 3.5, 22.6)
	body.Close()
	// The tuft, its tips trailing down in the air.
	droop := 0.8 * float32(p.step)
	tuft := move(new(ui.Path), 16.5, 6.5)
	quad(tuft, 16.2, 3.8, 16.9, 1.8+droop)
	move(tuft, 14, 6.8)
	quad(tuft, 13.2, 4.6, 12.6, 3.4+droop)
	move(tuft, 19, 6.8)
	quad(tuft, 19.8, 4.6, 20.6, 3.6+droop)
	tuftW := 2.2 * u
	beak := ellipse(new(ui.Path), x0+7.5*u, y0+20.4*u, 6.9*u, 5.1*u)

	// The edges first, half of them under the shapes.
	edge := max(0.8*u, 1)
	light := !a.pal.dark
	if light {
		pt.StrokePath(tuft, tuftW+2*edge, qoduckBodyEdge)
		pt.StrokePath(body, 2*edge, qoduckBodyEdge)
	}
	pt.StrokePath(tuft, tuftW, qoduckBody)
	pt.FillPath(body, qoduckBody)

	// The brow, and the eyes under it: shut, a slit.
	brow := move(new(ui.Path), 10.9, 11.5)
	quad(brow, 14.8, 14.6, 19.6, 12.1)
	pt.StrokePath(brow, 1.4*u, qoduckInk)
	ew, eh := 2.75*u, 3.75*u
	if p.blink {
		eh = 0.9 * u
	}
	for _, eye := range [2][2]float32{{11.4, 16.1}, {17.15, 16.9}} {
		x, y := at(eye[0], eye[1])
		pt.Fill(ui.Rect{X: x - ew/2, Y: y - eh/2, W: ew, H: eh}, qoduckInk, min(ew, eh)/2)
	}

	// The beak, over the near eye.
	if light {
		pt.StrokePath(beak, 2*edge, qoduckBeakEdge)
	}
	pt.FillPath(beak, qoduckBeak)
}

// ellipse adds an ellipse as a subpath, in four cubic curves.
func ellipse(p *ui.Path, cx, cy, rx, ry float32) *ui.Path {
	const k = 0.5522847498
	p.MoveTo(cx+rx, cy)
	p.CubeTo(cx+rx, cy+ry*k, cx+rx*k, cy+ry, cx, cy+ry)
	p.CubeTo(cx-rx*k, cy+ry, cx-rx, cy+ry*k, cx-rx, cy)
	p.CubeTo(cx-rx, cy-ry*k, cx-rx*k, cy-ry, cx, cy-ry)
	p.CubeTo(cx+rx*k, cy-ry, cx+rx, cy-ry*k, cx+rx, cy)
	return p.Close()
}
