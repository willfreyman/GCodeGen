package gen

import (
	"math"
	"testing"
)

// TestPxToMM_Origin: a point on the origin maps to (0, 0).
func TestPxToMM_Origin(t *testing.T) {
	e := NewEditor()
	x, y := e.PxToMM(e.Origin.X, e.Origin.Y)
	if x != 0 || y != 0 {
		t.Errorf("origin px should map to (0, 0); got (%v, %v)", x, y)
	}
}

// TestPxToMM_BottomLeftCorner: a point at the perim's bottom-left
// (the same coords as the default origin) should give (0, 0).
func TestPxToMM_BottomLeftCorner(t *testing.T) {
	e := NewEditor()
	x, y := e.PxToMM(e.Perim.X0, e.Perim.Y0)
	if x != 0 || y != 0 {
		t.Errorf("BL corner of perim should be (0, 0); got (%v, %v)", x, y)
	}
}

// TestPxToMM_TopRightCorner: TR corner of default perim should map
// to (WidthMM, HeightMM) = (50, 50).
func TestPxToMM_TopRightCorner(t *testing.T) {
	e := NewEditor()
	x, y := e.PxToMM(e.Perim.X1, e.Perim.Y1)
	if math.Abs(x-50) > 1e-9 || math.Abs(y-50) > 1e-9 {
		t.Errorf("TR corner should be (50, 50); got (%v, %v)", x, y)
	}
}

// TestPxToMM_RoundTrip: MMToPx ∘ PxToMM should be approximately
// identity for points inside the perim.
func TestPxToMM_RoundTrip(t *testing.T) {
	e := NewEditor()
	cases := []struct{ px, py float64 }{
		{200, 400}, {300, 300}, {500, 200}, {100.5, 480.0},
	}
	for _, c := range cases {
		xmm, ymm := e.PxToMM(c.px, c.py)
		bx, by := e.MMToPx(xmm, ymm)
		// MMToPx truncates to integer, so allow ±1 px slack.
		if math.Abs(bx-c.px) > 1.0 || math.Abs(by-c.py) > 1.0 {
			t.Errorf("round-trip (%v, %v) → (%v, %v) → (%v, %v); slack >1 px",
				c.px, c.py, xmm, ymm, bx, by)
		}
	}
}

func TestPxToMM_DegeneratePerim(t *testing.T) {
	e := NewEditor()
	e.Perim.X1 = e.Perim.X0 // zero width
	if x, y := e.PxToMM(200, 200); x != 0 || y != 0 {
		t.Errorf("degenerate perim should yield (0, 0); got (%v, %v)", x, y)
	}
}

func TestHitOrigin(t *testing.T) {
	e := NewEditor()
	if !e.HitOrigin(e.Origin.X+5, e.Origin.Y+5) {
		t.Error("point near origin should hit")
	}
	if e.HitOrigin(e.Origin.X+50, e.Origin.Y) {
		t.Error("far point should not hit origin")
	}
}

func TestHitHandle(t *testing.T) {
	e := NewEditor()
	if got := e.HitHandle(e.Perim.X0, e.Perim.Y0); got != DragBL {
		t.Errorf("BL corner: got %v want DragBL", got)
	}
	if got := e.HitHandle(e.Perim.X1, e.Perim.Y1); got != DragTR {
		t.Errorf("TR corner: got %v want DragTR", got)
	}
	if got := e.HitHandle(0, 0); got != DragNone {
		t.Errorf("(0,0): got %v want DragNone", got)
	}
}

func TestHitTestStroke(t *testing.T) {
	e := NewEditor()
	e.Strokes = []Stroke{
		{Points: []Point{{X: 100, Y: 100}, {X: 200, Y: 100}}, Name: "A"},
		{Points: []Point{{X: 300, Y: 300}, {X: 300, Y: 400}}, Name: "B"},
	}
	if i := e.HitTestStroke(150, 102); i != 0 {
		t.Errorf("on stroke A: got %d want 0", i)
	}
	if i := e.HitTestStroke(300, 350); i != 1 {
		t.Errorf("on stroke B: got %d want 1", i)
	}
	if i := e.HitTestStroke(500, 500); i != -1 {
		t.Errorf("far point: got %d want -1", i)
	}
}

func TestPtSegDist(t *testing.T) {
	cases := []struct {
		px, py, x0, y0, x1, y1 float64
		want                   float64
	}{
		{0, 0, 0, 0, 10, 0, 0},      // on endpoint
		{5, 0, 0, 0, 10, 0, 0},      // on segment
		{5, 3, 0, 0, 10, 0, 3},      // perpendicular distance
		{-5, 0, 0, 0, 10, 0, 5},     // beyond start endpoint
		{15, 0, 0, 0, 10, 0, 5},     // beyond end endpoint
		{0, 0, 5, 5, 5, 5, 7.0710678}, // degenerate segment (point)
	}
	for _, c := range cases {
		got := ptSegDist(c.px, c.py, c.x0, c.y0, c.x1, c.y1)
		if math.Abs(got-c.want) > 1e-6 {
			t.Errorf("ptSegDist(%v): got %v want %v", c, got, c.want)
		}
	}
}
