// Material-removal heightmap. Port of gcode_viewer_v2/scene/removal.py.
//
// A 2D heightmap of the stock surface — each cell holds the current
// top-of-material Z value. As the tool moves, every cell within the bit's
// XY footprint is lowered to the tool's Z (if that's deeper than the cell's
// current height). The result reads as a 3D carved surface and is enough
// for any 3-axis program without undercuts (the typical CNC-router
// workload).

package scene

import (
	"math"

	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"

	"gcodegen.local/viewer/internal/parser"
)

// HeightmapCellSize returns the recommended cell size (mm) for a given bit
// diameter. Tighter than v2 (which used bit/4 floored at 1.0) — bit/8
// floored at 0.4 mm gives ~4× more cells, so cuts read as crisp facets
// rather than coarse stair-steps. Memory cost: a 100×100 mm stock at
// 0.75 mm cells is ~134×134 ≈ 18 K cells = ~108 K vertices flat-shaded
// (~2.5 MB VBO), which still uploads comfortably at the 15 Hz throttle.
//
// If a really small bit (≤3 mm) is being used the floor kicks in to keep
// the grid from exploding past ~250×250 cells on big stock.
func HeightmapCellSize(bitDiameter float64) float64 {
	c := bitDiameter / 8.0
	if c < 0.4 {
		return 0.4
	}
	return c
}

// Heightmap is the mutable carved-surface grid. Coordinate frame is the
// same world frame as the parsed toolpath (mm, Z-up).
type Heightmap struct {
	X0, X1   float64
	Y0, Y1   float64
	TopZ     float64
	CellSize float64

	// MaterialThickness, when > 0, sets the bottom of the stock at
	// -MaterialThickness. Cuts that reach the bottom mark the cell as
	// "through" — RefreshMesh then drops fully-through quads from the
	// mesh, producing a real slice/cutout hole in the stock.
	MaterialThickness float64

	Nx, Ny int

	// heights[iy*Nx + ix] = current top-of-material Z for cell (ix, iy).
	heights []float32

	// through[iy*Nx + ix] = true once a cut has reached or passed the
	// bottom of the stock at this cell. Driven by Cut/CutSegment when
	// MaterialThickness > 0.
	through []bool

	// cell-center XY coords (precomputed once)
	xs []float32
	ys []float32

	// Mesh handles populated by BuildActor; used by RefreshMesh to push new
	// Z values to the GPU.
	actor   *graphic.Mesh
	posVBO  *gls.VBO
	normVBO *gls.VBO

	// surfaceColor lets us recompute lighting/coloring on refresh.
	surfaceColor math32.Color
}

// NewHeightmap builds a heightmap covering xRange × yRange at cellSize, with
// every cell starting at topZ.
func NewHeightmap(xRange, yRange [2]float64, topZ, cellSize float64) *Heightmap {
	if cellSize <= 0 {
		cellSize = 1.0
	}

	nx := int(math.Ceil((xRange[1]-xRange[0])/cellSize)) + 1
	ny := int(math.Ceil((yRange[1]-yRange[0])/cellSize)) + 1
	if nx < 2 {
		nx = 2
	}
	if ny < 2 {
		ny = 2
	}

	h := &Heightmap{
		X0:       xRange[0],
		X1:       xRange[1],
		Y0:       yRange[0],
		Y1:       yRange[1],
		TopZ:     topZ,
		CellSize: cellSize,
		Nx:       nx,
		Ny:       ny,
		heights:  make([]float32, nx*ny),
		through:  make([]bool, nx*ny),
		xs:       make([]float32, nx),
		ys:       make([]float32, ny),
	}

	for i := range h.heights {
		h.heights[i] = float32(topZ)
	}
	for ix := 0; ix < nx; ix++ {
		h.xs[ix] = float32(xRange[0] + float64(ix)*cellSize)
	}
	for iy := 0; iy < ny; iy++ {
		h.ys[iy] = float32(yRange[0] + float64(iy)*cellSize)
	}
	return h
}

// Cut lowers every cell within bitRadius of (toolX, toolY) to toolZ (only
// if the cell was higher than toolZ — never raises). Returns true if any
// cell was modified.
func (h *Heightmap) Cut(toolX, toolY, toolZ, bitRadius float64) bool {
	if toolZ >= h.TopZ {
		return false
	}

	// Bounding box in cell indices, clamped to grid.
	ix0 := int((toolX - bitRadius - h.X0) / h.CellSize)
	if ix0 < 0 {
		ix0 = 0
	}
	ix1 := int((toolX+bitRadius-h.X0)/h.CellSize) + 1
	if ix1 > h.Nx {
		ix1 = h.Nx
	}
	iy0 := int((toolY - bitRadius - h.Y0) / h.CellSize)
	if iy0 < 0 {
		iy0 = 0
	}
	iy1 := int((toolY+bitRadius-h.Y0)/h.CellSize) + 1
	if iy1 > h.Ny {
		iy1 = h.Ny
	}
	if ix0 >= ix1 || iy0 >= iy1 {
		return false
	}

	rsq := float32(bitRadius * bitRadius)
	tz := float32(toolZ)
	tx := float32(toolX)
	ty := float32(toolY)
	modified := false

	// If a material thickness is set, any cut at or below the bottom marks
	// the cell as through (the part is sliced out at this point).
	throughMode := h.MaterialThickness > 0
	bottomZ := float32(-h.MaterialThickness)
	cutThrough := throughMode && tz <= bottomZ
	clampedZ := tz
	if cutThrough {
		clampedZ = bottomZ
	}

	for iy := iy0; iy < iy1; iy++ {
		row := iy * h.Nx
		dy := h.ys[iy] - ty
		dy2 := dy * dy
		for ix := ix0; ix < ix1; ix++ {
			dx := h.xs[ix] - tx
			if dx*dx+dy2 <= rsq {
				idx := row + ix
				if cutThrough {
					if !h.through[idx] {
						h.through[idx] = true
						h.heights[idx] = bottomZ
						modified = true
					} else if h.heights[idx] > bottomZ {
						h.heights[idx] = bottomZ
						modified = true
					}
				} else if h.heights[idx] > clampedZ {
					h.heights[idx] = clampedZ
					modified = true
				}
			}
		}
	}
	return modified
}

// Reset returns the heightmap to its un-carved state — every cell back to
// TopZ and the through[] markers cleared. Used by the scrub-replay path so
// dragging the progress bar backward actually un-cuts material (the
// running heightmap stores only current state, no per-tick history).
//
// Doesn't touch grid dimensions or MaterialThickness — call this when you
// want to keep the same stock config but start fresh on cuts.
func (h *Heightmap) Reset() {
	topZ := float32(h.TopZ)
	for i := range h.heights {
		h.heights[i] = topZ
		h.through[i] = false
	}
}

// SetMaterialThickness sets (or clears, if mm <= 0) the stock thickness used
// to detect "cut through" cells. Re-evaluates every cell against the new
// bottom and updates the through[] array; call RefreshMesh afterwards to
// see the visual change.
func (h *Heightmap) SetMaterialThickness(mm float64) {
	h.MaterialThickness = mm
	if mm <= 0 {
		for i := range h.through {
			h.through[i] = false
		}
		return
	}
	bottom := float32(-mm)
	for i := range h.heights {
		if h.heights[i] <= bottom {
			h.heights[i] = bottom
			h.through[i] = true
		} else {
			h.through[i] = false
		}
	}
}

// CutSegment samples the segment from p1 to p2 at intervals of bitRadius/2
// (so footprints overlap and the swept volume looks continuous) and applies
// Cut at each sample.
func (h *Heightmap) CutSegment(p1, p2 parser.Point, bitRadius float64) {
	dx := p2.X - p1.X
	dy := p2.Y - p1.Y
	dz := p2.Z - p1.Z
	length := math.Sqrt(dx*dx + dy*dy)

	if length < 1e-9 {
		minZ := p1.Z
		if p2.Z < minZ {
			minZ = p2.Z
		}
		h.Cut(p1.X, p1.Y, minZ, bitRadius)
		return
	}

	step := bitRadius / 2
	if step < 0.1 {
		step = 0.1
	}
	n := int(length/step) + 1
	if n < 2 {
		n = 2
	}

	inv := 1.0 / float64(n)
	for k := 0; k <= n; k++ {
		t := float64(k) * inv
		h.Cut(
			p1.X+t*dx,
			p1.Y+t*dy,
			p1.Z+t*dz,
			bitRadius,
		)
	}
}

// Actor returns the g3n mesh node, building it lazily on first call.
// surfaceColor is the base color of the carved material.
func (h *Heightmap) Actor(surfaceColor math32.Color) *graphic.Mesh {
	if h.actor != nil {
		return h.actor
	}
	h.surfaceColor = surfaceColor
	h.buildActor()
	return h.actor
}

func (h *Heightmap) buildActor() {
	// Flat shading via vertex duplication — each triangle gets its own
	// three vertex copies sharing one face normal, so adjacent triangles
	// don't average their normals across the seam. Result: crisp facets,
	// rigid-looking machined surface (no melted/blurry cut walls).
	//
	// 2 triangles × 3 vertices = 6 unique vertex slots per quad cell.
	nVerts := (h.Nx - 1) * (h.Ny - 1) * 6

	positions := math32.NewArrayF32(nVerts*3, nVerts*3)
	normals := math32.NewArrayF32(nVerts*3, nVerts*3)
	indices := math32.NewArrayU32(nVerts, nVerts)
	for i := 0; i < nVerts; i++ {
		indices[i] = uint32(i)
	}

	geom := geometry.NewGeometry()
	h.posVBO = gls.NewVBO(positions).AddAttrib(gls.VertexPosition)
	h.normVBO = gls.NewVBO(normals).AddAttrib(gls.VertexNormal)
	geom.AddVBO(h.posVBO)
	geom.AddVBO(h.normVBO)
	geom.SetIndices(indices)

	mat := material.NewStandard(&h.surfaceColor)
	mat.SetSide(material.SideDouble)
	// Ambient = 50% body color so even down-facing slopes have a base
	// brightness; specular = subtle warm sheen for that "freshly machined
	// wood" highlight on flat tops.
	mat.SetAmbientColor(&math32.Color{
		R: h.surfaceColor.R * 0.50,
		G: h.surfaceColor.G * 0.50,
		B: h.surfaceColor.B * 0.50,
	})
	mat.SetSpecularColor(&math32.Color{R: 0.35, G: 0.30, B: 0.22})
	mat.SetShininess(28)

	h.actor = graphic.NewMesh(geom, mat)

	// Populate the VBOs with the initial flat surface.
	h.RefreshMesh()
}

// RefreshMesh regenerates positions + per-triangle face normals from the
// current heights[] grid and re-uploads to the GPU. Cost: ~6×Nx×Ny vertices
// of float32 work per call. For typical grids (≤100×100) this is well
// under a millisecond — safe to call at the throttled 15 Hz cadence.
func (h *Heightmap) RefreshMesh() {
	if h.posVBO == nil {
		return
	}

	posBuf := h.posVBO.Buffer()
	normBuf := h.normVBO.Buffer()

	pi := 0
	ni := 0

	throughMode := h.MaterialThickness > 0

	for iy := 0; iy < h.Ny-1; iy++ {
		for ix := 0; ix < h.Nx-1; ix++ {
			// Quad corners (CCW from above):
			//   v0 ─── v1
			//   │       │
			//   v2 ─── v3
			i00 := iy*h.Nx + ix
			i10 := iy*h.Nx + ix + 1
			i01 := (iy+1)*h.Nx + ix
			i11 := (iy+1)*h.Nx + ix + 1

			// All four corners cut through? Drop the quad — emit
			// degenerate (zero-area) triangles so the GPU draws nothing
			// and we end up with a real hole in the mesh where the part
			// was sliced out.
			if throughMode && h.through[i00] && h.through[i10] && h.through[i01] && h.through[i11] {
				pi = writePos(posBuf, pi, 0, 0, 0, 0, 0, 0, 0, 0, 0)
				ni = writeNorm(normBuf, ni, 0, 0, 1, 3)
				pi = writePos(posBuf, pi, 0, 0, 0, 0, 0, 0, 0, 0, 0)
				ni = writeNorm(normBuf, ni, 0, 0, 1, 3)
				continue
			}

			x0 := h.xs[ix]
			x1 := h.xs[ix+1]
			y0 := h.ys[iy]
			y1 := h.ys[iy+1]
			z00 := h.heights[i00]
			z10 := h.heights[i10]
			z01 := h.heights[i01]
			z11 := h.heights[i11]

			// Triangle A: v0(x0,y0,z00), v1(x1,y0,z10), v2(x0,y1,z01)
			n1x, n1y, n1z := triNormal(x0, y0, z00, x1, y0, z10, x0, y1, z01)
			pi = writePos(posBuf, pi, x0, y0, z00, x1, y0, z10, x0, y1, z01)
			ni = writeNorm(normBuf, ni, n1x, n1y, n1z, 3)

			// Triangle B: v1(x1,y0,z10), v3(x1,y1,z11), v2(x0,y1,z01)
			n2x, n2y, n2z := triNormal(x1, y0, z10, x1, y1, z11, x0, y1, z01)
			pi = writePos(posBuf, pi, x1, y0, z10, x1, y1, z11, x0, y1, z01)
			ni = writeNorm(normBuf, ni, n2x, n2y, n2z, 3)
		}
	}

	h.posVBO.Update()
	h.normVBO.Update()
}

// triNormal returns the unit face normal of the triangle (a, b, c) using the
// right-hand rule on cross((b-a), (c-a)). Inlined float math (no heap allocs).
func triNormal(ax, ay, az, bx, by, bz, cx, cy, cz float32) (float32, float32, float32) {
	ux, uy, uz := bx-ax, by-ay, bz-az
	vx, vy, vz := cx-ax, cy-ay, cz-az
	nx := uy*vz - uz*vy
	ny := uz*vx - ux*vz
	nz := ux*vy - uy*vx
	il := 1.0 / math32.Sqrt(nx*nx+ny*ny+nz*nz)
	return nx * il, ny * il, nz * il
}

// writePos appends three xyz vertices to buf starting at index i, returning
// the new write head.
func writePos(buf *math32.ArrayF32, i int,
	ax, ay, az, bx, by, bz, cx, cy, cz float32) int {
	(*buf)[i+0] = ax
	(*buf)[i+1] = ay
	(*buf)[i+2] = az
	(*buf)[i+3] = bx
	(*buf)[i+4] = by
	(*buf)[i+5] = bz
	(*buf)[i+6] = cx
	(*buf)[i+7] = cy
	(*buf)[i+8] = cz
	return i + 9
}

// writeNorm appends `count` copies of normal (nx,ny,nz) starting at index i.
func writeNorm(buf *math32.ArrayF32, i int, nx, ny, nz float32, count int) int {
	for k := 0; k < count; k++ {
		(*buf)[i+0] = nx
		(*buf)[i+1] = ny
		(*buf)[i+2] = nz
		i += 3
	}
	return i
}
