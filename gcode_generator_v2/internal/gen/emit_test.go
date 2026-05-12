package gen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStepDownPasses(t *testing.T) {
	cases := []struct {
		target, step float64
		want         []float64
	}{
		{-19.05, 2.0, []float64{-2, -4, -6, -8, -10, -12, -14, -16, -18, -19.05}},
		{-1.0, 0, []float64{-1.0}},          // step=0 disables → single pass
		{-1.0, 2.0, []float64{-1.0}},        // step larger than depth → single pass
		{-2.0, 2.0, []float64{-2.0}},        // step equals depth → single pass
		{-4.0, 2.0, []float64{-2.0, -4.0}},  // exact multiple
		{0, 2.0, []float64{0}},              // target=0 → trivial single pass
		{-19.05, -1, []float64{-19.05}},     // negative step → single pass
	}
	for _, c := range cases {
		got := stepDownPasses(c.target, c.step)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("stepDownPasses(%v, %v) = %v, want %v", c.target, c.step, got, c.want)
		}
	}
}

func TestEmit_StepDownClosedPathSkipsInterPassRetract(t *testing.T) {
	// Closed loop: ends where it started.
	ops := []Op{{
		Name: "Loop",
		Pts: []Point{
			{X: 0, Y: 0}, {X: 10, Y: 0},
			{X: 10, Y: 10}, {X: 0, Y: 10},
			{X: 0, Y: 0},
		},
		Depth: -6,
	}}
	m := Machine{SafeZ: 5, FeedXY: 300, FeedZ: 100, RPM: 12000, StepDown: 2}
	out := Emit(ops, m)
	// 3 plunges expected (-2, -4, -6).
	if got := strings.Count(out, "G1 Z-"); got != 3 {
		t.Errorf("got %d plunges, want 3:\n%s", got, out)
	}
	// Only file header retract + per-op leading retract + final retract = 3.
	// NO inter-pass retracts because path is closed (bit ends at start).
	if got := strings.Count(out, "G0 Z5.000"); got != 3 {
		t.Errorf("closed path: got %d safe-Z retracts, want 3 (no inter-pass needed):\n%s", got, out)
	}
}

func TestEmit_StepDownEmitsMultiplePasses(t *testing.T) {
	ops := []Op{{
		Name:  "Test",
		Pts:   []Point{{X: 0, Y: 0}, {X: 10, Y: 0}},
		Depth: -6,
	}}
	m := Machine{SafeZ: 5, FeedXY: 300, FeedZ: 100, RPM: 12000, StepDown: 2}
	out := Emit(ops, m)
	// Three plunges expected: -2, -4, -6.
	plunges := strings.Count(out, "G1 Z-")
	if plunges != 3 {
		t.Errorf("got %d G1 Z- plunge lines, want 3:\n%s", plunges, out)
	}
	// Safe-Z retracts: file header + per-op leading + 2 inter-pass + trailing = 5.
	retracts := strings.Count(out, "G0 Z5.000")
	if retracts != 5 {
		t.Errorf("got %d safe-Z retracts, want 5:\n%s", retracts, out)
	}
}

// TestEmit_Golden compares Emit's output against checked-in reference
// files. The references were hand-written following the exact format
// emitted by gcodegen.py:380-405; if Emit is regressed, this test
// breaks. To regenerate intentionally, set GCODEGEN_UPDATE_GOLDEN=1.
func TestEmit_Golden(t *testing.T) {
	cases := []struct {
		name    string
		ops     []Op
		machine Machine
	}{
		{
			name: "perim_only",
			ops: []Op{
				{
					Name: "Perimeter",
					Pts: []Point{
						{X: 0, Y: 0},
						{X: 50, Y: 0},
						{X: 50, Y: 50},
						{X: 0, Y: 50},
						{X: 0, Y: 0},
					},
					Depth: -1.0,
				},
			},
			machine: Machine{SafeZ: 5.0, FeedXY: 300, FeedZ: 100, RPM: 12000},
		},
		{
			name: "single_stroke",
			ops: []Op{
				{
					Name:  "Cut 1",
					Pts:   []Point{{X: 10, Y: 5}, {X: 20, Y: 5}, {X: 20, Y: 15}},
					Depth: -2.5,
				},
			},
			machine: Machine{SafeZ: 5.0, FeedXY: 300, FeedZ: 100, RPM: 12000},
		},
		{
			name: "three_strokes",
			ops: []Op{
				{
					Name: "Perimeter",
					Pts: []Point{
						{X: 0, Y: 0}, {X: 50, Y: 0}, {X: 50, Y: 50},
						{X: 0, Y: 50}, {X: 0, Y: 0},
					},
					Depth: -1.0,
				},
				{
					Name:  "Cut 1",
					Pts:   []Point{{X: 0, Y: 0}, {X: 10, Y: 10}},
					Depth: -1.5,
				},
				{
					Name: "Cut 2",
					Pts: []Point{
						{X: 20, Y: 5}, {X: 30, Y: 5}, {X: 30, Y: 20}, {X: 20, Y: 20},
					},
					Depth: -3.0,
				},
				{
					Name:  "Cut 3",
					Pts:   []Point{{X: 5, Y: 40}, {X: 15, Y: 40}},
					Depth: -0.5,
				},
			},
			machine: Machine{SafeZ: 8.0, FeedXY: 500, FeedZ: 80, RPM: 10000},
		},
	}

	updateGolden := os.Getenv("GCODEGEN_UPDATE_GOLDEN") == "1"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Emit(tc.ops, tc.machine)
			path := filepath.Join("testdata", tc.name+".nc")

			if updateGolden {
				if err := os.WriteFile(path, []byte(got), 0644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("updated %s", path)
				return
			}

			wantBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v", path, err)
			}
			want := string(wantBytes)
			if got != want {
				t.Errorf("Emit() byte-mismatch for %s\n--- got ---\n%s\n--- want ---\n%s\n--- diff (first mismatch) ---\n%s",
					tc.name, got, want, firstDiff(got, want))
			}
		})
	}
}

// firstDiff returns a short snippet around the first mismatched byte to
// make golden test failures debuggable without dumping the whole file.
func firstDiff(got, want string) string {
	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	for i := 0; i < n; i++ {
		if got[i] != want[i] {
			start := i - 20
			if start < 0 {
				start = 0
			}
			endG := i + 20
			if endG > len(got) {
				endG = len(got)
			}
			endW := i + 20
			if endW > len(want) {
				endW = len(want)
			}
			return "byte " + itoa(i) + ": got " + quote(got[start:endG]) + " vs want " + quote(want[start:endW])
		}
	}
	if len(got) != len(want) {
		return "lengths differ: got=" + itoa(len(got)) + " want=" + itoa(len(want))
	}
	return ""
}

func quote(s string) string {
	out := []byte{'"'}
	for _, r := range s {
		switch r {
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		case '"':
			out = append(out, '\\', '"')
		default:
			out = append(out, []byte(string(r))...)
		}
	}
	out = append(out, '"')
	return string(out)
}

// TestEmit_PyFloat verifies the Python-str-compatible float formatter
// for values commonly used as depth.
func TestEmit_PyFloat(t *testing.T) {
	cases := []struct{ in float64; want string }{
		{0.0, "0.0"},
		{1.0, "1.0"},
		{-1.0, "-1.0"},
		{1.5, "1.5"},
		{-2.5, "-2.5"},
		{-0.5, "-0.5"},
		{12.345, "12.345"},
	}
	for _, c := range cases {
		if got := pyFloat(c.in); got != c.want {
			t.Errorf("pyFloat(%v) = %q; want %q", c.in, got, c.want)
		}
	}
}
