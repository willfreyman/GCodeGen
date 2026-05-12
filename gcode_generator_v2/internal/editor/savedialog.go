package editor

import (
	"fmt"
	"os"

	"github.com/sqweek/dialog"
)

// saveGCodeFile pops a native file-save dialog and writes contents
// to the chosen path. Returns the path used, "" on cancel, or error.
// Mirrors gcodegen.py:filedialog.asksaveasfilename + open(path,"w").
func saveGCodeFile(contents string) (string, error) {
	path, err := dialog.File().
		Filter("G-code", "nc").
		Filter("Text", "txt").
		Filter("All files", "*").
		Title("Save G-code as").
		Save()
	if err != nil {
		if err.Error() == "Cancelled" {
			return "", nil
		}
		return "", err
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// showError pops a native error dialog. Used by the editor when emit
// fails or save fails.
func showError(title, format string, args ...interface{}) {
	dialog.Message("%s", fmt.Sprintf(format, args...)).
		Title(title).
		Error()
}

// showInfo pops a native info dialog (used for save confirmations).
func showInfo(title, format string, args ...interface{}) {
	dialog.Message("%s", fmt.Sprintf(format, args...)).
		Title(title).
		Info()
}
