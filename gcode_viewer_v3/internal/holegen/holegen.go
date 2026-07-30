// Package holegen generates CNC G-code for milling a grid of round holes
// into metal tube (sized for FRC / MAXTube robotics stock).
//
// Implemented to HoleGen_SPEC.md. The package is deliberately pure — only
// the Go standard library, no g3n / UI imports — so the whole thing is
// unit-testable headless and the §7.7 reference fixture can be asserted
// byte-for-byte (see holegen_test.go).
//
// Machine setup the generated program assumes (spec §2):
//
//	X = 0  the endmill EDGE just touching the tube's side face, so the
//	       tube's left face sits at X = bitDiameter/2
//	Y = 0  the exact center of the first row of holes
//	Z = 0  the top surface of the material, +Z up, cuts go negative
//
// Output is metric absolute XY-plane (G21 G90 G17) throughout.
package holegen

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MMPerInch is the inch→mm factor used by the per-field unit override.
const MMPerInch = 25.4

// Field describes one of the 12 input parameters. Order matters: it is both
// the on-screen top-to-bottom order and the iteration order used for
// reading, saving, and generating. Label text is user-facing and must stay
// verbatim (spec §3).
type Field struct {
	Key     string // stable identifier, also the JSON key in presets
	Label   string // exact on-screen text
	Default string // initial entry text
	IsInt   bool   // true => integer field, false => float
}

// Fields is the ordered parameter list (spec §3).
var Fields = []Field{
	{"bitDiameter", "Bit diameter (mm)", "6.0", false},
	{"targetHoleDiameter", "Target hole diameter (mm)", "28.57", false},
	{"xOffset", "X offset from tube edge to first column (mm)", "10.0", false},
	{"holeSpacingX", "Horizontal spacing between holes, X (mm)", "50.8", false},
	{"holeSpacingY", "Vertical spacing between holes, Y (mm)", "50.8", false},
	{"rowCount", "Number of rows (Y)", "2", true},
	{"columnCount", "Number of columns (X)", "2", true},
	{"spindleSpeed", "Spindle RPM (max 24000)", "18000", true},
	{"verticalFeedrate", "Helical plunge vertical feedrate (Z)", "150", true},
	{"horizontalFeedrate", "Horizontal circular feedrate (XY)", "600", true},
	{"tubeThickness", "Metal thickness (mm)", "3.0", false},
	{"pitchPerTurn", "Helical pitch, Z drop per 360 (mm)", "1.0", false},
}

// DiaPreset is one entry in the target-hole-diameter quick-fill dropdown.
type DiaPreset struct{ Value, Name string }

// DiameterPresets are the named diameters offered next to the
// targetHoleDiameter entry (spec §5). Selecting one writes Value into the
// entry; the entry stays freely editable.
var DiameterPresets = []DiaPreset{
	{"6.0", "6 mm hole"},
	{"12.7", `1/2" shaft`},
	{"28.585", "bearing hole"},
	{"50.8", `2" hole`},
}

// Option renders a preset as its dropdown label, e.g. "28.585 — bearing
// hole". The separator is an em-dash (U+2014) per spec §5.
func (d DiaPreset) Option() string {
	return fmt.Sprintf("%s — %s", d.Value, d.Name)
}

// DiameterPresetByOption maps a dropdown label back to its preset. Returns
// ok=false when the string doesn't match any known option.
func DiameterPresetByOption(option string) (DiaPreset, bool) {
	for _, p := range DiameterPresets {
		if p.Option() == option {
			return p, true
		}
	}
	return DiaPreset{}, false
}

// Params is the parsed, validated parameter set. Integer fields are held as
// int because rowCount/columnCount are loop bounds and the feedrates/RPM are
// emitted with %d.
type Params struct {
	BitDiameter        float64
	TargetHoleDiameter float64
	XOffset            float64
	HoleSpacingX       float64
	HoleSpacingY       float64
	RowCount           int
	ColumnCount        int
	SpindleSpeed       int
	VerticalFeedrate   int
	HorizontalFeedrate int
	TubeThickness      float64
	PitchPerTurn       float64
}

// DefaultValues returns the raw entry strings every field starts at, keyed
// by Field.Key. Used to seed the UI and as the fallback when a preset omits
// a key.
func DefaultValues() map[string]string {
	v := make(map[string]string, len(Fields))
	for _, f := range Fields {
		v[f.Key] = f.Default
	}
	return v
}

// ParseMeasurement parses one field's raw text, honouring an optional
// per-field unit suffix (spec §4). Inches are converted to mm for that
// field only; no suffix means millimetres.
//
// Suffix precedence is inch-mark, then "in", then "mm" — checked in that
// order so `1.125"`, `1.125in` and `1.125 in` all yield 28.575.
//
// Integer fields are rounded with math.Round, which rounds halves away from
// zero (Python's round() used banker's rounding). That only differs on exact
// x.5 inputs, which don't occur for any realistic field value nor in the
// §7.7 fixture — away-from-zero is the expected behaviour here.
func ParseMeasurement(raw string, isInt bool) (float64, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	inches := false

	switch {
	case strings.HasSuffix(s, `"`):
		s = strings.TrimSpace(s[:len(s)-1])
		inches = true
	case strings.HasSuffix(s, "in"):
		s = strings.TrimSpace(s[:len(s)-2])
		inches = true
	case strings.HasSuffix(s, "mm"):
		s = strings.TrimSpace(s[:len(s)-2])
	}

	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if inches {
		value *= MMPerInch
	}
	if isInt {
		value = math.Round(value)
	}
	return value, nil
}

// ReadParams parses every field from its raw entry text into Params
// (spec §11). values is keyed by Field.Key; a missing key is treated as
// empty text and therefore fails to parse.
//
// Only two rules are enforced: every field must parse, and rowCount /
// columnCount must each be at least 1. There is deliberately no RPM cap
// (the "(max 24000)" in the label is advisory only), no non-negative check
// on feedrates or spacings, and no upper bound on anything — the generator
// enforces the one remaining rule (cutRadius >= 0) itself.
func ReadParams(values map[string]string) (Params, error) {
	var p Params
	parsed := make(map[string]float64, len(Fields))

	for _, f := range Fields {
		v, err := ParseMeasurement(strings.TrimSpace(values[f.Key]), f.IsInt)
		if err != nil {
			typeName := "float"
			if f.IsInt {
				typeName = "int"
			}
			return Params{}, fmt.Errorf(
				"'%s' must be a valid %s (optionally suffixed with 'mm' or 'in', e.g. 1.125in).",
				f.Label, typeName)
		}
		parsed[f.Key] = v
	}

	p.BitDiameter = parsed["bitDiameter"]
	p.TargetHoleDiameter = parsed["targetHoleDiameter"]
	p.XOffset = parsed["xOffset"]
	p.HoleSpacingX = parsed["holeSpacingX"]
	p.HoleSpacingY = parsed["holeSpacingY"]
	p.RowCount = int(parsed["rowCount"])
	p.ColumnCount = int(parsed["columnCount"])
	p.SpindleSpeed = int(parsed["spindleSpeed"])
	p.VerticalFeedrate = int(parsed["verticalFeedrate"])
	p.HorizontalFeedrate = int(parsed["horizontalFeedrate"])
	p.TubeThickness = parsed["tubeThickness"]
	p.PitchPerTurn = parsed["pitchPerTurn"]

	if p.RowCount < 1 || p.ColumnCount < 1 {
		return Params{}, fmt.Errorf("Rows and columns must each be at least 1.")
	}
	// Beyond the spec's two rules — see ErrPitchNotPositive and MaxHoles for
	// why. Enforcing them HERE (not only in the generator) matters because the
	// live estimate calls ReadParams on every keystroke: it's what stops a
	// half-typed row count from allocating its way to an out-of-memory kill
	// before Generate is ever clicked.
	if p.PitchPerTurn <= 0 {
		return Params{}, fmt.Errorf("'%s' must be greater than zero.", fieldLabel("pitchPerTurn"))
	}
	if err := checkHoleCount(p); err != nil {
		return Params{}, err
	}
	return p, nil
}

// fieldLabel returns a field's on-screen label for use in error messages, so
// the text points at the box the user is actually looking at.
func fieldLabel(key string) string {
	for _, f := range Fields {
		if f.Key == key {
			return f.Label
		}
	}
	return key
}
