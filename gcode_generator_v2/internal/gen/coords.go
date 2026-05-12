package gen

import (
	"math"
	"strconv"
)

// PerimWH returns the perimeter's pixel width and height. y is inverted
// in Tk, so height = y0 - y1 (gcodegen.py:32-33).
func (e *Editor) PerimWH() (float64, float64) {
	return e.Perim.X1 - e.Perim.X0, e.Perim.Y0 - e.Perim.Y1
}

// PxToMM converts a canvas-pixel point to machine mm using the current
// origin and perim mm-extents. Rounded to 3 decimals to match
// gcodegen.py:35-41 byte-for-byte through the emitter.
func (e *Editor) PxToMM(cx, cy float64) (float64, float64) {
	pw, ph := e.PerimWH()
	if pw == 0 || ph == 0 {
		return 0, 0
	}
	wm := e.Perim.WidthMM
	hm := e.Perim.HeightMM
	xmm := round3((cx - e.Origin.X) * (wm / pw))
	ymm := round3((e.Origin.Y - cy) * (hm / ph))
	return xmm, ymm
}

// MMToPx is the inverse of PxToMM; uses int truncation to match
// gcodegen.py:43-48.
func (e *Editor) MMToPx(xm, ym float64) (float64, float64) {
	pw, ph := e.PerimWH()
	if pw == 0 || ph == 0 {
		return e.Origin.X, e.Origin.Y
	}
	wm := e.Perim.WidthMM
	hm := e.Perim.HeightMM
	px := math.Trunc(e.Origin.X + xm*(pw/wm))
	py := math.Trunc(e.Origin.Y - ym*(ph/hm))
	return px, py
}

// HitOriginRadius is gcodegen.py's HR = 12 for both origin and corner
// handles.
const HitOriginRadius = 12.0

// HitEdgeThreshold is the perim-edge-translate threshold (gcodegen.py:112).
const HitEdgeThreshold = 8.0

// HitStrokeThreshold is the default _nearest_stroke_idx distance.
const HitStrokeThreshold = 7.0

// HitOrigin returns true if (x,y) is within HR of the origin dot.
func (e *Editor) HitOrigin(x, y float64) bool {
	return math.Hypot(x-e.Origin.X, y-e.Origin.Y) < HitOriginRadius
}

// HitHandle returns which corner handle (DragBL/BR/TL/TR) is within HR
// of (x,y), or DragNone if none.
func (e *Editor) HitHandle(x, y float64) DragWhat {
	corners := []struct {
		what DragWhat
		hx, hy float64
	}{
		{DragBL, e.Perim.X0, e.Perim.Y0},
		{DragBR, e.Perim.X1, e.Perim.Y0},
		{DragTL, e.Perim.X0, e.Perim.Y1},
		{DragTR, e.Perim.X1, e.Perim.Y1},
	}
	for _, c := range corners {
		if math.Hypot(x-c.hx, y-c.hy) < HitOriginRadius {
			return c.what
		}
	}
	return DragNone
}

// HitEdge returns true if (x,y) is on (within 8 px of) a perim edge.
func (e *Editor) HitEdge(x, y float64) bool {
	x0, y0, x1, y1 := e.Perim.X0, e.Perim.Y0, e.Perim.X1, e.Perim.Y1
	xmin, xmax := minF(x0, x1), maxF(x0, x1)
	ymin, ymax := minF(y0, y1), maxF(y0, y1)
	t := HitEdgeThreshold
	onTopOrBot := xmin <= x && x <= xmax && (math.Abs(y-y0) < t || math.Abs(y-y1) < t)
	onLeftOrRight := ymin <= y && y <= ymax && (math.Abs(x-x0) < t || math.Abs(x-x1) < t)
	return onTopOrBot || onLeftOrRight
}

// HitTestStroke returns the index of the closest stroke to (x,y) within
// HitStrokeThreshold pixels, or -1 if none.
func (e *Editor) HitTestStroke(x, y float64) int {
	bestI := -1
	bestD := HitStrokeThreshold
	for i, s := range e.Strokes {
		pts := s.Points
		for j := 0; j+1 < len(pts); j++ {
			d := ptSegDist(x, y, pts[j].X, pts[j].Y, pts[j+1].X, pts[j+1].Y)
			if d <= bestD {
				bestI = i
				bestD = d
			}
		}
	}
	return bestI
}

// ptSegDist is the standard point-to-segment distance with the projection
// parameter clamped to [0, 1] (gcodegen.py:116-123).
func ptSegDist(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	if dx == 0 && dy == 0 {
		return math.Hypot(px-x0, py-y0)
	}
	t := ((px-x0)*dx + (py-y0)*dy) / (dx*dx + dy*dy)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	qx, qy := x0+t*dx, y0+t*dy
	return math.Hypot(px-qx, py-qy)
}

// round3 mirrors Python's round(x, 3) using banker's rounding semantics
// close enough for our purposes — emitter inputs come from this and feed
// %.3f format strings, so off-by-one-ulp issues are absorbed by the
// formatter. Pure half-away-from-zero rounding would also work; we pick
// math.Round (half-away-from-zero) for predictability.
func round3(x float64) float64 {
	return math.Round(x*1000) / 1000
}

// itoa is a one-line wrapper used by the model package.
func itoa(n int) string { return strconv.Itoa(n) }
