//go:build !windows

package ui

import "errors"

// RegisterFileTypes / UnregisterFileTypes are no-ops on non-Windows
// platforms. Mac uses Info.plist + lsregister (handled by build.sh);
// Linux uses xdg-mime (which we haven't wired up yet).
func RegisterFileTypes() error {
	return errors.New("file-type registration is Windows-only on this build")
}

func UnregisterFileTypes() error {
	return errors.New("file-type registration is Windows-only on this build")
}
