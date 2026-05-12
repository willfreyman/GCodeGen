// Package img is the image-tracing toolbox the editor uses to turn a
// PNG/JPG into one or more polylines that can be added to the editor
// as Strokes. The package is pure-Go (stdlib + math) so it can be
// unit-tested without booting Ebiten.
//
// Public entry point is Trace; it dispatches to the contour or
// centerline pipeline based on opts.Mode.
package img

import (
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"os"

	_ "golang.org/x/image/webp" // register WebP decoder
)

// Mode selects between contour outline tracing and centerline (medial
// axis) tracing.
type Mode int

const (
	// ModeContour produces closed polylines around dark regions
	// (silhouettes, logos, lettering, clipart).
	ModeContour Mode = iota
	// ModeCenterline produces open polylines along the skeleton of
	// dark regions (handwriting, sketches, line art).
	ModeCenterline
)

// TraceOptions controls the Trace pipeline.
type TraceOptions struct {
	// Threshold is the grayscale cutoff: pixels with luma < Threshold
	// are foreground (drawn / cut), pixels >= are background. 128 is a
	// reasonable starting point for typical line art.
	Threshold uint8
	// Mode picks the algorithm. See the Mode constants.
	Mode Mode
	// IncludeInner controls contour mode only: when true, holes inside
	// shapes (e.g., the bowl of an "O") are traced as additional
	// polylines.
	IncludeInner bool
	// SimplifyEps is the Ramer-Douglas-Peucker tolerance in pixels.
	// 0 disables simplification; 1-3 is a reasonable range to drop
	// noise without smoothing real features.
	SimplifyEps float64
	// MinPoints discards traced polylines with fewer than this many
	// points after simplification (filters out specks). 0 means keep all.
	MinPoints int
}

// LoadFile decodes an image from disk. Supports PNG, JPEG, and WebP.
func LoadFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	im, format, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	_ = format
	return im, nil
}

// Trace produces a set of polylines from img based on opts. Each
// polyline is a slice of image.Point in source-image pixel coordinates
// (origin top-left, y-down). Contour polylines are closed (first point
// equal to last); centerline polylines are open.
func Trace(im image.Image, opts TraceOptions) [][]image.Point {
	bin := Binarize(im, opts.Threshold)
	var out [][]image.Point
	switch opts.Mode {
	case ModeCenterline:
		out = Centerlines(bin)
	default:
		out = Contours(bin, opts.IncludeInner)
	}
	if opts.SimplifyEps > 0 {
		for i, p := range out {
			out[i] = RDPSimplify(p, opts.SimplifyEps)
		}
	}
	if opts.MinPoints > 0 {
		filtered := out[:0]
		for _, p := range out {
			if len(p) >= opts.MinPoints {
				filtered = append(filtered, p)
			}
		}
		out = filtered
	}
	return out
}
