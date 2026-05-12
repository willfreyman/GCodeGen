package editor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"gcodegen.local/generator/internal/gen"
)

// childProc holds a spawned aux subprocess plus its stdin pipe. The
// editor sends UpdateMessage JSON lines to stdin; the child parses
// them in its own goroutine and re-renders.
type childProc struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	mu  sync.Mutex // serializes writes to in
}

// spawnAux launches one of the aux subprocesses (mode = "sim" or
// "preview"). Returns a childProc handle, or nil + error if launch
// fails.
func spawnAux(mode string) (*childProc, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate self: %w", err)
	}
	cmd := exec.Command(exe, mode)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		pipe.Close()
		return nil, fmt.Errorf("start %s: %w", mode, err)
	}
	return &childProc{cmd: cmd, in: pipe}, nil
}

// send serializes m as one JSON line and writes it. Errors are silent
// because a closed-pipe failure just means the child went away — the
// caller's wait goroutine will clear the handle.
func (cp *childProc) send(m gen.UpdateMessage) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	_ = gen.EncodeMessage(cp.in, m)
}

// close politely shuts down the child by closing its stdin (the child
// sees EOF and exits) then waits.
func (cp *childProc) close() {
	if cp.in != nil {
		_ = cp.in.Close()
	}
	if cp.cmd != nil && cp.cmd.Process != nil {
		_ = cp.cmd.Wait()
	}
}

// onSimulateClicked launches the sim subprocess and immediately sends
// a state snapshot. If sim is already running, brings it forward by
// sending a fresh snapshot.
func (g *Game) onSimulateClicked() {
	if g.simProc == nil {
		cp, err := spawnAux("sim")
		if err != nil {
			showError("Simulate", "could not start sim subprocess: %v", err)
			return
		}
		g.simProc = cp
		go g.waitForExit("sim", cp, &g.simProc)
	}
	g.simProc.send(g.editor.SnapshotState())
}

// onPreviewClicked launches the preview subprocess.
func (g *Game) onPreviewClicked() {
	if g.previewProc == nil {
		cp, err := spawnAux("preview")
		if err != nil {
			showError("Preview", "could not start preview subprocess: %v", err)
			return
		}
		g.previewProc = cp
		go g.waitForExit("preview", cp, &g.previewProc)
	}
	g.previewProc.send(g.editor.SnapshotState())
}

// waitForExit blocks on cmd.Wait then atomically clears the matching
// Game pointer so the next click respawns. mode is just for logging.
func (g *Game) waitForExit(mode string, cp *childProc, target **childProc) {
	if err := cp.cmd.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "%s subprocess exited: %v\n", mode, err)
	}
	g.procMu.Lock()
	*target = nil
	g.procMu.Unlock()
}

// broadcastState sends the current snapshot to whichever aux
// subprocesses are running. Called from the main game loop every few
// frames; the JSON encode + pipe write is cheap and the children
// drop intermediate updates by overwriting their atomic state ptr.
func (g *Game) broadcastState() {
	g.procMu.Lock()
	sim, preview := g.simProc, g.previewProc
	g.procMu.Unlock()
	if sim == nil && preview == nil {
		return
	}
	snap := g.editor.SnapshotState()
	if sim != nil {
		sim.send(snap)
	}
	if preview != nil {
		preview.send(snap)
	}
}

// shutdownAux closes both subprocesses cleanly. Called when the
// editor window closes.
func (g *Game) shutdownAux() {
	g.procMu.Lock()
	sim, preview := g.simProc, g.previewProc
	g.simProc, g.previewProc = nil, nil
	g.procMu.Unlock()
	if sim != nil {
		sim.close()
	}
	if preview != nil {
		preview.close()
	}
}
