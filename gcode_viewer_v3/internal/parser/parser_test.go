package parser

import (
	"math"
	"testing"
)

// Expected values computed by running the Python reference parser
// (gcode_viewer_v2/parser.py) against testdata/sample.nc. Any divergence
// means the Go port has drifted from the Python reference.

const eps = 1e-6

type expectedMove struct {
	Kind       string
	Sx, Sy, Sz float64
	Ex, Ey, Ez float64
	Feed       float64
	Spindle    bool
	Length     float64
	Duration   float64
	NPoints    int
}

var expectedMoves = []expectedMove{
	{"G0", 0, 0, 0, 0, 0, 5, 500, false, 5.000000, 0.120000, 2},
	{"G0", 0, 0, 5, 0, 0, 5, 500, true, 0.000000, 0.000000, 2},
	{"G0", 0, 0, 5, 0, 0, 1, 500, true, 4.000000, 0.096000, 2},
	{"G1", 0, 0, 1, 0, 0, -2, 300, true, 3.000000, 0.600000, 2},
	{"G1", 0, 0, -2, 10, 0, -2, 800, true, 10.000000, 0.750000, 2},
	{"G1", 10, 0, -2, 10, 10, -2, 800, true, 10.000000, 0.750000, 2},
	{"G1", 10, 10, -2, 0, 10, -2, 800, true, 10.000000, 0.750000, 2},
	{"G1", 0, 10, -2, 0, 0, -2, 800, true, 10.000000, 0.750000, 2},
	{"G2", 0, 0, -2, 10, 0, -2, 800, true, 15.682742, 1.176206, 17},
	{"G3", 10, 0, -2, 0, 0, -2, 800, true, 15.682742, 1.176206, 17},
	{"G0", 0, 0, -2, 0, 0, 5, 800, true, 7.000000, 0.168000, 2},
	{"G0", 0, 0, 5, 0, 0, 5, 800, true, 0.000000, 0.000000, 2},
}

func TestParseSampleFile(t *testing.T) {
	moves, err := ParseFile("testdata/sample.nc")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if got, want := len(moves), len(expectedMoves); got != want {
		t.Fatalf("move count: got %d, want %d", got, want)
	}
	for i, want := range expectedMoves {
		m := moves[i]
		if m.Kind != want.Kind {
			t.Errorf("move[%d].Kind = %q, want %q", i, m.Kind, want.Kind)
		}
		if m.Spindle != want.Spindle {
			t.Errorf("move[%d].Spindle = %v, want %v", i, m.Spindle, want.Spindle)
		}
		if !near(m.Feed, want.Feed) {
			t.Errorf("move[%d].Feed = %v, want %v", i, m.Feed, want.Feed)
		}
		if !near(m.Sx, want.Sx) || !near(m.Sy, want.Sy) || !near(m.Sz, want.Sz) {
			t.Errorf("move[%d] start = (%v,%v,%v), want (%v,%v,%v)",
				i, m.Sx, m.Sy, m.Sz, want.Sx, want.Sy, want.Sz)
		}
		if !near(m.Ex, want.Ex) || !near(m.Ey, want.Ey) || !near(m.Ez, want.Ez) {
			t.Errorf("move[%d] end = (%v,%v,%v), want (%v,%v,%v)",
				i, m.Ex, m.Ey, m.Ez, want.Ex, want.Ey, want.Ez)
		}
		if !near(m.Length, want.Length) {
			t.Errorf("move[%d].Length = %v, want %v", i, m.Length, want.Length)
		}
		if !near(m.Duration, want.Duration) {
			t.Errorf("move[%d].Duration = %v, want %v", i, m.Duration, want.Duration)
		}
		if len(m.Points) != want.NPoints {
			t.Errorf("move[%d] npoints = %d, want %d", i, len(m.Points), want.NPoints)
		}
	}
}

func TestBounds(t *testing.T) {
	moves, err := ParseFile("testdata/sample.nc")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	min, max, ok := Bounds(moves)
	if !ok {
		t.Fatal("Bounds returned ok=false on non-empty moves")
	}
	if !near(min.X, 0) || !near(min.Y, 0) || !near(min.Z, -2) {
		t.Errorf("min = %+v, want (0, 0, -2)", min)
	}
	if !near(max.X, 10) || !near(max.Y, 10) || !near(max.Z, 5) {
		t.Errorf("max = %+v, want (10, 10, 5)", max)
	}
}

func TestBoundsEmpty(t *testing.T) {
	if _, _, ok := Bounds(nil); ok {
		t.Error("Bounds(nil) returned ok=true, want false")
	}
}

func TestDeepestCutZ(t *testing.T) {
	moves, err := ParseFile("testdata/sample.nc")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if got := DeepestCutZ(moves); !near(got, -2.0) {
		t.Errorf("DeepestCutZ = %v, want -2.0", got)
	}
}

func TestDeepestCutZNoCuts(t *testing.T) {
	// Only rapids — should fall back to -1.0
	moves := Parse("G0 X10 Y10\nG0 Z5\n")
	if got := DeepestCutZ(moves); !near(got, -1.0) {
		t.Errorf("DeepestCutZ (no cuts) = %v, want -1.0 fallback", got)
	}
}

func TestComments(t *testing.T) {
	// Both paren and semicolon comments must be stripped.
	moves := Parse("(this is a header)\nG0 X1 ; trailing\nG1 X2 (mid line) Y3\n")
	if len(moves) != 2 {
		t.Fatalf("got %d moves, want 2", len(moves))
	}
	if !near(moves[1].Ex, 2) || !near(moves[1].Ey, 3) {
		t.Errorf("comment-stripped move end = (%v, %v), want (2, 3)", moves[1].Ex, moves[1].Ey)
	}
}

func TestRelativeMode(t *testing.T) {
	// G91 → axes are deltas, not absolutes
	moves := Parse("G90\nG0 X5 Y5\nG91\nG1 X3 Y2\nG1 X-1\n")
	if len(moves) != 3 {
		t.Fatalf("got %d moves, want 3", len(moves))
	}
	if !near(moves[1].Ex, 8) || !near(moves[1].Ey, 7) {
		t.Errorf("relative move 1 end = (%v, %v), want (8, 7)", moves[1].Ex, moves[1].Ey)
	}
	if !near(moves[2].Ex, 7) || !near(moves[2].Ey, 7) {
		t.Errorf("relative move 2 end = (%v, %v), want (7, 7)", moves[2].Ex, moves[2].Ey)
	}
}

func TestSpindleToggle(t *testing.T) {
	moves := Parse("G0 X1\nM3\nG1 X2\nM5\nG1 X3\n")
	if len(moves) != 3 {
		t.Fatalf("got %d moves, want 3", len(moves))
	}
	if moves[0].Spindle {
		t.Error("move[0] spindle should be off (before M3)")
	}
	if !moves[1].Spindle {
		t.Error("move[1] spindle should be on (after M3)")
	}
	if moves[2].Spindle {
		t.Error("move[2] spindle should be off (after M5)")
	}
}

func TestArcG2HalfCircle(t *testing.T) {
	// Half-circle of radius 5 about origin: from (5,0) to (-5,0), CW. Length ≈ π·5.
	pts := ArcPoints(5, 0, 0, -5, 0, 0, -5, 0, true)
	if len(pts) < 17 {
		t.Errorf("arc points = %d, want at least 17 (steps>=16)", len(pts))
	}
	// First and last must hit the endpoints exactly
	if !near(pts[0].X, 5) || !near(pts[0].Y, 0) {
		t.Errorf("arc[0] = %+v, want (5, 0)", pts[0])
	}
	last := pts[len(pts)-1]
	if !near(last.X, -5) || !near(last.Y, 0) {
		t.Errorf("arc[last] = %+v, want (-5, 0)", last)
	}
	// All intermediate points must lie on the radius-5 circle
	for i, p := range pts {
		if !near(math.Hypot(p.X, p.Y), 5) {
			t.Errorf("arc[%d] off circle: |p| = %v, want 5", i, math.Hypot(p.X, p.Y))
		}
	}
}

// Zero-padded G-words must resolve to their real motion mode. The old
// substring chain read "G01"/"G03" as "G0" because "G0" is a prefix of both,
// which turned every cut and every helical arc into a rapid.
func TestZeroPaddedMotionWords(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"G00 X1", "G0"},
		{"G01 X1", "G1"},
		{"G02 X1 Y0 I-1 J0", "G2"},
		{"G03 X1 Y0 I-1 J0", "G3"},
		{"G0 X1", "G0"},
		{"G1 X1", "G1"},
	}
	for _, c := range cases {
		moves := Parse(c.line + "\n")
		if len(moves) != 1 {
			t.Fatalf("%q: got %d moves, want 1", c.line, len(moves))
		}
		if moves[0].Kind != c.want {
			t.Errorf("%q: Kind = %q, want %q", c.line, moves[0].Kind, c.want)
		}
	}
}

// Non-motion G-words must not touch the sticky motion mode. "G21" contains
// "G2" and "G17" contains "G1", which used to leave the parser in arc / feed
// mode and mis-linearize the next coordinate line.
func TestNonMotionGWordsLeaveModeAlone(t *testing.T) {
	// After the preamble the mode is still the G0 default, so the bare
	// coordinate line must be a rapid — not an arc from the "G2" in "G21".
	moves := Parse("G90 G21 G17\nX10 Y10\n")
	if len(moves) != 1 {
		t.Fatalf("got %d moves, want 1", len(moves))
	}
	if moves[0].Kind != "G0" {
		t.Errorf("Kind = %q, want \"G0\" (G21/G17 must not set the motion mode)",
			moves[0].Kind)
	}
	if len(moves[0].Points) != 2 {
		t.Errorf("npoints = %d, want 2 (not arc-linearized)", len(moves[0].Points))
	}

	// G4 dwell, G54 work offset, G17 plane: none are motion words.
	for _, line := range []string{"G4 P4000", "G54", "G17", "G28.1", "G43 H1"} {
		if mode, ok := motionMode(line); ok {
			t.Errorf("motionMode(%q) = %q, want no match", line, mode)
		}
	}
}

// A full-circle arc (end XY == start XY, the I/J way to program one) must
// sweep a whole revolution instead of collapsing to a point.
func TestArcFullCircle(t *testing.T) {
	// Circle of radius 5 centered at the origin, starting at (5,0), CCW.
	pts := ArcPoints(5, 0, 0, 5, 0, 0, -5, 0, false)
	if len(pts) < 17 {
		t.Fatalf("full circle produced %d points, want >= 17", len(pts))
	}
	for i, p := range pts {
		if !near(math.Hypot(p.X, p.Y), 5) {
			t.Errorf("point[%d] off circle: |p| = %v, want 5", i, math.Hypot(p.X, p.Y))
		}
	}
	// Must close: last point back at the start.
	last := pts[len(pts)-1]
	if !near(last.X, 5) || !near(last.Y, 0) {
		t.Errorf("last = (%v, %v), want (5, 0)", last.X, last.Y)
	}
	// And must actually go around — the halfway point is diametrically opposite.
	mid := pts[len(pts)/2]
	if !near(mid.X, -5) || math.Abs(mid.Y) > 1e-6 {
		t.Errorf("midpoint = (%v, %v), want (-5, 0)", mid.X, mid.Y)
	}
	// Perimeter must be ~2πr, not ~0.
	total := 0.0
	for i := 1; i < len(pts); i++ {
		total += dist3(pts[i-1], pts[i])
	}
	if total < 0.99*2*math.Pi*5 || total > 2*math.Pi*5 {
		t.Errorf("perimeter = %v, want just under %v", total, 2*math.Pi*5)
	}
}

// A helical pass: full circle with a Z drop. Must descend monotonically while
// staying on the circle.
func TestArcFullCircleHelical(t *testing.T) {
	pts := ArcPoints(24.285, 0, 0, 24.285, 0, -1, -11.285, 0, false)
	if len(pts) < 17 {
		t.Fatalf("got %d points, want >= 17", len(pts))
	}
	if !near(pts[0].Z, 0) {
		t.Errorf("start Z = %v, want 0", pts[0].Z)
	}
	if !near(pts[len(pts)-1].Z, -1) {
		t.Errorf("end Z = %v, want -1", pts[len(pts)-1].Z)
	}
	for i := 1; i < len(pts); i++ {
		if pts[i].Z > pts[i-1].Z {
			t.Errorf("Z rose at point %d: %v > %v", i, pts[i].Z, pts[i-1].Z)
		}
	}
	// Centered at x=13 (24.285 - 11.285), radius 11.285.
	for i, p := range pts {
		if !near(math.Hypot(p.X-13.0, p.Y), 11.285) {
			t.Errorf("point[%d] off bore circle: r = %v, want 11.285",
				i, math.Hypot(p.X-13.0, p.Y))
		}
	}
}

// Clockwise full circles wind the other way.
func TestArcFullCircleClockwise(t *testing.T) {
	pts := ArcPoints(5, 0, 0, 5, 0, 0, -5, 0, true)
	if len(pts) < 17 {
		t.Fatalf("got %d points, want >= 17", len(pts))
	}
	// Second point should have negative Y going clockwise (CCW would be +Y).
	if pts[1].Y >= 0 {
		t.Errorf("clockwise circle turned the wrong way: point[1].Y = %v, want < 0", pts[1].Y)
	}
}

// A nearly-closed arc must still be treated as an arc, not snapped to a full
// circle by the epsilon.
func TestArcNearlyClosedIsNotFullCircle(t *testing.T) {
	// End 0.01 mm away from the start — a real 359.9° arc.
	pts := ArcPoints(5, 0, 0, 5, 0.01, 0, -5, 0, false)
	total := 0.0
	for i := 1; i < len(pts); i++ {
		total += dist3(pts[i-1], pts[i])
	}
	if total > 0.1 {
		t.Errorf("nearly-closed arc swept %v mm, want a short arc (epsilon must not snap it)", total)
	}
}

func near(a, b float64) bool {
	return math.Abs(a-b) < eps
}
