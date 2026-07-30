package holegen

import (
	"fmt"
	"math"
)

// RapidRate is the assumed G0 speed (mm/min) used ONLY by the run-time
// estimate. Real rapid speed is a machine setting that isn't encoded in the
// program, so it can't be derived from the G-code (spec §8).
const RapidRate = 3000.0

// EstimateRuntime returns the estimated total machining time in seconds.
//
// Cutting moves use the real feedrates and are exact; rapids use RapidRate.
// Deliberately rough — spindle spin-up beyond the 4 s dwell, accel/decel
// ramping and GRBL look-ahead are not modelled, so it tends to
// under-estimate slightly.
//
// Time for a move is distance_mm / feed_mm_per_min * 60.
func EstimateRuntime(p Params) float64 {
	cutRadius := (p.TargetHoleDiameter - p.BitDiameter) / 2
	totalDepth := p.TubeThickness + Breakthrough

	// Guard the divisors: a zero feedrate is legal input (§11 enforces no
	// lower bound) but would produce +Inf here.
	vf := math.Max(float64(p.VerticalFeedrate), 1)
	hf := math.Max(float64(p.HorizontalFeedrate), 1)
	pitch := math.Max(p.PitchPerTurn, 1e-6)

	seconds := 4.0 // G4 P4000 spin-up dwell
	prevX, prevY := 0.0, 0.0

	for _, h := range HoleCenters(p) {
		seconds += math.Hypot(h.X-prevX, h.Y-prevY) / RapidRate * 60 // rapid to hole XY
		seconds += 4.0 / RapidRate * 60                              // rapid Z5 → Z1

		if cutRadius > 0 {
			circ := 2 * math.Pi * cutRadius
			nTurns := math.Ceil(totalDepth / pitch)
			helixLen := nTurns * math.Hypot(circ, pitch)

			seconds += cutRadius / hf * 60 // feed out to the start edge
			seconds += helixLen / vf * 60  // helical descent
			seconds += circ / hf * 60      // flat spring pass
			seconds += cutRadius / hf * 60 // feed back to center
		} else {
			seconds += (1.0 + totalDepth) / vf * 60 // center plunge from Z+1
		}

		seconds += (5.0 + totalDepth) / RapidRate * 60 // rapid back up to Z5
		prevX, prevY = h.X, h.Y
	}

	seconds += math.Hypot(prevX, prevY) / RapidRate * 60 // final rapid home
	return seconds
}

// FormatDuration renders seconds as "45s", "3m 20s" or "1h 04m 09s"
// (spec §9).
func FormatDuration(seconds float64) string {
	total := int(math.Round(seconds))
	h := total / 3600
	rem := total % 3600
	m := rem / 60
	s := rem % 60

	switch {
	case h > 0:
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm %02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// Summary is the live one-line estimate shown while the user edits fields,
// e.g. "4 holes  |  Ø28.57mm  |  ~10m 14s" (spec §12).
func Summary(p Params) string {
	return fmt.Sprintf("%d holes  |  Ø%.2fmm  |  ~%s",
		p.RowCount*p.ColumnCount,
		p.TargetHoleDiameter,
		FormatDuration(EstimateRuntime(p)))
}
