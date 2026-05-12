// gcodegen is the GcodeGenV1 entry point — a Go port of the Tkinter
// editor (gcodegen.py). One binary dispatches into three Ebiten windows:
//
//	gcodegen           # editor (main window)
//	gcodegen sim       # toolpath simulation subprocess (stdin = JSON)
//	gcodegen preview   # finished-product preview subprocess (stdin = JSON)
//
// The editor spawns the two aux subprocesses via os/exec and writes
// newline-delimited UpdateMessage JSON records to each stdin on every
// state change. See ../../README.md for design notes.
package main

import (
	"fmt"
	"os"

	"gcodegen.local/generator/internal/editor"
	"gcodegen.local/generator/internal/preview"
	"gcodegen.local/generator/internal/sim"
)

func main() {
	mode := ""
	if len(os.Args) >= 2 {
		mode = os.Args[1]
	}

	switch mode {
	case "":
		editor.Run()
	case "sim":
		sim.Run()
	case "preview":
		preview.Run()
	default:
		fmt.Fprintf(os.Stderr, "gcodegen: unknown mode %q (expected: <empty> | sim | preview)\n", mode)
		os.Exit(2)
	}
}
