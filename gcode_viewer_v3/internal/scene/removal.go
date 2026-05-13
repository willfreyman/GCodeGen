// Material-removal heightmap. Port of gcode_viewer_v2/scene/removal.py.
//
// A 2D heightmap of the stock surface — each cell holds the current
// top-of-material Z value. As the tool moves, every cell within the bit's
// XY footprint is lowered to the tool's Z (if that's deeper than the cell's
// current height). The result reads as a 3D carved surface and is enough
// for any 3-axis program without undercuts (the typical CNC-router
// workload).
//
// Known limitation: cell discretization makes circular bit features
// (corner fillets, external roundings) look jagged. A polygon-union
// renderer is designed but unimplemented — see
// docs/POLYGON_RENDERER.md for the full plan. The heightmap stays in
// place regardless because it handles Z-varying moves (plunge ramps,
// 3D surfacing) that the swept-stadium model can't represent.

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

	// MaterialThickness, when > 0, marks cells whose absolute cut depth is
	// greater than this value as through-cut. RefreshMesh renders fully
	// through-cut quads into a separate transparent mesh instead of dropping
	// them or making the whole stock transparent.
	MaterialThickness float64

	Nx, Ny int

	// heights[iy*Nx + ix] = current top-of-material Z for cell (ix, iy).
	heights []float32

	// through[iy*Nx + ix] = true once abs(heights[i]) exceeds
	// MaterialThickness. Driven by Cut/CutSegment and SetMaterialThickness
	// when MaterialThickness > 0.
	through []bool

	// cell-center XY coords (precomputed once)
	xs []float32
	ys []float32

	// Mesh handles populated by BuildActor; used by RefreshMesh to push new
	// Z values to the GPU. Opaque and through-cut transparent quads are split
	// into separate meshes because transparency is material-level in g3n.
	actor *core.Node

	opaquePosVBO   *gls.VBO
	opaqueNormVBO  *gls.VBO
	throughPosVBO  *gls.VBO
	throughNormVBO *gls.VBO
	shellNode      *core.Node
	shellPosVBO    *gls.VBO
	shellNormVBO   *gls.VBO

	// ShowShell controls whether the material-thickness walls and bottom are
	// visible. The shell is only meaningful when MaterialThickness > 0.
	ShowShell bool

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
		X0:        xRange[0],
		X1:        xRange[1],
		Y0:        yRange[0],
		Y1:        yRange[1],
		TopZ:      topZ,
		CellSize:  cellSize,
		Nx:        nx,
		Ny:        ny,
		heights:   make([]float32, nx*ny),
		through:   make([]bool, nx*ny),
		ShowShell: true,
		xs:        make([]float32, nx),
		ys:        make([]float32, ny),
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

	for iy := iy0; iy < iy1; iy++ {
		row := iy * h.Nx
		dy := h.ys[iy] - ty
		dy2 := dy * dy
		for ix := ix0; ix < ix1; ix++ {
			dx := h.xs[ix] - tx
			if dx*dx+dy2 <= rsq {
				idx := row + ix
				if h.heights[idx] > tz {
					h.heights[idx] = tz
					h.through[idx] = h.isThroughDepth(tz)
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
// to detect through-cut cells. Re-evaluates every cell against the new strict
// rule, abs(depth) > materialThickness; call RefreshMesh afterwards to see the
// visual change. Stored heights are not clamped, so changing the thickness can
// immediately move quads between the opaque and transparent meshes.
func (h *Heightmap) SetMaterialThickness(mm float64) {
	h.MaterialThickness = mm
	for i := range h.heights {
		h.through[i] = h.isThroughDepth(h.heights[i])
	}
}

func (h *Heightmap) isThroughDepth(depth float32) bool {
	return h.MaterialThickness > 0 && math.Abs(float64(depth)) > h.MaterialThickness
}

// SetShowShell toggles the material-thickness side walls and bottom mesh. The
// shell still stays hidden when MaterialThickness <= 0 because no bottom plane
// can be inferred from a disabled thickness setting.
func (h *Heightmap) SetShowShell(show bool) {
	h.ShowShell = show
	if h.shellNode != nil {
		h.shellNode.SetVisible(show && h.MaterialThickness > 0)
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

// Actor returns the g3n node containing the opaque and transparent heightmap
// meshes, building it lazily on first call. surfaceColor is the base color of
// the carved material.
func (h *Heightmap) Actor(surfaceColor math32.Color) *core.Node {
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
	indices := sequentialIndices(nVerts)

	opaqueGeom, opaquePos, opaqueNorm := newHeightmapGeometry(nVerts, indices)
	h.opaquePosVBO = opaquePos
	h.opaqueNormVBO = opaqueNorm

	throughGeom, throughPos, throughNorm := newHeightmapGeometry(nVerts, indices)
	h.throughPosVBO = throughPos
	h.throughNormVBO = throughNorm

	opaqueMat := h.newSurfaceMaterial(1.0, false)
	throughMat := h.newSurfaceMaterial(0.28, true)
	throughMat.SetDepthMask(false)

	shellQuads := (h.Nx-1)*(h.Ny-1) + 2*(h.Nx-1) + 2*(h.Ny-1)
	shellGeom, shellPos, shellNorm := newHeightmapGeometry(shellQuads*6, sequentialIndices(shellQuads*6))
	h.shellPosVBO = shellPos
	h.shellNormVBO = shellNorm
	shellMat := h.newSurfaceMaterial(0.92, false)

	h.actor = core.NewNode()
	h.actor.SetName("heightmap")
	h.actor.Add(graphic.NewMesh(opaqueGeom, opaqueMat))
	h.actor.Add(graphic.NewMesh(throughGeom, throughMat))
	h.shellNode = core.NewNode()
	h.shellNode.SetName("heightmap-shell")
	h.shellNode.Add(graphic.NewMesh(shellGeom, shellMat))
	h.actor.Add(h.shellNode)

	// Populate the VBOs with the initial flat surface.
	h.RefreshMesh()
}

func sequentialIndices(nVerts int) math32.ArrayU32 {
	indices := math32.NewArrayU32(nVerts, nVerts)
	for i := 0; i < nVerts; i++ {
		indices[i] = uint32(i)
	}
	return indices
}

func newHeightmapGeometry(nVerts int, indices math32.ArrayU32) (*geometry.Geometry, *gls.VBO, *gls.VBO) {
	positions := math32.NewArrayF32(nVerts*3, nVerts*3)
	normals := math32.NewArrayF32(nVerts*3, nVerts*3)

	geom := geometry.NewGeometry()
	posVBO := gls.NewVBO(positions).AddAttrib(gls.VertexPosition)
	normVBO := gls.NewVBO(normals).AddAttrib(gls.VertexNormal)
	geom.AddVBO(posVBO)
	geom.AddVBO(normVBO)
	geom.SetIndices(indices)
	return geom, posVBO, normVBO
}

func (h *Heightmap) newSurfaceMaterial(opacity float32, transparent bool) *material.Standard {
	mat := material.NewStandard(&h.surfaceColor)
	mat.SetSide(material.SideDouble)
	mat.SetTransparent(transparent)
	mat.SetOpacity(opacity)
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
	return mat
}

// RefreshMesh regenerates positions + per-triangle face normals from the
// current heights[] grid and re-uploads to the GPU. Cost: ~6×Nx×Ny vertices
// of float32 work per call. For typical grids (≤100×100) this is well
// under a millisecond — safe to call at the throttled 15 Hz cadence.
func (h *Heightmap) RefreshMesh() {
	if h.opaquePosVBO == nil || h.throughPosVBO == nil {
		return
	}

	opaquePosBuf := h.opaquePosVBO.Buffer()
	opaqueNormBuf := h.opaqueNormVBO.Buffer()
	throughPosBuf := h.throughPosVBO.Buffer()
	throughNormBuf := h.throughNormVBO.Buffer()

	opi, oni := 0, 0
	tpi, tni := 0, 0

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

			x0 := h.xs[ix]
			x1 := h.xs[ix+1]
			y0 := h.ys[iy]
			y1 := h.ys[iy+1]
			z00 := h.heights[i00]
			z10 := h.heights[i10]
			z01 := h.heights[i01]
			z11 := h.heights[i11]

			throughQuad := h.through[i00] && h.through[i10] && h.through[i01] && h.through[i11]
			if throughQuad {
				opi, oni = writeDegenerateQuad(opaquePosBuf, opaqueNormBuf, opi, oni)
				tpi, tni = writeQuad(throughPosBuf, throughNormBuf, tpi, tni, x0, x1, y0, y1, z00, z10, z01, z11)
			} else {
				opi, oni = writeQuad(opaquePosBuf, opaqueNormBuf, opi, oni, x0, x1, y0, y1, z00, z10, z01, z11)
				tpi, tni = writeDegenerateQuad(throughPosBuf, throughNormBuf, tpi, tni)
			}
		}
	}

	h.writeShellMesh()

	h.opaquePosVBO.Update()
	h.opaqueNormVBO.Update()
	h.throughPosVBO.Update()
	h.throughNormVBO.Update()
}

func (h *Heightmap) writeShellMesh() {
	if h.shellNode != nil {
		h.shellNode.SetVisible(h.ShowShell && h.MaterialThickness > 0)
	}
	if h.shellPosVBO == nil || h.shellNormVBO == nil {
		return
	}

	posBuf := h.shellPosVBO.Buffer()
	normBuf := h.shellNormVBO.Buffer()
	pi, ni := 0, 0
	bottomZ := float32(-h.MaterialThickness)

	for iy := 0; iy < h.Ny-1; iy++ {
		for ix := 0; ix < h.Nx-1; ix++ {
			i00 := iy*h.Nx + ix
			i10 := iy*h.Nx + ix + 1
			i01 := (iy+1)*h.Nx + ix
			i11 := (iy+1)*h.Nx + ix + 1

			throughQuad := h.MaterialThickness > 0 && h.through[i00] && h.through[i10] && h.through[i01] && h.through[i11]
			if h.MaterialThickness <= 0 || throughQuad {
				pi, ni = writeDegenerateQuad(posBuf, normBuf, pi, ni)
				continue
			}

			x0 := h.xs[ix]
			x1 := h.xs[ix+1]
			y0 := h.ys[iy]
			y1 := h.ys[iy+1]
			pi, ni = writeQuadFixedNormal(posBuf, normBuf, pi, ni, 0, 0, -1,
				x0, y0, bottomZ,
				x0, y1, bottomZ,
				x1, y0, bottomZ,
				x1, y1, bottomZ,
			)
		}
	}

	// South and north exterior walls. Through-cut edge segments are omitted so
	// cuts that pass completely through can open the shell at the stock edge.
	for ix := 0; ix < h.Nx-1; ix++ {
		i0 := ix
		i1 := ix + 1
		if x0, y0, z0, x1, y1, z1, ok := h.shellEdgeEndpoints(i0, i1, bottomZ); ok {
			pi, ni = writeQuadFixedNormal(posBuf, normBuf, pi, ni, 0, -1, 0, x0, y0, z0, x0, y0, bottomZ, x1, y1, z1, x1, y1, bottomZ)
		} else {
			pi, ni = writeDegenerateQuad(posBuf, normBuf, pi, ni)
		}

		i0 = (h.Ny-1)*h.Nx + ix
		i1 = (h.Ny-1)*h.Nx + ix + 1
		if x0, y0, z0, x1, y1, z1, ok := h.shellEdgeEndpoints(i0, i1, bottomZ); ok {
			pi, ni = writeQuadFixedNormal(posBuf, normBuf, pi, ni, 0, 1, 0, x0, y0, z0, x1, y1, z1, x0, y0, bottomZ, x1, y1, bottomZ)
		} else {
			pi, ni = writeDegenerateQuad(posBuf, normBuf, pi, ni)
		}
	}

	// West and east exterior walls.
	for iy := 0; iy < h.Ny-1; iy++ {
		i0 := iy * h.Nx
		i1 := (iy + 1) * h.Nx
		if x0, y0, z0, x1, y1, z1, ok := h.shellEdgeEndpoints(i0, i1, bottomZ); ok {
			pi, ni = writeQuadFixedNormal(posBuf, normBuf, pi, ni, -1, 0, 0, x0, y0, z0, x1, y1, z1, x0, y0, bottomZ, x1, y1, bottomZ)
		} else {
			pi, ni = writeDegenerateQuad(posBuf, normBuf, pi, ni)
		}

		i0 = iy*h.Nx + h.Nx - 1
		i1 = (iy+1)*h.Nx + h.Nx - 1
		if x0, y0, z0, x1, y1, z1, ok := h.shellEdgeEndpoints(i0, i1, bottomZ); ok {
			pi, ni = writeQuadFixedNormal(posBuf, normBuf, pi, ni, 1, 0, 0, x0, y0, z0, x0, y0, bottomZ, x1, y1, z1, x1, y1, bottomZ)
		} else {
			pi, ni = writeDegenerateQuad(posBuf, normBuf, pi, ni)
		}
	}

	h.shellPosVBO.Update()
	h.shellNormVBO.Update()
}

func (h *Heightmap) shellEdgeEndpoints(i0, i1 int, bottomZ float32) (float32, float32, float32, float32, float32, float32, bool) {
	if h.MaterialThickness <= 0 || (h.through[i0] && h.through[i1]) {
		return 0, 0, 0, 0, 0, 0, false
	}
	z0 := h.heights[i0]
	if z0 < bottomZ {
		z0 = bottomZ
	}
	z1 := h.heights[i1]
	if z1 < bottomZ {
		z1 = bottomZ
	}
	if z0 == bottomZ && z1 == bottomZ {
		return 0, 0, 0, 0, 0, 0, false
	}
	return h.xs[i0%h.Nx], h.ys[i0/h.Nx], z0, h.xs[i1%h.Nx], h.ys[i1/h.Nx], z1, true
}

func writeQuad(posBuf, normBuf *math32.ArrayF32, pi, ni int, x0, x1, y0, y1, z00, z10, z01, z11 float32) (int, int) {
	return writeQuad3D(posBuf, normBuf, pi, ni,
		x0, y0, z00,
		x1, y0, z10,
		x0, y1, z01,
		x1, y1, z11,
	)
}

func writeQuad3D(posBuf, normBuf *math32.ArrayF32, pi, ni int,
	ax, ay, az, bx, by, bz, cx, cy, cz, dx, dy, dz float32) (int, int) {
	// Triangle A: a, b, c
	n1x, n1y, n1z := triNormal(ax, ay, az, bx, by, bz, cx, cy, cz)
	pi = writePos(posBuf, pi, ax, ay, az, bx, by, bz, cx, cy, cz)
	ni = writeNorm(normBuf, ni, n1x, n1y, n1z, 3)

	// Triangle B: b, d, c
	n2x, n2y, n2z := triNormal(bx, by, bz, dx, dy, dz, cx, cy, cz)
	pi = writePos(posBuf, pi, bx, by, bz, dx, dy, dz, cx, cy, cz)
	ni = writeNorm(normBuf, ni, n2x, n2y, n2z, 3)
	return pi, ni
}

func writeQuadFixedNormal(posBuf, normBuf *math32.ArrayF32, pi, ni int, nx, ny, nz float32,
	ax, ay, az, bx, by, bz, cx, cy, cz, dx, dy, dz float32) (int, int) {
	pi = writePos(posBuf, pi, ax, ay, az, bx, by, bz, cx, cy, cz)
	ni = writeNorm(normBuf, ni, nx, ny, nz, 3)
	pi = writePos(posBuf, pi, bx, by, bz, dx, dy, dz, cx, cy, cz)
	ni = writeNorm(normBuf, ni, nx, ny, nz, 3)
	return pi, ni
}

func writeDegenerateQuad(posBuf, normBuf *math32.ArrayF32, pi, ni int) (int, int) {
	pi = writePos(posBuf, pi, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	ni = writeNorm(normBuf, ni, 0, 0, 1, 3)
	pi = writePos(posBuf, pi, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	ni = writeNorm(normBuf, ni, 0, 0, 1, 3)
	return pi, ni
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
