//go:build !windows

package shared

import "os"

// redirectStderr swaps the process's stderr to a file. On non-Windows
// this is a syscall.Dup2 dance; we only ship on Windows + macOS, so
// the macOS variant lives here as a best-effort no-op for now (Go's
// portable file-descriptor manipulation requires syscall.Dup2 which
// is platform-specific). Crash logs from native errors will be lost
// on macOS until this gets a proper implementation.
func redirectStderr(path string) error {
	_ = path
	_ = os.Stderr
	return nil
}
