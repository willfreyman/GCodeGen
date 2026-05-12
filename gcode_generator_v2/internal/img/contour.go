package img

import "image"

// Contours finds the outer boundary of each connected foreground
// region in b. Output polylines are closed (first == last). If
// includeInner is true, "holes" (background regions fully enclosed by
// foreground) are also traced and appended after their parent contour.
func Contours(b *Binary, includeInner bool) [][]image.Point {
	out := [][]image.Point{}
	labels := labelComponents(b, true)
	for id := 1; id <= labels.count; id++ {
		start, ok := labels.topLeft(id)
		if !ok {
			continue
		}
		poly := mooreTrace(b, start, true)
		if len(poly) >= 3 {
			out = append(out, poly)
		}
	}
	if includeInner {
		// Inner contours = background components that don't touch the
		// edge of the image (i.e., they're enclosed by foreground).
		bg := invert(b)
		bgLabels := labelComponents(bg, true)
		for id := 1; id <= bgLabels.count; id++ {
			if bgLabels.touchesEdge(id, bg.W, bg.H) {
				continue
			}
			start, ok := bgLabels.topLeft(id)
			if !ok {
				continue
			}
			poly := mooreTrace(bg, start, true)
			if len(poly) >= 3 {
				out = append(out, poly)
			}
		}
	}
	return out
}

// invert returns a copy of b with foreground/background swapped.
func invert(b *Binary) *Binary {
	out := &Binary{W: b.W, H: b.H, Pix: make([]bool, len(b.Pix))}
	for i, v := range b.Pix {
		out.Pix[i] = !v
	}
	return out
}

// componentLabels stores per-pixel component IDs (0 = none) plus a count.
type componentLabels struct {
	W, H  int
	Lab   []int32
	count int
}

func (c *componentLabels) at(x, y int) int32 {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return 0
	}
	return c.Lab[y*c.W+x]
}

// topLeft returns the smallest (y, x) pixel with label id.
func (c *componentLabels) topLeft(id int) (image.Point, bool) {
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			if c.Lab[y*c.W+x] == int32(id) {
				return image.Point{X: x, Y: y}, true
			}
		}
	}
	return image.Point{}, false
}

// touchesEdge reports whether any pixel of label id is on the image
// border. Background components that touch the edge are the "outside",
// not enclosed holes.
func (c *componentLabels) touchesEdge(id int, w, h int) bool {
	for x := 0; x < w; x++ {
		if c.Lab[x] == int32(id) || c.Lab[(h-1)*w+x] == int32(id) {
			return true
		}
	}
	for y := 0; y < h; y++ {
		if c.Lab[y*w] == int32(id) || c.Lab[y*w+(w-1)] == int32(id) {
			return true
		}
	}
	return false
}

// labelComponents flood-fills b's foreground pixels into connected
// components. 4-neighborhood when 8conn is false, 8-neighborhood when
// true. IDs start at 1; 0 means no component.
func labelComponents(b *Binary, eightConn bool) *componentLabels {
	cl := &componentLabels{W: b.W, H: b.H, Lab: make([]int32, b.W*b.H)}
	var stackX, stackY []int
	next := int32(0)
	for y := 0; y < b.H; y++ {
		for x := 0; x < b.W; x++ {
			if !b.Pix[y*b.W+x] || cl.Lab[y*b.W+x] != 0 {
				continue
			}
			next++
			stackX = append(stackX[:0], x)
			stackY = append(stackY[:0], y)
			for len(stackX) > 0 {
				cx := stackX[len(stackX)-1]
				cy := stackY[len(stackY)-1]
				stackX = stackX[:len(stackX)-1]
				stackY = stackY[:len(stackY)-1]
				if cx < 0 || cy < 0 || cx >= b.W || cy >= b.H {
					continue
				}
				if !b.Pix[cy*b.W+cx] || cl.Lab[cy*b.W+cx] != 0 {
					continue
				}
				cl.Lab[cy*b.W+cx] = next
				stackX = append(stackX, cx+1, cx-1, cx, cx)
				stackY = append(stackY, cy, cy, cy+1, cy-1)
				if eightConn {
					stackX = append(stackX, cx+1, cx-1, cx+1, cx-1)
					stackY = append(stackY, cy+1, cy+1, cy-1, cy-1)
				}
			}
		}
	}
	cl.count = int(next)
	return cl
}

// Moore-neighborhood scan order: clockwise around center, starting east.
// Encoded as 8 (dx, dy) pairs. Indexed by direction 0..7.
var mooreDirs = [8][2]int{
	{1, 0},   // 0: E
	{1, 1},   // 1: SE
	{0, 1},   // 2: S
	{-1, 1},  // 3: SW
	{-1, 0},  // 4: W
	{-1, -1}, // 5: NW
	{0, -1},  // 6: N
	{1, -1},  // 7: NE
}

// mooreTrace walks the outer boundary of the connected region
// containing `start` (which must be a foreground pixel at the top-left
// of its component). Returns the closed polyline (last == first). If
// closed is false, the trailing duplicate is omitted.
func mooreTrace(b *Binary, start image.Point, closed bool) []image.Point {
	if !b.Get(start.X, start.Y) {
		return nil
	}
	// Single-pixel component: return a trivial 1-point poly.
	if !hasForegroundNeighbor(b, start.X, start.Y) {
		if closed {
			return []image.Point{start, start}
		}
		return []image.Point{start}
	}
	out := []image.Point{start}
	cur := start
	// prevDir is the direction FROM cur TO the previous pixel (the one
	// we just came from). For the starting pixel, the prior raster-scan
	// position was the background pixel to the west, so prevDir = 4 (W).
	// To find the next boundary pixel, we scan clockwise around cur
	// starting one CW step after prevDir — i.e. (prevDir + 1) % 8.
	prevDir := 4 // came from W
	maxSteps := b.W * b.H * 4
	for step := 0; step < maxSteps; step++ {
		startDir := (prevDir + 1) % 8
		found := false
		for k := 0; k < 8; k++ {
			d := (startDir + k) % 8
			nx, ny := cur.X+mooreDirs[d][0], cur.Y+mooreDirs[d][1]
			if b.Get(nx, ny) {
				// Stop if we'd close the loop.
				if nx == start.X && ny == start.Y && len(out) > 1 {
					if closed {
						out = append(out, start)
					}
					return out
				}
				out = append(out, image.Point{X: nx, Y: ny})
				// We arrived at the new pixel from the OPPOSITE side
				// of the move we just took.
				prevDir = (d + 4) % 8
				cur = image.Point{X: nx, Y: ny}
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	if closed && len(out) > 0 && out[len(out)-1] != start {
		out = append(out, start)
	}
	return out
}

// hasForegroundNeighbor reports whether (x, y) has at least one
// foreground neighbor in its 8-neighborhood (excluding self).
func hasForegroundNeighbor(b *Binary, x, y int) bool {
	for d := 0; d < 8; d++ {
		if b.Get(x+mooreDirs[d][0], y+mooreDirs[d][1]) {
			return true
		}
	}
	return false
}
