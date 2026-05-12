package preview

import "image/color"

// MatPalette is the surface/groove/shadow color triple for one
// material — mirrors gcodegen.py's MAT_PAL (line 610-618).
type MatPalette struct {
	Surface, Groove, Shadow color.RGBA
}

// MatPalettes is the per-material color palette table. The string
// keys match gen.MaterialPresets so the editor's preset choice can
// drive both feed/rpm settings and the preview's appearance.
var MatPalettes = map[string]MatPalette{
	"Wood":      mp(0xc8, 0xa0, 0x6a, 0x5c, 0x30, 0x10, 0x9a, 0x70, 0x40),
	"MDF":       mp(0xc0, 0xa8, 0x82, 0x6b, 0x4a, 0x2a, 0x9a, 0x7a, 0x52),
	"Aluminium": mp(0xb8, 0xc0, 0xcc, 0x50, 0x60, 0x6e, 0x88, 0x98, 0xa8),
	"Acrylic":   mp(0xcc, 0xe8, 0xff, 0x1a, 0x55, 0xaa, 0x7a, 0xb0, 0xe0),
	"Foam":      mp(0xe8, 0xe8, 0xe8, 0x90, 0x90, 0x90, 0xc0, 0xc0, 0xc0),
	"Brass":     mp(0xd4, 0xa8, 0x40, 0x7a, 0x58, 0x00, 0xa0, 0x78, 0x20),
	"Stone":     mp(0xa0, 0xa0, 0x98, 0x50, 0x50, 0x48, 0x78, 0x78, 0x70),
}

func mp(sr, sg, sb, gr, gg, gb, dr, dg, db uint8) MatPalette {
	return MatPalette{
		Surface: color.RGBA{R: sr, G: sg, B: sb, A: 0xff},
		Groove:  color.RGBA{R: gr, G: gg, B: gb, A: 0xff},
		Shadow:  color.RGBA{R: dr, G: dg, B: db, A: 0xff},
	}
}

// MaterialList is the in-display order for the dropdown — same as
// gcodegen.py:603 MATS.
var MaterialList = []string{"Wood", "MDF", "Aluminium", "Acrylic", "Foam", "Brass", "Stone"}

// PaletteFor returns the palette for `name` or the default Wood palette.
func PaletteFor(name string) MatPalette {
	if p, ok := MatPalettes[name]; ok {
		return p
	}
	return MatPalettes["Wood"]
}
