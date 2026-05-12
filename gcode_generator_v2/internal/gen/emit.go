package gen

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// isClosedPath reports whether the polyline's first and last points
// coincide (within a tight tolerance — points are round3'd through
// PxToMM so equality is effectively exact). Closed paths can plunge
// in place between step-down passes; open paths need to retract to
// safe Z and rapid back to the start so the bit doesn't drag through
// the half-cut material.
func isClosedPath(pts []Point) bool {
	if len(pts) < 3 {
		return false
	}
	first, last := pts[0], pts[len(pts)-1]
	const tol = 0.001
	return math.Abs(first.X-last.X) < tol && math.Abs(first.Y-last.Y) < tol
}

// Emit renders ops to a G-code text. Output is byte-identical to
// gcodegen.py's generate_gcode for the same inputs (line 380-405).
//
// Format rules to preserve:
//   - All numeric coords use "%.3f" (G0/G1 X/Y/Z lines).
//   - Feed rates and rpm use int() truncation.
//   - The header "z=<depth>mm" comment lines use Python's str(float)
//     (no trailing ".0" stripped), so 1.0 → "1.0", -1.5 → "-1.5".
//   - Lines are joined with "\n" and the list ends with an empty entry,
//     so the file ends with a trailing "\n".
func Emit(ops []Op, m Machine) string {
	var b strings.Builder

	b.WriteString("; G-code — Draw-to-Gcode\n")
	b.WriteString("\n")
	for _, op := range ops {
		fmt.Fprintf(&b, ";  %s  z=%smm\n", op.Name, pyFloat(op.Depth))
	}
	b.WriteString("\n")
	b.WriteString("G21\n")
	b.WriteString("G90\n")
	b.WriteString("G17\n")
	fmt.Fprintf(&b, "G0 Z%.3f\n", m.SafeZ)
	fmt.Fprintf(&b, "M3 S%d\n", int(m.RPM))
	b.WriteString("\n")

	for _, op := range ops {
		if len(op.Pts) < 2 {
			continue
		}
		fmt.Fprintf(&b, "; --- %s ---\n", op.Name)
		fmt.Fprintf(&b, "G0 Z%.3f\n", m.SafeZ)
		fmt.Fprintf(&b, "G0 X%.3f Y%.3f\n", op.Pts[0].X, op.Pts[0].Y)
		passes := stepDownPasses(op.Depth, m.StepDown)
		closed := isClosedPath(op.Pts)
		for i, z := range passes {
			fmt.Fprintf(&b, "G1 Z%.3f F%d\n", z, int(m.FeedZ))
			for _, p := range op.Pts[1:] {
				fmt.Fprintf(&b, "G1 X%.3f Y%.3f F%d\n", p.X, p.Y, int(m.FeedXY))
			}
			if i < len(passes)-1 && !closed {
				// Open path: the bit ended at op.Pts[last], not at the
				// start. Retract + rapid back to the start so the next
				// plunge happens at the right XY and doesn't drag the
				// bit through the half-cut material.
				fmt.Fprintf(&b, "G0 Z%.3f\n", m.SafeZ)
				fmt.Fprintf(&b, "G0 X%.3f Y%.3f\n", op.Pts[0].X, op.Pts[0].Y)
			}
			// Closed path: the bit is already at the start point after
			// completing the loop, so the next G1 Z just plunges in
			// place — no retract, no rapid, no wasted motion.
		}
		fmt.Fprintf(&b, "G0 Z%.3f\n", m.SafeZ)
		b.WriteString("\n")
	}

	b.WriteString("M5\n")
	b.WriteString("M30\n")
	// Trailing empty list entry in Python ("" at end of L) joined with
	// "\n" produces a final "\n" — the writes above already end the M30
	// line with "\n", and we don't add another trailing newline beyond
	// what the M30 line wrote.
	return b.String()
}

// stepDownPasses returns the sequence of Z depths to plunge to for an
// op with the given target depth (negative or zero) under a step-down
// of `step` (positive). Step=0 disables step-down — returns a single
// pass at target, identical to the original single-plunge behavior.
//
// For target=-19.05, step=2.0 the sequence is
// [-2, -4, -6, -8, -10, -12, -14, -16, -18, -19.05]; the final pass
// always lands exactly on target (not a multiple of step).
func stepDownPasses(target, step float64) []float64 {
	if step <= 0 || target >= 0 || -target <= step {
		return []float64{target}
	}
	var passes []float64
	for z := -step; z > target; z -= step {
		passes = append(passes, z)
	}
	passes = append(passes, target)
	return passes
}

// pyFloat formats a float64 the way Python's str() does for typical
// values: integer floats get a ".0" suffix, decimals are printed at
// shortest round-trip precision. For values without a fractional part,
// "1.0", "-1.0", "0.0". For decimals, "1.5", "-0.5", "1.25".
//
// Edge cases (very large/small numbers using scientific notation) match
// Go's strconv 'g' format closely enough for the editor's depth values
// (typically -10..10 mm); the golden test pins this for the common case.
func pyFloat(x float64) string {
	s := strconv.FormatFloat(x, 'g', -1, 64)
	// Python str(1.0) => "1.0"; Go's 'g' gives "1". Detect missing
	// decimal/exponent and append ".0".
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}
