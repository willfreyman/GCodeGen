package preview

import (
	"image/color"
	"math"
	"math/rand"

	"gcodegen.local/generator/internal/gen"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	PreviewW = 700
	PreviewH = 520
)

// renderSurface paints the full preview canvas onto dst from current
// state. Mirrors gcodegen.py:render (line 626-704). Designed to be
// idempotent — caller may call this whenever state, material, or bit
// diameter changes.
func renderSurface(dst *ebiten.Image, st *State, material string, bitMM float64) {
	if st == nil || (len(st.Strokes) == 0 && !st.PerimCut) {
		dst.Fill(color.RGBA{R: 0x2a, G: 0x2a, B: 0x2a, A: 0xff})
		// Empty-state hint is drawn separately by the Game.Draw caller.
		return
	}
	pal := PaletteFor(material)
	// Surface fill.
	dst.Fill(pal.Surface)
	// Material-specific texture overlay.
	overlayTexture(dst, material, pal.Shadow)
	// Bit radius in pixels.
	pxmm := pixelsPerMM(st)
	if pxmm <= 0 {
		return
	}
	br := math.Max(1.0, (bitMM/2)*pxmm)
	grooveW := float32(math.Max(2.0, math.Round(br*2)))
	for _, s := range st.Strokes {
		drawGroovePolyline(dst, s.Points, pal.Shadow, grooveW+1)
		drawGroovePolyline(dst, s.Points, pal.Groove, grooveW)
	}
	if st.PerimCut {
		// Render perim as a groove too.
		px := perimRect(st)
		pts := []gen.Point{
			{X: px[0], Y: px[1]}, {X: px[2], Y: px[1]},
			{X: px[2], Y: px[3]}, {X: px[0], Y: px[3]},
			{X: px[0], Y: px[1]},
		}
		drawGroovePolyline(dst, ptsFromPoints(pts), pal.Shadow, grooveW+1)
		drawGroovePolyline(dst, ptsFromPoints(pts), pal.Groove, grooveW)
	}
	// Perim outline (always — white dashed).
	if st.HasPerim {
		px := perimRect(st)
		drawDashedRect(dst, px[0], px[1], px[2], px[3], 4, 4, 1,
			color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	}
}

// pixelsPerMM converts the perim's pixel width and mm width into a
// scale factor. Returns 0 if perim is degenerate.
func pixelsPerMM(s *State) float64 {
	if s.PerimWMM <= 0 {
		return 0
	}
	pxw := math.Abs(s.PerimX1 - s.PerimX0)
	if pxw <= 0 {
		return 0
	}
	return pxw / s.PerimWMM
}

// perimRect returns [lx, ty, rx, by] in pixel space.
func perimRect(s *State) [4]float64 {
	lx, rx := minF(s.PerimX0, s.PerimX1), maxF(s.PerimX0, s.PerimX1)
	ty, by := minF(s.PerimY0, s.PerimY1), maxF(s.PerimY0, s.PerimY1)
	return [4]float64{lx, ty, rx, by}
}

// ptsFromPoints adapts gen.Point slice to the same [][2]float64 shape
// used for stroke wire points.
func ptsFromPoints(p []gen.Point) [][2]float64 {
	out := make([][2]float64, len(p))
	for i, q := range p {
		out[i] = [2]float64{q.X, q.Y}
	}
	return out
}

// drawGroovePolyline strokes a thick line over a polyline; cap+join is
// implicitly handled by overdrawing endpoint circles at each joint.
func drawGroovePolyline(dst *ebiten.Image, pts [][2]float64, c color.RGBA, w float32) {
	for i := 0; i+1 < len(pts); i++ {
		x0, y0 := pts[i][0], pts[i][1]
		x1, y1 := pts[i+1][0], pts[i+1][1]
		vector.StrokeLine(dst, float32(x0), float32(y0), float32(x1), float32(y1), w, c, true)
		// Round caps via overdrawn circles at endpoints.
		vector.DrawFilledCircle(dst, float32(x0), float32(y0), w/2, c, true)
	}
	if len(pts) > 0 {
		last := pts[len(pts)-1]
		vector.DrawFilledCircle(dst, float32(last[0]), float32(last[1]), w/2, c, true)
	}
}

// overlayTexture draws the per-material random pattern. The seed
// matches gcodegen.py (random.Random(99)) so the pattern is stable
// across renders, though Python's MT and Go's PRNG differ — visual
// character is preserved (random with same density), exact pixel
// positions don't match.
func overlayTexture(dst *ebiten.Image, material string, shadow color.RGBA) {
	rng := rand.New(rand.NewSource(99))
	switch material {
	case "Wood", "MDF":
		for i := 0; i < 40; i++ {
			yg := rng.Intn(PreviewH)
			dy := rng.Intn(25) - 12
			c := shadow
			if rng.Float64() > 0.35 {
				c = color.RGBA{R: 0, G: 0, B: 0, A: 0} // surface — skip
			}
			if c.A == 0 {
				continue
			}
			vector.StrokeLine(dst, 0, float32(yg), float32(PreviewW), float32(yg+dy), 1, c, false)
		}
	case "Stone":
		for i := 0; i < 60; i++ {
			x1 := rng.Intn(PreviewW)
			y1 := rng.Intn(PreviewH)
			x2 := x1 + rng.Intn(121) - 60
			y2 := y1 + rng.Intn(81) - 40
			vector.StrokeLine(dst, float32(x1), float32(y1), float32(x2), float32(y2), 1, shadow, false)
		}
	case "Brass":
		for i := 0; i < 25; i++ {
			yg := rng.Intn(PreviewH)
			vector.StrokeLine(dst, 0, float32(yg), float32(PreviewW), float32(yg), 1, shadow, false)
		}
	}
}

// QualityIndicator returns the (label, color) pair shown in the
// bottom-right corner. Mirrors the bit/area ratio thresholds at
// gcodegen.py:686-694.
func QualityIndicator(bitMM, areaMM2 float64) (string, color.RGBA) {
	if areaMM2 <= 0 {
		return "Set perimeter size", color.RGBA{R: 0xaa, G: 0xaa, B: 0xaa, A: 0xff}
	}
	ratio := math.Pi * (bitMM / 2) * (bitMM / 2) / areaMM2
	switch {
	case ratio < 0.002:
		return "Excellent detail", color.RGBA{R: 0x00, G: 0xff, B: 0x88, A: 0xff}
	case ratio < 0.008:
		return "Good detail", color.RGBA{R: 0x88, G: 0xff, B: 0x00, A: 0xff}
	case ratio < 0.02:
		return "Moderate detail", color.RGBA{R: 0xf0, G: 0xc0, B: 0x40, A: 0xff}
	case ratio < 0.06:
		return "Low detail", color.RGBA{R: 0xff, G: 0x88, B: 0x00, A: 0xff}
	default:
		return "Very coarse", color.RGBA{R: 0xff, G: 0x33, B: 0x00, A: 0xff}
	}
}

// drawDashedRect strokes a dashed rectangle outline.
func drawDashedRect(dst *ebiten.Image, x0, y0, x1, y1 float64, dash, gap, width float32, clr color.RGBA) {
	for _, e := range [4][4]float64{
		{x0, y0, x1, y0}, {x1, y0, x1, y1}, {x1, y1, x0, y1}, {x0, y1, x0, y0},
	} {
		drawDashedLine(dst, e[0], e[1], e[2], e[3], dash, gap, width, clr)
	}
}

func drawDashedLine(dst *ebiten.Image, x0, y0, x1, y1 float64, dash, gap, width float32, clr color.RGBA) {
	length := float32(math.Hypot(x1-x0, y1-y0))
	if length == 0 {
		return
	}
	ux, uy := float32(x1-x0)/length, float32(y1-y0)/length
	cycle := dash + gap
	for d := float32(0); d < length; d += cycle {
		end := d + dash
		if end > length {
			end = length
		}
		vector.StrokeLine(dst, float32(x0)+ux*d, float32(y0)+uy*d,
			float32(x0)+ux*end, float32(y0)+uy*end, width, clr, false)
	}
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
