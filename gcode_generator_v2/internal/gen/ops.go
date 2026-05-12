package gen

import (
	"image/color"
	"sort"
)

// orderOpsByDepth sorts ops so the shallowest cut runs first and the
// deepest runs last. Stable so same-depth ops retain their relative
// drawing order. Reason for the order: the deepest cut on a part is
// often the through-cut that releases the workpiece from the stock —
// running it last means earlier ops stay aligned in the (still-rigid)
// stock instead of cutting into a piece that's already been freed.
func orderOpsByDepth(ops []Op) {
	sort.SliceStable(ops, func(i, j int) bool {
		// Higher Depth = less negative = shallower → earlier.
		return ops[i].Depth > ops[j].Depth
	})
}

// Op is one cutting operation: name, ordered cut points (mm or px depending
// on which builder produced it), and target Z depth.
type Op struct {
	Name  string
	Pts   []Point
	Depth float64
}

// RenderOp adds a color so the simulator and preview can show ops in
// distinct colors. Pixel coordinates.
type RenderOp struct {
	Op
	Color color.RGBA
}

// PerimColor matches gcodegen.py:_ops_px which uses #888888 for the
// perimeter overlay.
var PerimColor = color.RGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}

// PerimColorPreview matches gcodegen.py:_all_strokes_px which uses
// #555555 for the perimeter in the finished-product preview window.
var PerimColorPreview = color.RGBA{R: 0x55, G: 0x55, B: 0x55, A: 0xff}

// OpsMM returns all ops in machine mm — what the G-code emitter consumes.
// Mirrors gcodegen.py:_ops_mm. If Cut is true, prepends a Perimeter op
// running CCW around the work area at the perim depth.
func (e *Editor) OpsMM() []Op {
	ops := make([]Op, 0, len(e.Strokes)+1)
	if e.Perim.Cut {
		w, h := e.Perim.WidthMM, e.Perim.HeightMM
		ops = append(ops, Op{
			Name: "Perimeter",
			Pts: []Point{
				{X: 0, Y: 0},
				{X: w, Y: 0},
				{X: w, Y: h},
				{X: 0, Y: h},
				{X: 0, Y: 0},
			},
			Depth: e.Perim.DepthMM,
		})
	}
	for _, s := range e.Strokes {
		pts := make([]Point, len(s.Points))
		for i, p := range s.Points {
			x, y := e.PxToMM(p.X, p.Y)
			pts[i] = Point{X: x, Y: y}
		}
		ops = append(ops, Op{Name: s.Name, Pts: pts, Depth: s.Depth})
	}
	orderOpsByDepth(ops)
	return ops
}

// OpsPx returns all ops in pixel coords with their display colors —
// what the toolpath simulator consumes (gcodegen.py:_ops_px).
func (e *Editor) OpsPx() []RenderOp {
	ops := make([]RenderOp, 0, len(e.Strokes)+1)
	if e.Perim.Cut {
		lx, rx := minF(e.Perim.X0, e.Perim.X1), maxF(e.Perim.X0, e.Perim.X1)
		by, ty := maxF(e.Perim.Y0, e.Perim.Y1), minF(e.Perim.Y0, e.Perim.Y1)
		ops = append(ops, RenderOp{
			Op: Op{
				Name: "Perimeter",
				Pts: []Point{
					{X: lx, Y: by},
					{X: rx, Y: by},
					{X: rx, Y: ty},
					{X: lx, Y: ty},
					{X: lx, Y: by},
				},
				Depth: e.Perim.DepthMM,
			},
			Color: PerimColor,
		})
	}
	for _, s := range e.Strokes {
		ops = append(ops, RenderOp{
			Op:    Op{Name: s.Name, Pts: append([]Point(nil), s.Points...), Depth: s.Depth},
			Color: s.Color,
		})
	}
	// Match OpsMM ordering so the toolpath simulation plays back in
	// the same sequence the G-code will execute.
	sort.SliceStable(ops, func(i, j int) bool {
		return ops[i].Op.Depth > ops[j].Op.Depth
	})
	return ops
}

// AllStrokesPx returns every stroke (and perim if Cut) for the
// finished-product preview window (gcodegen.py:_all_strokes_px).
func (e *Editor) AllStrokesPx() []RenderOp {
	out := make([]RenderOp, 0, len(e.Strokes)+1)
	if e.Perim.Cut {
		lx, rx := minF(e.Perim.X0, e.Perim.X1), maxF(e.Perim.X0, e.Perim.X1)
		by, ty := maxF(e.Perim.Y0, e.Perim.Y1), minF(e.Perim.Y0, e.Perim.Y1)
		out = append(out, RenderOp{
			Op: Op{
				Name: "Perimeter",
				Pts: []Point{
					{X: lx, Y: by},
					{X: rx, Y: by},
					{X: rx, Y: ty},
					{X: lx, Y: ty},
					{X: lx, Y: by},
				},
				Depth: -1.0,
			},
			Color: PerimColorPreview,
		})
	}
	for _, s := range e.Strokes {
		out = append(out, RenderOp{
			Op:    Op{Name: s.Name, Pts: append([]Point(nil), s.Points...), Depth: s.Depth},
			Color: s.Color,
		})
	}
	return out
}
