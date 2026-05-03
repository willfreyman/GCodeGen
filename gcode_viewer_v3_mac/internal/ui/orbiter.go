package ui

import (
	"math"

	"github.com/g3n/engine/camera"
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/window"
)

// Orbiter is a Z-up orbit camera controller. We can't use g3n's built-in
// camera.OrbitControl because it's hard-coded to Y-up:
//
//	oc.up = *math32.NewVector3(0, 1, 0)
//
// and there's no public setter for that field. Every Rotate / Pan call inside
// OrbitControl ends with cam.LookAt(target, oc.up), so the camera silently
// gets re-oriented with Y as vertical after every input — which collides
// with our CNC-convention scene where Z is vertical.
//
// This orbiter uses (yaw, pitch, distance) about the target with a fixed
// world-Z up vector. Same control bindings as OrbitControl: left-drag
// rotates, right-drag pans, scroll zooms.
//
// Mouse events come through gui.Manager (NOT the raw window) so clicks on
// toolbar buttons / sliders don't also rotate the camera — gui.Manager only
// dispatches mouse events to external subscribers when no GUI panel is
// under the cursor. Cursor-move events go straight to the orbiter via its
// own Dispatcher once a drag starts (we grab cursor focus on mouse-down).
type Orbiter struct {
	core.Dispatcher

	cam    *camera.Camera
	win    *window.GlfwWindow
	target math32.Vector3

	yaw   float32 // azimuth around world +Z, radians
	pitch float32 // elevation from XY plane, radians
	dist  float32 // distance from target

	// Drag state
	rotating, panning bool
	lastX, lastY      float32

	// Tunable speeds
	RotSpeed  float32 // radians per pixel
	PanSpeed  float32 // mm per pixel per unit distance
	ZoomSpeed float32 // fraction per scroll notch
}

// NewOrbiter wires the orbiter to the given camera and window. Initial
// yaw/pitch/distance are derived from the camera's current position relative
// to the (initially zero) target — call SetTarget to point it at your model.
func NewOrbiter(cam *camera.Camera, win *window.GlfwWindow) *Orbiter {
	o := &Orbiter{
		cam:       cam,
		win:       win,
		RotSpeed:  0.005,
		PanSpeed:  0.0015,
		ZoomSpeed: 0.10,
	}
	o.Dispatcher.Initialize()
	o.SyncFromCamera()

	// Mouse-down/up/scroll go through gui.Manager so panel hits (toolbar
	// buttons, sliders) absorb the event and don't trigger camera input.
	gui.Manager().SubscribeID(window.OnMouseDown, &o, o.onMouseDown)
	gui.Manager().SubscribeID(window.OnMouseUp, &o, o.onMouseUp)
	gui.Manager().SubscribeID(window.OnScroll, &o, o.onScroll)

	// OnCursor we subscribe on ourselves; gui.Manager.SetCursorFocus(o) on
	// mouse-down routes cursor events here for the duration of the drag.
	o.SubscribeID(window.OnCursor, &o, o.onCursor)
	return o
}

// Target returns the current orbit center.
func (o *Orbiter) Target() math32.Vector3 { return o.target }

// SetTarget moves the orbit center and re-derives yaw/pitch/dist from the
// camera's current position so the view doesn't jump.
func (o *Orbiter) SetTarget(t math32.Vector3) {
	o.target = t
	o.SyncFromCamera()
}

// SyncFromCamera reads the camera's current position and recomputes
// yaw/pitch/distance so that subsequent orbits start from where the
// camera actually is. Call after any external SetPosition (e.g. a
// view-cube face snap).
func (o *Orbiter) SyncFromCamera() {
	p := o.cam.Position()
	dx := p.X - o.target.X
	dy := p.Y - o.target.Y
	dz := p.Z - o.target.Z
	o.dist = math32.Sqrt(dx*dx + dy*dy + dz*dz)
	if o.dist < 1e-6 {
		o.dist = 1
	}
	o.pitch = math32.Asin(dz / o.dist)
	o.yaw = math32.Atan2(dy, dx)
}

// applyToCamera writes the current (yaw, pitch, dist) state to the camera.
// Camera up is always world +Z.
func (o *Orbiter) applyToCamera() {
	cosP := math32.Cos(o.pitch)
	sinP := math32.Sin(o.pitch)
	cosY := math32.Cos(o.yaw)
	sinY := math32.Sin(o.yaw)
	o.cam.SetPosition(
		o.target.X+o.dist*cosP*cosY,
		o.target.Y+o.dist*cosP*sinY,
		o.target.Z+o.dist*sinP,
	)
	o.cam.LookAt(&o.target, &math32.Vector3{X: 0, Y: 0, Z: 1})
}

func (o *Orbiter) onMouseDown(_ string, ev interface{}) {
	me, ok := ev.(*window.MouseEvent)
	if !ok {
		return
	}
	o.lastX = me.Xpos
	o.lastY = me.Ypos
	switch me.Button {
	case window.MouseButtonLeft:
		o.rotating = true
	case window.MouseButtonRight, window.MouseButtonMiddle:
		o.panning = true
	default:
		return
	}
	// Capture cursor focus so onCursor fires here for the rest of the drag,
	// even when the cursor passes over a toolbar panel.
	gui.Manager().SetCursorFocus(o)
}

func (o *Orbiter) onMouseUp(_ string, ev interface{}) {
	me, ok := ev.(*window.MouseEvent)
	if !ok {
		return
	}
	switch me.Button {
	case window.MouseButtonLeft:
		o.rotating = false
	case window.MouseButtonRight, window.MouseButtonMiddle:
		o.panning = false
	}
	if !o.rotating && !o.panning {
		gui.Manager().SetCursorFocus(nil)
	}
}

func (o *Orbiter) onCursor(_ string, ev interface{}) {
	ce, ok := ev.(*window.CursorEvent)
	if !ok {
		return
	}
	dx := ce.Xpos - o.lastX
	dy := ce.Ypos - o.lastY
	o.lastX = ce.Xpos
	o.lastY = ce.Ypos

	switch {
	case o.rotating:
		// Drag-right = orbit camera CCW around +Z (model rotates left under
		// cursor, which feels natural). Drag-down = pitch camera up (look
		// from above). Both signs match common CAD/3D conventions.
		o.yaw -= dx * o.RotSpeed
		o.pitch += dy * o.RotSpeed
		const maxPitch = float32(math.Pi/2 - 0.01)
		if o.pitch > maxPitch {
			o.pitch = maxPitch
		}
		if o.pitch < -maxPitch {
			o.pitch = -maxPitch
		}
		o.applyToCamera()

	case o.panning:
		// Pan: shift target perpendicular to view direction.
		fwd := math32.Vector3{
			X: o.target.X - o.cam.Position().X,
			Y: o.target.Y - o.cam.Position().Y,
			Z: o.target.Z - o.cam.Position().Z,
		}
		normalize(&fwd)
		worldUp := math32.Vector3{X: 0, Y: 0, Z: 1}
		right := cross(fwd, worldUp)
		normalize(&right)
		up := cross(right, fwd)
		normalize(&up)

		s := o.dist * o.PanSpeed
		o.target.X += -right.X*dx*s + up.X*dy*s
		o.target.Y += -right.Y*dx*s + up.Y*dy*s
		o.target.Z += -right.Z*dx*s + up.Z*dy*s
		o.applyToCamera()
	}
}

func (o *Orbiter) onScroll(_ string, ev interface{}) {
	se, ok := ev.(*window.ScrollEvent)
	if !ok {
		return
	}
	factor := 1 - se.Yoffset*o.ZoomSpeed
	if factor < 0.1 {
		factor = 0.1
	}
	o.dist *= factor
	if o.dist < 1 {
		o.dist = 1
	}
	o.applyToCamera()
}

// ── small vec3 helpers ──

func normalize(v *math32.Vector3) {
	l := math32.Sqrt(v.X*v.X + v.Y*v.Y + v.Z*v.Z)
	if l > 1e-12 {
		v.X /= l
		v.Y /= l
		v.Z /= l
	}
}

func cross(a, b math32.Vector3) math32.Vector3 {
	return math32.Vector3{
		X: a.Y*b.Z - a.Z*b.Y,
		Y: a.Z*b.X - a.X*b.Z,
		Z: a.X*b.Y - a.Y*b.X,
	}
}
