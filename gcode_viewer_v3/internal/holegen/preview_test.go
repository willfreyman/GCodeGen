package holegen_test

// End-to-end check that what HoleGen emits is what the viewer's parser reads
// back. This is the contract that makes "generate, then preview in the 3D
// scene" work, and it is the reason internal/parser matches G-words by whole
// number instead of by substring: every helical pass HoleGen writes is a
// zero-padded "G03", and "G0" is a prefix of that.
//
// Lives in package holegen_test (not holegen) so the holegen package itself
// stays dependency-free — the spec requires the logic layer be pure stdlib.

import (
	"math"
	"testing"

	"gcodegen.local/viewer/internal/holegen"
	"gcodegen.local/viewer/internal/parser"
)

func defaults(t *testing.T) holegen.Params {
	t.Helper()
	p, err := holegen.ReadParams(holegen.DefaultValues())
	if err != nil {
		t.Fatalf("ReadParams: %v", err)
	}
	return p
}

func generate(t *testing.T, p holegen.Params) []*parser.Move {
	t.Helper()
	text, _, _, err := holegen.Program(p)
	if err != nil {
		t.Fatalf("Program: %v", err)
	}
	moves := parser.Parse(text)
	if len(moves) == 0 {
		t.Fatal("parser produced no moves from generated program")
	}
	return moves
}

// The helical passes must come back as G3 arcs — not rapids. If this fails the
// holes render as dashed blue travel lines and remove no material.
func TestGeneratedArcsParseAsArcs(t *testing.T) {
	moves := generate(t, defaults(t))

	var arcs, rapids, cuts int
	for _, m := range moves {
		switch m.Kind {
		case "G3":
			arcs++
		case "G0":
			rapids++
		case "G1":
			cuts++
		}
	}

	// 4 holes × (5 helical passes + 1 spring pass) = 24 arcs.
	if arcs != 24 {
		t.Errorf("G3 arcs = %d, want 24", arcs)
	}
	// 4 holes × (feed out + return to center) = 8 linear cuts.
	if cuts != 8 {
		t.Errorf("G1 cuts = %d, want 8", cuts)
	}
	// Startup retract + per hole (XY + Z1 + Z5) + final home.
	if rapids != 1+4*3+1 {
		t.Errorf("G0 rapids = %d, want %d", rapids, 1+4*3+1)
	}
}

// Every arc must linearize into a real circle, and must be flagged as a
// spindle-on cutting move so the material-removal heightmap erodes along it.
func TestGeneratedArcsAreCuttableCircles(t *testing.T) {
	p := defaults(t)
	p.RowCount, p.ColumnCount = 1, 1
	moves := generate(t, p)

	const wantCenterX = 13.0
	const wantRadius = 11.285

	seen := 0
	for _, m := range moves {
		if m.Kind != "G3" {
			continue
		}
		seen++
		if !m.Spindle {
			t.Error("arc has spindle off — heightmap would skip it")
		}
		if len(m.Points) < 17 {
			t.Errorf("arc linearized to %d points, want >= 17 (full circle)", len(m.Points))
		}
		for i, pt := range m.Points {
			r := math.Hypot(pt.X-wantCenterX, pt.Y)
			if math.Abs(r-wantRadius) > 1e-6 {
				t.Fatalf("arc point[%d] at radius %v, want %v", i, r, wantRadius)
			}
		}
		// A full circle is ~2πr of travel. A collapsed arc would be ~0.
		if m.Length < 0.99*2*math.Pi*wantRadius {
			t.Errorf("arc length = %v, want ≈%v (collapsed to a point?)",
				m.Length, 2*math.Pi*wantRadius)
		}
	}
	if seen != 6 {
		t.Errorf("saw %d arcs, want 6 (5 helical + 1 spring)", seen)
	}
}

// The deepest cut the viewer reports must be the breakthrough depth, which is
// what drives the depth-gradient colouring and the through-cut transparency.
func TestGeneratedProgramDepth(t *testing.T) {
	moves := generate(t, defaults(t))
	if got := parser.DeepestCutZ(moves); math.Abs(got-(-4.5)) > 1e-9 {
		t.Errorf("DeepestCutZ = %v, want -4.5", got)
	}
}

// Bounds must cover the whole hole grid including the bore radius, so the
// auto-framed camera and the stock block actually contain the holes.
//
// Tolerance is loose in Y because a linearized circle inscribes the true one:
// the arc's vertices land on the circle but not exactly on its topmost point,
// so the extent comes in a hair under the radius. X is exact because every
// arc starts at the +X extreme.
func TestGeneratedProgramBounds(t *testing.T) {
	moves := generate(t, defaults(t))
	min, max, ok := parser.Bounds(moves)
	if !ok {
		t.Fatal("Bounds returned ok=false")
	}
	const radius = 11.285
	const tol = 0.01 // linearization slack

	// Rightmost bore edge: last column center 63.8 + cut radius, hit exactly.
	if math.Abs(max.X-75.085) > 1e-6 {
		t.Errorf("max.X = %v, want 75.085", max.X)
	}
	// The program returns home to X0 Y0, and no bore reaches left of it
	// (first column's left edge is 13 - 11.285 = 1.715).
	if math.Abs(min.X) > 1e-9 {
		t.Errorf("min.X = %v, want 0", min.X)
	}
	// The Y=0 row is a full circle, so the stock must extend below Y=0.
	if math.Abs(min.Y-(-radius)) > tol {
		t.Errorf("min.Y = %v, want ≈%v (bore around the first row)", min.Y, -radius)
	}
	// Top row center plus the bore radius.
	if math.Abs(max.Y-(50.8+radius)) > tol {
		t.Errorf("max.Y = %v, want ≈%v", max.Y, 50.8+radius)
	}
	if math.Abs(min.Z-(-4.5)) > 1e-9 || math.Abs(max.Z-5.0) > 1e-9 {
		t.Errorf("Z range = [%v, %v], want [-4.5, 5]", min.Z, max.Z)
	}
}

// The center-plunge degenerate case must also read back as a cut.
func TestGeneratedPlungeParsesAsCut(t *testing.T) {
	p := defaults(t)
	p.RowCount, p.ColumnCount = 1, 1
	p.TargetHoleDiameter = p.BitDiameter
	moves := generate(t, p)

	var plunges int
	for _, m := range moves {
		if m.Kind == "G1" && m.Ez < -4.4 && m.Spindle {
			plunges++
		}
		if m.Kind == "G2" || m.Kind == "G3" {
			t.Error("plunge program must contain no arcs")
		}
	}
	if plunges != 1 {
		t.Errorf("got %d plunge cuts, want 1", plunges)
	}
}
