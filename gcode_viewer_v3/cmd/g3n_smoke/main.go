//go:build smoke

// Package main is a single-file g3n API smoke test that exercises every
// pattern we plan to use in the production CNC viewer.
//
// IMPORTANT: this file uses `engine/app`, which transitively imports the
// audio packages and forces a runtime dependency on OpenAL32.dll +
// libvorbis.dll. The production viewer (cmd/gcodesim) deliberately avoids
// `engine/app` for this reason. To keep the smoke from polluting the
// dependency graph for the production binary, this file is gated behind a
// `smoke` build tag and is NOT compiled by default.
//
// Build/run with:
//
//	go run -tags smoke ./cmd/g3n_smoke
package main

import (
	"fmt"
	"time"

	"github.com/g3n/engine/app"
	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/geometry"
	"github.com/g3n/engine/gls"
	"github.com/g3n/engine/graphic"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/light"
	"github.com/g3n/engine/material"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/renderer"
	"github.com/g3n/engine/util/helper"
	"github.com/g3n/engine/window"
)

func main() {
	// ---------------------------------------------------------------- 1. App
	// app.App() is a singleton constructor: first call creates the GLFW window
	// + GL context at the requested size and title; subsequent calls return
	// the same *Application. The struct embeds window.IWindow, so methods
	// like Subscribe, Gls, GetSize, GetFramebufferSize come "for free".
	a := app.App(1024, 768, "g3n smoke test")

	// ---------------------------------------------------- 2. Scene + camera
	scene := core.NewNode()

	// gui.Manager().Set(scene) wires the GUI layer (panels, buttons, etc.)
	// to our scene root. Even if you have no GUI widgets, this is required
	// for the renderer to walk the GUI subtree without panicking.
	gui.Manager().Set(scene)

	// camera.New(aspect) builds a perspective camera with default fov/near/far
	// (the alternative explicit form is camera.NewPerspective(aspect, near,
	// far, fov, axis)). The camera is itself a core.Node, so SetPosition is
	// inherited from Node.
	cam := camera.New(1)
	cam.SetPosition(3, 2.5, 5)
	cam.LookAt(&math32.Vector3{X: 0, Y: 0, Z: 0}, &math32.Vector3{X: 0, Y: 1, Z: 0})
	scene.Add(cam)

	// IMPORTANT: NewOrbitControl takes ONLY the camera in the current API.
	// Older blog posts show a (cam, win) signature; that is gone. The orbit
	// control hooks the singleton window itself for mouse events.
	camera.NewOrbitControl(cam)

	// Resize handler: keep GL viewport + camera aspect in sync with the
	// framebuffer. This is the exact pattern from engine/README.md.
	onResize := func(evname string, ev interface{}) {
		w, h := a.GetSize()
		a.Gls().Viewport(0, 0, int32(w), int32(h))
		cam.SetAspect(float32(w) / float32(h))
	}
	a.Subscribe(window.OnWindowSize, onResize)
	onResize("", nil)

	// ------------------------------------------------------------- 3. Lights
	// Ambient is just (color, intensity). Directional is (color, intensity)
	// and uses its WORLD POSITION as the direction vector (light direction
	// goes from the light's position toward the origin in shader space).
	scene.Add(light.NewAmbient(&math32.Color{R: 1, G: 1, B: 1}, 0.4))

	dir := light.NewDirectional(&math32.Color{R: 1, G: 1, B: 1}, 0.8)
	dir.SetPosition(5, 8, 5) // pos -> origin defines the light direction
	scene.Add(dir)

	// Helper: small RGB axis triad at the origin so orientation is obvious.
	scene.Add(helper.NewAxes(0.6))

	// ----------------------------------------- 4. Solid box (Standard mat)
	solidGeom := geometry.NewBox(1, 1, 1)
	solidMat := material.NewStandard(math32.NewColor("SteelBlue"))
	solidMat.SetSide(material.SideDouble) // both faces visible (overkill for a box, but demonstrates the API)
	solidMat.SetAmbientColor(&math32.Color{R: 0.10, G: 0.12, B: 0.18})
	solidMat.SetSpecularColor(&math32.Color{R: 1, G: 1, B: 1})
	solidMat.SetShininess(64)
	solidMesh := graphic.NewMesh(solidGeom, solidMat)
	solidMesh.SetPosition(-1.2, 0.5, 0)

	// ------------------------------------- 5. Translucent box (alpha blend)
	// SetTransparent(true) flips the material into the transparent draw
	// pass; SetOpacity is the alpha multiplier. You almost always want to
	// pair this with SetSide(SideDouble) on thin geometry so back faces
	// don't disappear.
	glassGeom := geometry.NewBox(1, 1, 1)
	glassMat := material.NewStandard(math32.NewColor("Gold"))
	glassMat.SetTransparent(true)
	glassMat.SetOpacity(0.25)
	glassMat.SetSide(material.SideDouble)
	glassMesh := graphic.NewMesh(glassGeom, glassMat)
	glassMesh.SetPosition(1.2, 0.5, 0)

	// ----------- 7. Parent Node grouping the two boxes (assembly pattern)
	// This is the equivalent of vtkAssembly: translating/rotating "group"
	// moves both children together. Useful for our future tool-actor model
	// where flute + shank + LED need to translate as one rigid body.
	group := core.NewNode()
	group.Add(solidMesh)
	group.Add(glassMesh)
	group.SetUserData(map[string]string{"role": "demo-assembly"}) // arbitrary data attach
	scene.Add(group)

	// ------------------------- 6. Multi-segment line strip w/ vertex colors
	//
	// Canonical g3nd pattern (demos/geometry/line_strip.go):
	//   - Build positions VBO (3 floats per vertex).
	//   - Build colors VBO (3 floats per vertex).
	//   - Add both to a fresh geometry.Geometry via AddVBO.
	//   - Wrap in graphic.NewLineStrip(geom, material.NewBasic()).
	//
	// material.NewBasic() is the ONLY built-in material that respects the
	// VertexColor attribute on the geometry. Standard / Physical ignore it
	// (their shaders sample the diffuse uniform instead). If you need lit
	// per-vertex colors you'd have to author a custom shader.
	pathGeom := geometry.NewGeometry()

	positions := math32.NewArrayF32(0, 0)
	colors := math32.NewArrayF32(0, 0)

	// Build a small 16-segment helix so we see colors interpolated between
	// adjacent vertices.
	const segs = 32
	for i := 0; i <= segs; i++ {
		t := float32(i) / float32(segs)
		angle := t * 2 * math32.Pi * 2 // two full turns
		x := 1.5 * math32.Cos(angle)
		z := 1.5 * math32.Sin(angle)
		y := 1.5*t - 0.25
		positions.Append(x, y, z)

		// rainbow gradient along the path
		r := t
		g := 1 - math32.Abs(2*t-1)
		b := 1 - t
		colors.Append(r, g, b)
	}

	pathGeom.AddVBO(gls.NewVBO(positions).AddAttrib(gls.VertexPosition))
	pathGeom.AddVBO(gls.NewVBO(colors).AddAttrib(gls.VertexColor))

	pathMat := material.NewBasic()
	pathStrip := graphic.NewLineStrip(pathGeom, pathMat)
	scene.Add(pathStrip)

	// (If you wanted disconnected segments instead of a strip, swap to
	// graphic.NewLines(pathGeom, pathMat). Same geometry/material API; only
	// the underlying GL primitive differs - LINES vs LINE_STRIP.)

	// ----------------------------------------- 8. Keyboard event handling
	// Subscribe via the embedded IWindow on the app; the event payload is
	// *window.KeyEvent with Key + Mods (a bitmask of ModShift / ModControl
	// / ModAlt / ModSuper).
	a.Subscribe(window.OnKeyDown, func(evname string, ev interface{}) {
		ke, ok := ev.(*window.KeyEvent)
		if !ok {
			return
		}
		ctrl := ke.Mods&window.ModControl != 0
		shift := ke.Mods&window.ModShift != 0
		alt := ke.Mods&window.ModAlt != 0
		fmt.Printf("key=%d  ctrl=%v shift=%v alt=%v\n", ke.Key, ctrl, shift, alt)

		// Ctrl+O -> would normally pop an Open File dialog.
		if ke.Key == window.KeyO && ctrl {
			fmt.Println(">>> Ctrl+O detected (open-file hook would fire here)")
		}
		// ESC = quit, handy during development.
		if ke.Key == window.KeyEscape {
			a.Exit()
		}
	})

	// Set the GL clear color. The IWindow embed exposes Gls() -> *gls.GLS,
	// and ClearColor takes RGBA in [0,1].
	a.Gls().ClearColor(0.10, 0.11, 0.13, 1.0)

	// --------------------------------------------------------- Main loop
	// The Run callback is invoked once per frame with the renderer and the
	// elapsed wall-clock since the previous frame. Standard pattern: clear
	// the GL buffers, then renderer.Render(scene, cam).
	start := time.Now()
	a.Run(func(rend *renderer.Renderer, deltaTime time.Duration) {
		_ = deltaTime
		// Animate the parent group to prove children inherit the transform.
		t := float32(time.Since(start).Seconds())
		group.SetPositionY(0.25 * math32.Sin(2*t))
		group.SetRotationY(t * 0.6)

		a.Gls().Clear(gls.DEPTH_BUFFER_BIT | gls.STENCIL_BUFFER_BIT | gls.COLOR_BUFFER_BIT)
		if err := rend.Render(scene, cam); err != nil {
			fmt.Println("render error:", err)
		}
	})
}
