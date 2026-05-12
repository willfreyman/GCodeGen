package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// crashLogPath returns where the crash log goes — same dir as
// settings.json so users can find both when reporting an issue.
func crashLogPath(mode string) string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "GcodeGen")
	_ = os.MkdirAll(dir, 0o755)
	stamp := time.Now().Format("2006-01-02_150405")
	return filepath.Join(dir, fmt.Sprintf("crash_%s_%s.log", mode, stamp))
}

// RecoverAndLog wraps a goroutine or main entry. If the wrapped code
// panics, the panic value + stack are written to a crash log file and
// the process exits non-zero. The path is also printed to stderr for
// users who launched the binary from a terminal.
//
// Use as: defer RecoverAndLog("editor")
func RecoverAndLog(mode string) {
	r := recover()
	if r == nil {
		return
	}
	stack := debug.Stack()
	path := crashLogPath(mode)
	body := fmt.Sprintf("panic in %s: %v\n\n%s\n", mode, r, stack)
	_ = os.WriteFile(path, []byte(body), 0o644)
	fmt.Fprintln(os.Stderr, "===== gcodegen crashed =====")
	fmt.Fprintln(os.Stderr, body)
	fmt.Fprintln(os.Stderr, "log:", path)
	fmt.Fprintln(os.Stderr, "============================")
	os.Exit(2)
}
