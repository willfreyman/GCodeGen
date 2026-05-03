// gcodesim is the GcodeSimV3 entry point — a Go port of gcode_viewer_v2
// using the g3n engine. See ../../README.md for design notes.
//
// Usage:
//
//	gcodesim                  # opens an empty window; load with Ctrl+O
//	gcodesim path/to/file.nc  # opens that file on startup
//
// Keys:
//
//	Ctrl+O   open .nc file
//	R        reframe camera to fit the model
//	Esc      quit
package main

import (
	"flag"

	"gcodegen.local/viewer/internal/ui"
)

func main() {
	flag.Parse()
	var initialPath string
	if flag.NArg() >= 1 {
		initialPath = flag.Arg(0)
	} else if path := ui.CapturedOpenFile(); path != "" {
		// macOS: Launch Services delivered a file via Apple Event
		// (double-clicked .nc, dragged onto app icon, Open With...).
		// On non-Mac builds CapturedOpenFile() always returns "".
		initialPath = path
	}
	ui.Run(initialPath)
}
