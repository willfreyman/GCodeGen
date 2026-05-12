package img

import (
	"image"
	"math"
)

// RDPSimplify reduces a polyline using the Ramer-Douglas-Peucker
// algorithm. epsilon is the maximum perpendicular deviation (in
// pixels) a point can have from a chord before it must be kept. A
// straight 1000-point line collapses to 2 points; a jagged line keeps
// only the inflections.
func RDPSimplify(pts []image.Point, epsilon float64) []image.Point {
	if len(pts) < 3 || epsilon <= 0 {
		return pts
	}
	// Iterative RDP using an explicit stack of [lo, hi] index ranges,
	// to avoid recursion depth on long polylines.
	keep := make([]bool, len(pts))
	keep[0] = true
	keep[len(pts)-1] = true
	type span struct{ lo, hi int }
	stack := []span{{0, len(pts) - 1}}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if s.hi <= s.lo+1 {
			continue
		}
		var (
			maxDist float64
			maxIdx  int
		)
		ax, ay := float64(pts[s.lo].X), float64(pts[s.lo].Y)
		bx, by := float64(pts[s.hi].X), float64(pts[s.hi].Y)
		for i := s.lo + 1; i < s.hi; i++ {
			d := perpDistance(float64(pts[i].X), float64(pts[i].Y), ax, ay, bx, by)
			if d > maxDist {
				maxDist = d
				maxIdx = i
			}
		}
		if maxDist > epsilon {
			keep[maxIdx] = true
			stack = append(stack, span{s.lo, maxIdx}, span{maxIdx, s.hi})
		}
	}
	out := pts[:0:0]
	for i, k := range keep {
		if k {
			out = append(out, pts[i])
		}
	}
	return out
}

func perpDistance(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	if dx == 0 && dy == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	// Cross product magnitude / segment length = perpendicular distance.
	num := math.Abs(dy*px - dx*py + bx*ay - by*ax)
	den := math.Hypot(dx, dy)
	return num / den
}
