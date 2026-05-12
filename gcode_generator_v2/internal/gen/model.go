// Package gen is the pure data model + G-code emitter for the editor.
// It has no UI dependencies so it can be exercised in unit tests and
// reused unchanged from the editor and any subprocess that needs to
// understand strokes (e.g., the simulation playback).
//
// The semantics mirror gcodegen.py exactly: the canvas is in pixel
// coordinates with Tk's y-down convention, the perimeter is a rectangle
// whose y0 corresponds to the bottom edge (perim["y0"] > perim["y1"]),
// and the user-draggable origin defines the (0, 0) point in machine mm.
package gen

import (
	"image/color"
	"math"
)

// CanvasWidth and CanvasHeight match gcodegen.py's CANVAS_W / CANVAS_H.
const (
	CanvasWidth  = 700
	CanvasHeight = 580
)

// Default machine settings (gcodegen.py:8-11).
const (
	DefaultSafeZ    = 5.0
	DefaultFeedXY   = 300.0
	DefaultFeedZ    = 100.0
	DefaultRPM      = 12000.0
	DefaultStepDown = 2.0 // mm — non-zero by default so a user who never
	// touches the Step Down field still gets multi-pass cutting on deep
	// ops instead of a single suicidal plunge.
	DefaultPerimMM = 50.0
)

// Default initial geometry (gcodegen.py:22-23).
var (
	DefaultPerim = Perim{
		X0: 100, Y0: 480, X1: 580, Y1: 80,
		WidthMM: DefaultPerimMM, HeightMM: DefaultPerimMM, DepthMM: -1.0,
	}
	DefaultOrigin = Origin{X: 100, Y: 480}
)

// Point is a 2D pixel coordinate using Tk's y-down convention.
type Point struct{ X, Y float64 }

// Stroke is one user-drawn cut with its name, target Z depth (mm), and
// display color. Points are in canvas pixel space.
type Stroke struct {
	Points []Point
	Name   string
	Depth  float64
	Color  color.RGBA
}

// Perim is the rectangular work area, defined in canvas pixels with
// y0 = bottom (numerically larger because Tk y is inverted), and the
// real-world dimensions in mm. Cut means the perimeter itself becomes
// an op when emitting G-code.
type Perim struct {
	X0, Y0, X1, Y1               float64
	WidthMM, HeightMM, DepthMM   float64
	Cut                          bool
}

// Origin is the user-draggable orange dot — the (0, 0) point in machine mm.
type Origin struct{ X, Y float64 }

// DragWhat enumerates what the user is currently dragging.
type DragWhat int

const (
	DragNone DragWhat = iota
	DragDraw
	DragOrigin
	DragPerim
	DragBL
	DragBR
	DragTL
	DragTR
)

// DragState captures enough info to undo a drag if needed and to compute
// deltas relative to where the drag started.
type DragState struct {
	What       DragWhat
	Sx, Sy     float64
	OrigOrigin Origin
	OrigPerim  Perim
}

// Machine holds the numeric settings that feed G-code emission. StepDown
// is the per-pass plunge depth in mm — 0 disables step-down (the cutter
// plunges to full depth in one pass, matching the original behavior).
// A positive value (e.g. 2.0) emits multiple cut passes per op, each
// going one StepDown deeper than the last, until the op's full depth
// is reached.
type Machine struct {
	SafeZ, FeedXY, FeedZ, RPM, StepDown float64
}

// Editor is the full mutable state of the editor window. UI code calls
// methods on it; UI never mutates fields directly. The methods here are
// pure (no I/O) so they can be unit-tested headlessly.
type Editor struct {
	Strokes     []Stroke
	Current     []Point // in-progress stroke (raw pixel points)
	Drawing     bool
	HoverIdx    int // -1 = none, matches Python's None
	SelectedIdx int // -1 = none; persistent click-selection for editing
	Perim       Perim
	Origin      Origin
	Drag        DragState
	Machine     Machine
	NewOpName   string
	NewOpDepth  float64
	ColorIdx    int
}

// NewEditor returns an Editor with the same initial state as gcodegen.py
// at startup.
func NewEditor() *Editor {
	return &Editor{
		HoverIdx:    -1,
		SelectedIdx: -1,
		Perim:       DefaultPerim,
		Origin:      DefaultOrigin,
		Machine: Machine{
			SafeZ:    DefaultSafeZ,
			FeedXY:   DefaultFeedXY,
			FeedZ:    DefaultFeedZ,
			RPM:      DefaultRPM,
			StepDown: DefaultStepDown,
		},
		NewOpDepth:  -1.0,
	}
}

// CurrentColor returns the next palette color the editor will assign to
// a finalized stroke.
func (e *Editor) CurrentColor() color.RGBA {
	return Palette[e.ColorIdx%len(Palette)]
}

// AddPoint appends a point to the in-progress stroke.
func (e *Editor) AddPoint(p Point) { e.Current = append(e.Current, p) }

// MinStrokeLengthPx is the minimum total pixel length a freehand
// stroke must have to be committed. Shorter "strokes" are almost
// always misclicks — a tap with a one-frame drift, or the cursor
// grazing the canvas edge for a frame on its way into the right-side
// panel. Without this filter those zero-length artifacts get saved as
// strokes and the user sees mysterious cuts appear from clicks they
// thought happened on the panel.
const MinStrokeLengthPx = 5.0

// FinalizeStroke commits the in-progress stroke into Strokes with the
// current color, default name, and current new-op depth, then rotates
// the palette for the next stroke. Returns false if there were too few
// points OR the total path is shorter than MinStrokeLengthPx — both of
// which signal a misclick rather than an intentional cut.
func (e *Editor) FinalizeStroke() bool {
	if len(e.Current) < 2 {
		e.Current = nil
		return false
	}
	if pathLengthPx(e.Current) < MinStrokeLengthPx {
		e.Current = nil
		return false
	}
	name := e.NewOpName
	if name == "" {
		name = defaultStrokeName(len(e.Strokes) + 1)
	}
	s := Stroke{
		Points: append([]Point(nil), e.Current...),
		Name:   name,
		Depth:  e.NewOpDepth,
		Color:  e.CurrentColor(),
	}
	e.Strokes = append(e.Strokes, s)
	e.ColorIdx = (e.ColorIdx + 1) % len(Palette)
	e.Current = nil
	return true
}

// DeleteStroke removes the i-th stroke (no-op if out of range).
func (e *Editor) DeleteStroke(i int) {
	if i < 0 || i >= len(e.Strokes) {
		return
	}
	e.Strokes = append(e.Strokes[:i], e.Strokes[i+1:]...)
	if e.HoverIdx == i {
		e.HoverIdx = -1
	} else if e.HoverIdx > i {
		e.HoverIdx--
	}
	if e.SelectedIdx == i {
		e.SelectedIdx = -1
	} else if e.SelectedIdx > i {
		e.SelectedIdx--
	}
}

// ClearAll wipes all strokes.
func (e *Editor) ClearAll() { e.Strokes = nil; e.HoverIdx = -1; e.SelectedIdx = -1 }

// SnapOrigin moves the origin to the bottom-left corner of the perimeter
// (gcodegen.py:220-224).
func (e *Editor) SnapOrigin() {
	e.Origin.X = minF(e.Perim.X0, e.Perim.X1)
	e.Origin.Y = maxF(e.Perim.Y0, e.Perim.Y1)
}

// SetDepth updates the i-th stroke's depth (no-op if out of range).
// Mirrors gcodegen.py:_edit_depth which rounds to 3 decimals.
func (e *Editor) SetDepth(i int, depth float64) {
	if i < 0 || i >= len(e.Strokes) {
		return
	}
	e.Strokes[i].Depth = round3(depth)
}

func defaultStrokeName(n int) string { return "Cut " + itoa(n) }

// pathLengthPx returns the total path length of a polyline in pixels.
func pathLengthPx(pts []Point) float64 {
	if len(pts) < 2 {
		return 0
	}
	var total float64
	for i := 1; i < len(pts); i++ {
		total += math.Hypot(pts[i].X-pts[i-1].X, pts[i].Y-pts[i-1].Y)
	}
	return total
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
