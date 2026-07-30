package parser

import "math"

// fullCircleEps is how close start and end XY must be for the arc to count
// as a programmed full circle rather than a nearly-closed one. Tight enough
// that a genuine 359.9° arc still linearizes as an arc.
const fullCircleEps = 1e-9

// ArcPoints linearizes a G2/G3 arc from (sx,sy) to (ex,ey) about center
// offsets (i,j). The center is at (sx+i, sy+j). Z linearly interpolates
// from sz to ez along the arc parameter. Step count scales with arc length
// to keep facets ~1.5mm.
//
// When the end XY equals the start XY the arc is a full 360° circle — the
// standard way to program one in I/J form, and what helical boring emits on
// every turn. Without that case the sweep computes as zero and the whole
// circle collapses to a single point (a bare Z descent), so a bored hole
// rendered as nothing at all.
//
// R-form arcs are not supported (silently produce a degenerate move) — same
// behavior as the Python reference.
func ArcPoints(sx, sy, sz, ex, ey, ez, i, j float64, clockwise bool) []Point {
	cx := sx + i
	cy := sy + j
	r := math.Hypot(sx-cx, sy-cy)

	a1 := math.Atan2(sy-cy, sx-cx)
	a2 := math.Atan2(ey-cy, ex-cx)

	switch {
	case math.Abs(ex-sx) < fullCircleEps && math.Abs(ey-sy) < fullCircleEps:
		// Full circle. Direction still decides which way we wind, which
		// matters for the Z ramp on a helical pass.
		if clockwise {
			a2 = a1 - 2*math.Pi
		} else {
			a2 = a1 + 2*math.Pi
		}
	case clockwise:
		if a2 > a1 {
			a2 -= 2 * math.Pi
		}
	default:
		if a2 < a1 {
			a2 += 2 * math.Pi
		}
	}

	steps := int(math.Abs(a2-a1) * r / 1.5)
	if steps < 16 {
		steps = 16
	}

	pts := make([]Point, 0, steps+1)
	for n := 0; n <= steps; n++ {
		t := float64(n) / float64(steps)
		a := a1 + (a2-a1)*t
		pts = append(pts, Point{
			X: cx + math.Cos(a)*r,
			Y: cy + math.Sin(a)*r,
			Z: sz + (ez-sz)*t,
		})
	}
	return pts
}
