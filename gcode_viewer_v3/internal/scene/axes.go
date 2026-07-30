package scene

import (
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"

	"gcodegen.local/viewer/internal/parser"
)

// Axis colors follow the near-universal CAD convention: X red, Y green,
// Z blue. Kept bright so they stay legible against both the dark background
// and the warm stock tone.
var (
	axisColorX = math32.Color{R: 0.92, G: 0.28, B: 0.28}
	axisColorY = math32.Color{R: 0.35, G: 0.88, B: 0.38}
	axisColorZ = math32.Color{R: 0.38, G: 0.58, B: 1.00}
)

// DefaultAxisLength is the triad size (mm) used when there's no model to
// scale against — i.e. the empty startup scene.
const DefaultAxisLength = 20.0

// Axis-length sizing relative to the model. 15% of the largest span reads as
// a reference marker rather than a fourth object in the scene; the clamp keeps
// it usable on both a 20 mm test cut and a 1 m sheet.
const (
	axisSpanFraction = 0.15
	axisLengthMin    = 8.0
	axisLengthMax    = 40.0
)

// NewAxes builds a small XYZ triad — three lines of the given length running
// from the node's local origin along +X, +Y and +Z.
//
// The geometry is local to (0,0,0); position it with Node.SetPosition.
//
// Uses the same per-vertex-color + material.Basic pipeline as the toolpath and
// stock wireframe (Basic is the only built-in g3n material that honors
// VertexColor).
func NewAxes(length float64) *core.Node {
	if length <= 0 {
		length = DefaultAxisLength
	}
	l := float32(length)

	root := core.NewNode()
	root.SetName("axes")

	positions := math32.NewArrayF32(0, 18)
	colors := math32.NewArrayF32(0, 18)

	addAxis := func(ex, ey, ez float32, c math32.Color) {
		positions.Append(0, 0, 0, ex, ey, ez)
		colors.Append(c.R, c.G, c.B, c.R, c.G, c.B)
	}
	addAxis(l, 0, 0, axisColorX)
	addAxis(0, l, 0, axisColorY)
	addAxis(0, 0, l, axisColorZ)

	geom := geometry.NewGeometry()
	geom.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition))
	geom.AddVBO(gls.NewVBO(colors).AddAttrib(gls.VertexColor))

	root.Add(graphic.NewLines(geom, material.NewBasic()))
	return root
}

// NewAxesForStock places a triad at the stock block's low XY corner, at the
// work surface (topZ).
//
// The triad is nudged outward in -X and -Y rather than sitting exactly on the
// corner: the material-removal heightmap covers the stock footprint at the
// same topZ, so a triad anchored on the corner would put its X and Y lines
// coplanar with the carved surface and z-fight along both edges. Offsetting it
// clear of the block also makes it read as an orientation marker instead of a
// stray cut.
//
// Axis length scales with the model so the marker stays proportionate.
func NewAxesForStock(min, max parser.Point, topZ float64) *core.Node {
	length := axisLengthForBounds(min, max)
	gap := length * 0.12

	axes := NewAxes(length)
	axes.SetPosition(
		float32(min.X-StockMargin-gap),
		float32(min.Y-StockMargin-gap),
		float32(topZ),
	)
	return axes
}

// axisLengthForBounds sizes the triad from the model's largest XY/Z span.
func axisLengthForBounds(min, max parser.Point) float64 {
	span := max.X - min.X
	if dy := max.Y - min.Y; dy > span {
		span = dy
	}
	if dz := max.Z - min.Z; dz > span {
		span = dz
	}

	length := span * axisSpanFraction
	if length < axisLengthMin {
		length = axisLengthMin
	}
	if length > axisLengthMax {
		length = axisLengthMax
	}
	return length
}
