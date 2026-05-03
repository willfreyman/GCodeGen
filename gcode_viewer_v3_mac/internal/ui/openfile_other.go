//go:build !darwin

package ui

// CapturedOpenFile is the no-op stub for non-macOS platforms. Windows /
// Linux pass file paths via command-line arguments instead of Apple
// Events, so the launcher main.go handles those paths via os.Args
// directly.
func CapturedOpenFile() string { return "" }
