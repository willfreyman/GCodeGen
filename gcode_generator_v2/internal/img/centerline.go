package img

import "image"

// Centerlines reduces b to its 1-pixel-wide skeleton via Zhang-Suen
// thinning, then walks the skeleton to produce open polylines. Lines
// break at junctions (pixels with > 2 skeleton neighbors).
func Centerlines(b *Binary) [][]image.Point {
	sk := ZhangSuen(b.Clone())
	return walkSkeleton(sk)
}

// ZhangSuen thins b in place using the classic two-subiteration
// algorithm. Returns the same b for chaining.
func ZhangSuen(b *Binary) *Binary {
	for {
		changed := false
		// Sub-iteration 1.
		toDel := []int{}
		for y := 1; y < b.H-1; y++ {
			for x := 1; x < b.W-1; x++ {
				if !b.Pix[y*b.W+x] {
					continue
				}
				if zsCondition(b, x, y, 1) {
					toDel = append(toDel, y*b.W+x)
				}
			}
		}
		if len(toDel) > 0 {
			changed = true
			for _, i := range toDel {
				b.Pix[i] = false
			}
		}
		// Sub-iteration 2.
		toDel = toDel[:0]
		for y := 1; y < b.H-1; y++ {
			for x := 1; x < b.W-1; x++ {
				if !b.Pix[y*b.W+x] {
					continue
				}
				if zsCondition(b, x, y, 2) {
					toDel = append(toDel, y*b.W+x)
				}
			}
		}
		if len(toDel) > 0 {
			changed = true
			for _, i := range toDel {
				b.Pix[i] = false
			}
		}
		if !changed {
			return b
		}
	}
}

// zsCondition checks the Zhang-Suen deletion test for pixel (x, y) in
// the given sub-iteration (1 or 2). Neighbors are labeled clockwise
// starting from north:
//
//	P9 P2 P3
//	P8 P1 P4    (P1 = (x, y))
//	P7 P6 P5
func zsCondition(b *Binary, x, y, sub int) bool {
	// Eight neighbors, clockwise from north.
	p2 := bi(b, x, y-1)
	p3 := bi(b, x+1, y-1)
	p4 := bi(b, x+1, y)
	p5 := bi(b, x+1, y+1)
	p6 := bi(b, x, y+1)
	p7 := bi(b, x-1, y+1)
	p8 := bi(b, x-1, y)
	p9 := bi(b, x-1, y-1)
	bp := p2 + p3 + p4 + p5 + p6 + p7 + p8 + p9
	if bp < 2 || bp > 6 {
		return false
	}
	// A(P) = number of 0→1 transitions in clockwise sequence p2..p9, p2.
	seq := [9]int{p2, p3, p4, p5, p6, p7, p8, p9, p2}
	a := 0
	for i := 0; i < 8; i++ {
		if seq[i] == 0 && seq[i+1] == 1 {
			a++
		}
	}
	if a != 1 {
		return false
	}
	if sub == 1 {
		if p2*p4*p6 != 0 {
			return false
		}
		if p4*p6*p8 != 0 {
			return false
		}
	} else {
		if p2*p4*p8 != 0 {
			return false
		}
		if p2*p6*p8 != 0 {
			return false
		}
	}
	return true
}

func bi(b *Binary, x, y int) int {
	if b.Get(x, y) {
		return 1
	}
	return 0
}

// walkSkeleton extracts open polylines from a 1-pixel-wide skeleton.
// Each polyline starts at either an endpoint (1 neighbor) or, if no
// endpoints remain, an arbitrary point on a closed loop. Lines break
// at junctions (≥3 neighbors).
func walkSkeleton(sk *Binary) [][]image.Point {
	w, h := sk.W, sk.H
	visited := make([]bool, w*h)
	neighbor := func(x, y int) int {
		n := 0
		for d := 0; d < 8; d++ {
			if sk.Get(x+mooreDirs[d][0], y+mooreDirs[d][1]) {
				n++
			}
		}
		return n
	}
	out := [][]image.Point{}

	walk := func(sx, sy int) []image.Point {
		line := []image.Point{{X: sx, Y: sy}}
		visited[sy*w+sx] = true
		cx, cy := sx, sy
		for {
			// Prefer 4-neighbors then diagonals to keep lines tidy.
			next := -1
			for _, order := range [8]int{0, 2, 4, 6, 1, 3, 5, 7} {
				nx, ny := cx+mooreDirs[order][0], cy+mooreDirs[order][1]
				if !sk.Get(nx, ny) || visited[ny*w+nx] {
					continue
				}
				if neighbor(nx, ny) > 2 {
					// Junction — include it as terminator, but don't
					// continue past it (other branches will start
					// their own line from it as an endpoint).
					line = append(line, image.Point{X: nx, Y: ny})
					return line
				}
				next = order
				break
			}
			if next == -1 {
				return line
			}
			cx += mooreDirs[next][0]
			cy += mooreDirs[next][1]
			visited[cy*w+cx] = true
			line = append(line, image.Point{X: cx, Y: cy})
		}
	}

	// Pass 1: walk from every endpoint.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !sk.Get(x, y) || visited[y*w+x] {
				continue
			}
			if neighbor(x, y) != 1 {
				continue
			}
			line := walk(x, y)
			if len(line) >= 2 {
				out = append(out, line)
			}
		}
	}
	// Pass 2: walk from remaining unvisited skeleton pixels (closed loops).
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !sk.Get(x, y) || visited[y*w+x] {
				continue
			}
			line := walk(x, y)
			if len(line) >= 2 {
				out = append(out, line)
			}
		}
	}
	return out
}
