package img

import (
	"image"
	"image/color"
	"testing"
)

// makeBinary constructs a Binary from a row-major list of strings where
// '#' is foreground and any other char is background. Useful for
// hand-crafting fixtures in tests.
func makeBinary(rows []string) *Binary {
	h := len(rows)
	w := len(rows[0])
	b := &Binary{W: w, H: h, Pix: make([]bool, w*h)}
	for y, r := range rows {
		for x, c := range r {
			if c == '#' {
				b.Pix[y*w+x] = true
			}
		}
	}
	return b
}

// makeImage constructs an RGBA image from a row-major list of strings.
// '#' = black (foreground after binarize at threshold > 0); '.' = white.
func makeImage(rows []string) image.Image {
	h := len(rows)
	w := len(rows[0])
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y, r := range rows {
		for x, c := range r {
			if c == '#' {
				im.Set(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				im.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	return im
}

func TestBinarize_BlackAndWhite(t *testing.T) {
	im := makeImage([]string{
		"#.#",
		".#.",
		"#.#",
	})
	b := Binarize(im, 128)
	want := []bool{
		true, false, true,
		false, true, false,
		true, false, true,
	}
	for i, v := range want {
		if b.Pix[i] != v {
			t.Errorf("pix[%d] = %v, want %v", i, b.Pix[i], v)
		}
	}
}

func TestBinarize_AlphaIsBackground(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 2, 1))
	im.Set(0, 0, color.RGBA{0, 0, 0, 255})   // opaque black → fg
	im.Set(1, 0, color.RGBA{0, 0, 0, 0})     // transparent → bg (treated as white)
	b := Binarize(im, 128)
	if !b.Pix[0] {
		t.Error("opaque black should be foreground")
	}
	if b.Pix[1] {
		t.Error("transparent should be background")
	}
}

func TestContours_SinglePixelBlock(t *testing.T) {
	b := makeBinary([]string{
		".....",
		".###.",
		".###.",
		".###.",
		".....",
	})
	polys := Contours(b, false)
	if len(polys) != 1 {
		t.Fatalf("got %d polys, want 1", len(polys))
	}
	p := polys[0]
	// Closed loop: first == last.
	if p[0] != p[len(p)-1] {
		t.Errorf("polyline not closed: first=%v last=%v", p[0], p[len(p)-1])
	}
	// All points should be on the boundary of the 3x3 square at (1..3, 1..3).
	for _, pt := range p {
		if pt.X < 1 || pt.X > 3 || pt.Y < 1 || pt.Y > 3 {
			t.Errorf("point %v outside expected bounding box", pt)
		}
	}
	// Top-left corner of the block is the start.
	if p[0] != (image.Point{X: 1, Y: 1}) {
		t.Errorf("start = %v, want (1, 1)", p[0])
	}
}

func TestContours_TwoSeparateBlocks(t *testing.T) {
	b := makeBinary([]string{
		"##...##",
		"##...##",
		".......",
		"##...##",
		"##...##",
	})
	polys := Contours(b, false)
	if len(polys) != 4 {
		t.Errorf("got %d polys, want 4 (one per 2x2 block)", len(polys))
	}
}

func TestContours_IncludeInner(t *testing.T) {
	// A frame: outer black ring with a hole in the middle.
	b := makeBinary([]string{
		"#####",
		"#...#",
		"#...#",
		"#...#",
		"#####",
	})
	outer := Contours(b, false)
	if len(outer) != 1 {
		t.Fatalf("outer-only: got %d polys, want 1", len(outer))
	}
	withInner := Contours(b, true)
	if len(withInner) != 2 {
		t.Errorf("with inner: got %d polys, want 2 (outer + hole)", len(withInner))
	}
}

func TestRDPSimplify_StraightLineCollapses(t *testing.T) {
	pts := []image.Point{}
	for i := 0; i < 10; i++ {
		pts = append(pts, image.Point{X: i, Y: 0})
	}
	out := RDPSimplify(pts, 0.5)
	if len(out) != 2 {
		t.Errorf("straight line: got %d points, want 2", len(out))
	}
	if out[0] != pts[0] || out[1] != pts[len(pts)-1] {
		t.Errorf("endpoints not preserved: %v", out)
	}
}

func TestRDPSimplify_LShapeKeepsCorner(t *testing.T) {
	// 5 points along x-axis then 5 down y-axis; corner at (4, 0).
	pts := []image.Point{
		{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0},
		{4, 1}, {4, 2}, {4, 3}, {4, 4},
	}
	out := RDPSimplify(pts, 0.5)
	if len(out) != 3 {
		t.Errorf("L-shape: got %d points, want 3 (endpoints + corner)", len(out))
	}
	if out[1] != (image.Point{X: 4, Y: 0}) {
		t.Errorf("corner = %v, want (4, 0)", out[1])
	}
}

func TestRDPSimplify_BelowEpsilonNoiseDropped(t *testing.T) {
	// Line with a tiny 1px bump that's below the epsilon=3 threshold.
	pts := []image.Point{
		{0, 0}, {1, 0}, {2, 0}, {3, 1}, {4, 0}, {5, 0}, {6, 0},
	}
	out := RDPSimplify(pts, 3)
	if len(out) != 2 {
		t.Errorf("noise-below-eps: got %d points, want 2", len(out))
	}
}

func TestZhangSuen_ThicksHorizontalCollapsesTo1Pixel(t *testing.T) {
	b := makeBinary([]string{
		".........",
		".#######.",
		".#######.",
		".#######.",
		".........",
	})
	sk := ZhangSuen(b.Clone())
	count := 0
	for _, v := range sk.Pix {
		if v {
			count++
		}
	}
	// 7 pixels wide originally; Zhang-Suen erodes endpoints from one
	// side per pass before they hit B=1 (the endpoint-preserving
	// terminal state), so a 7-wide thick bar typically settles to a
	// 3-5 pixel skeleton.
	if count < 3 || count > 7 {
		t.Errorf("skeleton pixel count = %d, want 3..7", count)
	}
	// Whatever remains must be in a single horizontal row, not a
	// 2D blob.
	rowsWithFG := map[int]bool{}
	for y := 0; y < sk.H; y++ {
		for x := 0; x < sk.W; x++ {
			if sk.Pix[y*sk.W+x] {
				rowsWithFG[y] = true
			}
		}
	}
	if len(rowsWithFG) != 1 {
		t.Errorf("skeleton spans %d rows, want 1 (got rows %v)", len(rowsWithFG), rowsWithFG)
	}
}

func TestCenterlines_SingleStrokeLine(t *testing.T) {
	b := makeBinary([]string{
		".........",
		".#######.",
		".#######.",
		".#######.",
		".........",
	})
	lines := Centerlines(b)
	if len(lines) != 1 {
		t.Fatalf("got %d centerlines, want 1", len(lines))
	}
	if len(lines[0]) < 2 {
		t.Errorf("line too short: %d points", len(lines[0]))
	}
}

func TestTrace_DispatchesByMode(t *testing.T) {
	// Contour mode: closed outline of a filled square (≥3 points).
	square := makeImage([]string{
		".....",
		".###.",
		".###.",
		".###.",
		".....",
	})
	contour := Trace(square, TraceOptions{Threshold: 128, Mode: ModeContour})
	if len(contour) == 0 {
		t.Error("contour: expected at least one polyline")
	}
	// Centerline mode: a long thin stripe collapses to an open line.
	// A 3x3 block thins to a single pixel (no neighbors) and is dropped;
	// use a 9-pixel stripe instead so the skeleton has at least 2 points.
	stripe := makeImage([]string{
		"...........",
		".#########.",
		".#########.",
		".#########.",
		"...........",
	})
	center := Trace(stripe, TraceOptions{Threshold: 128, Mode: ModeCenterline})
	if len(center) == 0 {
		t.Error("centerline: expected at least one polyline")
	}
}

func TestTrace_MinPointsFilter(t *testing.T) {
	// A 2x2 block traces to a 5-point closed polyline.
	im := makeImage([]string{
		"....",
		".##.",
		".##.",
		"....",
	})
	all := Trace(im, TraceOptions{Threshold: 128, Mode: ModeContour, MinPoints: 0})
	if len(all) != 1 {
		t.Fatalf("MinPoints=0: got %d polys, want 1", len(all))
	}
	filtered := Trace(im, TraceOptions{Threshold: 128, Mode: ModeContour, MinPoints: 20})
	if len(filtered) != 0 {
		t.Errorf("MinPoints=20: got %d polys, want 0", len(filtered))
	}
}
