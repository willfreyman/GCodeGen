package gen

import "image/color"

// Palette mirrors gcodegen.py's COLORS — 7 hex colors cycled through as
// strokes are finalized.
var Palette = []color.RGBA{
	{R: 0xe6, G: 0x39, B: 0x46, A: 0xff}, // #e63946
	{R: 0x2a, G: 0x9d, B: 0x8f, A: 0xff}, // #2a9d8f
	{R: 0xe9, G: 0xc4, B: 0x6a, A: 0xff}, // #e9c46a
	{R: 0x26, G: 0x46, B: 0x53, A: 0xff}, // #264653
	{R: 0xf4, G: 0xa2, B: 0x61, A: 0xff}, // #f4a261
	{R: 0xa8, G: 0xda, B: 0xdc, A: 0xff}, // #a8dadc
	{R: 0x45, G: 0x7b, B: 0x9d, A: 0xff}, // #457b9d
}

// MaterialPreset is the in-use subset of presets — feed_xy, feed_z, rpm
// plus the step-down depth in mm (gcodegen.py:MAT_MACHINE_PRESETS at
// line 335-343). StepDown is the per-pass plunge depth: a reasonable
// value for a 6 mm bit in each material, chosen conservatively so a
// novice doesn't snap a bit on first run.
type MaterialPreset struct {
	FeedXY, FeedZ, RPM, StepDown float64
}

// MaterialPresets is the keyed table the right-panel option-menu offers.
// Order matters — it's the display order used in the dropdown.
var MaterialPresets = []struct {
	Name   string
	Preset MaterialPreset
}{
	{"Wood", MaterialPreset{FeedXY: 900, FeedZ: 60, RPM: 12000, StepDown: 3.0}},
	{"MDF", MaterialPreset{FeedXY: 800, FeedZ: 50, RPM: 12000, StepDown: 2.5}},
	{"Aluminium", MaterialPreset{FeedXY: 180, FeedZ: 15, RPM: 12000, StepDown: 0.5}},
	{"Acrylic", MaterialPreset{FeedXY: 600, FeedZ: 40, RPM: 12000, StepDown: 1.5}},
	{"Foam", MaterialPreset{FeedXY: 800, FeedZ: 40, RPM: 12000, StepDown: 5.0}},
	{"Brass", MaterialPreset{FeedXY: 200, FeedZ: 17, RPM: 12000, StepDown: 0.3}},
	{"Stone", MaterialPreset{FeedXY: 120, FeedZ: 10, RPM: 10000, StepDown: 0.8}},
}

// PresetByName returns the preset for material `name`, or zero + false
// if unknown.
func PresetByName(name string) (MaterialPreset, bool) {
	for _, m := range MaterialPresets {
		if m.Name == name {
			return m.Preset, true
		}
	}
	return MaterialPreset{}, false
}

// ApplyPreset writes the preset's feed/rpm into the editor's machine
// settings. Mirrors gcodegen.py:_apply_material_preset.
func (e *Editor) ApplyPreset(name string) bool {
	p, ok := PresetByName(name)
	if !ok {
		return false
	}
	e.Machine.FeedXY = p.FeedXY
	e.Machine.FeedZ = p.FeedZ
	e.Machine.RPM = p.RPM
	e.Machine.StepDown = p.StepDown
	return true
}
