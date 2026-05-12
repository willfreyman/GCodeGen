package sim

import (
	"image/color"

	"gcodegen.local/generator/internal/gen"
)

// Op is the local rendering form of a stroke (or the perim) — same
// idea as gen.RenderOp but with all coords in pixel space and the
// color already RGBA. Constructed from incoming UpdateMessage.
type Op struct {
	Name  string
	Depth float64
	Pts   [][2]float64
	Color color.RGBA
}

// State is the simulator's complete world: the ops to play through,
// the perim outline, the origin, plus the playback head's position.
// One State is owned by the Game and mutated on the main loop.
type State struct {
	Ops    []Op
	PerimL, PerimT, PerimR, PerimB float64 // pixel rect (l/t/r/b)
	PerimCut bool
	HasPerim bool
	OriginX, OriginY float64

	// Playback head + cumulative-trail-tracking. Cuts already drawn are
	// kept in DrawnCuts (so we can replay onto a persistent image
	// without re-checking which segments are done). Rapids the same.
	Running bool
	Done    bool
	OpIdx   int
	PtIdx   int
	PtsDone int
	TotalPts int

	// HeadX/Y are the toolhead marker; HeadCutting tells the renderer
	// which color (cyan vs orange).
	HeadX, HeadY float64
	HeadCutting  bool
	HeadActive   bool

	Speed int
}

// loadFromMessage rebuilds the State's ops and perim from a fresh
// UpdateMessage. Returns true if anything actually changed (i.e.
// the caller should reset playback). If running, ops appended at the
// end won't reset; only structural changes (different stroke count or
// changed first-stroke length) trigger reset.
func (s *State) loadFromMessage(m gen.UpdateMessage) (changed bool) {
	// Build new op slice.
	newOps := make([]Op, 0, len(m.Strokes)+1)
	if m.Perim != nil && m.Perim.Cut {
		lx, rx := minF(m.Perim.X0, m.Perim.X1), maxF(m.Perim.X0, m.Perim.X1)
		by, ty := maxF(m.Perim.Y0, m.Perim.Y1), minF(m.Perim.Y0, m.Perim.Y1)
		newOps = append(newOps, Op{
			Name: "Perimeter",
			Pts: [][2]float64{
				{lx, by}, {rx, by}, {rx, ty}, {lx, ty}, {lx, by},
			},
			Color: color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff},
		})
	}
	for _, sw := range m.Strokes {
		newOps = append(newOps, Op{
			Name:  sw.Name,
			Depth: sw.Depth,
			Pts:   append([][2]float64(nil), sw.Points...),
			Color: parseHex(sw.Color),
		})
	}

	// Detect structural change: different op count, or any op's point
	// count differs.
	changed = len(newOps) != len(s.Ops)
	if !changed {
		for i := range newOps {
			if len(newOps[i].Pts) != len(s.Ops[i].Pts) {
				changed = true
				break
			}
		}
	}
	s.Ops = newOps
	if m.Perim != nil {
		s.HasPerim = true
		s.PerimL = minF(m.Perim.X0, m.Perim.X1)
		s.PerimR = maxF(m.Perim.X0, m.Perim.X1)
		s.PerimT = minF(m.Perim.Y0, m.Perim.Y1)
		s.PerimB = maxF(m.Perim.Y0, m.Perim.Y1)
		s.PerimCut = m.Perim.Cut
	}
	if m.Origin != nil {
		s.OriginX = m.Origin.X
		s.OriginY = m.Origin.Y
	}
	// Recompute total point count for progress.
	total := 0
	for _, op := range s.Ops {
		total += len(op.Pts)
	}
	s.TotalPts = total
	return changed
}

// reset rewinds playback (mirrors gcodegen.py:_reset).
func (s *State) reset() {
	s.Running = false
	s.Done = false
	s.OpIdx = 0
	s.PtIdx = 0
	s.PtsDone = 0
	s.HeadActive = false
}

// step advances playback by one logical tick — i.e. by Speed segments.
// Returns up to Speed (drawnCut, drawnRapid) events that the caller
// must paint onto the persistent trails image. Mirrors gcodegen.py:_step.
func (s *State) step() (cuts []segment, rapids []segment) {
	if !s.Running || s.Done || len(s.Ops) == 0 {
		return
	}
	n := s.Speed
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		if s.OpIdx >= len(s.Ops) {
			s.Running = false
			s.Done = true
			s.HeadActive = false
			return
		}
		op := s.Ops[s.OpIdx]
		pts := op.Pts
		if s.PtIdx == 0 {
			// Rapid travel from previous op end (or origin) to first
			// point of this op.
			var prevX, prevY float64
			if s.OpIdx > 0 {
				prev := s.Ops[s.OpIdx-1].Pts
				if len(prev) > 0 {
					last := prev[len(prev)-1]
					prevX, prevY = last[0], last[1]
				}
			} else {
				prevX, prevY = s.OriginX, s.OriginY
			}
			sx, sy := pts[0][0], pts[0][1]
			rapids = append(rapids, segment{prevX, prevY, sx, sy, color.RGBA{R: 0xff, G: 0x88, B: 0x00, A: 0xff}})
			s.HeadX, s.HeadY = sx, sy
			s.HeadCutting = false
			s.HeadActive = true
			s.PtIdx = 1
		} else if s.PtIdx < len(pts) {
			x0, y0 := pts[s.PtIdx-1][0], pts[s.PtIdx-1][1]
			x1, y1 := pts[s.PtIdx][0], pts[s.PtIdx][1]
			cuts = append(cuts, segment{x0, y0, x1, y1, op.Color})
			s.HeadX, s.HeadY = x1, y1
			s.HeadCutting = true
			s.HeadActive = true
			s.PtIdx++
			s.PtsDone++
		} else {
			s.OpIdx++
			s.PtIdx = 0
		}
	}
	return
}

// segment is one drawn line — used to ship per-tick events from the
// playback engine to the renderer.
type segment struct {
	X0, Y0, X1, Y1 float64
	Clr            color.RGBA
}

// progress returns 0-1 fraction of total points drawn.
func (s *State) progress() float64 {
	if s.TotalPts == 0 {
		return 0
	}
	return float64(s.PtsDone) / float64(s.TotalPts)
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

// parseHex turns "#rrggbb" into a color.RGBA. On parse failure
// returns a sentinel magenta so the renderer doesn't silently lose ops.
func parseHex(s string) color.RGBA {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{R: 0xff, G: 0x00, B: 0xff, A: 0xff}
	}
	r := hexByte(s[1])<<4 | hexByte(s[2])
	g := hexByte(s[3])<<4 | hexByte(s[4])
	b := hexByte(s[5])<<4 | hexByte(s[6])
	return color.RGBA{R: r, G: g, B: b, A: 0xff}
}
func hexByte(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}
