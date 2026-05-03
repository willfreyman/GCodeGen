// Package scene builds the g3n actors (toolpath, stock, tool, future view-cube)
// shown in the viewer's 3D scene.
package scene

import "math"

// DepthColor returns a 5-stop gradient color for a given Z value.
//
// minCutZ is the deepest cut in the loaded program (a negative number).
// The gradient runs:
//
//	z >=  0      → bright green   (above stock surface)
//	z =  minCutZ → purple         (deepest cut)
//	in between   → yellow → orange → red, interpolated
//
// Matches the v2 Python `_depth_lookup_table` in gcode_viewer_v2/scene/path.py.
//
// Returned floats are in [0,1] and intended to be uploaded directly to the
// VertexColor VBO (g3n's Basic material reads them as RGB).
func DepthColor(z, minCutZ float64) (r, g, b float32) {
	// 5 color stops at normalized depth t in [0,1] where t=0 is at z=0
	// (surface) and t=1 is at z=minCutZ (deepest cut).
	stops := [5][3]float32{
		{0.20, 0.85, 0.20}, // 0.00  green   (surface)
		{0.95, 0.95, 0.20}, // 0.25  yellow
		{1.00, 0.55, 0.10}, // 0.50  orange
		{0.90, 0.10, 0.10}, // 0.75  red
		{0.55, 0.10, 0.75}, // 1.00  purple  (deepest)
	}

	if minCutZ >= 0 || z >= 0 {
		return stops[0][0], stops[0][1], stops[0][2]
	}

	t := z / minCutZ
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	// Find the segment between two adjacent stops.
	scaled := t * float64(len(stops)-1)
	idx := int(math.Floor(scaled))
	if idx >= len(stops)-1 {
		idx = len(stops) - 2
	}
	frac := float32(scaled - float64(idx))

	a := stops[idx]
	bColor := stops[idx+1]
	return lerp(a[0], bColor[0], frac),
		lerp(a[1], bColor[1], frac),
		lerp(a[2], bColor[2], frac)
}

func lerp(a, b, t float32) float32 { return a + (b-a)*t }
