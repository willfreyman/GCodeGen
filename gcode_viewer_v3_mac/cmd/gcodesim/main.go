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
	"fmt"
	"os"

	"gcodegen.local/viewer/internal/ui"
)

func main() {
	registerFlag := flag.Bool("register-file-types", false,
		"Windows: register .nc/.gcode/etc with this exe so double-click opens in GcodeSim, then exit.")
	unregisterFlag := flag.Bool("unregister-file-types", false,
		"Windows: undo --register-file-types and exit.")
	flag.Parse()

	switch {
	case *registerFlag:
		if err := ui.RegisterFileTypes(); err != nil {
			fmt.Fprintln(os.Stderr, "register failed:", err)
			os.Exit(1)
		}
		fmt.Println("Registered G-code file types with GcodeSim. Double-click any .nc/.gcode/.ngc/.tap/etc to open.")
		return
	case *unregisterFlag:
		if err := ui.UnregisterFileTypes(); err != nil {
			fmt.Fprintln(os.Stderr, "unregister failed:", err)
			os.Exit(1)
		}
		fmt.Println("Unregistered G-code file types from GcodeSim.")
		return
	}

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
