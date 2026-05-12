// Package editor implements the main Ebiten game for the G-code editor —
// a left 700×580 freehand sketch canvas plus a right control panel built
// with ebitenui. The editor is a port of gcodegen.py's Tkinter UI; see
// the plan at /home/will/.claude/plans/look-at-how-gcode-spicy-boole.md.
package editor

import (
	"fmt"
	"os"
	"sync"

	"gcodegen.local/generator/internal/gen"
	"gcodegen.local/generator/internal/shared"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

const (
	WindowWidth  = 1024
	WindowHeight = 600
	WindowTitle  = "GcodeGen — Toolpath Editor"
	PanelX       = gen.CanvasWidth
	PanelWidth   = WindowWidth - gen.CanvasWidth
)

// Game owns the editor state and dispatches input/render each frame.
type Game struct {
	editor *gen.Editor
	canvas *ebiten.Image
	// viewport is the 700x580 buffer the canvas image is blitted into
	// after applying the zoom/pan transform. Drawing through a fixed-
	// size viewport gives us free clipping — zoomed-in content that
	// overflows the work-area rect simply doesn't reach the screen.
	viewport *ebiten.Image
	zoom     float64 // canvas zoom factor; 1.0 = 1 model px → 1 screen px
	panX     float64 // canvas pan offset in screen pixels (relative to viewport origin)
	panY     float64
	ui       *ebitenui.UI

	// Live widget refs touched by callbacks outside their construction
	// site (e.g. preset application has to update the machine inputs;
	// the global Depth field has to refresh the perim depth input).
	safeZInput      *widget.TextInput
	feedXYInput     *widget.TextInput
	feedZInput      *widget.TextInput
	rpmInput        *widget.TextInput
	stepDownInput   *widget.TextInput
	perimDepthInput *widget.TextInput

	opListContainer   *widget.Container
	opListInner       *widget.Container
	refreshOpListFace *text.Face
	// opRows mirrors Strokes after each refreshOpList — kept so the
	// selection-highlight update can mutate just the background of the
	// affected rows instead of rebuilding the whole list (which would
	// destroy any in-progress depth-input typing).
	opRows []*widget.Container

	// Edit-selected status text refreshed when SelectedIdx changes.
	selectedStatus *widget.Text

	// Bookkeeping for cross-frame state changes that should trigger an
	// op-list rebuild.
	prevStrokeCount int
	prevSelectedIdx int
	strokesDirty    bool

	// Aux subprocesses. procMu serializes access to the pointers
	// because the wait-for-exit goroutine clears them.
	procMu      sync.Mutex
	simProc     *childProc
	previewProc *childProc
	frameTick   int

	// Image-tracing state. See internal/editor/image.go and
	// internal/img.
	image *imageState
}

// NewGame returns a freshly-initialized Game with default editor state
// and a built UI tree.
func NewGame() *Game {
	g := &Game{
		editor:          gen.NewEditor(),
		canvas:          ebiten.NewImage(gen.CanvasWidth, gen.CanvasHeight),
		viewport:        ebiten.NewImage(gen.CanvasWidth, gen.CanvasHeight),
		image:           newImageState(),
		prevSelectedIdx: -1,
		zoom:            1.0,
	}
	g.ui = g.buildUI()
	return g
}

func (g *Game) Update() error {
	g.frameTick++
	g.handleInput()
	if g.ui != nil {
		g.ui.Update()
	}
	g.rebuildImagePreviewIfDirty()
	if len(g.editor.Strokes) != g.prevStrokeCount || g.strokesDirty {
		g.refreshOpList()
		g.refreshSelectedSection()
		g.prevStrokeCount = len(g.editor.Strokes)
		g.strokesDirty = false
	}
	if g.editor.SelectedIdx != g.prevSelectedIdx {
		g.refreshSelectedSection()
		// Update only the row backgrounds — a full refreshOpList()
		// would destroy any depth-input the user is mid-typing into.
		g.updateRowHighlights()
		g.prevSelectedIdx = g.editor.SelectedIdx
	}
	if g.frameTick%3 == 0 {
		g.broadcastState()
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(bgPanel)
	drawCanvas(g.canvas, g)
	// Project the canvas through the zoom/pan transform into the
	// fixed-size viewport so overflow from zoomed-in content is
	// clipped to the work-area rect (not bleeding into the bottom
	// strip below the canvas or under the panel before the UI draws).
	g.viewport.Fill(bgPanel)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(g.zoom, g.zoom)
	op.GeoM.Translate(g.panX, g.panY)
	g.viewport.DrawImage(g.canvas, op)
	screen.DrawImage(g.viewport, nil)
	if g.ui != nil {
		g.ui.Draw(screen)
	}
}

// zoomAroundCursor zooms by `factor` while keeping the model point
// under the mouse cursor visually fixed — the "zoom toward where I'm
// pointing" feel. Used by Ctrl+wheel.
func (g *Game) zoomAroundCursor(factor float64) {
	cx, cy := ebiten.CursorPosition()
	g.zoomAroundPoint(factor, float64(cx), float64(cy))
}

// zoomAroundPoint is the underlying zoom: scale g.zoom by `factor`
// and adjust the pan so the screen point (sx, sy) maps to the same
// model coordinate before and after. Clamped to [0.5, 4.0].
func (g *Game) zoomAroundPoint(factor, sx, sy float64) {
	newZoom := g.zoom * factor
	if newZoom < 0.5 {
		newZoom = 0.5
	}
	if newZoom > 4.0 {
		newZoom = 4.0
	}
	if newZoom == g.zoom {
		return
	}
	ratio := newZoom / g.zoom
	g.panX = sx - (sx-g.panX)*ratio
	g.panY = sy - (sy-g.panY)*ratio
	g.zoom = newZoom
}

// zoomInButton / zoomOutButton zoom centered on the canvas viewport
// midpoint — the right pivot for button-driven zoom (the user's
// cursor is on the panel, far from the canvas, so cursor-anchored
// zoom would feel disconnected).
func (g *Game) zoomInButton() {
	g.zoomAroundPoint(1.25, gen.CanvasWidth/2, gen.CanvasHeight/2)
}
func (g *Game) zoomOutButton() {
	g.zoomAroundPoint(1.0/1.25, gen.CanvasWidth/2, gen.CanvasHeight/2)
}

// resetView snaps the zoom + pan back to defaults.
func (g *Game) resetView() {
	g.zoom = 1.0
	g.panX = 0
	g.panY = 0
}

func (g *Game) Layout(_, _ int) (int, int) { return WindowWidth, WindowHeight }

// Run boots the editor window and blocks until it closes.
func Run() {
	shared.DiagInit("editor")
	defer shared.RecoverAndLog("editor")
	fmt.Fprintln(os.Stderr, "gcodegen: starting editor")
	game := NewGame()
	checkForUpdates()
	// Auto-open the finished-product preview window (mirrors
	// gcodegen.py:834 — window.after(120, open_preview)).
	if cp, err := spawnAux("preview"); err == nil {
		game.previewProc = cp
		go game.waitForExit("preview", cp, &game.previewProc)
		cp.send(game.editor.SnapshotState())
	}
	ebiten.SetWindowSize(WindowWidth, WindowHeight)
	ebiten.SetWindowTitle(WindowTitle)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	if err := ebiten.RunGame(game); err != nil {
		fmt.Fprintln(os.Stderr, "editor exited:", err)
	}
	game.shutdownAux()
}
