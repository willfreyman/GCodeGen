package editor

import (
	"image/color"
	"math"

	"gcodegen.local/generator/internal/gen"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// drawCanvas renders the full sketch surface: background, grid,
// optional loaded-image trace preview, perim (with corner handles),
// origin marker, finalized strokes (with hover highlight), and the
// in-progress stroke. Mirrors gcodegen.py's redraw_all (line 56-65)
// plus the live preview at line 193.
func drawCanvas(dst *ebiten.Image, g *Game) {
	e := g.editor
	dst.Fill(bgCanvas)
	drawGrid(dst)
	// Image preview sits between the grid and the perim outline so
	// strokes and perim handles are always visible on top.
	g.drawImagePreview(dst)
	drawPerim(dst, e)
	drawOrigin(dst, e)
	for i, s := range e.Strokes {
		w := float32(2)
		if i == e.HoverIdx {
			w = 4
		}
		if i == e.SelectedIdx {
			w = 5
		}
		drawStroke(dst, s.Points, s.Color, w)
	}
	if len(e.Current) > 1 {
		drawStroke(dst, e.Current, e.CurrentColor(), 2)
	}
	vector.StrokeRect(dst, 0.5, 0.5,
		float32(gen.CanvasWidth)-1, float32(gen.CanvasHeight)-1,
		1, canvasBorder, false)
}

func drawGrid(dst *ebiten.Image) {
	for x := float32(0); x < gen.CanvasWidth; x += 20 {
		vector.StrokeLine(dst, x, 0, x, float32(gen.CanvasHeight), 1, gridLine, false)
	}
	for y := float32(0); y < gen.CanvasHeight; y += 20 {
		vector.StrokeLine(dst, 0, y, float32(gen.CanvasWidth), y, 1, gridLine, false)
	}
}

func drawPerim(dst *ebiten.Image, e *gen.Editor) {
	x0, y0, x1, y1 := e.Perim.X0, e.Perim.Y0, e.Perim.X1, e.Perim.Y1
	xmin, xmax := math.Min(x0, x1), math.Max(x0, x1)
	ymin, ymax := math.Min(y0, y1), math.Max(y0, y1)

	// Outline: dashed when not cut, solid when cut.
	if e.Perim.Cut {
		vector.StrokeRect(dst, float32(xmin), float32(ymin),
			float32(xmax-xmin), float32(ymax-ymin), 2, perimOutline, false)
	} else {
		drawDashedRect(dst, xmin, ymin, xmax, ymax, 6, 4, 2, perimOutline)
	}

	// Four corner handles, 16x16 squares centered on the corner.
	for _, c := range []struct{ x, y float64 }{
		{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1},
	} {
		hw := float32(8)
		vector.DrawFilledRect(dst, float32(c.x)-hw, float32(c.y)-hw, hw*2, hw*2, cornerHandle, false)
		vector.StrokeRect(dst, float32(c.x)-hw, float32(c.y)-hw, hw*2, hw*2, 1, cornerOutline, false)
	}
}

func drawOrigin(dst *ebiten.Image, e *gen.Editor) {
	r := float32(10)
	vector.DrawFilledCircle(dst, float32(e.Origin.X), float32(e.Origin.Y), r, originFill, true)
	vector.StrokeCircle(dst, float32(e.Origin.X), float32(e.Origin.Y), r, 2, originOutline, true)
	if face := fontFace(11); face != nil {
		op := &text.DrawOptions{}
		op.GeoM.Translate(e.Origin.X+14, e.Origin.Y-22)
		op.ColorScale.ScaleWithColor(originText)
		text.Draw(dst, "0,0", face, op)
	}
}

func drawStroke(dst *ebiten.Image, pts []gen.Point, clr color.RGBA, width float32) {
	for i := 0; i+1 < len(pts); i++ {
		x0, y0 := pts[i].X, pts[i].Y
		x1, y1 := pts[i+1].X, pts[i+1].Y
		// Skip segments with NaN/Inf or absurd coords — they'd push
		// vector.StrokeLine's float32 math into Inf and crash the GPU
		// path. Realistic canvas coords are 0..700 / 0..580.
		if !finite(x0) || !finite(y0) || !finite(x1) || !finite(y1) {
			continue
		}
		vector.StrokeLine(dst,
			float32(x0), float32(y0),
			float32(x1), float32(y1),
			width, clr, true)
	}
}

func finite(v float64) bool {
	return v == v && v > -1e6 && v < 1e6
}

// drawSeparator strokes a vertical 1-pixel line on dst from y0 to y0+h
// at column x. GPU-accelerated alternative to looping screen.Set per pixel.
func drawSeparator(dst *ebiten.Image, x, y0, h int, clr color.RGBA) {
	vector.StrokeLine(dst, float32(x), float32(y0), float32(x), float32(y0+h), 1, clr, false)
}

// drawDashedRect strokes a dashed rectangle outline. Tk's dash=(6,4)
// means 6-pixel dashes with 4-pixel gaps. We approximate by stepping
// along each edge in 10-pixel cycles (6 on, 4 off).
func drawDashedRect(dst *ebiten.Image, x0, y0, x1, y1 float64, dash, gap, width float32, clr color.RGBA) {
	corners := [][2][2]float64{
		{{x0, y0}, {x1, y0}},
		{{x1, y0}, {x1, y1}},
		{{x1, y1}, {x0, y1}},
		{{x0, y1}, {x0, y0}},
	}
	for _, edge := range corners {
		drawDashedLine(dst,
			float32(edge[0][0]), float32(edge[0][1]),
			float32(edge[1][0]), float32(edge[1][1]),
			dash, gap, width, clr)
	}
}

func drawDashedLine(dst *ebiten.Image, x0, y0, x1, y1, dash, gap, width float32, clr color.RGBA) {
	dx, dy := x1-x0, y1-y0
	length := float32(math.Hypot(float64(dx), float64(dy)))
	if length == 0 {
		return
	}
	ux, uy := dx/length, dy/length
	cycle := dash + gap
	for d := float32(0); d < length; d += cycle {
		end := d + dash
		if end > length {
			end = length
		}
		vector.StrokeLine(dst,
			x0+ux*d, y0+uy*d,
			x0+ux*end, y0+uy*end,
			width, clr, false)
	}
}
