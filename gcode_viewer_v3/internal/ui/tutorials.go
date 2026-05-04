package ui

import (
	"embed"
	"fmt"
)

// Tutorial .nc files are baked into the binary so first-time users have
// runnable programs to load from the toolbar without hunting for files.
// Listed in display order; the leading number in the filename matches the
// number in DisplayName so the dropdown reads as a curriculum.
//
//go:embed tutorials/*.nc
var tutorialsFS embed.FS

// Tutorial pairs a friendly menu label with the embedded source file.
type Tutorial struct {
	DisplayName string
	File        string // path inside tutorialsFS, e.g. "tutorials/01_basic_square.nc"
}

// Tutorials is the master list rendered into the dropdown. Order here
// is the order the buttons appear.
var Tutorials = []Tutorial{
	{"01: Basic Square Outline", "tutorials/01_basic_square.nc"},
	{"02: Pocket Clearing", "tutorials/02_pocket_clear.nc"},
	{"03: Arc + Circle (G2/G3)", "tutorials/03_arc_circle.nc"},
	{"04: Layered Pyramid", "tutorials/04_layered_pyramid.nc"},
	{"05: Through-Cut Profile", "tutorials/05_through_cutout.nc"},
	{"06: Mixed Operations", "tutorials/06_complex_motion.nc"},
}

// LoadTutorialBytes reads the embedded .nc file for the given DisplayName.
// Returns an error only if the embed entry is missing — which would mean
// the binary was built without the tutorials/*.nc files (broken build).
func LoadTutorialBytes(displayName string) ([]byte, error) {
	for _, t := range Tutorials {
		if t.DisplayName == displayName {
			data, err := tutorialsFS.ReadFile(t.File)
			if err != nil {
				return nil, fmt.Errorf("read embedded %s: %w", t.File, err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("unknown tutorial %q", displayName)
}
