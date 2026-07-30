package holegen

import (
	"errors"
	"fmt"
	"strings"
)

// Breakthrough is how far past the material the tool cuts so the hole is
// fully open (spec §7.1: totalDepth = tubeThickness + 1.5).
const Breakthrough = 1.5

// Hole is one hole center in cut order, carrying its 0-based grid indices
// (the emitted comment prints them 1-based).
type Hole struct {
	Col, Row int
	X, Y     float64
}

// HoleCenters returns every hole center in cut order (spec §6).
//
// Ordering is snake / boustrophedon: column-by-column with alternate columns
// running the opposite way in Y, so each column's last hole is at the same Y
// as the next column's first hole and the travel between columns is pure X.
//
// Column X includes the bit radius because X=0 is the endmill EDGE touching
// the tube face, not the tube face itself (spec §2).
//
// Shared by the generator AND the estimator so both see identical geometry
// in an identical order.
func HoleCenters(p Params) []Hole {
	holes := make([]Hole, 0, p.ColumnCount*p.RowCount)
	for i := 0; i < p.ColumnCount; i++ {
		for o := 0; o < p.RowCount; o++ {
			cx := p.BitDiameter/2.0 + p.XOffset + p.HoleSpacingX*float64(i)
			var cy float64
			if i%2 == 0 {
				cy = p.HoleSpacingY * float64(o)
			} else {
				cy = p.HoleSpacingY * float64(p.RowCount-o-1)
			}
			holes = append(holes, Hole{Col: i, Row: o, X: cx, Y: cy})
		}
	}
	return holes
}

// ErrBitTooLarge is returned when the bit can't fit the requested hole.
// The message is user-facing verbatim (spec §7.2).
var ErrBitTooLarge = errors.New("Your bit diameter cannot be larger than your target hole diameter!")

// ErrPitchNotPositive guards the helical descent loop.
//
// DELIBERATE DEVIATION from the spec: §11 lists only two validation rules and
// puts no lower bound on the helical pitch, but §7.4's descent loop subtracts
// the pitch from currentZ until it passes -totalDepth. A pitch of zero never
// decreases it and a negative pitch increases it, so either one loops forever
// appending G03 lines until the process is killed. The spec is internally
// inconsistent here — §8's estimator DOES clamp the pitch away from zero —
// and a hang is worse than a rejected input, so this is enforced.
var ErrPitchNotPositive = errors.New("Helical pitch must be greater than zero.")

// MaxHoles caps the generated grid.
//
// Also a deliberate deviation (§11 sets no upper bounds). rowCount and
// columnCount are free-typed integers and the grid allocates one Hole per
// cell plus ~10 G-code lines per hole, so a fat-fingered "999999" asks for
// 10^12 holes and dies allocating. 100k is far past any real job — a
// hobbyist tube panel is tens of holes — while still refusing only input
// that could not have been meant.
const MaxHoles = 100_000

// GenerateGCode emits the complete program (spec §7).
//
// Every returned line already ends in "\n"; the file is the plain
// concatenation of them, so callers write strings.Join(lines, "") — see
// Program.
//
// Each hole is bored helically: feed out to the +X edge of the finished
// circle, then full-circle CCW G03 arcs each descending PitchPerTurn, the
// last clamped exactly to -totalDepth, then a flat spring pass at depth
// (no Z word) to clean the floor, then back to center before lifting so the
// rim isn't gouged. When the hole diameter equals the bit diameter there is
// no circle to cut, so that degenerates to a straight center plunge.
func GenerateGCode(p Params) (lines []string, cutRadius, totalDepth float64, err error) {
	cutRadius = (p.TargetHoleDiameter - p.BitDiameter) / 2
	totalDepth = p.TubeThickness + Breakthrough

	if cutRadius < 0 {
		return nil, 0, 0, ErrBitTooLarge
	}
	// Both guards are repeated from ReadParams on purpose: this is an exported
	// entry point that can be handed a hand-built Params, and the failure modes
	// are a hang and an out-of-memory kill rather than bad output.
	if cutRadius > 0 && p.PitchPerTurn <= 0 {
		return nil, 0, 0, ErrPitchNotPositive
	}
	if err := checkHoleCount(p); err != nil {
		return nil, 0, 0, err
	}

	// Startup / safety block. The Z retract MUST precede the spindle-on so
	// the tool is never commanded to spin while still down in the material.
	// Do not reorder (spec §7.3).
	lines = append(lines,
		"G90 G21 G17\n",
		"G0 Z5.0000\n",
		fmt.Sprintf("M03 S%d\n", p.SpindleSpeed),
		"G4 P4000\n",
	)

	for _, h := range HoleCenters(p) {
		lines = append(lines,
			fmt.Sprintf("\n( --- HOLE LOCATION: Col %d, Row %d --- )\n", h.Col+1, h.Row+1),
			fmt.Sprintf("G0 X%.4f Y%.4f\n", h.X, h.Y),
			"G0 Z1.0000\n",
		)

		if cutRadius > 0 {
			// Arc start sits one cut-radius out in +X; the arc center is the
			// hole center, so I = -cutRadius and J = 0. Start and end XY are
			// identical, which makes every G03 a full 360° circle.
			startArcX := h.X + cutRadius
			lines = append(lines,
				fmt.Sprintf("G1 X%.4f F%d\n", startArcX, p.HorizontalFeedrate))

			// Helical descent. The first turn drops from Z+1.0 (where the
			// G0 Z1.0000 left us) to Z-1.0000 — a 2 mm effective descent
			// whose upper 1 mm is air. Preserved deliberately.
			//
			// float64 accumulation exactly as the reference does it, so the
			// emitted Z values match bit-for-bit.
			currentZ := 0.0
			for currentZ > -totalDepth {
				currentZ -= p.PitchPerTurn
				if currentZ < -totalDepth {
					currentZ = -totalDepth
				}
				lines = append(lines, fmt.Sprintf(
					"G03 X%.4f Y%.4f Z%.4f I%.4f J0.0000 F%d\n",
					startArcX, h.Y, currentZ, -cutRadius, p.VerticalFeedrate))
			}

			// Flat spring pass at final depth — no Z word.
			lines = append(lines, fmt.Sprintf(
				"G03 X%.4f Y%.4f I%.4f J0.0000 F%d\n",
				startArcX, h.Y, -cutRadius, p.HorizontalFeedrate))

			// Back to center before the lift.
			lines = append(lines, fmt.Sprintf("G1 X%.4f\n", h.X))
		} else {
			lines = append(lines, fmt.Sprintf("G1 Z%.4f F%d\n", -totalDepth, p.VerticalFeedrate))
		}

		lines = append(lines, "G0 Z5.0000\n")
	}

	lines = append(lines,
		"\n( --- END OF PROGRAM --- )\n",
		"M05\n",
		"G0 X0 Y0\n",
		"M30\n",
	)
	return lines, cutRadius, totalDepth, nil
}

// checkHoleCount rejects a grid too large to generate.
//
// The single-dimension tests come first so the multiplication below can't
// overflow: past them both counts are <= MaxHoles, so the product fits
// comfortably in an int64.
func checkHoleCount(p Params) error {
	if p.RowCount < 1 || p.ColumnCount < 1 {
		return nil // ReadParams owns the lower bound; nothing gets allocated
	}
	if p.RowCount > MaxHoles || p.ColumnCount > MaxHoles ||
		int64(p.RowCount)*int64(p.ColumnCount) > MaxHoles {
		return fmt.Errorf(
			"That grid is %d × %d holes — more than the %d this tool will generate. Reduce the row or column count.",
			p.ColumnCount, p.RowCount, MaxHoles)
	}
	return nil
}

// Program is GenerateGCode collapsed into the exact file text.
func Program(p Params) (text string, cutRadius, totalDepth float64, err error) {
	lines, cutRadius, totalDepth, err := GenerateGCode(p)
	if err != nil {
		return "", 0, 0, err
	}
	return strings.Join(lines, ""), cutRadius, totalDepth, nil
}
