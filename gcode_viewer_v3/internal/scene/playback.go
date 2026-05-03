package scene

import (
	"math"

	"gcodegen.local/viewer/internal/parser"
)

// Playback drives the simulated tool position through a parsed list of moves.
//
// Time model: each move has a Duration (real seconds at its feed rate). The
// Tick method advances by dt × SpeedMult playback-seconds; the position is
// interpolated linearly along the move's Points list using arc-length
// parameterization (so an arc-linearized curve plays at constant tip speed,
// not constant-vertex speed).
type Playback struct {
	moves     []*parser.Move
	totalTime float64

	// Public state — read by the UI for display, written via Reset/SetProgress
	// or as Tick advances them.
	MoveIndex int
	MoveT     float64 // [0, 1] within current move
	Running   bool
	SpeedMult float64

	// elapsedTime is the cumulative playback time up to (MoveIndex, MoveT),
	// in real seconds at 1× speed. Used for Progress() and seeking.
	elapsedTime float64

	// cumulative[i] = total Duration of moves[0..i-1] (so cumulative[0] = 0).
	// Lets SetProgress binary-seek without re-summing every call.
	cumulative []float64
}

// NewPlayback wraps the given moves and pre-computes the cumulative-time
// table used for seeking.
func NewPlayback(moves []*parser.Move) *Playback {
	p := &Playback{
		moves:     moves,
		SpeedMult: 1.0,
	}
	p.cumulative = make([]float64, len(moves)+1)
	for i, m := range moves {
		p.cumulative[i+1] = p.cumulative[i] + m.Duration
	}
	if n := len(moves); n > 0 {
		p.totalTime = p.cumulative[n]
	}
	return p
}

// TotalTime is the total real-time-at-1× duration of the loaded program.
func (p *Playback) TotalTime() float64 { return p.totalTime }

// Reset returns to the start of the program and pauses.
func (p *Playback) Reset() {
	p.MoveIndex = 0
	p.MoveT = 0
	p.elapsedTime = 0
	p.Running = false
}

// Progress returns the current playback position as a fraction in [0, 1].
func (p *Playback) Progress() float64 {
	if p.totalTime <= 0 {
		return 0
	}
	return p.elapsedTime / p.totalTime
}

// SetProgress seeks to a fractional position in [0, 1]. Clamped if out of range.
// Pauses playback (caller can Resume after if desired).
func (p *Playback) SetProgress(fraction float64) {
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	target := fraction * p.totalTime
	p.elapsedTime = target

	if len(p.moves) == 0 {
		return
	}

	// Binary search the cumulative table for the move containing `target`.
	lo, hi := 0, len(p.moves)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if p.cumulative[mid+1] <= target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	p.MoveIndex = lo
	d := p.moves[lo].Duration
	if d <= 0 {
		p.MoveT = 0
	} else {
		p.MoveT = (target - p.cumulative[lo]) / d
		if p.MoveT < 0 {
			p.MoveT = 0
		}
		if p.MoveT > 1 {
			p.MoveT = 1
		}
	}

	// If we seeked all the way to the end, settle on the very last point.
	if fraction >= 1 {
		p.MoveIndex = len(p.moves) - 1
		p.MoveT = 1
	}
}

// Done reports whether playback has reached the end of the program.
func (p *Playback) Done() bool {
	return p.elapsedTime >= p.totalTime
}

// Tick advances playback by realDeltaSec × SpeedMult. Returns the current
// tool position and whether the spindle is on at that moment.
func (p *Playback) Tick(realDeltaSec float64) (pos parser.Point, spindle bool) {
	if len(p.moves) == 0 {
		return parser.Point{}, false
	}
	if p.Running && !p.Done() {
		advance := realDeltaSec * p.SpeedMult
		for advance > 0 && p.MoveIndex < len(p.moves) {
			move := p.moves[p.MoveIndex]
			d := move.Duration
			if d <= 1e-9 {
				p.MoveIndex++
				p.MoveT = 0
				continue
			}
			timeLeftInMove := (1 - p.MoveT) * d
			if advance < timeLeftInMove {
				p.MoveT += advance / d
				p.elapsedTime += advance
				advance = 0
			} else {
				p.elapsedTime += timeLeftInMove
				advance -= timeLeftInMove
				p.MoveIndex++
				p.MoveT = 0
			}
		}
		if p.MoveIndex >= len(p.moves) {
			// Snap to the very last point and pause.
			p.MoveIndex = len(p.moves) - 1
			p.MoveT = 1
			p.elapsedTime = p.totalTime
			p.Running = false
		}
	}
	return p.currentPositionAndSpindle()
}

// CurrentPosition returns the interpolated tool position for the current
// (MoveIndex, MoveT) without advancing time. Used to refresh the tool
// after a programmatic seek.
func (p *Playback) CurrentPosition() parser.Point {
	pos, _ := p.currentPositionAndSpindle()
	return pos
}

func (p *Playback) currentPositionAndSpindle() (parser.Point, bool) {
	if p.MoveIndex >= len(p.moves) {
		if n := len(p.moves); n > 0 {
			last := p.moves[n-1]
			return last.Points[len(last.Points)-1], false
		}
		return parser.Point{}, false
	}
	m := p.moves[p.MoveIndex]
	return interpolateMove(m, p.MoveT), m.Spindle
}

// interpolateMove returns a point at parameter t in [0, 1] along the move's
// Points list, parameterized by arc length (so the tool moves at uniform
// tip speed across linearized arc segments).
func interpolateMove(m *parser.Move, t float64) parser.Point {
	pts := m.Points
	if len(pts) == 0 {
		return parser.Point{X: m.Sx, Y: m.Sy, Z: m.Sz}
	}
	if len(pts) == 1 || t <= 0 {
		return pts[0]
	}
	if t >= 1 {
		return pts[len(pts)-1]
	}

	// Cumulative segment lengths.
	total := 0.0
	for i := 1; i < len(pts); i++ {
		total += dist3(pts[i-1], pts[i])
	}
	if total <= 0 {
		return pts[0]
	}

	target := t * total
	walked := 0.0
	for i := 1; i < len(pts); i++ {
		segLen := dist3(pts[i-1], pts[i])
		if walked+segLen >= target {
			localT := (target - walked) / segLen
			a := pts[i-1]
			b := pts[i]
			return parser.Point{
				X: a.X + (b.X-a.X)*localT,
				Y: a.Y + (b.Y-a.Y)*localT,
				Z: a.Z + (b.Z-a.Z)*localT,
			}
		}
		walked += segLen
	}
	return pts[len(pts)-1]
}

func dist3(a, b parser.Point) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	dz := b.Z - a.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
