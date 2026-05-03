package scene

import (
	"math"

	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"

	"gcodegen.local/viewer/internal/parser"
)

// Rapid-dash visual constants. Rapids shorter than 2*dashLen+gapLen are drawn
// as a single solid line; longer rapids are pre-segmented into dashes for the
// classic CAM "rapid travel" look.
const (
	rapidDashLen = 2.0 // mm of visible dash
	rapidGapLen  = 2.0 // mm of gap between dashes
)

// pale-blue rapid color, broadcast to every rapid vertex
var rapidR, rapidG, rapidB float32 = 0.45, 0.65, 0.95

// NewPathActor builds a core.Node containing all the toolpath geometry:
//
//   - One LineStrip per "cut chain" (consecutive non-G0 moves, each vertex
//     colored by its Z depth via the shared DepthColor gradient).
//   - One Lines mesh holding ALL rapid (G0) segments as dashed line pairs,
//     drawn in pale blue via per-vertex color VBO (Basic material is the
//     only built-in g3n material that honors VertexColor; pumping a uniform
//     color through the same VBO keeps the rapid pipeline identical to the
//     cut pipeline — one shader, one material type for everything).
//
// Returns the parent node ready to be added to a scene root.
func NewPathActor(moves []*parser.Move, minCutZ float64) *core.Node {
	root := core.NewNode()
	root.SetName("path")

	if len(moves) == 0 {
		return root
	}

	var cutRun []*parser.Move
	var rapidPositions []float32

	flushCutRun := func() {
		if len(cutRun) == 0 {
			return
		}
		if mesh := buildCutLineStrip(cutRun, minCutZ); mesh != nil {
			root.Add(mesh)
		}
		cutRun = cutRun[:0]
	}

	for _, m := range moves {
		if m.Kind == "G0" {
			flushCutRun()
			rapidPositions = appendRapidDashes(rapidPositions, m.Points)
			continue
		}
		cutRun = append(cutRun, m)
	}
	flushCutRun()

	if len(rapidPositions) > 0 {
		root.Add(buildRapidLines(rapidPositions))
	}
	return root
}

func buildCutLineStrip(run []*parser.Move, minCutZ float64) *graphic.LineStrip {
	if len(run) == 0 {
		return nil
	}

	positions := math32.NewArrayF32(0, 0)
	colors := math32.NewArrayF32(0, 0)

	addVertex := func(p parser.Point) {
		positions.Append(float32(p.X), float32(p.Y), float32(p.Z))
		r, g, b := DepthColor(p.Z, minCutZ)
		colors.Append(r, g, b)
	}

	for _, p := range run[0].Points {
		addVertex(p)
	}
	// Skip the seam vertex when consecutive moves share an endpoint —
	// avoids doubled vertices in the strip.
	for i := 1; i < len(run); i++ {
		pts := run[i].Points
		if len(pts) == 0 {
			continue
		}
		prev := run[i-1].Points[len(run[i-1].Points)-1]
		start := 0
		if pointsEqual(prev, pts[0]) {
			start = 1
		}
		for _, p := range pts[start:] {
			addVertex(p)
		}
	}

	if positions.Size() < 6 { // fewer than 2 vertices
		return nil
	}

	geom := geometry.NewGeometry()
	geom.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition))
	geom.AddVBO(gls.NewVBO(colors).AddAttrib(gls.VertexColor))
	return graphic.NewLineStrip(geom, material.NewBasic())
}

func appendRapidDashes(out []float32, pts []parser.Point) []float32 {
	for i := 1; i < len(pts); i++ {
		out = appendSegmentDashes(out, pts[i-1], pts[i])
	}
	return out
}

func appendSegmentDashes(out []float32, p1, p2 parser.Point) []float32 {
	dx := p2.X - p1.X
	dy := p2.Y - p1.Y
	dz := p2.Z - p1.Z
	length := math.Sqrt(dx*dx + dy*dy + dz*dz)

	period := rapidDashLen + rapidGapLen
	if length < period+rapidDashLen {
		return appendPair(out, p1, p2)
	}

	n := int(length / period)
	for k := 0; k < n; k++ {
		t0 := (float64(k) * period) / length
		t1 := (float64(k)*period + rapidDashLen) / length
		if t1 > 1 {
			t1 = 1
		}
		out = appendPair(out,
			parser.Point{X: p1.X + t0*dx, Y: p1.Y + t0*dy, Z: p1.Z + t0*dz},
			parser.Point{X: p1.X + t1*dx, Y: p1.Y + t1*dy, Z: p1.Z + t1*dz},
		)
	}
	return out
}

func appendPair(out []float32, a, b parser.Point) []float32 {
	return append(out,
		float32(a.X), float32(a.Y), float32(a.Z),
		float32(b.X), float32(b.Y), float32(b.Z),
	)
}

func buildRapidLines(positions []float32) *graphic.Lines {
	posArr := math32.NewArrayF32(0, len(positions))
	posArr.Append(positions...)

	nVerts := len(positions) / 3
	colorArr := math32.NewArrayF32(0, nVerts*3)
	for i := 0; i < nVerts; i++ {
		colorArr.Append(rapidR, rapidG, rapidB)
	}

	geom := geometry.NewGeometry()
	geom.AddVBO(gls.NewVBO(posArr).AddAttrib(gls.VertexPosition))
	geom.AddVBO(gls.NewVBO(colorArr).AddAttrib(gls.VertexColor))
	return graphic.NewLines(geom, material.NewBasic())
}

func pointsEqual(a, b parser.Point) bool {
	return a.X == b.X && a.Y == b.Y && a.Z == b.Z
}
