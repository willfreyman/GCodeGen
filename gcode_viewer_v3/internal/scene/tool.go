package scene

import (
	"math"

	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
)

// End-mill geometry constants (mm). Match scene/tool.py.
const (
	ShaftHeight = 30.0
	FluteHeight = 12.0
	BandLow     = 11.6
	BandHigh    = 12.4
	LedHeight   = 1.6
	LedGap      = 2.0
)

// LED z-band: sits below the top of the shank with a small gap.
//
//	ledTop = ShaftHeight - LedGap
//	ledBot = ledTop - LedHeight
const (
	ledTop = ShaftHeight - LedGap
	ledBot = ledTop - LedHeight
)

// Per-part radial multipliers (relative to the bit's own radius).
const (
	bandRadiusFactor = 1.02
	ledRadiusFactor  = 1.18
	helixRadiusFactor = 1.005 // slight outset to avoid z-fighting with flute cylinder
)

// Material color palette (per scene/tool.py).
var (
	colorFlute = math32.Color{R: 0.42, G: 0.40, B: 0.39} // dark gunmetal
	colorHelix = math32.Color{R: 0.12, G: 0.12, B: 0.13} // near-black helix lines
	colorBand  = math32.Color{R: 0.18, G: 0.18, B: 0.20} // muted transition band
	colorShank = math32.Color{R: 0.84, G: 0.85, B: 0.88} // polished steel
	colorLedOn  = math32.Color{R: 0.20, G: 0.95, B: 0.20} // green when spindle on
	colorLedOff = math32.Color{R: 0.95, G: 0.55, B: 0.10} // orange when spindle off
)

// Tool wraps a g3n core.Node containing the 5-part end-mill assembly. All
// children translate together via Tool.SetPosition.
type Tool struct {
	Node *core.Node

	flute *graphic.Mesh
	band  *graphic.Mesh
	shank *graphic.Mesh
	led   *graphic.Mesh
	helix *core.Node // 2 line strips parented under here

	ledMat *material.Standard

	bitDiameter float64
}

// NewTool builds the end-mill assembly for a given bit diameter (mm).
func NewTool(bitDiameter float64) *Tool {
	t := &Tool{
		Node:        core.NewNode(),
		bitDiameter: bitDiameter,
	}
	t.Node.SetName("tool")

	r := bitDiameter / 2

	t.flute = makeCylinderZ(r, 0, FluteHeight, &colorFlute, 0.25, 0.7, 0.25, 20)
	t.band = makeCylinderZ(r*bandRadiusFactor, BandLow, BandHigh, &colorBand, 0.25, 0.7, 0.05, 10)
	t.shank = makeCylinderZ(r, FluteHeight, ShaftHeight, &colorShank, 0.30, 0.7, 0.80, 80)

	ledMesh, ledMat := makeLED(r * ledRadiusFactor)
	t.led = ledMesh
	t.ledMat = ledMat

	t.helix = makeHelixLines(r * helixRadiusFactor)

	t.Node.Add(t.flute)
	t.Node.Add(t.band)
	t.Node.Add(t.shank)
	t.Node.Add(t.led)
	t.Node.Add(t.helix)

	t.SetSpindle(false)
	return t
}

// SetPosition translates the entire assembly so the tool tip sits at (x, y, z).
//
// The bit's local origin is at the tip (z=0 of the flute), so SetPosition
// places the cutting point exactly where the controller thinks the tool is.
func (t *Tool) SetPosition(x, y, z float64) {
	t.Node.SetPosition(float32(x), float32(y), float32(z))
}

// SetSpindle toggles the LED color: green when spindle is on, orange when off.
func (t *Tool) SetSpindle(on bool) {
	if on {
		t.ledMat.SetColor(&colorLedOn)
	} else {
		t.ledMat.SetColor(&colorLedOff)
	}
}

// makeCylinderZ builds a Z-aligned cylinder mesh spanning [zBot, zTop] with the
// given radius and material parameters. g3n's NewCylinder is Y-aligned by
// default and centered on the origin; we rotate -90° around X to point it up
// (+Z) and translate so its base sits at zBot.
func makeCylinderZ(radius, zBot, zTop float64, color *math32.Color, ambient, diffuse, specular, shininess float32) *graphic.Mesh {
	height := zTop - zBot
	if height <= 0 {
		// Degenerate — return a tiny placeholder so the assembly stays valid.
		height = 0.001
		zBot = 0
	}

	const radialSegments = 32
	const heightSegments = 1
	geom := geometry.NewCylinder(radius, height, radialSegments, heightSegments, true, true)

	mat := material.NewStandard(color)
	mat.SetAmbientColor(&math32.Color{R: color.R * ambient, G: color.G * ambient, B: color.B * ambient})
	mat.SetSpecularColor(&math32.Color{R: specular, G: specular, B: specular})
	mat.SetShininess(shininess)

	mesh := graphic.NewMesh(geom, mat)
	// Rotate Y → Z (cylinder ends up pointing along +Z).
	mesh.SetRotationX(-math32.Pi / 2)
	// Lift to (zBot..zTop). The geometry is centered on Y=0 (now Z=0) after
	// rotation, so translate by zBot + height/2 in Z.
	mesh.SetPositionZ(float32(zBot + height/2))
	return mesh
}

// makeLED is a small wrapper that builds the LED ring and returns both the
// mesh and a typed handle to its material so the spindle indicator can flip
// the color cheaply.
func makeLED(radius float64) (*graphic.Mesh, *material.Standard) {
	const radialSegments = 32
	geom := geometry.NewCylinder(radius, LedHeight, radialSegments, 1, true, true)

	mat := material.NewStandard(&colorLedOff)
	mat.SetAmbientColor(&math32.Color{R: 0.7, G: 0.7, B: 0.7})
	mat.SetSpecularColor(&math32.Color{R: 0.1, G: 0.1, B: 0.1})
	mat.SetShininess(10)

	mesh := graphic.NewMesh(geom, mat)
	mesh.SetRotationX(-math32.Pi / 2)
	mesh.SetPositionZ(float32(ledBot + LedHeight/2))
	return mesh, mat
}

// makeHelixLines builds the two helical edge lines that visually mark the
// flute. Two flutes, phase-offset 180°, helix angle 30° (pitch derived from
// tan(60°) like the Python).
func makeHelixLines(radius float64) *core.Node {
	const helixAngleDeg = 30.0
	const segmentsPerRevolution = 32

	// Pitch: the axial distance traveled per revolution.
	// Same formula as scene/tool.py: pitch = FLUTE_HEIGHT * tan(90 - helix_angle).
	pitch := FluteHeight * math.Tan(math.Pi/2-helixAngleDeg*math.Pi/180)
	if pitch <= 0 {
		pitch = FluteHeight // fallback to a half-turn
	}
	revolutions := FluteHeight / pitch
	totalSegments := int(revolutions * segmentsPerRevolution)
	if totalSegments < 8 {
		totalSegments = 8
	}

	root := core.NewNode()
	root.SetName("helix")

	for fluteIndex := 0; fluteIndex < 2; fluteIndex++ {
		phase := math.Pi * float64(fluteIndex) // 0 and π — opposite sides

		positions := math32.NewArrayF32(0, (totalSegments+1)*3)
		colors := math32.NewArrayF32(0, (totalSegments+1)*3)

		for i := 0; i <= totalSegments; i++ {
			t := float64(i) / float64(totalSegments)
			angle := phase + 2*math.Pi*revolutions*t
			x := radius * math.Cos(angle)
			y := radius * math.Sin(angle)
			z := t * FluteHeight

			positions.Append(float32(x), float32(y), float32(z))
			colors.Append(colorHelix.R, colorHelix.G, colorHelix.B)
		}

		geom := geometry.NewGeometry()
		geom.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition))
		geom.AddVBO(gls.NewVBO(colors).AddAttrib(gls.VertexColor))
		root.Add(graphic.NewLineStrip(geom, material.NewBasic()))
	}

	return root
}
