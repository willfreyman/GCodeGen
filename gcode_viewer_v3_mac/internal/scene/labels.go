package scene

import (
	_ "embed"
	"fmt"

	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/text"
	"github.com/g3n/engine/texture"
)

// labelFontTTF is a free-software TrueType font (FreeSansBold) bundled into
// the binary so the view-cube labels render without any external file deps.
// The TTF is copied from g3n's own gui/assets/fonts at build time; ~400 KB
// in the .exe, acceptable for a self-contained distribution.
//
//go:embed FreeSansBold.ttf
var labelFontTTF []byte

// labelFont is the parsed TTF, lazily initialized on first label build so a
// font load failure doesn't kill the whole package init.
var labelFont *text.Font

func ensureLabelFont() *text.Font {
	if labelFont != nil {
		return labelFont
	}
	f, err := text.NewFontFromData(labelFontTTF)
	if err != nil {
		fmt.Printf("view-cube: failed to load label font: %v (labels disabled)\n", err)
		return nil
	}
	f.SetPointSize(64)
	f.SetDPI(96)
	f.SetFgColor(&math32.Color4{R: 0.95, G: 0.96, B: 1.0, A: 1.0})
	f.SetBgColor(&math32.Color4{R: 0, G: 0, B: 0, A: 0}) // transparent background
	labelFont = f
	return labelFont
}

// newFaceLabel builds a textured quad showing `label` text, oriented so it
// reads upright when the user looks at the face from outside the cube.
//
// The quad is positioned slightly outside the face plane (along the face
// normal) so it doesn't z-fight with the face mesh or its edge outline.
// Returns nil if the font failed to load.
func newFaceLabel(label string, normal, up math32.Vector3) *graphic.Mesh {
	f := ensureLabelFont()
	if f == nil {
		return nil
	}

	img := f.DrawText(label)
	if img == nil {
		return nil
	}

	bounds := img.Bounds()
	imgW := float32(bounds.Dx())
	imgH := float32(bounds.Dy())
	if imgW <= 0 || imgH <= 0 {
		return nil
	}

	// Size the quad so the text is ~36% of the face dimension tall, but
	// don't let width exceed 80% of face dimension (matches v2's clamp).
	const maxTextHeight = 0.36
	const maxTextWidth = 0.80
	quadH := float32(maxTextHeight)
	quadW := quadH * (imgW / imgH)
	if quadW > maxTextWidth {
		quadW = maxTextWidth
		quadH = quadW * (imgH / imgW)
	}

	n := normalize3(normal)
	vu := normalize3(up)
	right := cross3(vu, n)

	// Center is on the face, nudged slightly outward to clear the body and
	// the edge outline.
	cx := n.X * 0.504
	cy := n.Y * 0.504
	cz := n.Z * 0.504

	// Build quad vertices directly in world space using the face's local
	// (right, up) basis. CCW order from outside.
	hw := quadW / 2
	hh := quadH / 2
	corners := [4][3]float32{
		{cx + right.X*-hw + vu.X*-hh, cy + right.Y*-hw + vu.Y*-hh, cz + right.Z*-hw + vu.Z*-hh}, // bot-left
		{cx + right.X*+hw + vu.X*-hh, cy + right.Y*+hw + vu.Y*-hh, cz + right.Z*+hw + vu.Z*-hh}, // bot-right
		{cx + right.X*+hw + vu.X*+hh, cy + right.Y*+hw + vu.Y*+hh, cz + right.Z*+hw + vu.Z*+hh}, // top-right
		{cx + right.X*-hw + vu.X*+hh, cy + right.Y*-hw + vu.Y*+hh, cz + right.Z*-hw + vu.Z*+hh}, // top-left
	}

	positions := math32.NewArrayF32(0, 12)
	normals := math32.NewArrayF32(0, 12)
	for _, c := range corners {
		positions.Append(c[0], c[1], c[2])
		// All four label-quad vertices share the same normal — the face
		// normal — so the label is uniformly lit (and material.Standard
		// shades them as a flat surface).
		normals.Append(n.X, n.Y, n.Z)
	}

	// UVs: image is stored top-row-first; texture sampler with default flip
	// makes V=0 correspond to image-top. Quad bottom maps to image-bottom
	// (V=1), quad top maps to image-top (V=0).
	uvs := math32.NewArrayF32(0, 8)
	uvs.Append(
		0, 1, // bot-left  → image bot-left
		1, 1, // bot-right → image bot-right
		1, 0, // top-right → image top-right
		0, 0, // top-left  → image top-left
	)

	indices := math32.NewArrayU32(0, 6)
	indices.Append(0, 1, 2, 0, 2, 3)

	geom := geometry.NewGeometry()
	geom.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition))
	geom.AddVBO(gls.NewVBO(normals).AddAttrib(gls.VertexNormal))
	geom.AddVBO(gls.NewVBO(uvs).AddAttrib(gls.VertexTexcoord))
	geom.SetIndices(indices)

	tex := texture.NewTexture2DFromRGBA(img)
	// Standard's vertex shader does `texcoord.y = 1.0 - texcoord.y` when
	// the texture's FlipY flag is set (which it is by default). That works
	// for textures loaded from files where V=0 needs to flip to bottom for
	// GL convention. Our font.DrawText image already has row 0 at top, and
	// our UVs are written for that — disabling FlipY makes the sampling
	// match what we wrote, so text reads upright instead of upside down.
	tex.SetFlipY(false)

	// Standard material is the one that actually samples MatTexture — Basic's
	// fragment shader is hard-coded to `vec4(Color, 1.0)` and ignores any
	// AddTexture call (which is what produced the black-box bug). With high
	// ambient = white the texture color comes through as the literal pixel
	// value regardless of where the light is.
	white := math32.Color{R: 1, G: 1, B: 1}
	mat := material.NewStandard(&white)
	mat.SetAmbientColor(&white)
	mat.SetTransparent(true)
	mat.SetSide(material.SideDouble)
	mat.AddTexture(tex)

	return graphic.NewMesh(geom, mat)
}
