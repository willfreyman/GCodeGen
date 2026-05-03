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
	path, err := dialog.File().
		Filter("G-code", "nc", "gcode", "tap", "txt").
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
