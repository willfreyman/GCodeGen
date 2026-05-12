//go:build windows

package shared

import (
	"os"

	"golang.org/x/sys/windows"
)

// redirectStderr replaces the process's stderr file descriptor with
// one pointing at the given path. This captures runtime fatal errors
// like "fatal error: concurrent map writes" and native (purego /
// OpenGL driver) crash output — neither of which goes through Go's
// panic mechanism, so they bypass recover() and would otherwise be
// invisible when the user double-clicks the .exe (no console).
//
// We also reassign os.Stderr so any in-process Fprintln calls land
// in the same file.
func redirectStderr(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd())); err != nil {
		_ = f.Close()
		return err
	}
	os.Stderr = f
	return nil
}
