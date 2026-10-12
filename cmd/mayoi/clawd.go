package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Clawd, Claude Code's little character, as its banner draws it in
// quarter blocks: in Claude, it sits on the welcome page and at the start
// of the status bar. It blinks now and then and bobs once in a while, and
// while Git or search works in the background it hops. Like the rain, it
// steps rather than glides, and holds still while the window has not the
// keyboard or the desktop asks for less motion.

// clawdArt is Clawd's body, a cell to a character, a cell twice as tall
// as it is wide, as the quarter blocks are. The eyes are cut from it, and
// the legs are drawn apart, as they move.
var clawdArt = [...]string{
	"   ############   ",
	"   ############   ",
	" ################ ",
	"   ############   ",
}

const (
	clawdCols = 18
	clawdRows = 5 // the body's and the legs'
	// clawdEyes are the columns of the eyes, in the art's second row.
	clawdEyeL, clawdEyeR = 5, 12

	clawdPeriod = 4200 * time.Millisecond // between blinks
	clawdBlink  = 140 * time.Millisecond
	clawdBobAt  = 2100 * time.Millisecond // into every third period
	clawdBob    = 260 * time.Millisecond
	clawdHop    = 130 * time.Millisecond // a step of the hop
)

// clawdLegs are the columns of the legs standing, and with them apart
// mid-hop.
var clawdLegs = [2][4]int{{4, 6, 11, 13}, {3, 5, 12, 14}}

// clawdState is how Clawd is: moving whether the last paint asked for a
// next, and hopping whether it hopped.
type clawdState struct {
	moving, hopping bool
}

// clawdPose is a moment of Clawd's: the eyes shut, the body lifted by a
// number of cells, and which legs.
type clawdPose struct {
	blink bool
	lift  float32
	legs  int
}

// clawdAt returns Clawd's pose at a moment, and how long it holds.
func clawdAt(now time.Time, busy bool) (clawdPose, time.Duration) {
	ms := now.UnixMilli()
	if busy {
		step := clawdHop.Milliseconds()
		k := ms / step
		return clawdPose{lift: float32(k % 2), legs: int(k % 2)}, time.Duration(step-ms%step) * time.Millisecond
	}
	period := clawdPeriod.Milliseconds()
	in, n := ms%period, ms/period
	// The moments the pose changes, in a period.
	marks := []int64{clawdBlink.Milliseconds(), period}
	var p clawdPose
	switch {
	case in < clawdBlink.Milliseconds():
		p.blink = true
	case n%3 == 0:
		at, end := clawdBobAt.Milliseconds(), (clawdBobAt + clawdBob).Milliseconds()
		marks = []int64{clawdBlink.Milliseconds(), at, end, period}
		if in >= at && in < end {
			p.lift = 0.5
		}
	}
	for _, m := range marks {
		if m > in {
			return p, time.Duration(m-in) * time.Millisecond
		}
	}
	return p, time.Duration(period-in) * time.Millisecond
}

// clawdSize is the size of Clawd drawn in cells of a width.
func clawdSize(cell float32) (w, h float32) {
	return clawdCols * cell, (clawdRows + 1) * 2 * cell
}

// clawdBusy reports whether there is work for Clawd to hop to: a Git
// command or a search of the files running.
func (a *app) clawdBusy() bool {
	return a.git.busy != "" || a.inFiles.running
}

// drawClawd paints Clawd in r, centered, in cells of a width, its eyes
// the color of bg. still holds it standing, asking for no more frames.
func (a *app) drawClawd(pt *ui.Painter, r ui.Rect, cell float32, bg ui.Color, still bool) {
	cs := &a.clawd
	busy := a.clawdBusy()
	var p clawdPose
	cs.moving, cs.hopping = !still, !still && busy
	if !still {
		var hold time.Duration
		p, hold = clawdAt(pt.Now(), busy)
		pt.After(hold)
	}
	body := a.pal.accent
	w, h := clawdSize(cell)
	ch := 2 * cell
	// A cell of headroom over the body, for the hop.
	x0, y0 := r.X+(r.W-w)/2, r.Y+(r.H-h)/2+ch*(1-p.lift)
	for row, line := range clawdArt {
		// A run of cells at a time, so that the body has no seams.
		for i := 0; i < clawdCols; {
			if line[i] != '#' {
				i++
				continue
			}
			j := i
			for j < clawdCols && line[j] == '#' {
				j++
			}
			pt.Fill(ui.Rect{X: x0 + float32(i)*cell, Y: y0 + float32(row)*ch, W: float32(j-i) * cell, H: ch}, body, 0)
			i = j
		}
	}
	// The eyes, cut from the second row: shut, a slit.
	eh, ey := ch, y0+ch
	if p.blink {
		eh, ey = ch*0.25, y0+ch*1.5
	}
	for _, col := range []int{clawdEyeL, clawdEyeR} {
		pt.Fill(ui.Rect{X: x0 + float32(col)*cell, Y: ey, W: cell, H: eh}, bg, 0)
	}
	legY := y0 + float32(len(clawdArt))*ch
	for _, col := range clawdLegs[p.legs] {
		pt.Fill(ui.Rect{X: x0 + float32(col)*cell, Y: legY, W: cell, H: ch * (1 + p.lift)}, body, 0)
	}
}

// clawdStill reports whether Clawd holds still in the window now.
func (a *app) clawdStill(c *ui.Context) bool {
	return a.blurred || c.Preferences().ReduceMotion
}

// clawdTip says what Clawd is doing, under the pointer.
func (a *app) clawdTip() string {
	switch {
	case a.git.busy != "":
		return "Claude · " + a.git.busy
	case a.inFiles.running:
		return "Claude · Searching…"
	}
	return "Claude"
}

// statusClawd is Clawd at the start of the status bar, in Claude.
func (a *app) statusClawd(c *ui.Context) {
	const cell = 1
	w, h := clawdSize(cell)
	still := a.clawdStill(c)
	bg := a.pal.status
	ui.Row(c.Key("status:clawd")).Size(w+12, statusBarH).Tooltip(a.clawdTip()).Draw(func(pt *ui.Painter, r ui.Rect) {
		a.drawClawd(pt, ui.Rect{X: r.X + 6, Y: r.Y + (r.H-h)/2, W: w, H: h}, cell, bg, still)
	})
}
