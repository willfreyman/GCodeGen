// Package sim implements the toolpath simulation subprocess: a 740×670
// Ebiten window that renders ghost paths plus an animated toolhead under
// the parent editor's control. Reads UpdateMessage JSON lines from stdin.
//
// Mirrors gcodegen.py:open_simulation (line 411-561).
package sim

import (
	"bytes"
	"fmt"
	"image/color"
	"math"
	"os"
	"sync"
	"sync/atomic"

	"gcodegen.local/generator/internal/assets"
	"gcodegen.local/generator/internal/gen"
	"gcodegen.local/generator/internal/shared"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	WindowWidth  = 740
	WindowHeight = 700 // 580 sim + ~120 controls (header + status + progress + buttons)
	WindowTitle  = "GcodeGen — Toolpath Simulation"
	SimW         = 740
	SimH         = 580
)

var (
	bgWindow  = color.RGBA{R: 0x0d, G: 0x0d, B: 0x1a, A: 0xff}
	bgSim     = color.RGBA{R: 0x08, G: 0x08, B: 0x1a, A: 0xff}
	gridDark  = color.RGBA{R: 0x13, G: 0x13, B: 0x2a, A: 0xff}
	ghostClr  = color.RGBA{R: 0x1e, G: 0x1e, B: 0x44, A: 0xff}
	perimClr  = color.RGBA{R: 0x2a, G: 0x2a, B: 0x55, A: 0xff}
	statusBg  = color.RGBA{R: 0x55, G: 0x55, B: 0x66, A: 0xff}
	statusOk  = color.RGBA{R: 0x00, G: 0xff, B: 0x88, A: 0xff}
	statusOn  = color.RGBA{R: 0x00, G: 0xd4, B: 0xff, A: 0xff}
	headFill  = color.RGBA{R: 0x00, G: 0xd4, B: 0xff, A: 0xff}
	rapidClr  = color.RGBA{R: 0xff, G: 0x88, B: 0x00, A: 0xff}
	headWhite = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

// Game is the Ebiten game for the simulation subprocess.
type Game struct {
	mu       sync.Mutex
	state    State
	pending  atomic.Pointer[gen.UpdateMessage]
	trails   *ebiten.Image // persistent: cumulative cuts + rapids
	ghosts   *ebiten.Image // pre-rendered ghost paths layer
	ui       *ebitenui.UI
	statusW  *widget.Text
	progress *widget.ProgressBar
	playBtn  *widget.Button
	speed    *widget.Slider
	font     *text.GoTextFace
	monoFont *text.GoTextFace
}

// Run boots the sim window and blocks until it closes.
func Run() {
	defer shared.RecoverAndLog("sim")
	fmt.Fprintln(os.Stderr, "gcodegen: starting sim subprocess")
	game := newGame()
	go func() {
		err := gen.ReadMessages(os.Stdin, func(m gen.UpdateMessage) {
			cp := m
			game.pending.Store(&cp)
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "sim: stdin reader exited:", err)
		}
		os.Exit(0)
	}()
	ebiten.SetWindowSize(WindowWidth, WindowHeight)
	ebiten.SetWindowTitle(WindowTitle)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	if err := ebiten.RunGame(game); err != nil {
		fmt.Fprintln(os.Stderr, "sim exited:", err)
		os.Exit(1)
	}
}

func newGame() *Game {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(assets.FreeSansBoldTTF))
	if err != nil {
		fmt.Fprintln(os.Stderr, "sim: font load:", err)
	}
	g := &Game{
		state:  State{Speed: 6},
		trails: ebiten.NewImage(SimW, SimH),
		ghosts: ebiten.NewImage(SimW, SimH),
	}
	if src != nil {
		g.font = &text.GoTextFace{Source: src, Size: 12}
		g.monoFont = &text.GoTextFace{Source: src, Size: 11}
	}
	g.ui = g.buildUI()
	return g
}

// Update advances state from incoming IPC messages and ticks playback.
func (g *Game) Update() error {
	if msg := g.pending.Swap(nil); msg != nil {
		g.applyMessage(*msg)
	}
	if g.ui != nil {
		g.ui.Update()
	}
	cuts, rapids := g.state.step()
	if len(cuts) > 0 || len(rapids) > 0 {
		g.paintToTrails(cuts, rapids)
	}
	g.refreshStatusAndProgress()
	return nil
}

// applyMessage swaps in a new state. Triggers a playback reset only on
// structural change so steady 20 Hz update bursts don't restart playback.
func (g *Game) applyMessage(m gen.UpdateMessage) {
	g.mu.Lock()
	defer g.mu.Unlock()
	changed := g.state.loadFromMessage(m)
	if changed {
		g.state.reset()
		g.rebuildGhosts()
		g.trails.Clear()
	}
}

// rebuildGhosts redraws the ghost-paths layer from the current ops list.
func (g *Game) rebuildGhosts() {
	g.ghosts.Clear()
	for _, op := range g.state.Ops {
		for i := 0; i+1 < len(op.Pts); i++ {
			vector.StrokeLine(g.ghosts,
				float32(op.Pts[i][0]), float32(op.Pts[i][1]),
				float32(op.Pts[i+1][0]), float32(op.Pts[i+1][1]),
				1, ghostClr, false)
		}
	}
	if g.state.HasPerim {
		drawDashedRect(g.ghosts,
			g.state.PerimL, g.state.PerimT,
			g.state.PerimR, g.state.PerimB,
			4, 4, 1, perimClr)
	}
}

// paintToTrails appends new segments onto the persistent trails image.
func (g *Game) paintToTrails(cuts, rapids []segment) {
	for _, c := range cuts {
		vector.StrokeLine(g.trails,
			float32(c.X0), float32(c.Y0), float32(c.X1), float32(c.Y1),
			2, c.Clr, true)
	}
	for _, r := range rapids {
		drawDashedLine(g.trails, r.X0, r.Y0, r.X1, r.Y1, 2, 6, 1, r.Clr)
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(bgWindow)
	// Sim canvas region starts at (0, 40) — header occupies top.
	canvasOriginY := float32(40)
	// Sim background.
	vector.DrawFilledRect(screen, 0, canvasOriginY, SimW, SimH, bgSim, false)
	// Faint grid.
	for x := float32(0); x < SimW; x += 20 {
		vector.StrokeLine(screen, x, canvasOriginY, x, canvasOriginY+SimH, 1, gridDark, false)
	}
	for y := float32(0); y < SimH; y += 20 {
		vector.StrokeLine(screen, 0, canvasOriginY+y, SimW, canvasOriginY+y, 1, gridDark, false)
	}
	// Ghost paths and trails are stored in screen-relative-to-canvas
	// images; offset by canvasOriginY when blitting.
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(0, float64(canvasOriginY))
	screen.DrawImage(g.ghosts, op)
	op2 := &ebiten.DrawImageOptions{}
	op2.GeoM.Translate(0, float64(canvasOriginY))
	screen.DrawImage(g.trails, op2)
	// Toolhead.
	if g.state.HeadActive {
		col := headFill
		if !g.state.HeadCutting {
			col = rapidClr
		}
		hx, hy := float32(g.state.HeadX), float32(g.state.HeadY)+canvasOriginY
		vector.DrawFilledCircle(screen, hx, hy, 7, col, true)
		vector.StrokeCircle(screen, hx, hy, 7, 1, headWhite, true)
		vector.StrokeLine(screen, hx-12, hy, hx+12, hy, 1, col, false)
		vector.StrokeLine(screen, hx, hy-12, hx, hy+12, 1, col, false)
	}
	// Header text.
	if g.font != nil {
		drawHeader(screen, g.font)
	}
	// UI overlay (controls below the canvas).
	if g.ui != nil {
		g.ui.Draw(screen)
	}
}

func drawHeader(dst *ebiten.Image, face *text.GoTextFace) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(12, 8)
	op.ColorScale.ScaleWithColor(statusOn)
	text.Draw(dst, "TOOLPATH SIMULATION", face, op)
}

func (g *Game) Layout(_, _ int) (int, int) { return WindowWidth, WindowHeight }

// drawDashedLine + drawDashedRect are local copies of editor's helpers
// since the sim package shouldn't import editor.
func drawDashedLine(dst *ebiten.Image, x0, y0, x1, y1 float64, dash, gap, width float32, clr color.RGBA) {
	length := float32(math.Hypot(x1-x0, y1-y0))
	if length == 0 {
		return
	}
	ux, uy := float32(x1-x0)/length, float32(y1-y0)/length
	cycle := dash + gap
	for d := float32(0); d < length; d += cycle {
		end := d + dash
		if end > length {
			end = length
		}
		vector.StrokeLine(dst, float32(x0)+ux*d, float32(y0)+uy*d,
			float32(x0)+ux*end, float32(y0)+uy*end, width, clr, false)
	}
}

func drawDashedRect(dst *ebiten.Image, x0, y0, x1, y1 float64, dash, gap, width float32, clr color.RGBA) {
	for _, e := range [4][4]float64{
		{x0, y0, x1, y0}, {x1, y0, x1, y1}, {x1, y1, x0, y1}, {x0, y1, x0, y0},
	} {
		drawDashedLine(dst, e[0], e[1], e[2], e[3], dash, gap, width, clr)
	}
}
