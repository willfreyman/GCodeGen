package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DiagLog appends one timestamped line to a per-mode diagnostic file in
// the user's config dir. Used to localize crashes that bypass Go's
// recover() — runtime fatal errors, native (cgo) segfaults, or process
// kills — by writing a heartbeat log of what the program was doing
// just before death.
//
// Cheap enough to call frequently (once per ebiten tick is fine; we
// keep the file open for the process lifetime via a sync.Once).
//
// Diagnostic logs auto-rotate per launch — each Run() should call
// DiagInit(mode) once at startup which truncates any prior log so the
// file always reflects only the most recent session.
var (
	diagFile    *os.File
	diagPath    string
	diagOnce    sync.Once
	diagMu      sync.Mutex
	diagEnabled = os.Getenv("GCODEGEN_DIAG") != "0"
)

// DiagInit truncates and reopens the diagnostic log for the current
// process. Called once at the top of each Run() so each launch gets a
// fresh file.
func DiagInit(mode string) {
	if !diagEnabled {
		return
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "GcodeGen")
	_ = os.MkdirAll(dir, 0o755)
	diagPath = filepath.Join(dir, "diag_"+mode+".log")
	f, err := os.Create(diagPath)
	if err != nil {
		return
	}
	diagFile = f
	// Mirror runtime fatal errors (concurrent map access etc.) and any
	// native crash output into the diag file by redirecting stderr.
	// Without this, those errors silently disappear when the user
	// double-clicks the .exe (no console attached).
	if err := redirectStderr(diagPath + ".stderr"); err != nil {
		DiagLog("redirect stderr: %v", err)
	}
	DiagLog("=== %s start (pid=%d) ===", mode, os.Getpid())
}

// DiagLog writes a timestamped line. Safe to call from multiple
// goroutines.
func DiagLog(format string, args ...any) {
	if !diagEnabled || diagFile == nil {
		return
	}
	diagMu.Lock()
	defer diagMu.Unlock()
	stamp := time.Now().Format("15:04:05.000")
	fmt.Fprintf(diagFile, "%s "+format+"\n", append([]any{stamp}, args...)...)
	_ = diagFile.Sync() // flush every line — survives a hard crash
}

// FreshDiagLog tells DiagInit's once-only guard to fire on next call.
// Currently unused; reserved for tests.
func FreshDiagLog() { diagOnce = sync.Once{} }

// DiagPath returns where the diag log lives (empty if disabled).
func DiagPath() string { return diagPath }
