package editor

import (
	"gcodegen.local/generator/internal/gen"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// handleInput updates the editor state based on the current frame's
// mouse + button state. Mirrors gcodegen.py's on_press / on_drag /
// on_release / on_motion (line 144-201) collapsed into one polled
// per-tick handler — Ebiten's input model is poll-only, no event
// callbacks.
//
// Coordinate sanitization: ebiten.CursorPosition() returns
// (math.MinInt, math.MinInt) when the cursor leaves the window. Adding
// such a point to a stroke and then asking vector.StrokeLine to render
// it pushes float32 math into NaN/Inf territory and can crash the GPU
// path. We clamp every coord to the canvas bounds before the model
// ever sees it.
func (g *Game) handleInput() {
	cx, cy := ebiten.CursorPosition()
	rawX, rawY := float64(cx), float64(cy)

	pressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	justPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	justReleased := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)

	// Off-window sentinel: skip everything except a justReleased
	// (which we still want to land so a drag-into-the-void cleans up
	// state instead of leaving Drag.What stuck).
	cursorValid := rawX > -1e6 && rawX < 1e6 && rawY > -1e6 && rawY < 1e6
	// inCanvas is keyed off the SCREEN-space viewport — the visible
	// 700x580 work-area rect. Independent of zoom: the viewport size
	// doesn't change when the user zooms in.
	inCanvas := cursorValid && rawX >= 0 && rawX < gen.CanvasWidth && rawY >= 0 && rawY < gen.CanvasHeight

	// Convert screen coords to canvas model coords through the zoom/
	// pan transform so the model never sees the zoom. Hit-tests,
	// strokes, perim drags etc. all operate on the unscaled coords.
	modelX := (rawX - g.panX) / g.zoom
	modelY := (rawY - g.panY) / g.zoom
	x := clamp(modelX, 0, gen.CanvasWidth-1)
	y := clamp(modelY, 0, gen.CanvasHeight-1)

	// Zoom: ONLY Ctrl + wheel over the canvas viewport. Plain wheel
	// can never zoom the canvas — guaranteed by requiring an explicit
	// modifier key. The right-side panel's scroll container handles
	// plain wheel on its own widgets; nothing else.
	ctrlHeld := ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
	if inCanvas && ctrlHeld {
		if _, wy := ebiten.Wheel(); wy != 0 {
			factor := 1.1
			if wy < 0 {
				factor = 1.0 / 1.1
			}
			g.zoomAroundCursor(factor)
		}
	}
	// Ctrl+0 resets the canvas view (zoom 1:1, no pan).
	if ctrlHeld && inpututil.IsKeyJustPressed(ebiten.Key0) {
		g.resetView()
	}

	switch {
	case justPressed && inCanvas:
		g.onPress(x, y)
	case justPressed && !inCanvas:
		// Press started in the panel — nothing on the canvas should
		// happen. Explicitly wipe any in-flight draw state so a stray
		// prior-frame DragDraw can't be revived by the next on-canvas
		// motion. ebitenui handles widget activation independently.
		if g.editor.Drag.What == gen.DragDraw || g.editor.Drawing {
			g.editor.Current = g.editor.Current[:0]
			g.editor.Drawing = false
			g.editor.Drag.What = gen.DragNone
		}
	case pressed && g.editor.Drag.What != gen.DragNone:
		// During a drag the cursor may briefly leave the window. For
		// origin/perim drags the clamp keeps the dragged thing pinned
		// to the canvas edge (fine). For freehand drawing, appending a
		// clamped point would leave a vertical streak along the canvas
		// border — so just stop adding points until the cursor returns.
		if g.editor.Drag.What != gen.DragDraw || inCanvas {
			g.onDrag(x, y)
		}
	case justReleased:
		g.onRelease()
	default:
		// Hover: only when no drag is active and pointer is on canvas.
		if g.editor.Drag.What == gen.DragNone && inCanvas {
			idx := g.editor.HitTestStroke(x, y)
			g.editor.HoverIdx = idx
		} else if !inCanvas {
			g.editor.HoverIdx = -1
		}
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// onPress mirrors gcodegen.py:154-167, with an added click-to-select
// step that runs after the perim/origin hit-tests but before starting
// a new freehand stroke.
func (g *Game) onPress(x, y float64) {
	e := g.editor
	if e.HitOrigin(x, y) {
		e.Drag = gen.DragState{What: gen.DragOrigin, Sx: x, Sy: y, OrigOrigin: e.Origin}
		return
	}
	if h := e.HitHandle(x, y); h != gen.DragNone {
		e.Drag = gen.DragState{What: h, Sx: x, Sy: y, OrigPerim: e.Perim}
		return
	}
	if e.HitEdge(x, y) {
		e.Drag = gen.DragState{What: gen.DragPerim, Sx: x, Sy: y, OrigPerim: e.Perim}
		return
	}
	// Click on an existing stroke selects it for editing — no new
	// stroke begins. Click on empty canvas clears any selection and
	// starts a freehand draw.
	if i := e.HitTestStroke(x, y); i >= 0 {
		e.SelectedIdx = i
		return
	}
	e.SelectedIdx = -1
	// Start a stroke.
	e.Drag = gen.DragState{What: gen.DragDraw, Sx: x, Sy: y}
	e.Drawing = true
	e.Current = e.Current[:0]
	e.AddPoint(gen.Point{X: x, Y: y})
}

// onDrag mirrors gcodegen.py:169-194.
func (g *Game) onDrag(x, y float64) {
	e := g.editor
	dx, dy := x-e.Drag.Sx, y-e.Drag.Sy
	switch e.Drag.What {
	case gen.DragOrigin:
		e.Origin.X = e.Drag.OrigOrigin.X + dx
		e.Origin.Y = e.Drag.OrigOrigin.Y + dy
	case gen.DragBL, gen.DragBR, gen.DragTL, gen.DragTR:
		o := e.Drag.OrigPerim
		w := e.Drag.What
		if w == gen.DragBL || w == gen.DragTL {
			e.Perim.X0 = o.X0 + dx
		}
		if w == gen.DragBR || w == gen.DragTR {
			e.Perim.X1 = o.X1 + dx
		}
		if w == gen.DragBL || w == gen.DragBR {
			e.Perim.Y0 = o.Y0 + dy
		}
		if w == gen.DragTL || w == gen.DragTR {
			e.Perim.Y1 = o.Y1 + dy
		}
	case gen.DragPerim:
		o := e.Drag.OrigPerim
		e.Perim.X0 = o.X0 + dx
		e.Perim.X1 = o.X1 + dx
		e.Perim.Y0 = o.Y0 + dy
		e.Perim.Y1 = o.Y1 + dy
	case gen.DragDraw:
		if e.Drawing {
			e.AddPoint(gen.Point{X: x, Y: y})
		}
	}
}

// onRelease mirrors gcodegen.py:196-201. A stroke is committed only
// when the release happens INSIDE the canvas viewport AND the path is
// long enough to look intentional (FinalizeStroke checks the latter).
// Releasing in the right-side panel — typical when a user is really
// clicking on the panel and the cursor grazed the canvas edge — drops
// the stroke fragment silently.
func (g *Game) onRelease() {
	e := g.editor
	if e.Drag.What == gen.DragDraw && e.Drawing && len(e.Current) > 1 {
		cx, cy := ebiten.CursorPosition()
		rx, ry := float64(cx), float64(cy)
		inCanvas := rx >= 0 && rx < gen.CanvasWidth && ry >= 0 && ry < gen.CanvasHeight
		if inCanvas {
			e.FinalizeStroke()
		}
	}
	e.Current = e.Current[:0]
	e.Drawing = false
	e.Drag.What = gen.DragNone
}
