package holegen

import (
	"math"
	"strings"
	"testing"
)

// wantDefaultProgram is the reference output from HoleGen_SPEC.md §7.7 —
// the byte-for-byte target with default parameters (2 cols × 2 rows, bit 6.0,
// target 28.57, xOffset 10, spacing 50.8/50.8, spindle 18000, vFeed 150,
// hFeed 600, thickness 3.0, pitch 1.0).
//
// Note the snake order in the coordinates: Col 1 runs Y0 → Y50.8, Col 2 runs
// Y50.8 → Y0.
const wantDefaultProgram = `G90 G21 G17
G0 Z5.0000
M03 S18000
G4 P4000

( --- HOLE LOCATION: Col 1, Row 1 --- )
G0 X13.0000 Y0.0000
G0 Z1.0000
G1 X24.2850 F600
G03 X24.2850 Y0.0000 Z-1.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y0.0000 Z-2.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y0.0000 Z-3.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y0.0000 Z-4.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y0.0000 Z-4.5000 I-11.2850 J0.0000 F150
G03 X24.2850 Y0.0000 I-11.2850 J0.0000 F600
G1 X13.0000
G0 Z5.0000

( --- HOLE LOCATION: Col 1, Row 2 --- )
G0 X13.0000 Y50.8000
G0 Z1.0000
G1 X24.2850 F600
G03 X24.2850 Y50.8000 Z-1.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y50.8000 Z-2.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y50.8000 Z-3.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y50.8000 Z-4.0000 I-11.2850 J0.0000 F150
G03 X24.2850 Y50.8000 Z-4.5000 I-11.2850 J0.0000 F150
G03 X24.2850 Y50.8000 I-11.2850 J0.0000 F600
G1 X13.0000
G0 Z5.0000

( --- HOLE LOCATION: Col 2, Row 1 --- )
G0 X63.8000 Y50.8000
G0 Z1.0000
G1 X75.0850 F600
G03 X75.0850 Y50.8000 Z-1.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y50.8000 Z-2.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y50.8000 Z-3.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y50.8000 Z-4.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y50.8000 Z-4.5000 I-11.2850 J0.0000 F150
G03 X75.0850 Y50.8000 I-11.2850 J0.0000 F600
G1 X63.8000
G0 Z5.0000

( --- HOLE LOCATION: Col 2, Row 2 --- )
G0 X63.8000 Y0.0000
G0 Z1.0000
G1 X75.0850 F600
G03 X75.0850 Y0.0000 Z-1.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y0.0000 Z-2.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y0.0000 Z-3.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y0.0000 Z-4.0000 I-11.2850 J0.0000 F150
G03 X75.0850 Y0.0000 Z-4.5000 I-11.2850 J0.0000 F150
G03 X75.0850 Y0.0000 I-11.2850 J0.0000 F600
G1 X63.8000
G0 Z5.0000

( --- END OF PROGRAM --- )
M05
G0 X0 Y0
M30
`

func defaultParams(t *testing.T) Params {
	t.Helper()
	p, err := ReadParams(DefaultValues())
	if err != nil {
		t.Fatalf("ReadParams(defaults): %v", err)
	}
	return p
}

// TestGenerateDefaultProgram is the spec's verification fixture. Any
// divergence here means the generator has drifted from the reference.
func TestGenerateDefaultProgram(t *testing.T) {
	got, cutRadius, totalDepth, err := Program(defaultParams(t))
	if err != nil {
		t.Fatalf("Program: %v", err)
	}
	if !near(cutRadius, 11.285) {
		t.Errorf("cutRadius = %v, want 11.285", cutRadius)
	}
	if !near(totalDepth, 4.5) {
		t.Errorf("totalDepth = %v, want 4.5", totalDepth)
	}
	if got != wantDefaultProgram {
		t.Errorf("program mismatch:\n%s", firstDiff(got, wantDefaultProgram))
	}
}

// Every line must carry its own trailing newline so the file is a plain
// concatenation with no separator.
func TestGenerateLinesEndWithNewline(t *testing.T) {
	lines, _, _, err := GenerateGCode(defaultParams(t))
	if err != nil {
		t.Fatalf("GenerateGCode: %v", err)
	}
	for i, ln := range lines {
		if !strings.HasSuffix(ln, "\n") {
			t.Errorf("line[%d] = %q has no trailing newline", i, ln)
		}
	}
}

func TestDefaultsParse(t *testing.T) {
	p := defaultParams(t)
	if p.RowCount != 2 || p.ColumnCount != 2 {
		t.Errorf("grid = %d×%d, want 2×2", p.ColumnCount, p.RowCount)
	}
	if p.SpindleSpeed != 18000 || p.VerticalFeedrate != 150 || p.HorizontalFeedrate != 600 {
		t.Errorf("int fields = %d/%d/%d, want 18000/150/600",
			p.SpindleSpeed, p.VerticalFeedrate, p.HorizontalFeedrate)
	}
	if !near(p.TargetHoleDiameter, 28.57) || !near(p.PitchPerTurn, 1.0) {
		t.Errorf("float fields = %v/%v, want 28.57/1.0", p.TargetHoleDiameter, p.PitchPerTurn)
	}
}

// Worked results from spec §4.
func TestParseMeasurement(t *testing.T) {
	cases := []struct {
		raw   string
		isInt bool
		want  float64
	}{
		{"1.125in", false, 28.575},
		{`1.125"`, false, 28.575},
		{"1.125 in", false, 28.575},
		{"0.5IN", false, 12.7},
		{"28.585mm", false, 28.585},
		{"28.585", false, 28.585},
		{"2in", true, 51},
		{" 6.0 ", false, 6.0},
		{"1.125 \"", false, 28.575},
		{"18000", true, 18000},
	}
	for _, c := range cases {
		got, err := ParseMeasurement(c.raw, c.isInt)
		if err != nil {
			t.Errorf("ParseMeasurement(%q, %v): unexpected error %v", c.raw, c.isInt, err)
			continue
		}
		if !near(got, c.want) {
			t.Errorf("ParseMeasurement(%q, %v) = %v, want %v", c.raw, c.isInt, got, c.want)
		}
	}
}

func TestParseMeasurementErrors(t *testing.T) {
	for _, raw := range []string{"", "abc", "in", `"`, "mm", "1.2.3", "6mmm"} {
		if v, err := ParseMeasurement(raw, false); err == nil {
			t.Errorf("ParseMeasurement(%q) = %v, want error", raw, v)
		}
	}
}

// Exact examples from spec §9.
func TestFormatDuration(t *testing.T) {
	cases := []struct {
		sec  float64
		want string
	}{
		{45, "45s"},
		{200, "3m 20s"},
		{3849, "1h 04m 09s"},
		{0, "0s"},
		{60, "1m 00s"},
		{3600, "1h 00m 00s"},
	}
	for _, c := range cases {
		if got := FormatDuration(c.sec); got != c.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", c.sec, got, c.want)
		}
	}
}

// Sanity fixture from spec §8: default params ≈ 614 s → "10m 14s".
func TestEstimateRuntimeDefault(t *testing.T) {
	p := defaultParams(t)
	got := EstimateRuntime(p)
	if math.Abs(got-614.3567) > 0.01 {
		t.Errorf("EstimateRuntime = %v, want ≈614.3567", got)
	}
	if s := FormatDuration(got); s != "10m 14s" {
		t.Errorf("FormatDuration(estimate) = %q, want \"10m 14s\"", s)
	}
}

func TestSummary(t *testing.T) {
	want := "4 holes  |  Ø28.57mm  |  ~10m 14s"
	if got := Summary(defaultParams(t)); got != want {
		t.Errorf("Summary = %q, want %q", got, want)
	}
}

// Snake ordering: column outer, row inner, odd columns reversed in Y, and
// each column ends at the Y the next column starts at.
func TestHoleCentersSnakeOrder(t *testing.T) {
	p := defaultParams(t)
	p.ColumnCount = 3
	p.RowCount = 3

	holes := HoleCenters(p)
	if len(holes) != 9 {
		t.Fatalf("got %d holes, want 9", len(holes))
	}

	wantY := []float64{
		0, 50.8, 101.6, // col 0 bottom → top
		101.6, 50.8, 0, // col 1 top → bottom
		0, 50.8, 101.6, // col 2 bottom → top
	}
	for i, h := range holes {
		if !near(h.Y, wantY[i]) {
			t.Errorf("hole[%d].Y = %v, want %v", i, h.Y, wantY[i])
		}
	}

	// X includes the bit radius because X=0 is the endmill edge, not the
	// tube face: 6/2 + 10 + 50.8*col.
	for i, h := range holes {
		wantX := 3.0 + 10.0 + 50.8*float64(i/3)
		if !near(h.X, wantX) {
			t.Errorf("hole[%d].X = %v, want %v", i, h.X, wantX)
		}
	}
}

func TestHoleCentersSingleHole(t *testing.T) {
	p := defaultParams(t)
	p.RowCount, p.ColumnCount = 1, 1
	holes := HoleCenters(p)
	if len(holes) != 1 {
		t.Fatalf("got %d holes, want 1", len(holes))
	}
	if !near(holes[0].X, 13.0) || !near(holes[0].Y, 0) {
		t.Errorf("hole = (%v, %v), want (13, 0)", holes[0].X, holes[0].Y)
	}
}

func TestBitLargerThanHoleIsAnError(t *testing.T) {
	p := defaultParams(t)
	p.BitDiameter = 30
	p.TargetHoleDiameter = 28.57
	if _, _, _, err := GenerateGCode(p); err == nil {
		t.Fatal("want error when bit diameter exceeds hole diameter")
	}
}

// cutRadius == 0 (hole Ø == bit Ø) takes the straight-plunge branch: no arcs
// at all, one G1 Z plunge per hole.
func TestCenterPlungeWhenHoleEqualsBit(t *testing.T) {
	p := defaultParams(t)
	p.BitDiameter = 6.0
	p.TargetHoleDiameter = 6.0
	p.RowCount, p.ColumnCount = 1, 1

	got, cutRadius, totalDepth, err := Program(p)
	if err != nil {
		t.Fatalf("Program: %v", err)
	}
	if cutRadius != 0 {
		t.Errorf("cutRadius = %v, want 0", cutRadius)
	}
	if strings.Contains(got, "G03") {
		t.Error("plunge branch must not emit any arcs")
	}
	if !strings.Contains(got, "G1 Z-4.5000 F150\n") {
		t.Errorf("missing center plunge to -%.4f:\n%s", totalDepth, got)
	}
}

// Pass count is ceil(totalDepth/pitch) with the last pass clamped exactly to
// -totalDepth, so the floor is never overcut.
func TestHelicalPassCountAndClamp(t *testing.T) {
	cases := []struct {
		thickness, pitch float64
		wantPasses       int
		wantFinalZ       string
	}{
		{3.0, 1.0, 5, "Z-4.5000"},   // 4.5 / 1.0
		{3.0, 1.5, 3, "Z-4.5000"},   // exact multiple, no clamp needed
		{1.0, 0.5, 5, "Z-2.5000"},   // 2.5 / 0.5
		{10.0, 4.0, 3, "Z-11.5000"}, // 11.5 / 4.0 → 4, 8, 11.5
	}
	for _, c := range cases {
		p := defaultParams(t)
		p.RowCount, p.ColumnCount = 1, 1
		p.TubeThickness = c.thickness
		p.PitchPerTurn = c.pitch

		text, _, totalDepth, err := Program(p)
		if err != nil {
			t.Fatalf("Program: %v", err)
		}
		// Helical arcs carry a Z word; the spring pass does not.
		passes := strings.Count(text, "G03 ") - strings.Count(text, "J0.0000 F600")
		if passes != c.wantPasses {
			t.Errorf("thickness %v pitch %v: %d helical passes, want %d",
				c.thickness, c.pitch, passes, c.wantPasses)
		}
		if !strings.Contains(text, c.wantFinalZ) {
			t.Errorf("thickness %v pitch %v: final pass %s missing (totalDepth %v)",
				c.thickness, c.pitch, c.wantFinalZ, totalDepth)
		}
	}
}

// The safety ordering is load-bearing: retract must precede spindle-on.
func TestRetractPrecedesSpindleOn(t *testing.T) {
	text, _, _, err := Program(defaultParams(t))
	if err != nil {
		t.Fatalf("Program: %v", err)
	}
	retract := strings.Index(text, "G0 Z5.0000")
	spindle := strings.Index(text, "M03 S")
	if retract < 0 || spindle < 0 {
		t.Fatal("missing retract or spindle-on line")
	}
	if retract > spindle {
		t.Error("spindle starts before the Z retract — unsafe ordering")
	}
}

func TestReadParamsFieldError(t *testing.T) {
	values := DefaultValues()
	values["bitDiameter"] = "not a number"
	_, err := ReadParams(values)
	if err == nil {
		t.Fatal("want error for unparseable field")
	}
	want := "'Bit diameter (mm)' must be a valid float (optionally suffixed with 'mm' or 'in', e.g. 1.125in)."
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestReadParamsIntFieldErrorSaysInt(t *testing.T) {
	values := DefaultValues()
	values["rowCount"] = "abc"
	_, err := ReadParams(values)
	if err == nil {
		t.Fatal("want error for unparseable int field")
	}
	if !strings.Contains(err.Error(), "must be a valid int") {
		t.Errorf("error = %q, want it to say \"must be a valid int\"", err.Error())
	}
}

func TestReadParamsRejectsEmptyGrid(t *testing.T) {
	for _, key := range []string{"rowCount", "columnCount"} {
		values := DefaultValues()
		values[key] = "0"
		_, err := ReadParams(values)
		if err == nil {
			t.Fatalf("%s = 0: want error", key)
		}
		if err.Error() != "Rows and columns must each be at least 1." {
			t.Errorf("%s = 0: error = %q", key, err.Error())
		}
	}
}

// The RPM label says "max 24000" but that is advisory text only — nothing
// enforces it (spec §3 / §11).
func TestNoRPMCapEnforced(t *testing.T) {
	values := DefaultValues()
	values["spindleSpeed"] = "99000"
	p, err := ReadParams(values)
	if err != nil {
		t.Fatalf("ReadParams: %v", err)
	}
	if p.SpindleSpeed != 99000 {
		t.Errorf("SpindleSpeed = %d, want 99000 (no cap)", p.SpindleSpeed)
	}
}

// Per-field inch override must reach the emitted coordinates.
func TestInchSuffixReachesOutput(t *testing.T) {
	values := DefaultValues()
	values["targetHoleDiameter"] = `1.125"` // 28.575 mm
	values["rowCount"] = "1"
	values["columnCount"] = "1"
	p, err := ReadParams(values)
	if err != nil {
		t.Fatalf("ReadParams: %v", err)
	}
	text, cutRadius, _, err := Program(p)
	if err != nil {
		t.Fatalf("Program: %v", err)
	}
	if !near(cutRadius, 11.2875) { // (28.575 - 6) / 2
		t.Errorf("cutRadius = %v, want 11.2875", cutRadius)
	}
	if !strings.Contains(text, "I-11.2875 J0.0000") {
		t.Errorf("inch-derived cut radius missing from output:\n%s", text)
	}
}

func TestDiameterPresetOptions(t *testing.T) {
	want := []string{
		"6.0 — 6 mm hole",
		`12.7 — 1/2" shaft`,
		"28.585 — bearing hole",
		`50.8 — 2" hole`,
	}
	if len(DiameterPresets) != len(want) {
		t.Fatalf("got %d presets, want %d", len(DiameterPresets), len(want))
	}
	for i, p := range DiameterPresets {
		got := p.Option()
		if got != want[i] {
			t.Errorf("preset[%d].Option() = %q, want %q", i, got, want[i])
		}
		back, ok := DiameterPresetByOption(got)
		if !ok || back.Value != p.Value {
			t.Errorf("round-trip failed for %q", got)
		}
	}
	if _, ok := DiameterPresetByOption("nope"); ok {
		t.Error("DiameterPresetByOption matched an unknown option")
	}
}

func TestPresetsNamesSorted(t *testing.T) {
	p := Presets{
		"zeta":  {"bitDiameter": "6"},
		"alpha": {"bitDiameter": "3"},
		"mid":   {"bitDiameter": "4"},
	}
	got := p.Names()
	want := []string{"alpha", "mid", "zeta"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names() = %v, want %v", got, want)
		}
	}
}

func TestValuesTrimsAndCoversEveryField(t *testing.T) {
	in := DefaultValues()
	in["bitDiameter"] = "  6.0  "
	out := Values(in)
	if len(out) != len(Fields) {
		t.Errorf("Values() has %d keys, want %d", len(out), len(Fields))
	}
	if out["bitDiameter"] != "6.0" {
		t.Errorf("bitDiameter = %q, want %q", out["bitDiameter"], "6.0")
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// firstDiff reports the first differing line between got and want, which is
// far easier to read than a 60-line unified dump.
func firstDiff(got, want string) string {
	g := strings.Split(got, "\n")
	w := strings.Split(want, "\n")
	for i := 0; i < len(g) && i < len(w); i++ {
		if g[i] != w[i] {
			return "line " + itoa(i+1) + ":\n  got:  " + quote(g[i]) + "\n  want: " + quote(w[i])
		}
	}
	if len(g) != len(w) {
		return "line count: got " + itoa(len(g)) + ", want " + itoa(len(w))
	}
	return "(identical)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func quote(s string) string { return "\"" + s + "\"" }
