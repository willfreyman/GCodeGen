// Package parser implements a G-code parser. Pure logic — no UI deps.
//
// Ported 1:1 from gcode_viewer_v2/parser.py. Handles modal G-code:
// G0/G1/G2/G3 motion modes, G90/G91 distance modes, M3/M5 spindle, and
// per-axis position carry-over (X/Y/Z hold their previous value when
// omitted). Arcs (G2/G3) are linearized via I/J center-offset form.
package parser

import (
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	SafeZ       = 0.0
	DefaultFeed = 500.0
	RapidFeed   = 2500.0
)

// Point is a single 3D coordinate.
type Point struct {
	X, Y, Z float64
}

// Move is one toolpath segment (or arc, expanded into multiple points).
type Move struct {
	Kind       string
	Sx, Sy, Sz float64
	Ex, Ey, Ez float64
	Feed       float64
	Spindle    bool
	Points     []Point
	Length     float64
	Duration   float64
}

func newMove(kind string, sx, sy, sz, ex, ey, ez, feed float64, spindle bool, points []Point) *Move {
	if feed <= 0 {
		feed = DefaultFeed
	}
	if points == nil {
		points = []Point{{sx, sy, sz}, {ex, ey, ez}}
	}
	m := &Move{
		Kind: kind, Sx: sx, Sy: sy, Sz: sz, Ex: ex, Ey: ey, Ez: ez,
		Feed: feed, Spindle: spindle, Points: points,
	}
	for i := 1; i < len(points); i++ {
		m.Length += dist3(points[i-1], points[i])
	}
	usedFeed := feed
	if kind == "G0" {
		usedFeed = RapidFeed
	}
	if usedFeed > 0 {
		m.Duration = m.Length / (usedFeed / 60.0)
	} else {
		m.Duration = 0.01
	}
	return m
}

func dist3(a, b Point) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	dz := b.Z - a.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

var (
	reComment = regexp.MustCompile(`\(.*?\)`)

	// reGWord matches every G-word on a line so motionMode can compare whole
	// numbers instead of substrings.
	reGWord = regexp.MustCompile(`G(\d+)`)

	reLetterVal = map[byte]*regexp.Regexp{
		'X': regexp.MustCompile(`X(-?\d+\.?\d*)`),
		'Y': regexp.MustCompile(`Y(-?\d+\.?\d*)`),
		'Z': regexp.MustCompile(`Z(-?\d+\.?\d*)`),
		'I': regexp.MustCompile(`I(-?\d+\.?\d*)`),
		'J': regexp.MustCompile(`J(-?\d+\.?\d*)`),
		'F': regexp.MustCompile(`F(-?\d+\.?\d*)`),
	}
)

func clean(line string) string {
	line = reComment.ReplaceAllString(line, "")
	if i := strings.Index(line, ";"); i >= 0 {
		line = line[:i]
	}
	return strings.ToUpper(strings.TrimSpace(line))
}

// motionMode returns the motion mode named by the first G-word on the line
// that is one of G0/G1/G2/G3, with leading zeros allowed (so "G01" is G1 and
// "G03" is G3). ok is false when the line names no motion word at all, in
// which case the caller leaves the sticky mode alone.
//
// This deliberately compares whole numbers rather than substrings. The
// obvious strings.Contains(line, "G0") test — which this port and the Python
// reference both used originally — misfires on any G-word that merely
// contains those two characters:
//
//	"G01 X10"     contains "G0" → a linear cut was read as a rapid
//	"G03 X.. I.." contains "G0" → a helical arc was read as a rapid, so it
//	              neither linearized nor removed any material
//	"G90 G21 G17" contains "G1" (inside G17) → left the parser in G1 mode
//	"G21"         contains "G2" → left the parser in ARC mode, so the next
//	              bare coordinate line would be linearized as an arc
//
// Bare-notation files never tripped the first two, which is why the old
// behaviour survived; HoleGen output hits all of them.
func motionMode(line string) (string, bool) {
	for _, m := range reGWord.FindAllStringSubmatch(line, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue // absurdly long digit run — not a motion word
		}
		if n <= 3 {
			return "G" + strconv.Itoa(n), true
		}
	}
	return "", false
}

// val extracts the numeric value following the given letter.
// Returns (value, true) if found, (0, false) otherwise.
func val(line string, letter byte) (float64, bool) {
	re := reLetterVal[letter]
	m := re.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// Parse parses a G-code program (string) into Move objects.
//
// Modal state tracked: position (x/y/z), feed, spindle, distance mode (G90
// abs / G91 rel), and current motion mode (G0/G1/G2/G3 — sticky across lines).
//
// Lines with no XYZ change are skipped — feed-only lines like "F500" update
// state but don't appear in the output.
func Parse(text string) []*Move {
	var x, y, z float64
	feed := float64(DefaultFeed)
	spindle := false
	absolute := true
	mode := "G0"

	var moves []*Move

	for _, raw := range strings.Split(text, "\n") {
		line := clean(raw)
		if line == "" {
			continue
		}

		if strings.Contains(line, "G90") {
			absolute = true
		}
		if strings.Contains(line, "G91") {
			absolute = false
		}

		if strings.Contains(line, "M3") || strings.Contains(line, "M03") {
			spindle = true
		}
		if strings.Contains(line, "M5") || strings.Contains(line, "M05") {
			spindle = false
		}

		// Motion mode is sticky: a line with coordinates but no G-word keeps
		// whatever mode was last set. gcode_viewer_v2/parser.py carries the
		// same whole-number matching.
		if mm, ok := motionMode(line); ok {
			mode = mm
		}

		if nf, ok := val(line, 'F'); ok {
			feed = nf
		}

		nx, hx := val(line, 'X')
		ny, hy := val(line, 'Y')
		nz, hz := val(line, 'Z')
		if !hx && !hy && !hz {
			continue
		}

		sx, sy, sz := x, y, z
		ex := x
		if hx {
			if absolute {
				ex = nx
			} else {
				ex = x + nx
			}
		}
		ey := y
		if hy {
			if absolute {
				ey = ny
			} else {
				ey = y + ny
			}
		}
		ez := z
		if hz {
			if absolute {
				ez = nz
			} else {
				ez = z + nz
			}
		}

		var points []Point
		if mode == "G2" || mode == "G3" {
			i, _ := val(line, 'I')
			j, _ := val(line, 'J')
			points = ArcPoints(sx, sy, sz, ex, ey, ez, i, j, mode == "G2")
		} else {
			points = []Point{{sx, sy, sz}, {ex, ey, ez}}
		}

		moves = append(moves, newMove(mode, sx, sy, sz, ex, ey, ez, feed, spindle, points))
		x, y, z = ex, ey, ez
	}

	return moves
}

// ParseFile reads a .nc / .gcode file from disk and parses it.
func ParseFile(path string) ([]*Move, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(string(data)), nil
}

// Bounds returns ((min_x, min_y, min_z), (max_x, max_y, max_z)) over all
// move points. Returns ok=false if the moves slice is empty.
func Bounds(moves []*Move) (min, max Point, ok bool) {
	if len(moves) == 0 {
		return Point{}, Point{}, false
	}
	min = moves[0].Points[0]
	max = min
	for _, m := range moves {
		for _, p := range m.Points {
			if p.X < min.X {
				min.X = p.X
			}
			if p.Y < min.Y {
				min.Y = p.Y
			}
			if p.Z < min.Z {
				min.Z = p.Z
			}
			if p.X > max.X {
				max.X = p.X
			}
			if p.Y > max.Y {
				max.Y = p.Y
			}
			if p.Z > max.Z {
				max.Z = p.Z
			}
		}
	}
	return min, max, true
}

// DeepestCutZ returns the most-negative Z reached during any cutting (non-G0)
// move, or -1.0 as a fallback if no cut goes below zero.
func DeepestCutZ(moves []*Move) float64 {
	deepest := 0.0
	for _, m := range moves {
		if m.Kind == "G0" {
			continue
		}
		for _, p := range m.Points {
			if p.Z < deepest {
				deepest = p.Z
			}
		}
	}
	if deepest >= 0 {
		return -1.0
	}
	return deepest
}
