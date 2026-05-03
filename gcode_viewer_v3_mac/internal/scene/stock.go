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

// StockMargin is how much to expand the toolpath bounds in X/Y to size the
// stock block. Matches the v2 visual.
const StockMargin = 5.0 // mm

// StockColor is the warm wood-tone fill color used by the stock outline AND
// the carved-surface heightmap. Exported so removal.go can match the look.
var StockColor = math32.Color{R: 0.78, G: 0.62, B: 0.42}

// stockColor is kept as an alias for the previous unexported name to avoid
// touching every site at once during the refactor.
var stockColor = StockColor

// stockEdgeColor is the slightly darker tone for the wireframe edges.
var stockEdgeColor = math32.Color{R: 0.40, G: 0.30, B: 0.20}

// NewStockActor builds a translucent stock block sized to encompass the given
// XY bounds, with Z extending from minZ (or the deepest cut, whichever is
// lower) up to topZ (default 0 — the work surface).
//
// Returns a parent core.Node containing:
//   - a translucent box mesh (Standard material, double-sided so the inside
//     of the stock is visible when the camera tucks under it)
//   - a Lines mesh tracing the 12 edges of the box for clear silhouettes
//
// Position parameters are in mm in the same coordinate frame as the toolpath.
func NewStockActor(min, max parser.Point, topZ float64) *core.Node {
	// Pad XY by StockMargin so the stock visibly extends past the toolpath.
	x0 := min.X - StockMargin
	x1 := max.X + StockMargin
	y0 := min.Y - StockMargin
	y1 := max.Y + StockMargin

	// Z: from the deepest point (or zero) up to topZ.
	z0 := min.Z
	if z0 > 0 {
		z0 = 0
	}
	z1 := topZ

	w := float32(x1 - x0)
	h := float32(y1 - y0)
	d := float32(z1 - z0)
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	if d <= 0 {
		d = 1
	}

	root := core.NewNode()
	root.SetName("stock")

	// Translucent fill
	box := geometry.NewBox(w, h, d)
	mat := material.NewStandard(&stockColor)
	mat.SetTransparent(true)
	mat.SetOpacity(0.25)
	mat.SetSide(material.SideDouble)
	mesh := graphic.NewMesh(box, mat)
	// NewBox is centered on the origin — translate the mesh so its
	// min-corner sits at (x0, y0, z0).
	mesh.SetPosition(
		float32(x0)+w/2,
		float32(y0)+h/2,
		float32(z0)+d/2,
	)
	root.Add(mesh)

	// Wireframe over the 12 box edges
	edges := buildBoxEdges(float32(x0), float32(y0), float32(z0), float32(x1), float32(y1), float32(z1))
	root.Add(edges)

	return root
}

// NewStockWireframe returns just the 12 edges of the stock box (no
// translucent fill). Pair this with the heightmap material-removal surface
// — the heightmap shows the carved top, the wireframe shows the original
// stock outline, and the box fill is dropped because the carved surface
// occupies the same space.
func NewStockWireframe(min, max parser.Point, topZ float64) *core.Node {
	x0 := min.X - StockMargin
	x1 := max.X + StockMargin
	y0 := min.Y - StockMargin
	y1 := max.Y + StockMargin

	z0 := min.Z
	if z0 > 0 {
		z0 = 0
	}
	z1 := topZ

	root := core.NewNode()
	root.SetName("stockwire")
	root.Add(buildBoxEdges(float32(x0), float32(y0), float32(z0), float32(x1), float32(y1), float32(z1)))
	return root
}

func buildBoxEdges(x0, y0, z0, x1, y1, z1 float32) *graphic.Lines {
	corners := [8][3]float32{
		{x0, y0, z0}, {x1, y0, z0}, {x1, y1, z0}, {x0, y1, z0}, // bottom
		{x0, y0, z1}, {x1, y0, z1}, {x1, y1, z1}, {x0, y1, z1}, // top
	}
	// 12 edges as pairs of corner indices
	edges := [12][2]int{
		{0, 1}, {1, 2}, {2, 3}, {3, 0}, // bottom rectangle
		{4, 5}, {5, 6}, {6, 7}, {7, 4}, // top rectangle
		{0, 4}, {1, 5}, {2, 6}, {3, 7}, // verticals
	}

	positions := math32.NewArrayF32(0, len(edges)*6)
	colors := math32.NewArrayF32(0, len(edges)*6)

	for _, e := range edges {
		a := corners[e[0]]
		b := corners[e[1]]
		positions.Append(a[0], a[1], a[2], b[0], b[1], b[2])
		colors.Append(
			stockEdgeColor.R, stockEdgeColor.G, stockEdgeColor.B,
			stockEdgeColor.R, stockEdgeColor.G, stockEdgeColor.B,
		)
	}

	geom := geometry.NewGeometry()
	geom.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition))
	geom.AddVBO(gls.NewVBO(colors).AddAttrib(gls.VertexColor))
	return graphic.NewLines(geom, material.NewBasic())
}
