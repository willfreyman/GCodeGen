// Interactive view-cube widget — port of gcode_viewer_v2/scene/view_cube.py.
//
// 14 pickable polygons make up the cube:
//   - 6 octagonal main faces (each cube face has its 4 corners chamfered)
//   - 8 triangular corner faces (one per chamfered corner)
//
// Click a main face → main camera snaps to that orthogonal view.
// Click a corner → main camera snaps to the iso view that shows the three
// adjacent main faces simultaneously.
//
// The cube's camera mirrors the main camera so the cube re-orients in
// lock-step as the user orbits.
//
// Text labels are not yet ported — they require bundling a TTF font through
// g3n's text package and laying out per-face transforms. The chamfered
// geometry (with face shading via per-vertex normals) reads clearly enough
// without labels for now; labels land in a follow-up alongside the M5
// toolbar polish.

package scene

import (
	"fmt"

	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/experimental/collision"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/light"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
)

// ChamferDepth is how far in from each cube corner the chamfer extends, in
// units where the cube spans -0.5 to +0.5. Matches the v2 constant.
const ChamferDepth = 0.15

// Color palette — matches v2's view_cube.py.
var (
	cubeBodyColor    = math32.Color{R: 0.32, G: 0.36, B: 0.42} // cool gray
	cubeHighlightCol = math32.Color{R: 0.55, G: 0.70, B: 0.95} // soft blue
	cubeEdgeColor    = math32.Color{R: 0.10, G: 0.11, B: 0.13} // near-black outline
)

// ViewCubeFace is one pickable polygon of the cube — either a main octagonal
// face or a corner triangle. Normal and Up describe how the main camera
// should snap when this face is clicked.
//
// Group bundles the face mesh and its edge outline so we can add a single
// node to the scene per face. UserData on the Mesh (NOT the Group) is what
// the Raycaster recovers via inter.Object.GetNode().UserData().
type ViewCubeFace struct {
	Group  *core.Node
	Mesh   *graphic.Mesh
	Edges  *graphic.LineStrip
	Mat    *material.Standard
	Name   string         // "TOP", "BOTTOM", ..., or "CORNER_+1_-1_+1"
	Normal math32.Vector3 // outward normal in cube-local space
	Up     math32.Vector3 // view-up direction when looking AT this face
}

// ViewCube is the interactive widget rendered into a corner viewport.
type ViewCube struct {
	Scene  *core.Node
	Camera *camera.Camera
	Faces  []*ViewCubeFace

	hovered *ViewCubeFace
}

// faceSpec describes one of the 6 main cube faces.
type faceSpec struct {
	name   string
	normal math32.Vector3
	up     math32.Vector3
}

var mainFaceSpecs = []faceSpec{
	{"TOP", math32.Vector3{X: 0, Y: 0, Z: 1}, math32.Vector3{X: 0, Y: 1, Z: 0}},
	{"BOTTOM", math32.Vector3{X: 0, Y: 0, Z: -1}, math32.Vector3{X: 0, Y: 1, Z: 0}},
	{"FRONT", math32.Vector3{X: 0, Y: -1, Z: 0}, math32.Vector3{X: 0, Y: 0, Z: 1}},
	{"BACK", math32.Vector3{X: 0, Y: 1, Z: 0}, math32.Vector3{X: 0, Y: 0, Z: 1}},
	{"RIGHT", math32.Vector3{X: 1, Y: 0, Z: 0}, math32.Vector3{X: 0, Y: 0, Z: 1}},
	{"LEFT", math32.Vector3{X: -1, Y: 0, Z: 0}, math32.Vector3{X: 0, Y: 0, Z: 1}},
}

// cornerSign holds the ±1 sign of each axis for one of the 8 cube corners.
type cornerSign struct{ Sx, Sy, Sz int }

var cornerSigns = []cornerSign{
	{+1, +1, +1}, {+1, +1, -1}, {+1, -1, +1}, {+1, -1, -1},
	{-1, +1, +1}, {-1, +1, -1}, {-1, -1, +1}, {-1, -1, -1},
}

// cornerViewUp is the view-up direction used when snapping to ANY corner ISO
// view. +Z keeps the world's "up" reading at the top of every iso shot.
var cornerViewUp = math32.Vector3{X: 0, Y: 0, Z: 1}

// NewViewCube builds the chamfered cube widget.
func NewViewCube() *ViewCube {
	vc := &ViewCube{Scene: core.NewNode()}

	// Narrow-FOV perspective so the cube reads close to "orthographic" but
	// the Raycaster's SetFromCamera math (which assumes a single-origin
	// convergent ray) gives correct results. With orthographic the rays
	// should be parallel — g3n's SetFromCamera doesn't model that, which
	// caused the hover to pick the wrong face for off-center cursor
	// positions. Near/far are tight around the cube to keep depth precision
	// excellent at this scale.
	vc.Camera = camera.NewPerspective(1, 0.5, 100, 30, camera.Vertical)
	vc.Scene.Add(vc.Camera)

	// Pure full-strength ambient, no directional light. Combined with
	// material AmbientColor = body color (set in newPolygonMesh), every
	// face renders as a flat uniform body color regardless of orientation.
	// The chamfer transitions stay visible thanks to the edge outlines.
	vc.Scene.Add(light.NewAmbient(&math32.Color{R: 1, G: 1, B: 1}, 1.0))

	for _, spec := range mainFaceSpecs {
		f := buildOctagonFace(spec)
		vc.Faces = append(vc.Faces, f)
		vc.Scene.Add(f.Group)
	}
	for _, c := range cornerSigns {
		f := buildCornerTriangle(c)
		vc.Faces = append(vc.Faces, f)
		vc.Scene.Add(f.Group)
	}

	return vc
}

// buildOctagonFace builds one of the 6 chamfered cube faces (an octagon).
//
// Vertices are emitted in CCW order (viewed from +n), giving the face an
// outward-pointing normal without manual normal management.
func buildOctagonFace(spec faceSpec) *ViewCubeFace {
	n := normalize3(spec.normal)
	vu := normalize3(spec.up)
	right := cross3(vu, n)
	center := math32.Vector3{X: n.X * 0.5, Y: n.Y * 0.5, Z: n.Z * 0.5}

	const h = 0.5
	const d = ChamferDepth

	// 8 octagon vertices in (right, vu) plane coefficients, CCW from +n.
	uv := [8][2]float32{
		{h - d, h},
		{-(h - d), h},
		{-h, h - d},
		{-h, -(h - d)},
		{-(h - d), -h},
		{h - d, -h},
		{h, -(h - d)},
		{h, h - d},
	}
	verts := make([]math32.Vector3, 8)
	for i, p := range uv {
		u, v := p[0], p[1]
		verts[i] = math32.Vector3{
			X: center.X + right.X*u + vu.X*v,
			Y: center.Y + right.Y*u + vu.Y*v,
			Z: center.Z + right.Z*u + vu.Z*v,
		}
	}

	mesh, mat := newPolygonMesh(verts, n, &cubeBodyColor)
	edges := newPolygonEdges(verts, n)

	group := core.NewNode()
	group.Add(mesh)
	group.Add(edges)

	// Text label for the 6 main faces (TOP, BOTTOM, ...). Reads upright
	// when the user looks at the face from outside the cube — vu defines
	// the "up" axis on the face surface.
	if label := newFaceLabel(spec.name, n, vu); label != nil {
		group.Add(label)
	}

	f := &ViewCubeFace{
		Group:  group,
		Mesh:   mesh,
		Edges:  edges,
		Mat:    mat,
		Name:   spec.name,
		Normal: n,
		Up:     vu,
	}
	mesh.SetUserData(f)
	return f
}

// buildCornerTriangle builds the small triangle at one chamfered corner of
// the cube. Outward normal is the corner's diagonal (sx, sy, sz)/√3.
func buildCornerTriangle(c cornerSign) *ViewCubeFace {
	const h = 0.5
	const d = ChamferDepth

	sx, sy, sz := float32(c.Sx), float32(c.Sy), float32(c.Sz)

	vAlongX := math32.Vector3{X: sx * (h - d), Y: sy * h, Z: sz * h}
	vAlongY := math32.Vector3{X: sx * h, Y: sy * (h - d), Z: sz * h}
	vAlongZ := math32.Vector3{X: sx * h, Y: sy * h, Z: sz * (h - d)}

	// CCW-from-outside winding flips with the parity of the sign product.
	// (Same derivation as Python view_cube.py.)
	var verts []math32.Vector3
	if c.Sx*c.Sy*c.Sz > 0 {
		verts = []math32.Vector3{vAlongX, vAlongY, vAlongZ}
	} else {
		verts = []math32.Vector3{vAlongX, vAlongZ, vAlongY}
	}

	normal := normalize3(math32.Vector3{X: sx, Y: sy, Z: sz})

	mesh, mat := newPolygonMesh(verts, normal, &cubeBodyColor)
	edges := newPolygonEdges(verts, normal)

	group := core.NewNode()
	group.Add(mesh)
	group.Add(edges)

	name := fmt.Sprintf("CORNER_%+d_%+d_%+d", c.Sx, c.Sy, c.Sz)
	f := &ViewCubeFace{
		Group:  group,
		Mesh:   mesh,
		Edges:  edges,
		Mat:    mat,
		Name:   name,
		Normal: normal,
		Up:     cornerViewUp,
	}
	mesh.SetUserData(f)
	return f
}

// newPolygonMesh builds a g3n Mesh from a CCW-ordered vertex ring with a
// uniform per-vertex normal (so g3n's lighting shows the face as flat-shaded
// without averaging across adjacent meshes).
//
// Triangulation is a simple fan from vertex 0, which is correct for any
// convex polygon — both octagons and triangles are convex.
func newPolygonMesh(verts []math32.Vector3, normal math32.Vector3, color *math32.Color) (*graphic.Mesh, *material.Standard) {
	geom := geometry.NewGeometry()

	positions := math32.NewArrayF32(0, len(verts)*3)
	normals := math32.NewArrayF32(0, len(verts)*3)
	for _, v := range verts {
		positions.Append(v.X, v.Y, v.Z)
		normals.Append(normal.X, normal.Y, normal.Z)
	}

	indices := math32.NewArrayU32(0, (len(verts)-2)*3)
	for i := 1; i < len(verts)-1; i++ {
		indices.Append(0, uint32(i), uint32(i+1))
	}

	geom.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition))
	geom.AddVBO(gls.NewVBO(normals).AddAttrib(gls.VertexNormal))
	geom.SetIndices(indices)

	colorCopy := *color
	mat := material.NewStandard(&colorCopy)
	// Ambient color = body color so the face renders as flat uniform
	// color under the full-strength ambient light (no diffuse shading
	// since the cube scene has no directional light).
	mat.SetAmbientColor(&colorCopy)
	mat.SetSide(material.SideDouble)
	mat.SetShininess(0)

	return graphic.NewMesh(geom, mat), mat
}

// SyncToMainCamera mirrors the main camera's view direction onto the cube
// camera so the cube re-orients in lock-step with the main scene.
//
// `focal` is the point the main camera is currently orbiting (set by the
// main view's last LookAt — tracked by the UI layer in sceneState.focal).
func (vc *ViewCube) SyncToMainCamera(mainCam *camera.Camera, focal math32.Vector3) {
	mp := mainCam.Position()

	rx := mp.X - focal.X
	ry := mp.Y - focal.Y
	rz := mp.Z - focal.Z

	mag := math32.Sqrt(rx*rx + ry*ry + rz*rz)
	if mag < 1e-6 {
		return
	}
	// At distance 3.5 with the 30° vertical FOV the unit cube fills ~50% of
	// the viewport — readable but not overwhelming. Tweak this number with
	// the FOV in NewViewCube if the cube needs to look bigger or smaller.
	scale := float32(3.5) / mag
	vc.Camera.SetPosition(rx*scale, ry*scale, rz*scale)
	vc.Camera.LookAt(
		&math32.Vector3{X: 0, Y: 0, Z: 0},
		&math32.Vector3{X: 0, Y: 0, Z: 1},
	)
}

// SetHovered updates which face shows the highlight color (or nil to clear).
//
// Since the cube has no directional light, only AmbientColor contributes to
// the rendered color — but we update both AmbientColor and the base material
// Color so the face stays consistent if someone later re-introduces a
// directional light.
func (vc *ViewCube) SetHovered(face *ViewCubeFace) {
	if vc.hovered == face {
		return
	}
	if vc.hovered != nil {
		body := cubeBodyColor
		vc.hovered.Mat.SetColor(&body)
		vc.hovered.Mat.SetAmbientColor(&body)
	}
	if face != nil {
		hi := cubeHighlightCol
		face.Mat.SetColor(&hi)
		face.Mat.SetAmbientColor(&hi)
	}
	vc.hovered = face
}

// PickFace casts a ray from the cube camera through the given normalized
// device coordinates (NDC, both in [-1, 1]) and returns the hit face, or
// nil if no face was hit.
func (vc *ViewCube) PickFace(ndcX, ndcY float32) *ViewCubeFace {
	rc := collision.NewRaycaster(
		&math32.Vector3{X: 0, Y: 0, Z: 0},
		&math32.Vector3{X: 0, Y: 0, Z: -1},
	)
	if err := rc.SetFromCamera(vc.Camera, ndcX, ndcY); err != nil {
		return nil
	}

	intersects := rc.IntersectObjects(vc.Scene.Children(), true)
	for _, inter := range intersects {
		if inter.Object == nil {
			continue
		}
		if face, ok := inter.Object.GetNode().UserData().(*ViewCubeFace); ok {
			return face
		}
	}
	return nil
}

// newPolygonEdges builds a closed-loop LineStrip tracing the polygon's
// perimeter. Vertices are nudged outward along the polygon's normal by a
// tiny offset so the edge line draws cleanly in front of the face mesh
// (otherwise z-fighting causes visible flicker).
func newPolygonEdges(verts []math32.Vector3, normal math32.Vector3) *graphic.LineStrip {
	const nudge = 0.002

	n := len(verts)
	positions := math32.NewArrayF32(0, (n+1)*3)
	colors := math32.NewArrayF32(0, (n+1)*3)

	for i := 0; i <= n; i++ {
		v := verts[i%n]
		positions.Append(
			v.X+normal.X*nudge,
			v.Y+normal.Y*nudge,
			v.Z+normal.Z*nudge,
		)
		colors.Append(cubeEdgeColor.R, cubeEdgeColor.G, cubeEdgeColor.B)
	}

	geom := geometry.NewGeometry()
	geom.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition))
	geom.AddVBO(gls.NewVBO(colors).AddAttrib(gls.VertexColor))
	return graphic.NewLineStrip(geom, material.NewBasic())
}

// ── small vector helpers (kept local to avoid pulling math elsewhere) ──

func normalize3(v math32.Vector3) math32.Vector3 {
	l := math32.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
	if l < 1e-12 {
		return math32.Vector3{}
	}
	return math32.Vector3{X: v.X / l, Y: v.Y / l, Z: v.Z / l}
}

func cross3(a, b math32.Vector3) math32.Vector3 {
	return math32.Vector3{
		X: a.Y*b.Z - a.Z*b.Y,
		Y: a.Z*b.X - a.X*b.Z,
		Z: a.X*b.Y - a.Y*b.X,
	}
}
