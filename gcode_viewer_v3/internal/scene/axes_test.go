package scene

import (
	"math"
	"testing"

	"gcodegen.local/viewer/internal/parser"
)

// The triad scales with the model but stays inside sane bounds — big enough to
// see on a tiny test cut, small enough not to dominate a full sheet.
func TestAxisLengthForBounds(t *testing.T) {
	cases := []struct {
		name     string
		min, max parser.Point
		want     float64
	}{
		{
			name: "typical 100mm part scales to 15%",
			min:  parser.Point{X: 0, Y: 0, Z: -3},
			max:  parser.Point{X: 100, Y: 80, Z: 5},
			want: 15,
		},
		{
			name: "tiny part hits the lower clamp",
			min:  parser.Point{},
			max:  parser.Point{X: 10, Y: 10, Z: 1},
			want: axisLengthMin,
		},
		{
			name: "large sheet hits the upper clamp",
			min:  parser.Point{},
			max:  parser.Point{X: 1200, Y: 600, Z: 10},
			want: axisLengthMax,
		},
		{
			name: "degenerate zero-size bounds still yield a visible triad",
			min:  parser.Point{},
			max:  parser.Point{},
			want: axisLengthMin,
		},
		{
			name: "Z can be the dominant span",
			min:  parser.Point{X: 0, Y: 0, Z: -200},
			max:  parser.Point{X: 10, Y: 10, Z: 0},
			want: 30,
		},
	}

	for _, c := range cases {
		got := axisLengthForBounds(c.min, c.max)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: axisLengthForBounds = %v, want %v", c.name, got, c.want)
		}
		if got < axisLengthMin || got > axisLengthMax {
			t.Errorf("%s: %v is outside the clamp [%v, %v]",
				c.name, got, axisLengthMin, axisLengthMax)
		}
	}
}
