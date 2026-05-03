// Package ui owns the g3n window, top-level scene wiring, and OS-level
// dialogs (file open / message boxes via sqweek/dialog).
package ui

import (
	"fmt"

	"github.com/sqweek/dialog"
)

// OpenGCodeFile pops a native file-open dialog filtered for common G-code
// extensions and returns the selected absolute path.
//
// Returns an empty string and a nil error if the user cancels — callers
// should branch on `path == ""` rather than checking for a specific cancel
// sentinel error, since sqweek's `dialog.Cancelled` is package-private to
// the underlying platform code in some versions.
func OpenGCodeFile() (string, error) {
	// Common G-code extensions across the CAM / CNC / 3D-printer ecosystem.
	// Source order matters: first match becomes the default filter.
	//   .nc     near-universal CAM output
	//   .gcode  RepRap / 3D printer convention; also some CAM
	//   .ngc    LinuxCNC, GRBL
	//   .tap    Mach3 default
	//   .cnc    generic CAM
	//   .gco    3D printer slicers (Cura, etc.)
	//   .g      older / generic
	//   .mpf    Siemens controllers
	//   .nci    Mastercam intermediate
	//   .tab    Mach3 alternative
	//   .eia    older paper-tape controllers
	//   .dnc    direct-numerical-control transfers
	//   .txt    a lot of CAM tools dump as plain .txt
	path, err := dialog.File().
		Filter("G-code",
			"nc", "gcode", "ngc", "tap", "cnc", "gco",
			"g", "mpf", "nci", "tab", "eia", "dnc", "txt",
		).
		Filter("All files", "*").
		Title("Open G-code file").
		Load()

	if err != nil {
		// sqweek/dialog returns dialog.Cancelled for user cancel. We don't
		// import the sentinel directly to keep the surface minimal — match
		// on the standard error string instead.
		if err.Error() == "Cancelled" {
			return "", nil
		}
		return "", err
	}
	return path, nil
}

// ShowError displays a modal error dialog with the given title and a
// printf-formatted message.
func ShowError(title, format string, args ...interface{}) {
	dialog.Message("%s", fmt.Sprintf(format, args...)).
		Title(title).
		Error()
}
